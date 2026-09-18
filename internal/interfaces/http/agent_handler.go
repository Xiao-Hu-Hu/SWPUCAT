package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/fnv"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"SWPUCAT/internal/agent/telemetry"
	agentauditapp "SWPUCAT/internal/application/agentaudit"
	conversationapp "SWPUCAT/internal/application/conversation"
	"SWPUCAT/internal/application/knowledgeindex"
	"SWPUCAT/internal/domain/agentaudit"
	"SWPUCAT/internal/domain/conversation"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/infrastructure/agentclient"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	client        agentChatClient
	conversations *conversationapp.Service
	retrieval     *knowledgeindex.Service
	audits        *agentauditapp.Service
	// Keep the synchronous prefetch path bounded on the 2-core/2G deployment.
	// Agent tool calls are already serialized separately, but multiple users can
	// otherwise trigger concurrent embedding/database searches before reaching
	// the Agent service.
	retrievalSlots chan struct{}
	locks          [64]sync.Mutex
}

type agentChatClient interface {
	Chat(context.Context, agentclient.ChatRequest, string, string) (agentclient.ChatResponse, error)
}

func NewAgentHandler(client *agentclient.Client, conversations *conversationapp.Service, retrieval *knowledgeindex.Service, audits *agentauditapp.Service) *AgentHandler {
	var boundary agentChatClient
	if client != nil {
		boundary = client
	}
	return newAgentHandler(boundary, conversations, retrieval, audits)
}

func newAgentHandler(client agentChatClient, conversations *conversationapp.Service, retrieval *knowledgeindex.Service, audits *agentauditapp.Service) *AgentHandler {
	return &AgentHandler{client: client, conversations: conversations, retrieval: retrieval, audits: audits, retrievalSlots: make(chan struct{}, 1)}
}

type agentChatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

type agentCreateConversationRequest struct {
	Title string `json:"title,omitempty"`
}

type agentUpdateConversationRequest struct {
	Title string `json:"title"`
}

const (
	agentConversationListLimit    = 100
	agentConversationHistoryLimit = 200
	agentAuditWriteTimeout        = 3 * time.Second
	clientClosedRequestStatus     = 499
)

type agentChatAuditOutcome struct {
	requestID             string
	userID                *int64
	conversationID        *int64
	sessionID             *string
	actorRole             *string
	status                agentaudit.RunStatus
	failure               *agentauditapp.Failure
	startedAt             time.Time
	finishedAt            time.Time
	httpStatus            *int
	conversationPersisted *bool
	telemetryStatus       agentaudit.TelemetryStatus
	snapshot              *telemetry.Snapshot
}

func (h *AgentHandler) ListConversations(c *gin.Context) {
	if h == nil || h.conversations == nil {
		Error(c, http.StatusServiceUnavailable, 503, "agent service is not configured")
		return
	}
	items, err := h.conversations.ListRecent(c.Request.Context(), GetUserID(c), agentConversationListLimit)
	if err != nil {
		InternalError(c, "failed to load agent conversations")
		return
	}
	Success(c, items)
}

func (h *AgentHandler) CreateConversation(c *gin.Context) {
	if h == nil || h.conversations == nil {
		Error(c, http.StatusServiceUnavailable, 503, "agent service is not configured")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req agentCreateConversationRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			BadRequest(c, "invalid request body")
			return
		}
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = conversation.DefaultTitle
	}
	if utf8.RuneCountInString(title) > conversation.MaxTitleRunes {
		BadRequest(c, "title is too long")
		return
	}
	sessionID, err := normalizeAgentSessionID("")
	if err != nil {
		InternalError(c, "failed to create agent conversation")
		return
	}
	item, err := h.conversations.GetOrCreate(c.Request.Context(), GetUserID(c), sessionID, title)
	if err != nil {
		InternalError(c, "failed to create agent conversation")
		return
	}
	Created(c, item)
}

func (h *AgentHandler) GetConversation(c *gin.Context) {
	if h == nil || h.conversations == nil {
		Error(c, http.StatusServiceUnavailable, 503, "agent service is not configured")
		return
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		BadRequest(c, "session_id is required")
		return
	}
	if _, err := normalizeAgentSessionID(sessionID); err != nil {
		BadRequest(c, "session_id is invalid")
		return
	}
	item, err := h.conversations.LoadContext(c.Request.Context(), GetUserID(c), sessionID, agentConversationHistoryLimit)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			NotFound(c, "agent conversation not found")
			return
		}
		InternalError(c, "failed to load agent conversation")
		return
	}
	Success(c, item)
}

