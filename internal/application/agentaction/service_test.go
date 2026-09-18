package agentaction

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"SWPUCAT/internal/agent/confirmation"
	announcementdraftapp "SWPUCAT/internal/application/announcementdraft"
	checkinapp "SWPUCAT/internal/application/checkin"
	"SWPUCAT/internal/domain/checkin"
	"SWPUCAT/internal/domain/shared"
)

type fakeActionRepo struct {
	mu      sync.Mutex
	records map[string]*Record
}

func (r *fakeActionRepo) Claim(_ context.Context, record *Record) (*Record, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = map[string]*Record{}
	}
	if existing := r.records[record.TokenHash]; existing != nil {
		copy := *existing
		return &copy, false, nil
	}
	copy := *record
	r.records[record.TokenHash] = &copy
	return record, true, nil
}

func (r *fakeActionRepo) Complete(_ context.Context, tokenHash, status, resultJSON, errorCode string, completedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.records[tokenHash]
	if record == nil || record.Status != "executing" {
		return shared.ErrConflict
	}
	record.Status, record.ResultJSON, record.ErrorCode, record.CompletedAt = status, resultJSON, errorCode, &completedAt
	return nil
}

type fakeCheckinWriter struct {
	clockInCalls  int
	clockOutCalls int
	err           error
	keys          []string
}

func (w *fakeCheckinWriter) ClockIn(_ context.Context, userID int64, key string) (*checkinapp.CheckinRecordDTO, error) {
	w.clockInCalls++
	w.keys = append(w.keys, key)
	if w.err != nil {
		return nil, w.err
	}
	return &checkinapp.CheckinRecordDTO{ID: 31, Type: "in", Date: "2026-09-18", Time: "09:00:00"}, nil
}

func (w *fakeCheckinWriter) ClockOut(_ context.Context, userID int64, key string) (*checkinapp.CheckinRecordDTO, error) {
	w.clockOutCalls++
	w.keys = append(w.keys, key)
	if w.err != nil {
		return nil, w.err
	}
	return &checkinapp.CheckinRecordDTO{ID: 32, Type: "out", Date: "2026-09-18", Time: "18:00:00"}, nil
}

type fakeDraftWriter struct{ calls int }

func (w *fakeDraftWriter) Create(_ context.Context, actorID int64, req announcementdraftapp.CreateRequest) (*announcementdraftapp.DraftDTO, error) {
	w.calls++
	return &announcementdraftapp.DraftDTO{ID: 41, Status: "draft", Title: req.Title, Content: req.Content, CreatedBy: actorID}, nil
}

type expiredVerifier struct{}

func (expiredVerifier) Verify(string, int64, string, string, json.RawMessage) (*confirmation.Claims, json.RawMessage, error) {
	return nil, nil, confirmation.ErrExpiredToken
}

