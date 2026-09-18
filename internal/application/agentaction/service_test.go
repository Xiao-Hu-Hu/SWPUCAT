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
	evaluationapp "SWPUCAT/internal/application/evaluation"
	invitationapp "SWPUCAT/internal/application/invitation"
	"SWPUCAT/internal/domain/checkin"
	domainevaluation "SWPUCAT/internal/domain/evaluation"
	domaininvitation "SWPUCAT/internal/domain/invitation"
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

type fakeKnowledgeReviewWriter struct {
	approveCalls int
	rejectCalls  int
	itemID       int64
	reviewerID   int64
	reviewerName string
	reason       string
	err          error
}

func (w *fakeKnowledgeReviewWriter) ApproveItem(_ context.Context, itemID, reviewerID int64, reviewerName string) error {
	w.approveCalls++
	w.itemID, w.reviewerID, w.reviewerName = itemID, reviewerID, reviewerName
	return w.err
}

func (w *fakeKnowledgeReviewWriter) RejectItem(_ context.Context, itemID, reviewerID int64, reviewerName, reason string) (string, error) {
	w.rejectCalls++
	w.itemID, w.reviewerID, w.reviewerName, w.reason = itemID, reviewerID, reviewerName, reason
	return "rejected", w.err
}

type fakeInvitationWriter struct {
	calls     int
	creatorID int64
	typeValue domaininvitation.InvitationType
	err       error
}

func (w *fakeInvitationWriter) GenerateCode(_ context.Context, creatorID int64, codeType domaininvitation.InvitationType) (*invitationapp.InvitationDTO, error) {
	w.calls++
	w.creatorID, w.typeValue = creatorID, codeType
	if w.err != nil {
		return nil, w.err
	}
	return &invitationapp.InvitationDTO{ID: 81, Code: "INVITE-SECRET-81", Type: string(codeType), ExpiresAt: "2026-09-18 12:10:00"}, nil
}

type fakeEvaluationDraftWriter struct {
	createCalls int
	updateCalls int
	createReq   evaluationapp.CreateDraftRequest
	updateReq   evaluationapp.UpdateDraftRequest
	draftID     int64
	actorID     int64
	err         error
}

func (w *fakeEvaluationDraftWriter) CreateDraft(_ context.Context, actorID int64, req evaluationapp.CreateDraftRequest) (*evaluationapp.DraftDTO, error) {
	w.createCalls++
	w.actorID, w.createReq = actorID, req
	if w.err != nil {
		return nil, w.err
	}
	return &evaluationapp.DraftDTO{ID: 91, SnapshotID: req.SnapshotID, PeriodID: 31, Status: "draft", Audience: string(req.Audience), Title: req.Title, SummaryMarkdown: "SECRET_FULL_SUMMARY", Members: []evaluationapp.MemberDTO{{UserID: 7, DisplayName: "SECRET_MEMBER"}}}, nil
}

