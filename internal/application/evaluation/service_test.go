package evaluationapp

import (
	"SWPUCAT/internal/domain/evaluation"
	"SWPUCAT/internal/domain/shared"
	"context"
	"testing"
)

type fakeEvaluationRepo struct {
	snapshot *evaluation.Snapshot
	draft    *evaluation.Draft
}

func (r *fakeEvaluationRepo) FindSnapshotByID(context.Context, int64) (*evaluation.Snapshot, error) {
	return r.snapshot, nil
}

func (r *fakeEvaluationRepo) CreateDraft(_ context.Context, draft *evaluation.Draft) error {
	draft.ID = 1
	r.draft = draft
	return nil
}

func (r *fakeEvaluationRepo) FindDraftByID(context.Context, int64) (*evaluation.Draft, error) {
	return r.draft, nil
}

func (r *fakeEvaluationRepo) UpdateDraft(_ context.Context, draft *evaluation.Draft, _ int64, _ evaluation.DraftStatus) error {
	r.draft = draft
	return nil
}

func (r *fakeEvaluationRepo) SaveMemberComment(_ context.Context, member *evaluation.DraftMember, _ int64) error {
	for i := range r.draft.Members {
		if r.draft.Members[i].UserID == member.UserID {
			r.draft.Members[i].CommentMarkdown = member.CommentMarkdown
			r.draft.Members[i].Recommendation = member.Recommendation
			r.draft.Members[i].ManualOverride = true
			return nil
		}
	}
	return shared.ErrNotFound
}

func TestCreateDraftCopiesSnapshotEvidenceAndRequiresPublicationFlow(t *testing.T) {
	repo := &fakeEvaluationRepo{snapshot: &evaluation.Snapshot{ID: 7, PeriodID: 4, Status: evaluation.SnapshotPublished, Members: []evaluation.Member{{UserID: 3, DisplayName: "成员", TotalMinutes: 120, TotalHours: 2, AttendanceDays: 5, AchievedWeeks: 2, ReportStatus: evaluation.ReportReviewed, DataState: evaluation.DataComplete}}}}
	svc := NewService(repo)
	draft, err := svc.CreateDraft(context.Background(), 9, CreateDraftRequest{SnapshotID: 7, SummaryMarkdown: "总结"})
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Members) != 1 || draft.Members[0].EvidenceJSON == "" || draft.Members[0].ManualOverride {
		t.Fatalf("snapshot evidence was not copied into draft: %+v", draft.Members)
	}
	if _, err := svc.PublishDraft(context.Background(), 1, 9); err != evaluation.ErrSnapshotChanged {
		t.Fatalf("unreviewed draft was published: %v", err)
	}
	if _, err := svc.ReviewDraft(context.Background(), 1, 9, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveMemberComment(context.Background(), 1, 9, 3, MemberCommentRequest{CommentMarkdown: "表现稳定", Recommendation: "通过"}); err != shared.ErrConflict {
		t.Fatalf("edited a reviewed draft: %v", err)
	}
	if _, err := svc.ReviewDraft(context.Background(), 1, 9, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveMemberComment(context.Background(), 1, 9, 3, MemberCommentRequest{CommentMarkdown: "表现稳定", Recommendation: "通过"}); err != nil {
		t.Fatal(err)
	}
	if repo.draft.Members[0].EvidenceJSON == "" || !repo.draft.Members[0].ManualOverride {
		t.Fatalf("manual edit replaced or lost evidence: %+v", repo.draft.Members[0])
	}
	if _, err := svc.ReviewDraft(context.Background(), 1, 9, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishDraft(context.Background(), 1, 9); err != nil {
		t.Fatal(err)
	}
	if repo.draft.Status != evaluation.DraftStatusPublished {
		t.Fatalf("draft was not published: %s", repo.draft.Status)
	}
}

func TestInvalidatedSnapshotInvalidatesUnpublishedDraft(t *testing.T) {
	repo := &fakeEvaluationRepo{snapshot: &evaluation.Snapshot{ID: 7, PeriodID: 4, Status: evaluation.SnapshotPublished}}
	svc := NewService(repo)
	if _, err := svc.CreateDraft(context.Background(), 9, CreateDraftRequest{SnapshotID: 7, SummaryMarkdown: "总结"}); err != nil {
		t.Fatal(err)
	}
	repo.snapshot.Status = evaluation.SnapshotInvalidated
	if _, err := svc.GetDraft(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateDraft(context.Background(), 1, 9, UpdateDraftRequest{SummaryMarkdown: "修改"}); err != shared.ErrConflict {
		t.Fatalf("stale draft was accepted: %v", err)
	}
	if repo.draft.Status != evaluation.DraftStatusInvalidated {
		t.Fatalf("stale draft was not invalidated: %s", repo.draft.Status)
	}
}
