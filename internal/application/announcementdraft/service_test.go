package announcementdraftapp

import (
	announcementapp "SWPUCAT/internal/application/announcement"
	"SWPUCAT/internal/domain/announcementdraft"
	"context"
	"testing"
)

type fakeAnnouncementDraftRepo struct {
	draft *announcementdraft.Draft
}

func (r *fakeAnnouncementDraftRepo) Create(_ context.Context, draft *announcementdraft.Draft) error {
	draft.ID = 1
	r.draft = draft
	return nil
}

func (r *fakeAnnouncementDraftRepo) FindByID(_ context.Context, _ int64) (*announcementdraft.Draft, error) {
	if r.draft == nil {
		return nil, nil
	}
	copy := *r.draft
	return &copy, nil
}

func (r *fakeAnnouncementDraftRepo) Update(_ context.Context, draft *announcementdraft.Draft, _ int64, expected announcementdraft.Status) error {
	if r.draft.Status != expected {
		return announcementdraft.ErrInvalidDraft
	}
	r.draft = draft
	return nil
}

type fakeAnnouncementPublisher struct {
	created  int
	notified int
}

func (p *fakeAnnouncementPublisher) Create(_ context.Context, _ int64, _ string, _ announcementapp.CreateRequest) (*announcementapp.AnnouncementDTO, error) {
	p.created++
	return &announcementapp.AnnouncementDTO{ID: 88}, nil
}

func (p *fakeAnnouncementPublisher) NotifyMembers(_ context.Context, _, _ int64, ids []int64) (*announcementapp.NotifyResult, error) {
	p.notified += len(ids)
	return &announcementapp.NotifyResult{Sent: len(ids)}, nil
}

func TestServicePublishRequiresConfirmedDraftAndRecordsEmailResult(t *testing.T) {
	repo := &fakeAnnouncementDraftRepo{}
	publisher := &fakeAnnouncementPublisher{}
	service := NewService(repo, publisher)
	ctx := context.Background()

	draft, err := service.Create(ctx, 7, CreateRequest{Title: "通知", Content: "正文", RecipientIDs: []int64{4, 4, 2}, EmailRequested: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.RecipientIDs) != 2 || draft.RecipientIDs[0] != 2 || draft.RecipientIDs[1] != 4 {
		t.Fatalf("recipients were not normalized: %+v", draft.RecipientIDs)
	}
	if _, err := service.Publish(ctx, 7, "队长", 1); err == nil {
		t.Fatal("published an unconfirmed draft")
	}
	if _, err := service.Confirm(ctx, 7, 1); err != nil {
		t.Fatal(err)
	}
	published, err := service.Publish(ctx, 7, "队长", 1)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != string(announcementdraft.StatusPublished) || published.AnnouncementID != 88 {
		t.Fatalf("unexpected published draft: %+v", published)
	}
	if publisher.created != 1 || publisher.notified != 2 || published.EmailResultJSON == "" {
		t.Fatalf("publication side effects were not recorded: publisher=%+v draft=%+v", publisher, published)
	}
}

func TestServiceRejectsEmailDraftWithoutRecipients(t *testing.T) {
	service := NewService(&fakeAnnouncementDraftRepo{}, &fakeAnnouncementPublisher{})
	if _, err := service.Create(context.Background(), 7, CreateRequest{Title: "通知", Content: "正文", EmailRequested: true}); err == nil {
		t.Fatal("accepted email draft without recipients")
	}
}