func (h *AgentHandler) UpdateConversationTitle(c *gin.Context) {
	if h == nil || h.conversations == nil {
		Error(c, http.StatusServiceUnavailable, 503, "agent service is not configured")
		return
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		BadRequest(c, "session_id is required")
		return
	}
	if _, err := normalizeAgentSessionID(sessionID); err != nil {
		BadRequest(c, "session_id is invalid")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req agentUpdateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}
	item, err := h.conversations.UpdateTitle(c.Request.Context(), GetUserID(c), sessionID, req.Title)
	if err != nil {
		switch {
		case errors.Is(err, shared.ErrValidation):
			BadRequest(c, "title is required and must be at most 256 characters")
		case errors.Is(err, shared.ErrNotFound):
			NotFound(c, "agent conversation not found")
		default:
			InternalError(c, "failed to update agent conversation title")
		}
		return
	}
	Success(c, item)
}

func (h *AgentHandler) DeleteConversation(c *gin.Context) {
	if h == nil || h.conversations == nil {
		Error(c, http.StatusServiceUnavailable, 503, "agent service is not configured")
		return
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		BadRequest(c, "session_id is required")
		return
	}
	if _, err := normalizeAgentSessionID(sessionID); err != nil {
		BadRequest(c, "session_id is invalid")
		return
	}
	userID := GetUserID(c)
	unlock := h.lockConversation(userID, sessionID)
	defer unlock()
	if err := h.conversations.Delete(c.Request.Context(), userID, sessionID); err != nil {
		if errors.Is(err, shared.ErrValidation) {
			BadRequest(c, "session_id is invalid")
			return
		}
		InternalError(c, "failed to delete agent conversation")
		return
	}
	// Missing and cross-owner handles deliberately return the same idempotent
	// response so this endpoint does not reveal another user's session ids.
	Success(c, nil)
}

