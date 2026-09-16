package conversationapp

import (
	"context"
	"strconv"
	"testing"
	"time"

	"SWPUCAT/internal/domain/conversation"
	"SWPUCAT/internal/domain/shared"
)

type fakeConversationRepo struct {
	conversations map[string]*conversation.Conversation
	messages      map[string][]*conversation.Message
	nextID        int64
}

func newFakeConversationRepo() *fakeConversationRepo {
	return &fakeConversationRepo{conversations: map[string]*conversation.Conversation{}, messages: map[string][]*conversation.Message{}, nextID: 1}
}

func (r *fakeConversationRepo) Create(_ context.Context, item *conversation.Conversation) error {
	key := fakeConversationKey(item.UserID, item.SessionID)
	if _, exists := r.conversations[key]; exists {
		return shared.ErrConflict
	}
	item.ID = r.nextID
	r.nextID++
	copy := *item
	r.conversations[key] = &copy
	return nil
}

func (r *fakeConversationRepo) FindBySession(_ context.Context, userID int64, sessionID string) (*conversation.Conversation, error) {
	item := r.conversations[fakeConversationKey(userID, sessionID)]
	if item == nil {
		return nil, shared.ErrNotFound
	}
	copy := *item
	return &copy, nil
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

func (r *fakeConversationRepo) RecordTurn(_ context.Context, userID int64, sessionID, title string, userMessage, assistantMessage *conversation.Message, summary, sources string, activeAt time.Time) error {
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
