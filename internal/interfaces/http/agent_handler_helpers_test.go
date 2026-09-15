package http

import (
	"strings"
	"testing"

	conversationapp "SWPUCAT/internal/application/conversation"
)

func TestAgentIntentPrefetchRules(t *testing.T) {
	for _, message := range []string{"这个制度怎么申请？", "请给我知识库里的部署文档", "月报应该如何提交"} {
		if !shouldPrefetchKnowledge(message) {
			t.Fatalf("expected knowledge prefetch for %q", message)
		}
	}
	for _, message := range []string{"你好", "帮我打卡上班", "总结月报"} {
		if shouldPrefetchKnowledge(message) {
			t.Fatalf("unexpected knowledge prefetch for %q", message)
		}
	}
}

func TestContextualKnowledgeQueryUsesPreviousUserTurn(t *testing.T) {
	history := []conversationapp.MessageDTO{
		{Role: "user", Content: "帮我找一下服务器部署规范"},
		{Role: "assistant", Content: "找到了部署指南"},
	}
	query := contextualKnowledgeQuery("这个怎么下载？", history)
	if !strings.Contains(query, "服务器部署规范") || !strings.Contains(query, "怎么下载") {
		t.Fatalf("query = %q", query)
	}
}

func TestCompactEvictedHistoryOnlyAtWindowBoundary(t *testing.T) {
	short := make([]conversationapp.MessageDTO, 19)
	if summary, changed := compactEvictedHistory("old", short); changed || summary != "old" {
		t.Fatalf("unexpected compaction: %q, %v", summary, changed)
	}
	full := append(short, conversationapp.MessageDTO{Role: "assistant", Content: "answer"})
	full[0] = conversationapp.MessageDTO{Role: "user", Content: "old question"}
	summary, changed := compactEvictedHistory("", full)
	if !changed || !strings.Contains(summary, "old question") {
		t.Fatalf("summary = %q, changed = %v", summary, changed)
	}
}

func TestNormalizeAgentSessionID(t *testing.T) {
	generated, err := normalizeAgentSessionID("")
	if err != nil || len(generated) != 32 {
		t.Fatalf("generated = %q, err = %v", generated, err)
	}
	if _, err := normalizeAgentSessionID("bad/session"); err == nil {
		t.Fatal("expected invalid session error")
	}
}
