package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"SWPUCAT/internal/agent/confirmation"
	"SWPUCAT/internal/application/agentaction"
	announcementdraftapp "SWPUCAT/internal/application/announcementdraft"
	checkinapp "SWPUCAT/internal/application/checkin"
	"SWPUCAT/internal/domain/shared"

	"github.com/gin-gonic/gin"
)

type actionHandlerRepo struct {
	mu      sync.Mutex
	records map[string]*agentaction.Record
}

func (r *actionHandlerRepo) Claim(_ context.Context, record *agentaction.Record) (*agentaction.Record, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = make(map[string]*agentaction.Record)
	}
	if existing := r.records[record.TokenHash]; existing != nil {
		copy := *existing
		return &copy, false, nil
	}
	copy := *record
	r.records[record.TokenHash] = &copy
	return record, true, nil
}

func (r *actionHandlerRepo) Complete(_ context.Context, tokenHash, status, resultJSON, errorCode string, completedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.records[tokenHash]
	if record == nil || record.Status != "executing" {
		return shared.ErrConflict
	}
	record.Status, record.ResultJSON, record.ErrorCode, record.CompletedAt = status, resultJSON, errorCode, &completedAt
	return nil
}

type actionHandlerCheckins struct {
	clockInCalls  int
	clockOutCalls int
	err           error
}

func (w *actionHandlerCheckins) ClockIn(context.Context, int64, string) (*checkinapp.CheckinRecordDTO, error) {
	w.clockInCalls++
	if w.err != nil {
		return nil, w.err
	}
	return &checkinapp.CheckinRecordDTO{ID: 31, Type: "in", Date: "2026-09-18", Time: "09:00:00"}, nil
}

func (w *actionHandlerCheckins) ClockOut(context.Context, int64, string) (*checkinapp.CheckinRecordDTO, error) {
	w.clockOutCalls++
	if w.err != nil {
		return nil, w.err
	}
	return &checkinapp.CheckinRecordDTO{ID: 32, Type: "out", Date: "2026-09-18", Time: "18:00:00"}, nil
}

type actionHandlerDrafts struct{ calls int }

func (w *actionHandlerDrafts) Create(_ context.Context, actorID int64, req announcementdraftapp.CreateRequest) (*announcementdraftapp.DraftDTO, error) {
	w.calls++
	return &announcementdraftapp.DraftDTO{ID: 41, Status: "draft", Title: req.Title, Content: req.Content, CreatedBy: actorID}, nil
}

type actionHandlerEnvelope struct {
	Code    int                 `json:"code"`
	Message string              `json:"message"`
	Data    *agentaction.Result `json:"data"`
}

func TestConfirmActionAcceptsValidTokenAndReplaysWithoutAnotherWrite(t *testing.T) {
	signer := newActionHandlerSigner(t, time.Minute)
	proposal, err := signer.Propose(7, "session-7", confirmation.ToolClockIn, "上班打卡", "请确认", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	repo, checkins := &actionHandlerRepo{}, &actionHandlerCheckins{}
	handler := &AgentHandler{actions: agentaction.NewService(true, repo, signer, checkins, nil)}

	firstStatus, first := performActionConfirmation(t, handler, 7, "member", proposal.Tool, proposal.Token, "session-7", proposal.Parameters)
	if firstStatus != http.StatusOK || first.Code != 0 || first.Data == nil || first.Data.Status != "success" || first.Data.Replayed {
		t.Fatalf("first response status=%d body=%+v", firstStatus, first)
	}
	secondStatus, second := performActionConfirmation(t, handler, 7, "member", proposal.Tool, proposal.Token, "session-7", proposal.Parameters)
	if secondStatus != http.StatusOK || second.Data == nil || second.Data.Status != "success" || !second.Data.Replayed {
		t.Fatalf("replay response status=%d body=%+v", secondStatus, second)
	}
	if checkins.clockInCalls != 1 {
		t.Fatalf("clock-in calls = %d, want 1", checkins.clockInCalls)
	}
}

func TestConfirmActionRejectsTamperingCrossUserCrossSessionAndToolReplacement(t *testing.T) {
	signer := newActionHandlerSigner(t, time.Minute)
	proposal, err := signer.Propose(7, "session-7", confirmation.ToolClockIn, "上班打卡", "请确认", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	repo, checkins := &actionHandlerRepo{}, &actionHandlerCheckins{}
	handler := &AgentHandler{actions: agentaction.NewService(true, repo, signer, checkins, nil)}
	tests := []struct {
		name       string
		userID     int64
		tool       string
		token      string
		sessionID  string
		parameters json.RawMessage
	}{
		{name: "parameters", userID: 7, tool: proposal.Tool, token: proposal.Token, sessionID: "session-7", parameters: json.RawMessage(`{"changed":true}`)},
		{name: "user", userID: 8, tool: proposal.Tool, token: proposal.Token, sessionID: "session-7", parameters: proposal.Parameters},
		{name: "session", userID: 7, tool: proposal.Tool, token: proposal.Token, sessionID: "session-8", parameters: proposal.Parameters},
		{name: "tool", userID: 7, tool: confirmation.ToolClockOut, token: proposal.Token, sessionID: "session-7", parameters: proposal.Parameters},
		{name: "signature", userID: 7, tool: proposal.Tool, token: proposal.Token + "x", sessionID: "session-7", parameters: proposal.Parameters},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, response := performActionConfirmation(t, handler, test.userID, "member", test.tool, test.token, test.sessionID, test.parameters)
			if status != http.StatusBadRequest || response.Code != http.StatusBadRequest || response.Data != nil {
				t.Fatalf("status=%d response=%+v", status, response)
			}
		})
	}
	if checkins.clockInCalls != 0 || checkins.clockOutCalls != 0 || len(repo.records) != 0 {
		t.Fatalf("rejected requests changed state: in=%d out=%d records=%d", checkins.clockInCalls, checkins.clockOutCalls, len(repo.records))
	}
}