func (h *AgentHandler) Chat(c *gin.Context) {
	outcome := newAgentChatAuditOutcome(c)
	requestContext := c.Request.Context()
	defer func() {
		outcome.finishedAt = time.Now()
		httpStatus := c.Writer.Status()
		outcome.httpStatus = &httpStatus
		h.recordAgentChatAudit(requestContext, outcome)
	}()

	if h == nil || h.client == nil || h.conversations == nil {
		outcome.fail(agentaudit.RunStatusUnavailable, "main", "configuration", "agent_not_configured", false)
		Error(c, http.StatusServiceUnavailable, 503, "agent service is not configured")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var req agentChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		outcome.fail(agentaudit.RunStatusRejected, "main", "validation", "invalid_request_body", false)
		BadRequest(c, "invalid request body")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || utf8.RuneCountInString(req.Message) > 4000 {
		outcome.fail(agentaudit.RunStatusRejected, "main", "validation", "invalid_message", false)
		BadRequest(c, "message is required and must be at most 4000 characters")
		return
	}
	sessionID, err := normalizeAgentSessionID(req.SessionID)
	if err != nil {
		outcome.fail(agentaudit.RunStatusRejected, "main", "validation", "invalid_session", false)
		BadRequest(c, "session_id is invalid")
		return
	}
	userID := GetUserID(c)
	outcome.setSessionID(sessionID)
	unlock := h.lockConversation(userID, sessionID)
	defer unlock()

	conversationRecord, err := h.conversations.GetOrCreate(c.Request.Context(), userID, sessionID, truncateAgentText(req.Message, 80))
	if err != nil {
		writeAgentConversationFailure(c, outcome, err, "conversation_initialize", "failed to initialize agent conversation")
		return
	}
	outcome.setConversationID(conversationRecord.ID)
	conversationContext, err := h.conversations.LoadContext(c.Request.Context(), userID, sessionID, 20)
	if err != nil {
		writeAgentConversationFailure(c, outcome, err, "conversation_load", "failed to load agent conversation")
		return
	}

	boundedMessages := boundConversationHistory(conversationContext.Messages, 24000)
	history := make([]agentclient.HistoryMessage, 0, len(boundedMessages))
	for _, message := range boundedMessages {
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		history = append(history, agentclient.HistoryMessage{Role: message.Role, Content: message.Content})
	}
	searchQuery := contextualKnowledgeQuery(req.Message, conversationContext.Messages)
	knowledgeContext := make([]agentclient.KnowledgeSource, 0)
	if h.retrieval != nil && shouldPrefetchKnowledge(req.Message) {
		knowledgeContext = h.prefetchKnowledge(c.Request.Context(), searchQuery)
	}
	previousSources := make([]agentclient.KnowledgeSource, 0)
	if isContextualFollowUp(req.Message) && conversationContext.Conversation.LastSourcesJSON != "" {
		if err := json.Unmarshal([]byte(conversationContext.Conversation.LastSourcesJSON), &previousSources); err != nil {
			previousSources = nil
		}
	}

	result, err := h.client.Chat(c.Request.Context(), agentclient.ChatRequest{
		Message:          req.Message,
		SessionID:        sessionID,
		UserID:           userID,
		Role:             GetRole(c),
		HistoryProvided:  true,
		History:          history,
		RollingSummary:   conversationContext.Conversation.RollingSummary,
		KnowledgeContext: knowledgeContext,
		PreviousSources:  previousSources,
	}, c.GetHeader("Authorization"), GetRequestID(c))
	if err != nil {
		outcome.applyAgentClientFailure(err)
		writeAgentChatClientError(c, err)
		return
	}
	outcome.succeed(result.Audit)
	result.SessionID = sessionID
	sourcesJSON, marshalErr := json.Marshal(result.Sources)
	if marshalErr != nil {
		sourcesJSON = []byte("[]")
	}
	rollingSummary, changed := compactEvictedHistory(conversationContext.Conversation.RollingSummary, conversationContext.Messages)
	record := conversationapp.RecordTurnRequest{
		UserContent: req.Message, AssistantContent: result.Answer,
		CitationsJSON: string(sourcesJSON), LastSourcesJSON: agentStringPointer(string(sourcesJSON)),
	}
	if changed {
		record.RollingSummary = agentStringPointer(rollingSummary)
	}
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 3*time.Second)
	defer cancelPersist()
	if _, persistErr := h.conversations.RecordTurn(persistCtx, userID, sessionID, record); persistErr != nil {
		outcome.setConversationPersisted(false)
		// A tool may already have changed business state. Return that result to
		// the user instead of encouraging an unsafe retry, while exposing the
		// durability issue to operators.
		log.Printf("agent conversation persistence failed for user %d: %v", userID, persistErr)
		c.Header("X-Agent-Conversation-Persisted", "false")
	} else {
		outcome.setConversationPersisted(true)
	}
	Success(c, result)
}

func newAgentChatAuditOutcome(c *gin.Context) *agentChatAuditOutcome {
	retryable := false
	outcome := &agentChatAuditOutcome{
		status:          agentaudit.RunStatusError,
		telemetryStatus: agentaudit.TelemetryNotStarted,
		startedAt:       time.Now(),
		failure: &agentauditapp.Failure{
			Source:    "main",
			Category:  "internal",
			Code:      "unclassified_failure",
			Retryable: &retryable,
		},
	}
	if c == nil {
		return outcome
	}
	outcome.requestID = GetRequestID(c)
	if userID := GetUserID(c); userID > 0 {
		outcome.userID = &userID
		role := GetRole(c)
		switch role {
		case "member", "captain", "super_admin":
			outcome.actorRole = &role
		}
	}
	return outcome
}

func (o *agentChatAuditOutcome) fail(status agentaudit.RunStatus, source, category, code string, retryable bool) {
	if o == nil {
		return
	}
	retryableCopy := retryable
	o.status = status
	o.failure = &agentauditapp.Failure{
		Source:    source,
		Category:  category,
		Code:      code,
		Retryable: &retryableCopy,
	}
}

func (o *agentChatAuditOutcome) succeed(audit *telemetry.AuditSummary) {
	if o == nil {
		return
	}
	o.status = agentaudit.RunStatusSuccess
	o.failure = nil
	o.applyAudit(audit)
	if audit != nil && o.requestID != "" && audit.RequestID == o.requestID && audit.Telemetry == nil {
		// A successful private Agent response must carry its terminal snapshot.
		// A request-ID-only envelope is meaningful only for pre-engine failures.
		o.telemetryStatus = agentaudit.TelemetryInvalid
	}
}

