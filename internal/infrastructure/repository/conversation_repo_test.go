package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"SWPUCAT/internal/domain/conversation"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestConversationDeleteDetachesAuditAndStoresOnlySessionHash(t *testing.T) {
	auditRepo, mock := newAgentAuditMockRepo(t)
	repo := NewConversationRepo(auditRepo.db)
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	sessionID := "session-7"
	hash := conversationSessionHash(sessionID)
	if hash == sessionID || len(hash) != 64 {
		t.Fatalf("session hash = %q", hash)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "agent_conversations".*FOR UPDATE`).
		WithArgs(int64(7), sessionID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "session_id"}).AddRow(73, 7, sessionID))
	mock.ExpectExec("INSERT INTO agent_conversation_tombstones").
		WithArgs(int64(7), hash, "user", now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "agent_runs" SET "conversation_id"=$1,"session_id"=$2 WHERE conversation_id IN ($3)`)).
		WithArgs(nil, nil, int64(73)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "agent_conversations"`).
		WithArgs(int64(73), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.DeleteOwned(context.Background(), 7, sessionID, now); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveResponseFeedbackRequiresOwnedAssistantAndMatchingAudit(t *testing.T) {
	auditRepo, mock := newAgentAuditMockRepo(t)
	repo := NewConversationRepo(auditRepo.db)
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	feedback := &conversation.ResponseFeedback{
		UserID: 7, MessageID: 31, RequestID: "0123456789abcdef0123456789abcdef",
		Helpful: true, Note: "解决了问题", CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectExec("INSERT INTO agent_response_feedback").
		WithArgs(feedback.UserID, feedback.RequestID, feedback.Helpful, feedback.Note, now, now, feedback.RequestID, feedback.UserID, feedback.MessageID, feedback.RequestID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.SaveResponseFeedback(context.Background(), feedback); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveResponseFeedbackRejectsUnownedOrMismatchedResponse(t *testing.T) {
	auditRepo, mock := newAgentAuditMockRepo(t)
	repo := NewConversationRepo(auditRepo.db)
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	feedback := &conversation.ResponseFeedback{
		UserID: 7, MessageID: 31, RequestID: "0123456789abcdef0123456789abcdef",
		Helpful: false, CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectExec("INSERT INTO agent_response_feedback").
		WithArgs(feedback.UserID, feedback.RequestID, feedback.Helpful, feedback.Note, now, now, feedback.RequestID, feedback.UserID, feedback.MessageID, feedback.RequestID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	err := repo.SaveResponseFeedback(context.Background(), feedback)
	if !errors.Is(err, conversation.ErrInvalidFeedback) {
		t.Fatalf("error = %v, want ErrInvalidFeedback", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
