package evaluation

import "context"

type Repository interface {
	FindSnapshotByID(ctx context.Context, id int64) (*Snapshot, error)
	CreateDraft(ctx context.Context, draft *Draft) error
	FindDraftByID(ctx context.Context, id int64) (*Draft, error)
	UpdateDraft(ctx context.Context, draft *Draft, editorID int64, expectedStatus DraftStatus) error
	SaveMemberComment(ctx context.Context, member *DraftMember, editorID int64) error
}