func (o *agentChatAuditOutcome) setSessionID(sessionID string) {
	if o == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	o.sessionID = &sessionID
}

func (o *agentChatAuditOutcome) setConversationID(conversationID int64) {
	if o == nil || conversationID <= 0 {
		return
	}
	o.conversationID = &conversationID
}

func (o *agentChatAuditOutcome) setConversationPersisted(persisted bool) {
	if o == nil || o.conversationID == nil {
		return
	}
	persistedCopy := persisted
	o.conversationPersisted = &persistedCopy
}

func (o *agentChatAuditOutcome) applyAudit(audit *telemetry.AuditSummary) {
	if o == nil {
		return
	}
	o.snapshot = nil
	switch {
	case audit == nil:
		o.telemetryStatus = agentaudit.TelemetryUnavailable
	case o.requestID == "" || audit.RequestID != o.requestID:
		o.telemetryStatus = agentaudit.TelemetryInvalid
	case audit.Telemetry == nil:
		o.telemetryStatus = agentaudit.TelemetryNotStarted
	default:
		o.telemetryStatus = agentaudit.TelemetryCaptured
		o.snapshot = audit.Telemetry
	}
}

func (o *agentChatAuditOutcome) applyAgentClientFailure(err error) {
	if o == nil {
		return
	}
	o.applyAudit(nil)

	var clientErr *agentclient.ClientError
	if errors.As(err, &clientErr) && clientErr != nil {
		o.applyAudit(clientErr.Audit)
		o.applyAgentClientClassification(clientErr.Kind, clientErr.FailureCode, clientErr.StatusCode)
		return
	}

	var httpErr *agentclient.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		o.applyAudit(httpErr.Audit)
		o.applyAgentClientClassification(httpErr.Kind, httpErr.FailureCode, httpErr.StatusCode)
		return
	}

	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		o.fail(agentaudit.RunStatusCanceled, "transport", "canceled", string(agentclient.FailureCodeRequestCanceled), false)
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		o.fail(agentaudit.RunStatusTimeout, "transport", "timeout", string(agentclient.FailureCodeExecutionTimeout), true)
	case errors.As(err, &netErr):
		o.fail(agentaudit.RunStatusUnavailable, "transport", "network", string(agentclient.FailureCodeTransport), true)
	default:
		o.fail(agentaudit.RunStatusError, "main", "internal", "agent_client_failure", false)
	}
}

func (o *agentChatAuditOutcome) applyAgentClientClassification(kind agentclient.ErrorKind, code agentclient.FailureCode, statusCode int) {
	switch kind {
	case agentclient.KindTimeout:
		o.fail(agentaudit.RunStatusTimeout, "transport", "timeout", string(agentclient.FailureCodeExecutionTimeout), true)
		return
	case agentclient.KindCanceled:
		o.fail(agentaudit.RunStatusCanceled, "transport", "canceled", string(agentclient.FailureCodeRequestCanceled), false)
		return
	case agentclient.KindTransport:
		o.fail(agentaudit.RunStatusUnavailable, "transport", "network", string(agentclient.FailureCodeTransport), true)
		return
	case agentclient.KindProtocol:
		o.fail(agentaudit.RunStatusError, "transport", "protocol", string(agentclient.FailureCodeProtocol), false)
		return
	}

	switch code {
	case agentclient.FailureCodeTransport:
		o.fail(agentaudit.RunStatusUnavailable, "transport", "network", string(agentclient.FailureCodeTransport), true)
	case agentclient.FailureCodeProtocol:
		o.fail(agentaudit.RunStatusError, "transport", "protocol", string(agentclient.FailureCodeProtocol), false)
	case agentclient.FailureCodeAgentDisabled:
		o.fail(agentaudit.RunStatusUnavailable, "agent", "availability", string(code), false)
	case agentclient.FailureCodeInvalidInternalSecret:
		o.fail(agentaudit.RunStatusRejected, "agent", "authentication", string(code), false)
	case agentclient.FailureCodeInvalidRequestBody,
		agentclient.FailureCodeInvalidMessage,
		agentclient.FailureCodeInvalidSession,
		agentclient.FailureCodeInvalidHistory:
		o.fail(agentaudit.RunStatusRejected, "agent", "validation", string(code), false)
	case agentclient.FailureCodeAgentBusy, agentclient.FailureCodeQueueTimeout:
		o.fail(agentaudit.RunStatusBusy, "agent", "capacity", string(code), true)
	case agentclient.FailureCodeRequestCanceled:
		o.fail(agentaudit.RunStatusCanceled, "agent", "canceled", string(code), false)
	case agentclient.FailureCodeExecutionTimeout:
		o.fail(agentaudit.RunStatusTimeout, "agent", "timeout", string(code), true)
	case agentclient.FailureCodeExecutionFailed:
		o.fail(agentaudit.RunStatusError, "agent", "execution", string(code), false)
	case agentclient.FailureCodeLegacy, "":
		o.applyLegacyAgentFailure(statusCode)
	default:
		// The transport boundary rejects unknown codes, but keep this public
		// boundary closed as well for fakes and older alternate clients.
		o.fail(agentaudit.RunStatusError, "transport", "protocol", string(agentclient.FailureCodeProtocol), false)
	}
}

