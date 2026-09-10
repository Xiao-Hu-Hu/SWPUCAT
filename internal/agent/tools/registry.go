package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"SWPUCAT/internal/agent/business"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

type emptyInput struct{}

type searchInput struct {
	Query string `json:"query" jsonschema_description:"要搜索的知识库关键词或问题"`
}

type statsInput struct {
	Period string `json:"period,omitempty" jsonschema_description:"统计周期，只能是 today、week 或 month"`
}

type Registry struct {
	client       *business.Client
	writeEnabled bool
}

func NewRegistry(client *business.Client, writeEnabled bool) *Registry {
	return &Registry{client: client, writeEnabled: writeEnabled}
}

func (r *Registry) Build(ctx context.Context) ([]tool.BaseTool, error) {
	statusTool, err := toolutils.InferTool[emptyInput, string](
		"get_checkin_status",
		"查询当前登录成员今天的上班和下班打卡状态。",
		func(ctx context.Context, _ emptyInput) (string, error) {
			status, err := r.client.CheckinStatus(ctx)
			if err != nil {
				return "", err
			}
			return marshal(status)
		},
	)
	if err != nil {
		return nil, err
	}
	knowledgeTool, err := toolutils.InferTool[searchInput, string](
		"search_knowledge",
		"搜索已审核的知识库文件和说明，回答流程问题时优先调用。",
		func(ctx context.Context, input searchInput) (string, error) {
			if strings.TrimSpace(input.Query) == "" {
				return "", fmt.Errorf("query is required")
			}
			items, err := r.client.SearchKnowledge(ctx, strings.TrimSpace(input.Query))
			if err != nil {
				return "", err
			}
			return marshal(items)
		},
	)
	if err != nil {
		return nil, err
	}
	statsTool, err := toolutils.InferTool[statsInput, string](
		"get_checkin_stats",
		"队长查看 today、week 或 month 的成员打卡统计和异常标记。普通成员不能调用。",
		func(ctx context.Context, input statsInput) (string, error) {
			identity := business.RequestContextFrom(ctx)
			if identity.Role != "captain" && identity.Role != "super_admin" {
				return "", fmt.Errorf("captain or super_admin role is required")
			}
			period := strings.TrimSpace(input.Period)
			if period == "" {
				period = "week"
			}
			if period != "today" && period != "week" && period != "month" {
				return "", fmt.Errorf("period must be today, week, or month")
			}
			stats, err := r.client.CheckinStats(ctx, period)
			if err != nil {
				return "", err
			}
			return marshal(stats)
		},
	)
	if err != nil {
		return nil, err
	}
	result := []tool.BaseTool{statusTool, knowledgeTool, statsTool}
	if r.writeEnabled {
		clockInTool, err := toolutils.InferTool[emptyInput, string](
			"clock_in",
			"为当前登录成员打卡上班；只有工具返回成功后才能向用户确认。",
			func(ctx context.Context, _ emptyInput) (string, error) {
				identity := business.RequestContextFrom(ctx)
				key := operationKey(identity.RequestID, "clock-in")
				result, err := r.client.ClockIn(ctx, key)
				if err != nil {
					return "", err
				}
				return marshal(result)
			},
		)
		if err != nil {
			return nil, err
		}
		clockOutTool, err := toolutils.InferTool[emptyInput, string](
			"clock_out",
			"为当前登录成员打卡下班；只有工具返回成功后才能向用户确认。",
			func(ctx context.Context, _ emptyInput) (string, error) {
				identity := business.RequestContextFrom(ctx)
				key := operationKey(identity.RequestID, "clock-out")
				result, err := r.client.ClockOut(ctx, key)
				if err != nil {
					return "", err
				}
				return marshal(result)
			},
		)
		if err != nil {
			return nil, err
		}
		result = append(result, clockInTool, clockOutTool)
	}
	return result, nil
}

func operationKey(requestID, action string) string {
	if requestID == "" {
		return ""
	}
	return "agent:" + requestID + ":" + action
}

func marshal(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
