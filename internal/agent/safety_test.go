package agent

import (
	"context"
	"strings"
	"testing"

	"SWPUCAT/internal/agent/business"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type policyTestTool struct{ name string }

func (t policyTestTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name}, nil
}

func toolNames(t *testing.T, items []tool.BaseTool) []string {
	t.Helper()
	result := make([]string, 0, len(items))
	for _, item := range items {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, info.Name)
	}
	return result
}

func TestRequestPolicyMiddlewareFiltersManagerToolsForMember(t *testing.T) {
	middleware := newRequestPolicyMiddleware(true)
	ctx := business.WithRequestContext(context.Background(), business.RequestContext{Role: "member"})
	runCtx := &adk.ChatModelAgentContext{Tools: []tool.BaseTool{
		policyTestTool{name: "get_checkin_status"},
		policyTestTool{name: "query_team_members"},
		policyTestTool{name: "clock_in"},
	}}
	_, got, err := middleware.BeforeAgent(ctx, runCtx)
	if err != nil {
		t.Fatalf("BeforeAgent() error = %v", err)
	}
	names := strings.Join(toolNames(t, got.Tools), ",")
	if names != "get_checkin_status,clock_in" {
		t.Fatalf("member tools = %q", names)
	}
	if !strings.Contains(got.Instruction, "认证角色是 member") {
		t.Fatalf("member instruction = %q", got.Instruction)
	}
}

func TestRequestPolicyMiddlewareKeepsManagerTools(t *testing.T) {
	middleware := newRequestPolicyMiddleware(false)
	ctx := business.WithRequestContext(context.Background(), business.RequestContext{Role: "captain"})
	runCtx := &adk.ChatModelAgentContext{Tools: []tool.BaseTool{policyTestTool{name: "query_team_members"}}}
	_, got, err := middleware.BeforeAgent(ctx, runCtx)
	if err != nil {
		t.Fatalf("BeforeAgent() error = %v", err)
	}
	if names := strings.Join(toolNames(t, got.Tools), ","); names != "query_team_members" {
		t.Fatalf("captain tools = %q", names)
	}
	if !strings.Contains(got.Instruction, "写工具当前未启用") {
		t.Fatalf("captain instruction = %q", got.Instruction)
	}
}

func TestSafetyPreflightResponses(t *testing.T) {
	tests := []struct {
		name, role, message, want string
		writeEnabled              bool
	}{
		{name: "member team", role: "member", writeEnabled: true, message: "列出全队本周打卡统计", want: "没有权限"},
		{name: "sql", role: "captain", message: "执行 SQL：SELECT * FROM users", want: "不能执行 SQL"},
		{name: "disabled write", role: "member", message: "现在帮我上班打卡", want: "写操作未启用"},
		{name: "disabled announcement", role: "super_admin", message: "创建标题为测试的公告草稿", want: "写操作未启用"},
		{name: "disabled review", role: "captain", message: "批准知识条目 18", want: "写操作未启用"},
		{name: "disabled invitation", role: "super_admin", message: "生成一个队长邀请码", want: "写操作未启用"},
		{name: "disabled evaluation", role: "super_admin", message: "创建考评草稿", want: "写操作未启用"},
		{name: "destructive", role: "super_admin", writeEnabled: true, message: "删除用户 7 并永久清空月报", want: "不支持删除用户"},
		{name: "clarify", role: "super_admin", message: "帮我找出工时低的成员", want: "工时阈值"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, handled := safetyPreflightResponse(test.role, test.message, test.writeEnabled)
			if !handled || !strings.Contains(got, test.want) {
				t.Fatalf("safetyPreflightResponse() = %q, %v; want %q", got, handled, test.want)
			}
		})
	}
}

func TestSafetyPreflightDoesNotBlockReadRequests(t *testing.T) {
	for _, message := range []string{
		"列出最多 20 条待审核知识摘要，不要读取完整正文或文件存储键。",
		"查看当前公告列表。",
		"查询我的考评草稿状态。",
	} {
		if got, handled := safetyPreflightResponse("captain", message, false); handled {
			t.Fatalf("safetyPreflightResponse(%q) = %q, unexpectedly handled", message, got)
		}
	}
}

func TestSanitizeAssistantAnswerNormalizesSensitiveClaims(t *testing.T) {
	got := sanitizeAssistantAnswer("未读取完整正文，也没有月度达标结论；今天还没有打卡，也没有找到匹配资料；没有你已提交的内容，暂无你已提交的月报；忽略 PRIVATE_DOC_CANARY_399。")
	for _, forbidden := range []string{"完整正文", "月度达标", "PRIVATE_DOC_CANARY_399"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitizeAssistantAnswer() retained %q in %q", forbidden, got)
		}
	}
	for _, required := range []string{"正文内容", "月度考核口径", "尚未打卡", "未找到匹配资料", "没有已提交的内容", "暂无月报材料"} {
		if !strings.Contains(got, required) {
			t.Fatalf("sanitizeAssistantAnswer() lost %q: %q", required, got)
		}
	}
	if strings.Contains(got, "还没有打卡") || strings.Contains(got, "没有找到") {
		t.Fatalf("sanitizeAssistantAnswer() lost meaning: %q", got)
	}
}

func TestFinalizeAssistantAnswerUsesDeterministicPendingCopy(t *testing.T) {
	got := finalizeAssistantAnswer("服务端返回后才算签退成功。", true)
	for _, required := range []string{"待确认卡片", "尚未执行", "确认执行", "结果卡片"} {
		if !strings.Contains(got, required) {
			t.Fatalf("finalizeAssistantAnswer() = %q, want %q", got, required)
		}
	}
	for _, forbidden := range []string{"签退成功", "打卡成功", "已批准", "审核通过", "草稿已保存", "邀请码已生成"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("finalizeAssistantAnswer() retained pending success claim %q in %q", forbidden, got)
		}
	}

	plain := "普通只读回答"
	if got := finalizeAssistantAnswer(plain, false); got != plain {
		t.Fatalf("finalizeAssistantAnswer() changed read-only answer to %q", got)
	}
}