func (o *agentChatAuditOutcome) applyLegacyAgentFailure(statusCode int) {
	const legacyCode = string(agentclient.FailureCodeLegacy)
	switch {
	case statusCode == http.StatusRequestTimeout || statusCode == http.StatusGatewayTimeout:
		o.fail(agentaudit.RunStatusTimeout, "agent", "timeout", legacyCode, true)
	case statusCode == http.StatusTooManyRequests:
		o.fail(agentaudit.RunStatusBusy, "agent", "capacity", legacyCode, true)
	case statusCode == http.StatusServiceUnavailable:
		o.fail(agentaudit.RunStatusUnavailable, "agent", "availability", legacyCode, true)
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		o.fail(agentaudit.RunStatusRejected, "agent", "http", legacyCode, false)
	case statusCode >= http.StatusInternalServerError:
		o.fail(agentaudit.RunStatusError, "agent", "execution", legacyCode, true)
	default:
		o.fail(agentaudit.RunStatusError, "agent", "protocol", legacyCode, false)
	}
}

func writeAgentConversationFailure(c *gin.Context, outcome *agentChatAuditOutcome, err error, operation, message string) {
	var netErr net.Error
	switch {
	case errors.Is(err, conversation.ErrDeletedConversation):
		outcome.fail(agentaudit.RunStatusRejected, "main", "persistence", operation+"_deleted", false)
		Error(c, http.StatusGone, http.StatusGone, "agent conversation was permanently deleted")
	case errors.Is(err, context.Canceled):
		outcome.fail(agentaudit.RunStatusCanceled, "main", "persistence", operation+"_canceled", false)
		Error(c, clientClosedRequestStatus, clientClosedRequestStatus, "request was canceled")
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		outcome.fail(agentaudit.RunStatusTimeout, "main", "persistence", operation+"_timeout", true)
		Error(c, http.StatusGatewayTimeout, http.StatusGatewayTimeout, "agent service timed out")
	default:
		outcome.fail(agentaudit.RunStatusError, "main", "persistence", operation+"_failed", true)
		InternalError(c, message)
	}
}

func writeAgentChatClientError(c *gin.Context, err error) {
	var clientErr *agentclient.ClientError
	if errors.As(err, &clientErr) && clientErr != nil {
		switch clientErr.Kind {
		case agentclient.KindTimeout:
			Error(c, http.StatusGatewayTimeout, 504, "agent service timed out")
			return
		case agentclient.KindProtocol:
			Error(c, http.StatusBadGateway, 502, "agent service request failed")
			return
		case agentclient.KindCanceled:
			Error(c, clientClosedRequestStatus, clientClosedRequestStatus, "request was canceled")
			return
		case agentclient.KindTransport:
			InternalError(c, "agent service request failed")
			return
		}
		writeAgentChatHTTPError(c, clientErr.StatusCode, clientErr.Message)
		return
	}

	var httpErr *agentclient.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		switch httpErr.Kind {
		case agentclient.KindTimeout:
			Error(c, http.StatusGatewayTimeout, 504, "agent service timed out")
			return
		case agentclient.KindProtocol:
			Error(c, http.StatusBadGateway, 502, "agent service request failed")
			return
		case agentclient.KindCanceled:
			Error(c, clientClosedRequestStatus, clientClosedRequestStatus, "request was canceled")
			return
		case agentclient.KindTransport:
			InternalError(c, "agent service request failed")
			return
		}
		writeAgentChatHTTPError(c, httpErr.StatusCode, httpErr.Message)
		return
	}

	var netErr net.Error
	if errors.Is(err, context.Canceled) {
		Error(c, clientClosedRequestStatus, clientClosedRequestStatus, "request was canceled")
		return
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		Error(c, http.StatusGatewayTimeout, 504, "agent service timed out")
		return
	}
	InternalError(c, "agent service request failed")
}