func TestConfirmActionRejectsExpiredAndUnauthorizedDraft(t *testing.T) {
	expiringSigner := newActionHandlerSigner(t, time.Second)
	expiring, err := expiringSigner.Propose(7, "session-7", confirmation.ToolClockIn, "上班打卡", "请确认", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	expiresAt, err := time.Parse(time.RFC3339, expiring.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if wait := time.Until(expiresAt.Add(20 * time.Millisecond)); wait > 0 {
		time.Sleep(wait)
	}
	checkins := &actionHandlerCheckins{}
	handler := &AgentHandler{actions: agentaction.NewService(true, &actionHandlerRepo{}, expiringSigner, checkins, nil)}
	status, response := performActionConfirmation(t, handler, 7, "member", expiring.Tool, expiring.Token, "session-7", expiring.Parameters)
	if status != http.StatusGone || response.Code != http.StatusGone || checkins.clockInCalls != 0 {
		t.Fatalf("expired response status=%d body=%+v calls=%d", status, response, checkins.clockInCalls)
	}

	signer := newActionHandlerSigner(t, time.Minute)
	draft, err := signer.Propose(7, "session-7", confirmation.ToolDraftAnnouncement, "创建公告草稿", "请确认", confirmation.DraftAnnouncementParameters{Title: "通知", Content: "正文"})
	if err != nil {
		t.Fatal(err)
	}
	drafts := &actionHandlerDrafts{}
	handler = &AgentHandler{actions: agentaction.NewService(true, &actionHandlerRepo{}, signer, nil, drafts)}
	status, response = performActionConfirmation(t, handler, 7, "member", draft.Tool, draft.Token, "session-7", draft.Parameters)
	if status != http.StatusForbidden || response.Code != http.StatusForbidden || drafts.calls != 0 {
		t.Fatalf("unauthorized response status=%d body=%+v calls=%d", status, response, drafts.calls)
	}
}

func TestConfirmActionReturnsAndReplaysStructuredBusinessFailure(t *testing.T) {
	signer := newActionHandlerSigner(t, time.Minute)
	proposal, err := signer.Propose(7, "session-7", confirmation.ToolClockOut, "下班打卡", "请确认", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	checkins := &actionHandlerCheckins{err: errors.New("database unavailable")}
	handler := &AgentHandler{actions: agentaction.NewService(true, &actionHandlerRepo{}, signer, checkins, nil)}
	for attempt := 0; attempt < 2; attempt++ {
		status, response := performActionConfirmation(t, handler, 7, "member", proposal.Tool, proposal.Token, "session-7", proposal.Parameters)
		if status != http.StatusOK || response.Data == nil || response.Data.Status != "failed" || strings.Contains(response.Data.Title, "成功") {
			t.Fatalf("attempt %d status=%d response=%+v", attempt, status, response)
		}
		if (attempt == 1) != response.Data.Replayed {
			t.Fatalf("attempt %d replayed=%t", attempt, response.Data.Replayed)
		}
	}
	if checkins.clockOutCalls != 1 {
		t.Fatalf("clock-out calls = %d, want 1", checkins.clockOutCalls)
	}
}

func TestPersistedActionCardsNeverContainRawConfirmationToken(t *testing.T) {
	original := []confirmation.Proposal{{
		Tool: confirmation.ToolClockIn, Title: "上班打卡", Summary: "请确认",
		Parameters: json.RawMessage(`{}`), Token: "signed.payload", ExpiresAt: "2026-09-18T12:05:00Z",
	}}
	data := marshalPersistedAgentActions(original)
	if strings.Contains(string(data), "signed.payload") || strings.Contains(string(data), `"token"`) {
		t.Fatalf("persisted card contains raw token: %s", data)
	}
	if original[0].Token != "signed.payload" {
		t.Fatal("persistence projection mutated the live response proposal")
	}
	var persisted []confirmation.Proposal
	if err := json.Unmarshal(data, &persisted); err != nil || len(persisted) != 1 || persisted[0].Tool != confirmation.ToolClockIn || persisted[0].Token != "" {
		t.Fatalf("persisted card = %+v err=%v", persisted, err)
	}
}

func newActionHandlerSigner(t *testing.T, ttl time.Duration) *confirmation.Signer {
	t.Helper()
	signer, err := confirmation.NewSigner(strings.Repeat("s", 32), ttl)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func performActionConfirmation(t *testing.T, handler *AgentHandler, userID int64, role, toolName, token, sessionID string, parameters json.RawMessage) (int, actionHandlerEnvelope) {
	t.Helper()
	payload, err := json.Marshal(agentActionConfirmRequest{
		SessionID: sessionID, Tool: toolName, Token: token, Parameters: parameters,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/agent/actions/confirm", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(ContextUserID, userID)
	ctx.Set(ContextRole, role)
	handler.ConfirmAction(ctx)
	var response actionHandlerEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return recorder.Code, response
}
