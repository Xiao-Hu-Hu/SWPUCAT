package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"SWPUCAT/internal/agent/business"
	"SWPUCAT/internal/agent/confirmation"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

type emptyInput struct{}

type searchInput struct {
	Query string `json:"query" jsonschema_description:"要搜索的知识库关键词或问题"`
}

type recommendInput struct {
	Need string `json:"need" jsonschema_description:"成员想学习或使用的主题、技术方向或任务需求"`
}

type statsInput struct {
	Period string `json:"period,omitempty" jsonschema_description:"统计周期，只能是 today、week 或 month"`
}

type checkinHistoryInput struct {
	Limit int `json:"limit,omitempty" jsonschema_description:"最近打卡动作数量，默认 20，范围 1 到 100"`
}

type teamQueryInput struct {
	Period       string   `json:"period,omitempty" jsonschema_description:"统计周期，只能是 today、week 或 month"`
	PeriodID     int64    `json:"period_id,omitempty" jsonschema_description:"月报周期 ID；需要结合月报状态筛选时必须提供"`
	MinHours     *float64 `json:"min_hours,omitempty" jsonschema_description:"可选的最低工时阈值，只筛选低于该值的成员"`
	ReportStatus string   `json:"report_status,omitempty" jsonschema_description:"可选月报状态：missing、draft、submitted 或 reviewed"`
	Limit        int      `json:"limit,omitempty" jsonschema_description:"最大返回成员数，默认 100，最大 200"`
}

type reportMaterialsInput struct {
	PeriodID int64  `json:"period_id" jsonschema_description:"报告周期 ID，必须来自 list_report_periods 的返回值或由用户明确提供，禁止猜测"`
	Scope    string `json:"scope,omitempty" jsonschema_description:"材料范围，只能是 mine 或 team；team 仅队长可用"`
	Offset   int    `json:"offset,omitempty" jsonschema_description:"分页偏移，默认 0；团队总结应按 next_offset 继续读取"`
	Limit    int    `json:"limit,omitempty" jsonschema_description:"单页材料数，默认 8，最大 8"`
}

type reportPeriodsInput struct {
	PeriodKey string `json:"period_key,omitempty" jsonschema_description:"可选，按 YYYY-MM 精确筛选报告周期"`
	Status    string `json:"status,omitempty" jsonschema_description:"可选，只能是 open、closed 或 archived"`
	Limit     int    `json:"limit,omitempty" jsonschema_description:"最大返回周期数，默认 12，范围 1 到 24"`
}