func writeAgentChatHTTPError(c *gin.Context, statusCode int, message string) {
	switch statusCode {
	case http.StatusBadRequest:
		BadRequest(c, message)
	case http.StatusUnauthorized:
		Unauthorized(c, "agent authentication failed")
	case http.StatusForbidden:
		Forbidden(c, "agent access denied")
	case http.StatusServiceUnavailable:
		Error(c, http.StatusServiceUnavailable, 503, "agent service is unavailable")
	case http.StatusGatewayTimeout:
		Error(c, http.StatusGatewayTimeout, 504, "agent service timed out")
	default:
		Error(c, http.StatusBadGateway, 502, "agent service request failed")
	}
}

func (h *AgentHandler) recordAgentChatAudit(parent context.Context, outcome *agentChatAuditOutcome) {
	if h == nil || h.audits == nil || outcome == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), agentAuditWriteTimeout)
	defer cancel()
	_, err := h.audits.Record(auditCtx, agentauditapp.RecordRequest{
		RequestID:             outcome.requestID,
		UserID:                outcome.userID,
		ConversationID:        outcome.conversationID,
		SessionID:             outcome.sessionID,
		ActorRole:             outcome.actorRole,
		Status:                outcome.status,
		HTTPStatus:            outcome.httpStatus,
		Failure:               outcome.failure,
		StartedAt:             outcome.startedAt,
		FinishedAt:            outcome.finishedAt,
		ConversationPersisted: outcome.conversationPersisted,
		TelemetryStatus:       outcome.telemetryStatus,
		Snapshot:              outcome.snapshot,
	})
	if err != nil {
		log.Printf("agent audit persistence failed for request %s: %v", outcome.requestID, err)
	}
}

func (h *AgentHandler) prefetchKnowledge(ctx context.Context, query string) []agentclient.KnowledgeSource {
	if h == nil || h.retrieval == nil {
		return nil
	}
	// A missing slot is a graceful degradation: the Agent can still call its
	// authorized search_knowledge tool after the request enters the serialized
	// Agent worker. Do not queue unbounded requests behind a slow embedder.
	if h.retrievalSlots == nil {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	select {
	case h.retrievalSlots <- struct{}{}:
		defer func() { <-h.retrievalSlots }()
	case <-waitCtx.Done():
		if ctx.Err() == nil {
			log.Printf("agent knowledge prefetch skipped: retrieval is busy")
		}
		return nil
	}
	searchCtx, cancelSearch := context.WithTimeout(ctx, 10*time.Second)
	defer cancelSearch()
	items, err := h.retrieval.Search(searchCtx, query)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("agent knowledge prefetch failed: %v", err)
		}
		return nil
	}
	return toAgentKnowledgeSources(items)
}

func normalizeAgentSessionID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		var data [16]byte
		if _, err := rand.Read(data[:]); err != nil {
			return "", err
		}
		return hex.EncodeToString(data[:]), nil
	}
	if utf8.RuneCountInString(value) > 128 {
		return "", errors.New("session id is too long")
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return "", errors.New("session id contains unsupported characters")
	}
	return value, nil
}

func (h *AgentHandler) lockConversation(userID int64, sessionID string) func() {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strconv.FormatInt(userID, 10)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(sessionID))
	lock := &h.locks[hash.Sum32()%uint32(len(h.locks))]
	lock.Lock()
	return lock.Unlock
}