func (w *fakeEvaluationDraftWriter) UpdateDraft(_ context.Context, id, actorID int64, req evaluationapp.UpdateDraftRequest) (*evaluationapp.DraftDTO, error) {
	w.updateCalls++
	w.draftID, w.actorID, w.updateReq = id, actorID, req
	if w.err != nil {
		return nil, w.err
	}
	return &evaluationapp.DraftDTO{ID: id, SnapshotID: 71, PeriodID: 31, Status: "draft", Audience: "captain", Title: req.Title, SummaryMarkdown: "SECRET_FULL_SUMMARY", Members: []evaluationapp.MemberDTO{{UserID: 7, DisplayName: "SECRET_MEMBER"}}}, nil
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

func TestExecuteKnowledgeReviewUsesRBACValidationAndReplaysOnce(t *testing.T) {
	signer := newActionTestSigner(t, time.Minute)

	t.Run("approve", func(t *testing.T) {
		proposal, err := signer.Propose(9, "session-9", confirmation.ToolReviewKnowledge, "批准知识", "确认", confirmation.KnowledgeReviewParameters{ItemID: 19, Decision: "approve"})
		if err != nil {
			t.Fatal(err)
		}
		writer := &fakeKnowledgeReviewWriter{}
		service := NewService(true, &fakeActionRepo{}, signer, nil, nil, WithKnowledgeReviews(writer))
		req := ExecuteRequest{UserID: 9, Role: "captain", ActorName: "队长甲", SessionID: "session-9", Tool: proposal.Tool, Token: proposal.Token, Parameters: proposal.Parameters}
		first, err := service.Execute(context.Background(), req)
		if err != nil || first.Status != "success" || strings.Contains(string(first.Data), "SECRET") {
			t.Fatalf("first result=%+v err=%v", first, err)
		}
		second, err := service.Execute(context.Background(), req)
		if err != nil || !second.Replayed || writer.approveCalls != 1 || writer.rejectCalls != 0 || writer.itemID != 19 || writer.reviewerID != 9 || writer.reviewerName != "队长甲" {
			t.Fatalf("replay=%+v err=%v writer=%+v", second, err, writer)
		}
	})

	t.Run("reject", func(t *testing.T) {
		proposal, err := signer.Propose(1, "session-1", confirmation.ToolReviewKnowledge, "拒绝知识", "确认", confirmation.KnowledgeReviewParameters{ItemID: 20, Decision: "reject", Reason: "资料来源无法核验"})
		if err != nil {
			t.Fatal(err)
		}
		writer := &fakeKnowledgeReviewWriter{}
		service := NewService(true, &fakeActionRepo{}, signer, nil, nil, WithKnowledgeReviews(writer))
		req := ExecuteRequest{UserID: 1, Role: "super_admin", SessionID: "session-1", Tool: proposal.Tool, Token: proposal.Token, Parameters: proposal.Parameters}
		result, err := service.Execute(context.Background(), req)
		if err != nil || result.Status != "success" || writer.rejectCalls != 1 || writer.reason != "资料来源无法核验" || writer.reviewerName != "Agent 确认操作" {
			t.Fatalf("result=%+v err=%v writer=%+v", result, err, writer)
		}
	})

	t.Run("member and invalid reject blocked before claim", func(t *testing.T) {
		writer := &fakeKnowledgeReviewWriter{}
		repo := &fakeActionRepo{}
		service := NewService(true, repo, signer, nil, nil, WithKnowledgeReviews(writer))
		memberProposal, _ := signer.Propose(7, "session-7", confirmation.ToolReviewKnowledge, "批准知识", "确认", confirmation.KnowledgeReviewParameters{ItemID: 19, Decision: "approve"})
		_, err := service.Execute(context.Background(), ExecuteRequest{UserID: 7, Role: "member", SessionID: "session-7", Tool: memberProposal.Tool, Token: memberProposal.Token, Parameters: memberProposal.Parameters})
		if !errors.Is(err, shared.ErrForbidden) {
			t.Fatalf("member error = %v", err)
		}
		invalidProposal, _ := signer.Propose(9, "session-invalid", confirmation.ToolReviewKnowledge, "拒绝知识", "确认", confirmation.KnowledgeReviewParameters{ItemID: 19, Decision: "reject"})
		_, err = service.Execute(context.Background(), ExecuteRequest{UserID: 9, Role: "captain", SessionID: "session-invalid", Tool: invalidProposal.Tool, Token: invalidProposal.Token, Parameters: invalidProposal.Parameters})
		if !errors.Is(err, ErrInvalidConfirmation) || len(repo.records) != 0 || writer.approveCalls+writer.rejectCalls != 0 {
			t.Fatalf("invalid reject error=%v records=%d writer=%+v", err, len(repo.records), writer)
		}
	})
}

func TestExecuteInvitationRestrictsTypeAndReturnsCodeOnlyAfterConfirmation(t *testing.T) {
	signer := newActionTestSigner(t, time.Minute)

	captainProposal, _ := signer.Propose(9, "session-9", confirmation.ToolGenerateInvite, "生成邀请码", "确认", confirmation.InvitationParameters{Type: "member"})
	writer := &fakeInvitationWriter{}
	service := NewService(true, &fakeActionRepo{}, signer, nil, nil, WithInvitations(writer))
	req := ExecuteRequest{UserID: 9, Role: "captain", SessionID: "session-9", Tool: captainProposal.Tool, Token: captainProposal.Token, Parameters: captainProposal.Parameters}
	result, err := service.Execute(context.Background(), req)
	if err != nil || result.Status != "success" || !strings.Contains(string(result.Data), "INVITE-SECRET-81") || writer.calls != 1 || writer.creatorID != 9 || writer.typeValue != domaininvitation.TypeMember {
		t.Fatalf("captain result=%+v err=%v writer=%+v", result, err, writer)
	}
	replayed, err := service.Execute(context.Background(), req)
	if err != nil || !replayed.Replayed || writer.calls != 1 {
		t.Fatalf("replay=%+v err=%v calls=%d", replayed, err, writer.calls)
	}

	forbiddenProposal, _ := signer.Propose(9, "session-forbidden", confirmation.ToolGenerateInvite, "生成队长邀请码", "确认", confirmation.InvitationParameters{Type: "captain"})
	_, err = service.Execute(context.Background(), ExecuteRequest{UserID: 9, Role: "captain", SessionID: "session-forbidden", Tool: forbiddenProposal.Tool, Token: forbiddenProposal.Token, Parameters: forbiddenProposal.Parameters})
	if !errors.Is(err, shared.ErrForbidden) || writer.calls != 1 {
		t.Fatalf("captain captain-code error=%v calls=%d", err, writer.calls)
	}

	adminProposal, _ := signer.Propose(1, "session-1", confirmation.ToolGenerateInvite, "生成队长邀请码", "确认", confirmation.InvitationParameters{Type: "captain"})
	adminWriter := &fakeInvitationWriter{}
	adminService := NewService(true, &fakeActionRepo{}, signer, nil, nil, WithInvitations(adminWriter))
	adminResult, err := adminService.Execute(context.Background(), ExecuteRequest{UserID: 1, Role: "super_admin", SessionID: "session-1", Tool: adminProposal.Tool, Token: adminProposal.Token, Parameters: adminProposal.Parameters})
	if err != nil || adminResult.Status != "success" || adminWriter.calls != 1 || adminWriter.typeValue != domaininvitation.TypeCaptain {
		t.Fatalf("admin result=%+v err=%v writer=%+v", adminResult, err, adminWriter)
	}
}

func TestExecuteEvaluationOnlyCreatesOrUpdatesDraftAndReturnsMinimalProjection(t *testing.T) {
	signer := newActionTestSigner(t, time.Minute)
	tests := []struct {
		name       string
		parameters confirmation.EvaluationDraftParameters
		wantCreate int
		wantUpdate int
	}{
		{name: "create", parameters: confirmation.EvaluationDraftParameters{Mode: "create", SnapshotID: 71, Title: "九月考评", SummaryMarkdown: "完整总结", Audience: "captain"}, wantCreate: 1},
		{name: "update", parameters: confirmation.EvaluationDraftParameters{Mode: "update", DraftID: 91, Title: "九月考评修订", SummaryMarkdown: "修订总结"}, wantUpdate: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proposal, err := signer.Propose(9, "session-"+test.name, confirmation.ToolSaveEvaluation, "保存考评草稿", "确认", test.parameters)
			if err != nil {
				t.Fatal(err)
			}
			writer := &fakeEvaluationDraftWriter{}
			service := NewService(true, &fakeActionRepo{}, signer, nil, nil, WithEvaluationDrafts(writer))
			req := ExecuteRequest{UserID: 9, Role: "captain", SessionID: "session-" + test.name, Tool: proposal.Tool, Token: proposal.Token, Parameters: proposal.Parameters}
			result, err := service.Execute(context.Background(), req)
			if err != nil || result.Status != "success" || writer.createCalls != test.wantCreate || writer.updateCalls != test.wantUpdate {
				t.Fatalf("result=%+v err=%v writer=%+v", result, err, writer)
			}
			serialized := string(result.Data)
			for _, forbidden := range []string{"summary_markdown", "members", "SECRET_FULL_SUMMARY", "SECRET_MEMBER", "user_id", "evidence"} {
				if strings.Contains(serialized, forbidden) {
					t.Fatalf("result leaked %q: %s", forbidden, serialized)
				}
			}
			replayed, err := service.Execute(context.Background(), req)
			if err != nil || !replayed.Replayed || writer.createCalls != test.wantCreate || writer.updateCalls != test.wantUpdate {
				t.Fatalf("replay=%+v err=%v writer=%+v", replayed, err, writer)
			}
			if test.name == "create" && (writer.createReq.SnapshotID != 71 || writer.createReq.Audience != domainevaluation.DraftAudienceCaptain || writer.createReq.Model != "agent") {
				t.Fatalf("create request = %+v", writer.createReq)
			}
			if test.name == "update" && (writer.draftID != 91 || writer.updateReq.Title != "九月考评修订") {
				t.Fatalf("update request id=%d req=%+v", writer.draftID, writer.updateReq)
			}
		})
	}
}

func TestExecuteGovernedWriteFailureNeverClaimsSuccessAndIsNotRetried(t *testing.T) {
	signer := newActionTestSigner(t, time.Minute)
	proposal, _ := signer.Propose(9, "session-9", confirmation.ToolReviewKnowledge, "批准知识", "确认", confirmation.KnowledgeReviewParameters{ItemID: 19, Decision: "approve"})
	writer := &fakeKnowledgeReviewWriter{err: errors.New("database unavailable")}
	service := NewService(true, &fakeActionRepo{}, signer, nil, nil, WithKnowledgeReviews(writer))
	req := ExecuteRequest{UserID: 9, Role: "captain", SessionID: "session-9", Tool: proposal.Tool, Token: proposal.Token, Parameters: proposal.Parameters}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := service.Execute(context.Background(), req)
		if err != nil || result.Status != "failed" || strings.Contains(result.Title+result.Message, "成功") {
			t.Fatalf("attempt %d result=%+v err=%v", attempt, result, err)
		}
	}
	if writer.approveCalls != 1 {
		t.Fatalf("failed business write calls = %d, want 1", writer.approveCalls)
	}
}
