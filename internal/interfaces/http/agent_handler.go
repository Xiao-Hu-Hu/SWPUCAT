package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/fnv"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	conversationapp "SWPUCAT/internal/application/conversation"
	"SWPUCAT/internal/application/knowledgeindex"
	"SWPUCAT/internal/infrastructure/agentclient"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	client        *agentclient.Client
	conversations *conversationapp.Service
	retrieval     *knowledgeindex.Service
	// Keep the synchronous prefetch path bounded on the 2-core/2G deployment.
	// Agent tool calls are already serialized separately, but multiple users can
	// otherwise trigger concurrent embedding/database searches before reaching
	// the Agent service.
	retrievalSlots chan struct{}
	locks          [64]sync.Mutex
}

func NewAgentHandler(client *agentclient.Client, conversations *conversationapp.Service, retrieval *knowledgeindex.Service) *AgentHandler {
	return &AgentHandler{client: client, conversations: conversations, retrieval: retrieval, retrievalSlots: make(chan struct{}, 1)}
}

type agentChatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

func (h *AgentHandler) Chat(c *gin.Context) {
	if h == nil || h.client == nil || h.conversations == nil {
		Error(c, http.StatusServiceUnavailable, 503, "agent service is not configured")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var req agentChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request body")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || utf8.RuneCountInString(req.Message) > 4000 {
		BadRequest(c, "message is required and must be at most 4000 characters")
		return
	}
	sessionID, err := normalizeAgentSessionID(req.SessionID)
	if err != nil {
		BadRequest(c, "session_id is invalid")
		return
	}
	userID := GetUserID(c)
	unlock := h.lockConversation(userID, sessionID)
	defer unlock()

	if _, err := h.conversations.GetOrCreate(c.Request.Context(), userID, sessionID, truncateAgentText(req.Message, 80)); err != nil {
		InternalError(c, "failed to initialize agent conversation")
		return
	}
	conversationContext, err := h.conversations.LoadContext(c.Request.Context(), userID, sessionID, 20)
	if err != nil {
		InternalError(c, "failed to load agent conversation")
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
	}, c.GetHeader("Authorization"), c.GetHeader("X-Request-ID"))
	if err != nil {
		var httpErr *agentclient.HTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.StatusCode {
			case http.StatusBadRequest:
				BadRequest(c, httpErr.Message)
			case http.StatusUnauthorized:
				Unauthorized(c, "agent authentication failed")
			case http.StatusForbidden:
				Forbidden(c, "agent access denied")
			case http.StatusServiceUnavailable:
				Error(c, http.StatusServiceUnavailable, 503, "agent service is unavailable")
			default:
				Error(c, http.StatusBadGateway, 502, "agent service request failed")
			}
			return
		}
		InternalError(c, "agent service request failed")
		return
	}
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
		// A tool may already have changed business state. Return that result to
		// the user instead of encouraging an unsafe retry, while exposing the
		// durability issue to operators.
		log.Printf("agent conversation persistence failed for user %d: %v", userID, persistErr)
		c.Header("X-Agent-Conversation-Persisted", "false")
	}
	Success(c, result)
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