func shouldPrefetchKnowledge(message string) bool {
	value := strings.ToLower(strings.TrimSpace(message))
	if value == "" {
		return false
	}
	for _, greeting := range []string{"你好", "您好", "hello", "hi", "谢谢", "再见"} {
		if value == greeting {
			return false
		}
	}
	for _, operation := range []string{"帮我打卡", "打卡上班", "上班打卡", "打卡下班", "下班打卡", "帮我签到", "帮我签退", "生成月报", "总结月报", "考评成员", "考勤统计"} {
		if strings.Contains(value, operation) {
			return false
		}
	}
	for _, marker := range []string{"怎么", "如何", "为什么", "哪里", "什么", "是否", "能否", "?", "？", "说明", "制度", "流程", "规定", "规则", "要求", "标准", "办法", "政策", "指南", "教程", "文档", "资料", "文件", "链接", "下载", "知识库", "考勤", "打卡", "月报", "报告"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func isContextualFollowUp(message string) bool {
	value := strings.TrimSpace(message)
	if utf8.RuneCountInString(value) <= 24 {
		return true
	}
	for _, marker := range []string{"这个", "那个", "它", "上面", "刚才", "前面", "继续", "再详细", "还有呢", "怎么下载", "怎么用"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func contextualKnowledgeQuery(current string, history []conversationapp.MessageDTO) string {
	current = strings.TrimSpace(current)
	if !isContextualFollowUp(current) {
		return truncateAgentText(current, 512)
	}
	previous := ""
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			previous = history[i].Content
			break
		}
	}
	current = truncateAgentText(current, 512)
	available := 512 - utf8.RuneCountInString(current) - 1
	if available <= 0 || previous == "" {
		return current
	}
	return strings.TrimSpace(truncateContextBothEnds(previous, available) + "\n" + current)
}

func truncateContextBothEnds(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if limit <= 0 || len(runes) <= limit {
		return string(runes)
	}
	if limit == 1 {
		return string(runes[:1])
	}
	left := limit / 2
	right := limit - left - 1
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

func toAgentKnowledgeSources(items []knowledgeindex.SearchResult) []agentclient.KnowledgeSource {
	result := make([]agentclient.KnowledgeSource, 0, len(items))
	for _, item := range items {
		result = append(result, agentclient.KnowledgeSource{
			ID: item.ItemID, Type: item.Type, Name: item.Name,
			Description: item.Description, URL: item.URL, DownloadURL: item.DownloadURL,
			CategoryName: item.CategoryName, SourceKind: item.SourceKind,
			Excerpt: item.Excerpt, Score: item.Score, MatchMode: item.MatchMode,
		})
	}
	return result
}

func compactEvictedHistory(existing string, messages []conversationapp.MessageDTO) (string, bool) {
	if len(messages) < 20 {
		return existing, false
	}
	var b strings.Builder
	if strings.TrimSpace(existing) != "" {
		b.WriteString(strings.TrimSpace(existing))
		b.WriteByte('\n')
	}
	count := 2
	if len(messages) < count {
		count = len(messages)
	}
	for _, message := range messages[:count] {
		label := "用户"
		if message.Role == "assistant" {
			label = "助手"
		}
		b.WriteString(label)
		b.WriteString("：")
		b.WriteString(truncateAgentText(message.Content, 1000))
		b.WriteByte('\n')
	}
	runes := []rune(strings.TrimSpace(b.String()))
	if len(runes) > 8000 {
		runes = runes[len(runes)-8000:]
	}
	return string(runes), true
}

func boundConversationHistory(messages []conversationapp.MessageDTO, maxRunes int) []conversationapp.MessageDTO {
	if maxRunes <= 0 || len(messages) == 0 {
		return nil
	}
	total := 0
	selected := make([]conversationapp.MessageDTO, 0, len(messages))
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		contentRunes := utf8.RuneCountInString(strings.TrimSpace(message.Content))
		remaining := maxRunes - total
		if remaining <= 0 {
			break
		}
		if contentRunes > remaining {
			if message.Role == "assistant" {
				message.Content = truncateAgentText(message.Content, remaining)
				if strings.TrimSpace(message.Content) != "" {
					selected = append(selected, message)
				}
			}
			break
		}
		selected = append(selected, message)
		total += contentRunes
	}
	if len(selected) == 0 {
		return nil
	}
	result := make([]conversationapp.MessageDTO, len(selected))
	for i := range selected {
		result[len(selected)-1-i] = selected[i]
	}
	return result
}

func truncateAgentText(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "…"
}

func agentStringPointer(value string) *string { return &value }