type announcementDraftInput struct {
	Title        string  `json:"title" jsonschema_description:"公告标题，最多 256 个字符"`
	Content      string  `json:"content" jsonschema_description:"公告正文，支持 Markdown"`
	Pinned       bool    `json:"pinned,omitempty" jsonschema_description:"是否置顶"`
	RecipientIDs []int64 `json:"recipient_ids,omitempty" jsonschema_description:"可选的成员 ID 列表；仅保存为草稿收件人，不会自动发邮件"`
	SourceNote   string  `json:"source_note,omitempty" jsonschema_description:"可选的生成依据或说明"`
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

type reportPeriodsResult struct {
	PeriodKey    string                  `json:"period_key,omitempty"`
	Status       string                  `json:"status,omitempty"`
	TotalPeriods int                     `json:"total_periods"`
	Truncated    bool                    `json:"truncated"`
	Periods      []business.ReportPeriod `json:"periods"`
}

type knowledgeSearchResult struct {
	Query          string                     `json:"query"`
	Sources        []business.KnowledgeSource `json:"sources"`
	Truncated      bool                       `json:"truncated,omitempty"`
	OmittedSources int                        `json:"omitted_sources,omitempty"`
}

type checkinHistoryResult struct {
	Limit   int                      `json:"limit"`
	Records []business.CheckinRecord `json:"records"`
}

type actionProposalToolResult struct {
	Status    string `json:"status"`
	Tool      string `json:"tool"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	ExpiresAt string `json:"expires_at"`
}

type Registry struct {
	client        *business.Client
	writeEnabled  bool
	confirmations *confirmation.Signer
}

var reportPeriodKeyPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

type budgetTool struct {
	tool.InvokableTool
}

func (t *budgetTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	if err := business.ConsumeToolCall(ctx); err != nil {
		return "", err
	}
	return t.InvokableTool.InvokableRun(ctx, argumentsInJSON, opts...)
}

// WithToolBudget wraps Eino's invokable tools without changing their schema.
// The request context supplies the actual per-turn budget.
func WithToolBudget(items []tool.BaseTool) []tool.BaseTool {
	result := make([]tool.BaseTool, 0, len(items))
	for _, item := range items {
		if invokable, ok := item.(tool.InvokableTool); ok {
			result = append(result, &budgetTool{InvokableTool: invokable})
			continue
		}
		result = append(result, item)
	}
	return result
}

func NewRegistry(client *business.Client, writeEnabled bool, signers ...*confirmation.Signer) *Registry {
	var signer *confirmation.Signer
	if len(signers) > 0 {
		signer = signers[0]
	}
	return &Registry{client: client, writeEnabled: writeEnabled, confirmations: signer}
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
	personalWorkHoursTool, err := toolutils.InferTool[emptyInput, string](
		"get_my_work_hours",
		"查询当前登录成员本周和本月的累计工时、异常，以及后端确定性计算的每周要求、是否达标和还差多久；不接受用户 ID，也不能查询他人。",
		func(ctx context.Context, _ emptyInput) (string, error) {
			result, err := r.client.GetMyWorkHours(ctx)
			if err != nil {
				return "", err
			}
			return marshal(result)
		},
	)
	if err != nil {
		return nil, err
	}
	checkinHistoryTool, err := toolutils.InferTool[checkinHistoryInput, string](
		"get_my_checkin_history",
		"查询当前登录成员最近的打卡动作，按最新优先返回；结果是原始上班/下班动作，不是已配对工时，不接受用户 ID，也不能查询他人。",
		func(ctx context.Context, input checkinHistoryInput) (string, error) {
			limit := input.Limit
			if limit == 0 {
				limit = 20
			}
			if limit < 1 || limit > 100 {
				return "", fmt.Errorf("limit must be between 1 and 100")
			}
			records, err := r.client.GetMyCheckinRecords(ctx, limit)
			if err != nil {
				return "", err
			}
			if records == nil {
				records = make([]business.CheckinRecord, 0)
			}
			return marshal(checkinHistoryResult{Limit: limit, Records: records})
		},
	)
	if err != nil {
		return nil, err
	}
	checkinRequirementsTool, err := toolutils.InferTool[emptyInput, string](
		"get_checkin_requirements",
		"查询当前配置的各年级每周最低工时要求；用于回答通用制度问题。判断当前成员本人是否达标时应使用 get_my_work_hours 的后端计算结果。",
		func(ctx context.Context, _ emptyInput) (string, error) {
			result, err := r.client.GetCheckinRequirements(ctx)
			if err != nil {
				return "", err
			}
			if result.Requirements == nil {
				result.Requirements = make([]business.CheckinRequirement, 0)
			}
			return marshal(result)
		},
	)
	if err != nil {
		return nil, err
	}
	teamQueryTool, err := toolutils.InferTool[teamQueryInput, string](
		"query_team_members",
		"将队长的自然语言组合查询转换为受限条件，后端确定性计算打卡和月报状态；不能生成或执行 SQL，也不会自动发送通知。",
		func(ctx context.Context, input teamQueryInput) (string, error) {
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
			if input.MinHours != nil && (*input.MinHours < 0 || *input.MinHours > 10000) {
				return "", fmt.Errorf("min_hours is out of range")
			}
			if input.ReportStatus != "" && input.PeriodID <= 0 {
				return "", fmt.Errorf("period_id is required when report_status is set")
			}
			if input.ReportStatus != "" && input.ReportStatus != "missing" && input.ReportStatus != "draft" && input.ReportStatus != "submitted" && input.ReportStatus != "reviewed" {
				return "", fmt.Errorf("invalid report_status")
			}
			result, err := r.client.QueryTeam(ctx, period, input.PeriodID, input.MinHours, input.ReportStatus, input.Limit)
			if err != nil {
				return "", err
			}
			return marshal(result)
		},
	)
	if err != nil {
		return nil, err
	}
	recommendTool, err := toolutils.InferTool[recommendInput, string](
		"recommend_knowledge",
		"根据成员主动描述的学习或工作需求推荐已审核的知识库资料；推荐结果只来自当前可见资源，并附带推荐依据。",
		func(ctx context.Context, input recommendInput) (string, error) {
			need := strings.TrimSpace(input.Need)
			if need == "" {
				return "", fmt.Errorf("need is required")
			}
			items, err := r.client.RecommendKnowledge(ctx, need)
			if err != nil {
				return "", err
			}
			for _, item := range items {
				if collector := business.KnowledgeSourceCollectorFrom(ctx); collector != nil {
					collector.Add([]business.KnowledgeItem{{
						ID: item.ItemID, Type: item.Type, Name: item.Name,
						Description: item.Description, URL: item.URL, DownloadURL: item.DownloadURL,
						CategoryName: item.CategoryName, SourceKind: item.SourceKind,
						Excerpt: item.Excerpt, Score: item.Score, MatchMode: item.MatchMode,
					}})
				}
			}
			return marshal(items)
		},
	)
	if err != nil {
		return nil, err
	}
	knowledgeTool, err := toolutils.InferTool[searchInput, string](
		"search_knowledge",
		"使用关键词与语义混合检索已审核知识库的 Markdown 说明以及 PDF、DOCX、XLSX、Markdown、TXT 正文；回答流程、制度、资料内容或链接位置时优先调用。",
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
			return marshalKnowledgeSearchResult(query, sources)
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
	reportPeriodsTool, err := toolutils.InferTool[reportPeriodsInput, string](
		"list_report_periods",
		"查询可用月报周期及其 period_id，可按 YYYY-MM 和 open、closed、archived 状态筛选；读取月报材料前用它确定周期，不能猜测 ID。",
		func(ctx context.Context, input reportPeriodsInput) (string, error) {
			input.PeriodKey = strings.TrimSpace(input.PeriodKey)
			input.Status = strings.TrimSpace(input.Status)
			if input.PeriodKey != "" && !reportPeriodKeyPattern.MatchString(input.PeriodKey) {
				return "", fmt.Errorf("period_key must use YYYY-MM")
			}
			if input.Status != "" && input.Status != "open" && input.Status != "closed" && input.Status != "archived" {
				return "", fmt.Errorf("status must be open, closed, or archived")
			}
			limit := input.Limit
			if limit == 0 {
				limit = 12
			}
			if limit < 1 || limit > 24 {
				return "", fmt.Errorf("limit must be between 1 and 24")
			}
			periods, err := r.client.ListReportPeriods(ctx)
			if err != nil {
				return "", err
			}
			sort.SliceStable(periods, func(i, j int) bool {
				if periods[i].StartsOn == periods[j].StartsOn {
					return periods[i].ID > periods[j].ID
				}
				return periods[i].StartsOn > periods[j].StartsOn
			})
			matched := make([]business.ReportPeriod, 0, limit)
			total := 0
			for _, period := range periods {
				if (input.PeriodKey != "" && period.PeriodKey != input.PeriodKey) || (input.Status != "" && period.Status != input.Status) {
					continue
				}
				total++
				if len(matched) < limit {
					matched = append(matched, period)
				}
			}
			return marshal(reportPeriodsResult{PeriodKey: input.PeriodKey, Status: input.Status, TotalPeriods: total, Truncated: total > len(matched), Periods: matched})
		},
	)
	if err != nil {
		return nil, err
	}
	reportTool, err := toolutils.InferTool[reportMaterialsInput, string](
		"get_report_materials",
		"读取指定报告周期内可用于总结的已提交月报正文。period_id 必须来自 list_report_periods 或用户明确提供；scope=mine 读取当前用户，scope=team 仅队长读取团队材料；按 offset/limit 分页直到 next_offset 为空；不会读取草稿，也不会发布总结。",
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
				return marshalReportMaterialsResult(result)
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
			return marshalReportMaterialsResult(result)
		},
	)
	if err != nil {
		return nil, err
	}
	announcementDraftTool, err := toolutils.InferTool[announcementDraftInput, string](
		"draft_announcement",
		"根据队长要求提出公告草稿创建方案。只生成待确认提议，不会立即保存；用户必须在界面确认后才创建草稿。",
		func(ctx context.Context, input announcementDraftInput) (string, error) {
			identity := business.RequestContextFrom(ctx)
			if identity.Role != "captain" && identity.Role != "super_admin" {
				return "", fmt.Errorf("captain or super_admin role is required")
			}
			title, content := strings.TrimSpace(input.Title), strings.TrimSpace(input.Content)
			if title == "" || content == "" {
				return "", fmt.Errorf("title and content are required")
			}
			return r.propose(ctx, confirmation.ToolDraftAnnouncement, "创建公告草稿", "确认后将保存公告草稿，仍需人工审核后才能发布。", confirmation.DraftAnnouncementParameters{
				Title: title, Content: content, Pinned: input.Pinned, SourceNote: strings.TrimSpace(input.SourceNote), RecipientIDs: input.RecipientIDs,
			})
		},
	)
	if err != nil {
		return nil, err
	}
	result := []tool.BaseTool{statusTool, personalWorkHoursTool, checkinHistoryTool, checkinRequirementsTool, knowledgeTool, recommendTool, teamQueryTool, statsTool, reportPeriodsTool, reportTool}
	if r.writeEnabled {
		result = append(result, announcementDraftTool)
		clockInTool, err := toolutils.InferTool[emptyInput, string](
			"clock_in",
			"提出为当前登录成员上班打卡的待确认操作；不会立即打卡，用户必须在界面确认。",
			func(ctx context.Context, _ emptyInput) (string, error) {
				return r.propose(ctx, confirmation.ToolClockIn, "上班打卡", "确认后将为当前账号记录上班打卡。", emptyInput{})
			},
		)
		if err != nil {
			return nil, err
		}
		clockOutTool, err := toolutils.InferTool[emptyInput, string](
			"clock_out",
			"提出为当前登录成员下班打卡的待确认操作；不会立即打卡，用户必须在界面确认。",
			func(ctx context.Context, _ emptyInput) (string, error) {
				return r.propose(ctx, confirmation.ToolClockOut, "下班打卡", "确认后将为当前账号记录下班打卡。", emptyInput{})
			},
		)
		if err != nil {
			return nil, err
		}
		result = append(result, clockInTool, clockOutTool)
	}
	return result, nil
}

func (r *Registry) propose(ctx context.Context, toolName, title, summary string, parameters any) (string, error) {
	if r == nil || r.confirmations == nil {
		return "", errors.New("action confirmation is not configured")
	}
	identity := business.RequestContextFrom(ctx)
	proposal, err := r.confirmations.Propose(identity.UserID, identity.SessionID, toolName, title, summary, parameters)
	if err != nil {
		return "", err
	}
	business.ActionProposalCollectorFrom(ctx).Add(proposal)
	// The raw signed token is a browser-to-server confirmation credential. It
	// is returned only through the structured proposal collector and must not
	// be copied into the model context, free-form answer, or tool audit result.
	return marshal(actionProposalToolResult{
		Status: "confirmation_required", Tool: proposal.Tool, Title: proposal.Title,
		Summary: proposal.Summary, ExpiresAt: proposal.ExpiresAt,
	})
}

const (
	maxReportMaterialsPerPage = 8
	// A single report is intentionally bounded well below the previous 12k
	// rune limit.  The page-level budget below is the stronger guard: eight
	// reports cannot expand the tool result to eight times this value.
	maxReportBodyRunes = 4000
	// This budget applies to the report bodies before the JSON envelope is
	// encoded.  It leaves room for report metadata and the model's prompt.
	maxReportMaterialsBodyRunes = 12000
	// The final serialized tool result is also bounded.  Eino appends tool
	// output to the model context, so this protects the context from large
	// report bodies and parsed JSON together.
	maxReportMaterialsOutputRunes = 16000
	maxReportParsedJSONRunes      = 1024
	maxReportMetadataRunes        = 256
	// search_knowledge is also a tool response and is appended to the model
	// context. Keep its complete serialized payload below 12k runes even when
	// the API returns eight sources with long descriptions.
	maxKnowledgeToolOutputRunes      = 12000
	maxKnowledgeToolDescriptionRunes = 2000
	maxKnowledgeToolExcerptRunes     = 600
	maxKnowledgeToolTextRunes        = 256
	maxKnowledgeToolURLRunes         = 2048
	maxKnowledgeToolSources          = 8
)

// marshalKnowledgeSearchResult applies a second, tool-specific budget after
// BuildKnowledgeSources has created the safe source projection.  The
// projection is deliberately useful for the UI, but sending eight 4000-rune
// descriptions to the model would still consume most of its context.
func marshalKnowledgeSearchResult(query string, sources []business.KnowledgeSource) (string, error) {
	result := boundKnowledgeSearchResult(query, sources)
	for {
		data, err := json.Marshal(result)
		if err != nil {
			return "", err
		}
		if len([]rune(string(data))) <= maxKnowledgeToolOutputRunes {
			return string(data), nil
		}

		index, field := largestKnowledgeText(result.Sources)
		if index >= 0 {
			var current string
			switch field {
			case knowledgeTextDescription:
				current = result.Sources[index].Description
			case knowledgeTextExcerpt:
				current = result.Sources[index].Excerpt
			}
			currentRunes := len([]rune(current))
			excess := len([]rune(string(data))) - maxKnowledgeToolOutputRunes
			target := currentRunes - excess - 1
			if target < 0 {
				target = 0
			}
			trimmed := truncateToolRunes(current, target)
			if len([]rune(trimmed)) >= currentRunes {
				trimmed = ""
			}
			switch field {
			case knowledgeTextDescription:
				result.Sources[index].Description = trimmed
			case knowledgeTextExcerpt:
				result.Sources[index].Excerpt = trimmed
			}
			result.Truncated = true
			continue
		}

		// Descriptions and excerpts are gone.  Drop optional ranking metadata
		// before removing sources; ID, name and links remain available to the
		// model for citations.
		if stripKnowledgeOptionalField(result.Sources) {
			result.Truncated = true
			continue
		}
		if len(result.Sources) > 1 {
			result.OmittedSources++
			result.Sources = result.Sources[:len(result.Sources)-1]
			result.Truncated = true
			continue
		}

		// A single source should fit after the initial field bounds.  If an
		// unusually long required field still exceeds the budget, retain its
		// identity and trim the optional link fields as a last resort.
		if len(result.Sources) == 1 {
			source := &result.Sources[0]
			if source.URL != "" {
				source.URL = ""
				result.Truncated = true
				continue
			}
			if source.DownloadURL != "" {
				source.DownloadURL = ""
				result.Truncated = true
				continue
			}
			source.Name = truncateToolRunes(source.Name, maxKnowledgeToolTextRunes/2)
			result.Truncated = true
			continue
		}
		// No reducible field remains.  This is only reachable for malformed
		// input with an enormous required field; return the bounded projection
		// rather than looping forever.
		return string(data), nil
	}
}

func boundKnowledgeSearchResult(query string, sources []business.KnowledgeSource) knowledgeSearchResult {
	capacity := len(sources)
	if capacity > maxKnowledgeToolSources {
		capacity = maxKnowledgeToolSources
	}
	result := knowledgeSearchResult{Query: truncateToolRunes(query, maxKnowledgeToolTextRunes), Sources: make([]business.KnowledgeSource, 0, capacity)}
	if result.Query != query {
		result.Truncated = true
	}
	for _, source := range sources {
		if len(result.Sources) >= maxKnowledgeToolSources {
			result.OmittedSources++
			result.Truncated = true
			continue
		}
		bounded := source
		var changed bool
		bounded.Name, changed = truncateKnowledgeText(bounded.Name, maxKnowledgeToolTextRunes)
		result.Truncated = result.Truncated || changed
		bounded.Description, changed = truncateKnowledgeText(bounded.Description, maxKnowledgeToolDescriptionRunes)
		result.Truncated = result.Truncated || changed
		bounded.Excerpt, changed = truncateKnowledgeText(bounded.Excerpt, maxKnowledgeToolExcerptRunes)
		result.Truncated = result.Truncated || changed
		bounded.CategoryName, changed = truncateKnowledgeText(bounded.CategoryName, maxKnowledgeToolTextRunes)
		result.Truncated = result.Truncated || changed
		bounded.Type, changed = truncateKnowledgeText(bounded.Type, maxKnowledgeToolTextRunes)
		result.Truncated = result.Truncated || changed
		bounded.SourceKind, changed = truncateKnowledgeText(bounded.SourceKind, maxKnowledgeToolTextRunes)
		result.Truncated = result.Truncated || changed
		bounded.MatchMode, changed = truncateKnowledgeText(bounded.MatchMode, maxKnowledgeToolTextRunes)
		result.Truncated = result.Truncated || changed
		bounded.CreatedAt, changed = truncateKnowledgeText(bounded.CreatedAt, maxKnowledgeToolTextRunes)
		result.Truncated = result.Truncated || changed
		// A shortened URL is not a usable link.  Keep normal links intact and
		// omit only an abnormally long value; the source ID/name remain useful.
		if len([]rune(bounded.URL)) > maxKnowledgeToolURLRunes {
			bounded.URL = ""
			result.Truncated = true
		}
		if len([]rune(bounded.DownloadURL)) > maxKnowledgeToolURLRunes {
			bounded.DownloadURL = ""
			result.Truncated = true
		}
		result.Sources = append(result.Sources, bounded)
	}
	return result
}

func truncateKnowledgeText(value string, limit int) (string, bool) {
	trimmed := truncateToolRunes(value, limit)
	return trimmed, trimmed != value
}

func truncateToolRunes(value string, limit int) string {
	if limit <= 0 {
		if value == "" {
			return ""
		}
		return "…"
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

type knowledgeTextField uint8

const (
	knowledgeTextNone knowledgeTextField = iota
	knowledgeTextDescription
	knowledgeTextExcerpt
)

func largestKnowledgeText(sources []business.KnowledgeSource) (int, knowledgeTextField) {
	index, field, size := -1, knowledgeTextNone, 0
	for i := range sources {
		if current := len([]rune(sources[i].Description)); current > size {
			index, field, size = i, knowledgeTextDescription, current
		}
		if current := len([]rune(sources[i].Excerpt)); current > size {
			index, field, size = i, knowledgeTextExcerpt, current
		}
	}
	return index, field
}

func stripKnowledgeOptionalField(sources []business.KnowledgeSource) bool {
	for i := len(sources) - 1; i >= 0; i-- {
		source := &sources[i]
		switch {
		case source.CreatedAt != "":
			source.CreatedAt = ""
			return true
		case source.MatchMode != "":
			source.MatchMode = ""
			return true
		case source.SourceKind != "":
			source.SourceKind = ""
			return true
		case source.CategoryName != "":
			source.CategoryName = ""
			return true
		case source.Type != "":
			source.Type = ""
			return true
		}
	}
	return false
}

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
	return truncateReportBodyTo(value, maxReportBodyRunes)
}

// truncateReportBodyTo keeps the existing truncation marker contract while
// allowing the page-level budget to allocate a smaller share to each report.
// The limit is the number of source runes retained before the marker.
func truncateReportBodyTo(value string, limit int) (string, bool) {
	if limit < 0 {
		limit = 0
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]) + "…", true
}

// marshalReportMaterialsResult bounds both the report bodies and the final
// serialized result.  Tool output is part of the model context, therefore a
// per-item limit alone is insufficient when a page contains eight reports.
func marshalReportMaterialsResult(result reportMaterialsResult) (string, error) {
	result = boundReportMaterials(result)
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func boundReportMaterials(result reportMaterialsResult) reportMaterialsResult {
	materials := make([]reportMaterial, len(result.Materials))
	copy(materials, result.Materials)
	if len(materials) > maxReportMaterialsPerPage {
		result.SkippedReports += len(materials) - maxReportMaterialsPerPage
		materials = materials[:maxReportMaterialsPerPage]
	}
	result.Scope = truncateMetadata(result.Scope)

	// Normalize fields that can otherwise bypass the body budget.  ParsedJSON
	// is useful for structured reports, but an unexpectedly large parser result
	// should not be copied wholesale into the LLM context.
	for i := range materials {
		materials[i].AuthorName = truncateMetadata(materials[i].AuthorName)
		materials[i].Status = truncateMetadata(materials[i].Status)
		materials[i].Title = truncateMetadata(materials[i].Title)
		materials[i].ParseStatus = truncateMetadata(materials[i].ParseStatus)
		materials[i].ParseError = truncateMetadata(materials[i].ParseError)
		if materials[i].ParsedJSON != nil {
			if data, err := json.Marshal(materials[i].ParsedJSON); err != nil || len([]rune(string(data))) > maxReportParsedJSONRunes {
				materials[i].ParsedJSON = nil
				materials[i].Truncated = true
			}
		}
	}

	// Allocate the body budget fairly across the remaining reports.  A short
	// report leaves budget for later reports on the same page.
	remaining := maxReportMaterialsBodyRunes
	remainingReports := len(materials)
	for i := range materials {
		if remainingReports <= 0 {
			break
		}
		share := remaining / remainingReports
		if remaining%remainingReports != 0 {
			share++
		}
		if share > maxReportBodyRunes {
			share = maxReportBodyRunes
		}
		body, truncated := truncateReportBodyTo(materials[i].BodyMarkdown, share)
		materials[i].BodyMarkdown = body
		materials[i].Truncated = materials[i].Truncated || truncated
		used := len([]rune(body))
		if used > remaining {
			used = remaining
		}
		remaining -= used
		remainingReports--
	}
	result.Materials = materials

	// The body allocation leaves room for metadata, but parsed JSON and JSON
	// escaping can still push the envelope over the hard limit.  Trim the
	// largest body until the serialized result fits, then drop parsed JSON as a
	// final data-preserving fallback.
	for {
		data, err := json.Marshal(result)
		if err != nil || len([]rune(string(data))) <= maxReportMaterialsOutputRunes {
			break
		}
		index := largestReportBody(result.Materials)
		if index < 0 {
			index = largestParsedReport(result.Materials)
			if index < 0 {
				// User supplied names and parser error strings can contain control
				// characters that JSON expands substantially.  If all bodies and
				// parsed payloads are already gone, strip optional text metadata as
				// the final bounded fallback while retaining IDs and status markers.
				stripped := false
				for i := range result.Materials {
					item := &result.Materials[i]
					if item.AuthorName != "" || item.Status != "" || item.Title != "" || item.ParseStatus != "" || item.ParseError != "" {
						item.AuthorName = ""
						item.Status = ""
						item.Title = ""
						item.ParseStatus = ""
						item.ParseError = ""
						item.Truncated = true
						stripped = true
					}
				}
				if !stripped {
					break
				}
				continue
			}
			result.Materials[index].ParsedJSON = nil
			result.Materials[index].Truncated = true
			continue
		}
		bodyRunes := len([]rune(result.Materials[index].BodyMarkdown))
		excess := len([]rune(string(data))) - maxReportMaterialsOutputRunes
		// Account for the marker that truncateReportBodyTo appends when it
		// shortens a body.  Removing at least excess+1 source runes guarantees
		// progress even when the body contains JSON-escaped characters.
		target := bodyRunes - excess - 1
		if target < 0 {
			target = 0
		}
		body, _ := truncateReportBodyTo(result.Materials[index].BodyMarkdown, target)
		if len([]rune(body)) >= bodyRunes {
			body = ""
		}
		result.Materials[index].BodyMarkdown = body
		result.Materials[index].Truncated = true
	}

	return result
}

func truncateMetadata(value string) string {
	runes := []rune(value)
	if len(runes) <= maxReportMetadataRunes {
		return value
	}
	return string(runes[:maxReportMetadataRunes]) + "…"
}

func largestReportBody(materials []reportMaterial) int {
	index, size := -1, 0
	for i := range materials {
		if current := len([]rune(materials[i].BodyMarkdown)); current > size {
			index, size = i, current
		}
	}
	return index
}

func largestParsedReport(materials []reportMaterial) int {
	index, size := -1, 0
	for i := range materials {
		if materials[i].ParsedJSON == nil {
			continue
		}
		data, err := json.Marshal(materials[i].ParsedJSON)
		if err != nil {
			return i
		}
		if current := len(data); current > size {
			index, size = i, current
		}
	}
	return index
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
