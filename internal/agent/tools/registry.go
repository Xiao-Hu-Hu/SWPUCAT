package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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

type reportMaterialsInput struct {
	PeriodID int64  `json:"period_id" jsonschema_description:"报告周期 ID，必须由用户明确提供"`
	Scope    string `json:"scope,omitempty" jsonschema_description:"材料范围，只能是 mine 或 team；team 仅队长可用"`
	Offset   int    `json:"offset,omitempty" jsonschema_description:"分页偏移，默认 0；团队总结应按 next_offset 继续读取"`
	Limit    int    `json:"limit,omitempty" jsonschema_description:"单页材料数，默认 8，最大 8"`
}

type reportMaterial struct {
	ReportID     int64          `json:"report_id"`
	Version      int            `json:"version"`
	AuthorID     int64          `json:"author_id"`
	AuthorName   string         `json:"author_name"`
	Status       string         `json:"status"`
	Title        string         `json:"title"`
	BodyMarkdown string         `json:"body_markdown"`
	ParseStatus  string         `json:"parse_status"`
	ParseError   string         `json:"parse_error,omitempty"`
	ParsedJSON   map[string]any `json:"parsed_json,omitempty"`
	Truncated    bool           `json:"truncated,omitempty"`
}

type reportMaterialsResult struct {
	PeriodID       int64            `json:"period_id"`
	Scope          string           `json:"scope"`
	Offset         int              `json:"offset"`
	Limit          int              `json:"limit"`
	TotalReports   int              `json:"total_reports"`
	NextOffset     *int             `json:"next_offset,omitempty"`
	Materials      []reportMaterial `json:"materials"`
	SkippedReports int              `json:"skipped_reports,omitempty"`
}

type knowledgeSearchResult struct {
	Query   string                     `json:"query"`
	Sources []business.KnowledgeSource `json:"sources"`
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
			query := strings.TrimSpace(input.Query)
			if query == "" {
				return "", fmt.Errorf("query is required")
			}
			items, err := r.client.SearchKnowledge(ctx, query)
			if err != nil {
				return "", err
			}
			sources := business.BuildKnowledgeSources(items)
			if collector := business.KnowledgeSourceCollectorFrom(ctx); collector != nil {
				collector.Add(items)
			}
			return marshal(knowledgeSearchResult{Query: query, Sources: sources})
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
	reportTool, err := toolutils.InferTool[reportMaterialsInput, string](
		"get_report_materials",
		"读取指定报告周期内可用于总结的已提交月报正文。scope=mine 读取当前用户，scope=team 仅队长读取团队材料；按 offset/limit 分页直到 next_offset 为空；不会读取草稿，也不会发布总结。",
		func(ctx context.Context, input reportMaterialsInput) (string, error) {
			if input.PeriodID <= 0 {
				return "", fmt.Errorf("period_id is required")
			}
			if input.Offset < 0 {
				return "", fmt.Errorf("offset must be zero or greater")
			}
			limit := input.Limit
			if limit == 0 {
				limit = 8
			}
			if limit < 1 || limit > maxReportMaterialsPerPage {
				return "", fmt.Errorf("limit must be between 1 and %d", maxReportMaterialsPerPage)
			}
			scope := strings.TrimSpace(input.Scope)
			if scope == "" {
				scope = "mine"
			}
			if scope != "mine" && scope != "team" {
				return "", fmt.Errorf("scope must be mine or team")
			}
			identity := business.RequestContextFrom(ctx)
			if scope == "team" && identity.Role != "captain" && identity.Role != "super_admin" {
				return "", fmt.Errorf("captain or super_admin role is required for team materials")
			}
			reports, err := r.client.ListReports(ctx, input.PeriodID, "submitted")
			if err != nil {
				return "", err
			}
			reviewed, err := r.client.ListReports(ctx, input.PeriodID, "reviewed")
			if err != nil {
				return "", err
			}
			reports = mergeReports(reports, reviewed, scope, identity.UserID)
			result := reportMaterialsResult{PeriodID: input.PeriodID, Scope: scope, Offset: input.Offset, Limit: limit, TotalReports: len(reports), Materials: make([]reportMaterial, 0, limit)}
			if input.Offset >= len(reports) {
				return marshal(result)
			}
			end := input.Offset + limit
			if end > len(reports) {
				end = len(reports)
			}
			if end < len(reports) {
				next := end
				result.NextOffset = &next
			}
			for _, item := range reports[input.Offset:end] {
				versionNumber := item.CurrentVersion
				if versionNumber <= 0 {
					result.SkippedReports++
					continue
				}
				version, err := r.client.GetReportVersion(ctx, item.ID, versionNumber)
				if err != nil {
					result.SkippedReports++
					continue
				}
				body, truncated := truncateReportBody(version.BodyMarkdown)
				result.Materials = append(result.Materials, reportMaterial{ReportID: item.ID, Version: version.Version, AuthorID: item.AuthorID, AuthorName: item.AuthorName, Status: version.Status, Title: version.Title, BodyMarkdown: body, ParseStatus: version.ParseStatus, ParseError: version.ParseError, ParsedJSON: version.ParsedJSON, Truncated: truncated})
			}
			return marshal(result)
		},
	)
	if err != nil {
		return nil, err
	}
	result := []tool.BaseTool{statusTool, knowledgeTool, statsTool, reportTool}
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

const (
	maxReportMaterialsPerPage = 8
	maxReportBodyRunes        = 12000
)

func mergeReports(submitted, reviewed []business.Report, scope string, userID int64) []business.Report {
	result := make([]business.Report, 0, len(submitted)+len(reviewed))
	seen := make(map[int64]struct{}, len(submitted)+len(reviewed))
	for _, item := range append(submitted, reviewed...) {
		if scope == "mine" && item.AuthorID != userID {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func truncateReportBody(value string) (string, bool) {
	runes := []rune(value)
	if len(runes) <= maxReportBodyRunes {
		return value, false
	}
	return string(runes[:maxReportBodyRunes]) + "…", true
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
