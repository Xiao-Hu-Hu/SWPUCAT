package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"SWPUCAT/internal/application/agentaction"
	"SWPUCAT/internal/domain/shared"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAgentActionClaimStoresOnlyDigestAndWinsOnce(t *testing.T) {
	auditRepo, mock := newAgentAuditMockRepo(t)
	repo := NewAgentActionRepo(auditRepo.db)
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	record := &agentaction.Record{
		TokenHash: stringOfLength("a", 64), UserID: 7, SessionID: "session-7", ToolName: "clock_in",
		ParametersDigest: stringOfLength("b", 64), Status: "executing", ResultJSON: "{}",
		ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now,
	}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "agent_action_confirmations"`).
		WithArgs(record.TokenHash, record.UserID, record.SessionID, record.ToolName, record.ParametersDigest, record.Status, record.ResultJSON, nil, record.ExpiresAt, record.CreatedAt, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	existing, claimed, err := repo.Claim(context.Background(), record)
	if err != nil || !claimed || existing != record {
		t.Fatalf("claim existing=%+v claimed=%t err=%v", existing, claimed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentActionClaimLoadsExistingWinnerAfterConflict(t *testing.T) {
	auditRepo, mock := newAgentAuditMockRepo(t)
	repo := NewAgentActionRepo(auditRepo.db)
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	record := &agentaction.Record{
		TokenHash: stringOfLength("a", 64), UserID: 7, SessionID: "session-7", ToolName: "clock_in",
		ParametersDigest: stringOfLength("b", 64), Status: "executing", ResultJSON: "{}",
		ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now,
	}
	completedAt := now.Add(time.Second)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "agent_action_confirmations"`).
		WithArgs(record.TokenHash, record.UserID, record.SessionID, record.ToolName, record.ParametersDigest, record.Status, record.ResultJSON, nil, record.ExpiresAt, record.CreatedAt, nil).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .* FROM "agent_action_confirmations" WHERE token_hash = \$1.*LIMIT \$2`).
		WithArgs(record.TokenHash, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"token_hash", "user_id", "session_id", "tool_name", "parameters_digest", "status", "result_json", "error_code", "expires_at", "created_at", "completed_at",
		}).AddRow(record.TokenHash, record.UserID, record.SessionID, record.ToolName, record.ParametersDigest, "succeeded", `{"status":"success"}`, nil, record.ExpiresAt, record.CreatedAt, completedAt))

	existing, claimed, err := repo.Claim(context.Background(), record)
	if err != nil || claimed || existing == nil || existing.Status != "succeeded" || existing.CompletedAt == nil || !existing.CompletedAt.Equal(completedAt) {
		t.Fatalf("existing=%+v claimed=%t err=%v", existing, claimed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentActionCompleteTransitionsOnlyExecutingClaim(t *testing.T) {
	auditRepo, mock := newAgentAuditMockRepo(t)
	repo := NewAgentActionRepo(auditRepo.db)
	completedAt := time.Date(2026, 9, 18, 12, 0, 1, 0, time.UTC)
	tokenHash := stringOfLength("a", 64)
	query := regexp.QuoteMeta(`UPDATE "agent_action_confirmations" SET "completed_at"=$1,"error_code"=$2,"result_json"=$3,"status"=$4 WHERE token_hash = $5 AND status = $6`)

	mock.ExpectBegin()
	mock.ExpectExec(query).
		WithArgs(completedAt, "business_operation_failed", `{"status":"failed"}`, "failed", tokenHash, "executing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.Complete(context.Background(), tokenHash, "failed", `{"status":"failed"}`, "business_operation_failed", completedAt); err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(query).
		WithArgs(completedAt, nil, `{"status":"success"}`, "succeeded", tokenHash, "executing").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := repo.Complete(context.Background(), tokenHash, "succeeded", `{"status":"success"}`, "", completedAt); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func stringOfLength(value string, count int) string {
	result := ""
	for index := 0; index < count; index++ {
		result += value
	}
	return result
}