func newActionTestSigner(t *testing.T, ttl time.Duration) *confirmation.Signer {
	t.Helper()
	signer, err := confirmation.NewSigner(strings.Repeat("s", 32), ttl)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func TestExecuteClaimsOnceAndReplaysStoredResult(t *testing.T) {
	signer := newActionTestSigner(t, time.Minute)
	proposal, err := signer.Propose(7, "session-7", confirmation.ToolClockIn, "上班打卡", "确认", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeActionRepo{}
	checkins := &fakeCheckinWriter{}
	service := NewService(true, repo, signer, checkins, nil)
	req := ExecuteRequest{UserID: 7, Role: "member", SessionID: "session-7", Tool: proposal.Tool, Token: proposal.Token, Parameters: proposal.Parameters}

	first, err := service.Execute(context.Background(), req)
	if err != nil || first.Status != "success" || first.Replayed {
		t.Fatalf("first result = %+v err=%v", first, err)
	}
	second, err := service.Execute(context.Background(), req)
	if err != nil || second.Status != "success" || !second.Replayed {
		t.Fatalf("replay result = %+v err=%v", second, err)
	}
	if checkins.clockInCalls != 1 || len(checkins.keys) != 1 || !strings.HasPrefix(checkins.keys[0], "agent-confirm-") {
		t.Fatalf("write calls/keys = %d %#v", checkins.clockInCalls, checkins.keys)
	}
}

func TestExecuteRejectsTamperCrossActorSessionAndToolBeforeClaim(t *testing.T) {
	signer := newActionTestSigner(t, time.Minute)
	proposal, _ := signer.Propose(7, "session-7", confirmation.ToolClockIn, "上班打卡", "确认", map[string]any{})
	tampered := json.RawMessage(`{"unexpected":true}`)
	tests := []ExecuteRequest{
		{UserID: 8, Role: "member", SessionID: "session-7", Tool: proposal.Tool, Token: proposal.Token, Parameters: proposal.Parameters},
		{UserID: 7, Role: "member", SessionID: "other", Tool: proposal.Tool, Token: proposal.Token, Parameters: proposal.Parameters},
		{UserID: 7, Role: "member", SessionID: "session-7", Tool: confirmation.ToolClockOut, Token: proposal.Token, Parameters: proposal.Parameters},
		{UserID: 7, Role: "member", SessionID: "session-7", Tool: proposal.Tool, Token: proposal.Token, Parameters: tampered},
	}
	repo := &fakeActionRepo{}
	checkins := &fakeCheckinWriter{}
	service := NewService(true, repo, signer, checkins, nil)
	for index, req := range tests {
		if _, err := service.Execute(context.Background(), req); !errors.Is(err, ErrInvalidConfirmation) {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
	if len(repo.records) != 0 || checkins.clockInCalls != 0 || checkins.clockOutCalls != 0 {
		t.Fatalf("rejected confirmation changed state: records=%d in=%d out=%d", len(repo.records), checkins.clockInCalls, checkins.clockOutCalls)
	}
}

func TestExecuteRejectsExpiredTokenAndUnauthorizedDraft(t *testing.T) {
	service := NewService(true, &fakeActionRepo{}, expiredVerifier{}, &fakeCheckinWriter{}, nil)
	_, err := service.Execute(context.Background(), ExecuteRequest{UserID: 7, Role: "member", SessionID: "session-7", Tool: confirmation.ToolClockIn, Token: "expired", Parameters: json.RawMessage(`{}`)})
	if !errors.Is(err, ErrExpiredConfirmation) {
		t.Fatalf("expired error = %v", err)
	}

	signer := newActionTestSigner(t, time.Minute)
	draft, _ := signer.Propose(7, "session-7", confirmation.ToolDraftAnnouncement, "创建草稿", "确认", confirmation.DraftAnnouncementParameters{Title: "通知", Content: "正文"})
	drafts := &fakeDraftWriter{}
	service = NewService(true, &fakeActionRepo{}, signer, nil, drafts)
	_, err = service.Execute(context.Background(), ExecuteRequest{UserID: 7, Role: "member", SessionID: "session-7", Tool: draft.Tool, Token: draft.Token, Parameters: draft.Parameters})
	if !errors.Is(err, shared.ErrForbidden) || drafts.calls != 0 {
		t.Fatalf("unauthorized draft error=%v calls=%d", err, drafts.calls)
	}
}

func TestExecutePersistsFailureAndDoesNotRetryBusinessWrite(t *testing.T) {
	signer := newActionTestSigner(t, time.Minute)
	proposal, _ := signer.Propose(7, "session-7", confirmation.ToolClockOut, "下班打卡", "确认", map[string]any{})
	repo := &fakeActionRepo{}
	checkins := &fakeCheckinWriter{err: checkin.ErrNotClockedIn}
	service := NewService(true, repo, signer, checkins, nil)
	req := ExecuteRequest{UserID: 7, Role: "member", SessionID: "session-7", Tool: proposal.Tool, Token: proposal.Token, Parameters: proposal.Parameters}
	first, err := service.Execute(context.Background(), req)
	if err != nil || first.Status != "failed" {
		t.Fatalf("failure result = %+v err=%v", first, err)
	}
	second, err := service.Execute(context.Background(), req)
	if err != nil || second.Status != "failed" || !second.Replayed || checkins.clockOutCalls != 1 {
		t.Fatalf("failure replay = %+v err=%v calls=%d", second, err, checkins.clockOutCalls)
	}
}
