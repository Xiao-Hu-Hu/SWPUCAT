package conversationapp

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"SWPUCAT/internal/domain/conversation"
	"SWPUCAT/internal/domain/shared"
)

type fakeConversationRepo struct {
	mu            sync.Mutex
	conversations map[string]*conversation.Conversation
	messages      map[string][]*conversation.Message
	tombstones    map[string]time.Time
	auditDetached map[int64]bool
	nextID        int64
}

func newFakeConversationRepo() *fakeConversationRepo {
	return &fakeConversationRepo{
		conversations: map[string]*conversation.Conversation{}, messages: map[string][]*conversation.Message{},
		tombstones: map[string]time.Time{}, auditDetached: map[int64]bool{}, nextID: 1,
	}
}

func (r *fakeConversationRepo) Create(_ context.Context, item *conversation.Conversation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fakeConversationKey(item.UserID, item.SessionID)
	if _, deleted := r.tombstones[key]; deleted {
		return conversation.ErrDeletedConversation
	}
	if _, exists := r.conversations[key]; exists {
		return shared.ErrConflict
	}
	item.ID = r.nextID
	r.nextID++
	copy := *item
	r.conversations[key] = &copy
	return nil
}

func (r *fakeConversationRepo) DeleteOwned(_ context.Context, userID int64, sessionID string, deletedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fakeConversationKey(userID, sessionID)
	item := r.conversations[key]
	if item == nil {
		return nil
	}
	r.tombstones[key] = deletedAt
	r.auditDetached[item.ID] = true
	delete(r.messages, key)
	delete(r.conversations, key)
	return nil
}

func (r *fakeConversationRepo) CountExpired(_ context.Context, before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var count int64
	for _, item := range r.conversations {
		if item.LastActiveAt.Before(before) {
			count++
		}
	}
	return count, nil
}

func (r *fakeConversationRepo) DeleteExpired(_ context.Context, before time.Time, limit int, deletedAt time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var deleted int64
	for key, item := range r.conversations {
		if deleted >= int64(limit) || !item.LastActiveAt.Before(before) {
			continue
		}
		r.tombstones[key] = deletedAt
		r.auditDetached[item.ID] = true
		delete(r.messages, key)
		delete(r.conversations, key)
		deleted++
	}
	return deleted, nil
}

func (r *fakeConversationRepo) FindBySession(_ context.Context, userID int64, sessionID string) (*conversation.Conversation, error) {
	item := r.conversations[fakeConversationKey(userID, sessionID)]
	if item == nil {
		return nil, shared.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (r *fakeConversationRepo) UpdateTitle(_ context.Context, userID int64, sessionID, title string) error {
	item := r.conversations[fakeConversationKey(userID, sessionID)]
	if item == nil {
		return shared.ErrNotFound
	}
	item.Title = title
	item.TitleCustomized = true
	item.UpdatedAt = time.Now()
	return nil
}

func (r *fakeConversationRepo) ListRecent(_ context.Context, userID int64, limit int) ([]*conversation.Conversation, error) {
	items := make([]*conversation.Conversation, 0)
	for _, item := range r.conversations {
		if item.UserID != userID {
			continue
		}
		copy := *item
		items = append(items, &copy)
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *fakeConversationRepo) ListRecentMessages(_ context.Context, userID int64, sessionID string, limit int) ([]*conversation.Message, error) {
	key := fakeConversationKey(userID, sessionID)
	if r.conversations[key] == nil {
		return nil, shared.ErrNotFound
	}
	items := append([]*conversation.Message(nil), r.messages[key]...)
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	result := make([]*conversation.Message, 0, len(items))
	for _, item := range items {
		copy := *item
		result = append(result, &copy)
	}
	return result, nil
}

func (r *fakeConversationRepo) RecordTurn(_ context.Context, userID int64, sessionID, title string, titleCustomized bool, userMessage, assistantMessage *conversation.Message, summary, sources string, activeAt time.Time) error {
	key := fakeConversationKey(userID, sessionID)
	stored := r.conversations[key]
	if stored == nil || stored.ID != userMessage.ConversationID || stored.ID != assistantMessage.ConversationID {
		return shared.ErrNotFound
	}
	for _, item := range []*conversation.Message{userMessage, assistantMessage} {
		item.ID = r.nextID
		r.nextID++
		if item.CreatedAt.IsZero() {
			item.CreatedAt = time.Now()
		}
		copy := *item
		r.messages[key] = append(r.messages[key], &copy)
	}
	if err := stored.UpdateContext(summary, sources, activeAt); err != nil {
		return err
	}
	stored.Title = title
	stored.TitleCustomized = titleCustomized
	return nil
}

func fakeConversationKey(userID int64, sessionID string) string {
	return strconv.FormatInt(userID, 10) + ":" + sessionID
}

func TestServiceScopesSameSessionIDToItsOwner(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	ctx := context.Background()

	first, err := service.GetOrCreate(ctx, 11, "same-client-session", "第一个用户")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordTurn(ctx, 11, first.SessionID, RecordTurnRequest{UserContent: "部署文档在哪里", AssistantContent: "在知识库", CitationsJSON: `[{"source_id":5}]`}); err != nil {
		t.Fatal(err)
	}

	second, err := service.GetOrCreate(ctx, 22, "same-client-session", "第二个用户")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("different users unexpectedly share a conversation")
	}
	loaded, err := service.LoadContext(ctx, 22, second.SessionID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 0 || loaded.Conversation.Title != "第二个用户" {
		t.Fatalf("owner received another user's history: %+v", loaded)
	}
}

func TestNewConversationGetsDefaultTitleAndFirstTurnTitle(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	ctx := context.Background()

	created, err := service.GetOrCreate(ctx, 7, "new-session", "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != conversation.DefaultTitle {
		t.Fatalf("default title = %q", created.Title)
	}
	updated, err := service.RecordTurn(ctx, 7, created.SessionID, RecordTurnRequest{
		UserContent:      "请帮我查询本周的打卡记录",
		AssistantContent: "好的，我来查询。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "请帮我查询本周的打卡记录" {
		t.Fatalf("auto title = %q", updated.Title)
	}
}

func TestListRecentOnlyReturnsOwnedConversations(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	ctx := context.Background()
	if _, err := service.GetOrCreate(ctx, 7, "owner-session", "自己的会话"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetOrCreate(ctx, 8, "other-session", "别人的会话"); err != nil {
		t.Fatal(err)
	}
	items, err := service.ListRecent(ctx, 7, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "自己的会话" {
		t.Fatalf("owned conversations = %+v", items)
	}
}

func TestUpdateTitlePersistsAndPreventsAutomaticOverwrite(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	ctx := context.Background()
	created, err := service.GetOrCreate(ctx, 7, "rename-session", "")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateTitle(ctx, 7, created.SessionID, "我指定的标题")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "我指定的标题" {
		t.Fatalf("updated title = %q", updated.Title)
	}
	turn, err := service.RecordTurn(ctx, 7, created.SessionID, RecordTurnRequest{
		UserContent:      "首条消息不应覆盖标题",
		AssistantContent: "明白。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if turn.Title != "我指定的标题" {
		t.Fatalf("custom title was overwritten: %q", turn.Title)
	}
}

func TestUpdateTitleValidatesAndScopesOwner(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	ctx := context.Background()
	created, err := service.GetOrCreate(ctx, 7, "rename-validation", "标题")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateTitle(ctx, 7, created.SessionID, "   "); err != shared.ErrValidation {
		t.Fatalf("blank title error = %v", err)
	}
	if _, err := service.UpdateTitle(ctx, 8, created.SessionID, "越权修改"); err != shared.ErrNotFound {
		t.Fatalf("cross-owner error = %v", err)
	}
	longTitle := make([]rune, conversation.MaxTitleRunes+1)
	for i := range longTitle {
		longTitle[i] = '长'
	}
	if _, err := service.UpdateTitle(ctx, 7, created.SessionID, string(longTitle)); err != shared.ErrValidation {
		t.Fatalf("long title error = %v", err)
	}
}

func TestRecordTurnStoresChronologicalMessagesAndRollingSummary(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	ctx := context.Background()
	created, err := service.GetOrCreate(ctx, 7, "session-7", "部署咨询")
	if err != nil {
		t.Fatal(err)
	}
	summary := "用户在询问部署和数据库配置。"
	sources := `[{"source_id":12,"title":"部署手册"}]`
	updated, err := service.RecordTurn(ctx, 7, created.SessionID, RecordTurnRequest{
		UserContent:         "数据库如何配置？",
		AssistantContent:    "请参考部署手册中的数据库章节。",
		CitationsJSON:       `[{"source_id":12}]`,
		RollingSummary:      &summary,
		LastSourcesJSON:     &sources,
		AssistantTokenCount: 33,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.RollingSummary != summary || updated.LastSourcesJSON != sources {
		t.Fatalf("rolling state = %+v", updated)
	}
	loaded, err := service.LoadContext(ctx, 7, created.SessionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 2 || loaded.Messages[0].Role != string(conversation.RoleUser) || loaded.Messages[1].Role != string(conversation.RoleAssistant) {
		t.Fatalf("messages not stored chronologically: %+v", loaded.Messages)
	}
	if loaded.Messages[1].CitationsJSON != `[{"source_id":12}]` || loaded.Messages[1].TokenCount != 33 {
		t.Fatalf("assistant metadata not persisted: %+v", loaded.Messages[1])
	}
}

func TestLoadContextLimitsRecentMessages(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	ctx := context.Background()
	created, err := service.GetOrCreate(ctx, 7, "session-7", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"问题一", "回答一"}, {"问题二", "回答二"}} {
		if _, err := service.RecordTurn(ctx, 7, created.SessionID, RecordTurnRequest{UserContent: pair[0], AssistantContent: pair[1]}); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := service.LoadContext(ctx, 7, created.SessionID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 3 || loaded.Messages[0].Content != "回答一" || loaded.Messages[2].Content != "回答二" {
		t.Fatalf("recent messages = %+v", loaded.Messages)
	}
}

func TestNormalizeJSONArrayRejectsObject(t *testing.T) {
	if _, err := normalizeJSONArray(`{"source_id":12}`); err == nil {
		t.Fatal("accepted a non-array JSON payload")
	}
}

func TestDeleteIsOwnerScopedIdempotentAndPreventsRecreation(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	ctx := context.Background()
	owned, err := service.GetOrCreate(ctx, 7, "same-session", "自己的会话")
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.GetOrCreate(ctx, 8, "same-session", "别人的会话")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordTurn(ctx, 7, owned.SessionID, RecordTurnRequest{UserContent: "敏感问题", AssistantContent: "敏感回答"}); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, 7, owned.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, 7, owned.SessionID); err != nil {
		t.Fatalf("repeated deletion failed: %v", err)
	}
	if _, err := service.LoadContext(ctx, 7, owned.SessionID, 20); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("deleted context error = %v", err)
	}
	if _, err := service.GetOrCreate(ctx, 7, owned.SessionID, "试图复活"); !errors.Is(err, conversation.ErrDeletedConversation) {
		t.Fatalf("deleted session was recreated: %v", err)
	}
	if _, err := service.LoadContext(ctx, 8, other.SessionID, 20); err != nil {
		t.Fatalf("other owner's same session handle was deleted: %v", err)
	}
	if !repo.auditDetached[owned.ID] || repo.auditDetached[other.ID] {
		t.Fatalf("audit detach state = %+v", repo.auditDetached)
	}
}

func TestConcurrentDeleteIsIdempotent(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	created, err := service.GetOrCreate(context.Background(), 7, "delete-race", "并发删除")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- service.Delete(context.Background(), 7, created.SessionID)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent deletion failed: %v", err)
		}
	}
	if len(repo.tombstones) != 1 || !repo.auditDetached[created.ID] {
		t.Fatalf("concurrent deletion state = tombstones:%+v audits:%+v", repo.tombstones, repo.auditDetached)
	}
}

func TestCleanupExpiredRequiresExplicitExecution(t *testing.T) {
	repo := newFakeConversationRepo()
	service := NewService(repo)
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	old, _ := service.GetOrCreate(context.Background(), 7, "old-session", "旧会话")
	recent, _ := service.GetOrCreate(context.Background(), 7, "recent-session", "新会话")
	repo.conversations[fakeConversationKey(7, old.SessionID)].LastActiveAt = now.AddDate(0, 0, -31)
	repo.conversations[fakeConversationKey(7, recent.SessionID)].LastActiveAt = now.AddDate(0, 0, -29)

	preview, err := service.CleanupExpired(context.Background(), now, 30, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || preview.Eligible != 1 || preview.Deleted != 0 || len(repo.conversations) != 2 {
		t.Fatalf("dry-run mutated data: result=%+v conversations=%d", preview, len(repo.conversations))
	}
	executed, err := service.CleanupExpired(context.Background(), now, 30, 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if executed.DryRun || executed.Eligible != 1 || executed.Deleted != 1 || len(repo.conversations) != 1 {
		t.Fatalf("cleanup result=%+v conversations=%d", executed, len(repo.conversations))
	}
	if repo.conversations[fakeConversationKey(7, recent.SessionID)] == nil || !repo.auditDetached[old.ID] {
		t.Fatalf("cleanup removed wrong conversation or kept audit link: %+v", repo.conversations)
	}
}
