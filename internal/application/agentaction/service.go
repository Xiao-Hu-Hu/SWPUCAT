package agentaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"SWPUCAT/internal/agent/confirmation"
	announcementdraftapp "SWPUCAT/internal/application/announcementdraft"
	checkinapp "SWPUCAT/internal/application/checkin"
	evaluationapp "SWPUCAT/internal/application/evaluation"
	invitationapp "SWPUCAT/internal/application/invitation"
	domaincheckin "SWPUCAT/internal/domain/checkin"
	domainevaluation "SWPUCAT/internal/domain/evaluation"
	domaininvitation "SWPUCAT/internal/domain/invitation"
	"SWPUCAT/internal/domain/shared"
)

var (
	ErrInvalidConfirmation = errors.New("invalid action confirmation")
	ErrExpiredConfirmation = errors.New("action confirmation expired")
	ErrActionDisabled      = errors.New("agent actions are disabled")
)

type Record struct {
	TokenHash        string
	UserID           int64
	SessionID        string
	ToolName         string
	ParametersDigest string
	Status           string
	ResultJSON       string
	ErrorCode        string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	CompletedAt      *time.Time
}

type Repository interface {
	Claim(ctx context.Context, record *Record) (existing *Record, claimed bool, err error)
	Complete(ctx context.Context, tokenHash, status, resultJSON, errorCode string, completedAt time.Time) error
	FindLatest(ctx context.Context, userID int64, sessionID, toolName, parametersDigest string) (*Record, error)
}

type CheckinWriter interface {
	ClockIn(ctx context.Context, userID int64, idempotencyKey string) (*checkinapp.CheckinRecordDTO, error)
	ClockOut(ctx context.Context, userID int64, idempotencyKey string) (*checkinapp.CheckinRecordDTO, error)
}

type DraftWriter interface {
	Create(ctx context.Context, actorID int64, req announcementdraftapp.CreateRequest) (*announcementdraftapp.DraftDTO, error)
}

type ConfirmationVerifier interface {
	Verify(token string, userID int64, sessionID, toolName string, parameters json.RawMessage) (*confirmation.Claims, json.RawMessage, error)
}

type ConfirmationSigner interface {
	ConfirmationVerifier
	Propose(userID int64, sessionID, toolName, title, summary string, parameters any) (*confirmation.Proposal, error)
}

type KnowledgeReviewWriter interface {
	ApproveItem(ctx context.Context, itemID, reviewerID int64, reviewerName string) error
	RejectItem(ctx context.Context, itemID, reviewerID int64, reviewerName, reason string) (string, error)
}

type InvitationWriter interface {
	GenerateCode(ctx context.Context, creatorID int64, codeType domaininvitation.InvitationType) (*invitationapp.InvitationDTO, error)
}

type EvaluationDraftWriter interface {
	CreateDraft(ctx context.Context, actorID int64, req evaluationapp.CreateDraftRequest) (*evaluationapp.DraftDTO, error)
	UpdateDraft(ctx context.Context, id, actorID int64, req evaluationapp.UpdateDraftRequest) (*evaluationapp.DraftDTO, error)
}

type ExecuteRequest struct {
	UserID     int64
	Role       string
	ActorName  string
	SessionID  string
	Tool       string
	Token      string
	Parameters json.RawMessage
}

type RecoverRequest struct {
	UserID     int64
	Role       string
	ActorName  string
	SessionID  string
	Tool       string
	Title      string
	Summary    string
	Parameters json.RawMessage
}

type RecoverResult struct {
	Result   *Result                `json:"result,omitempty"`
	Proposal *confirmation.Proposal `json:"proposal,omitempty"`
}

type Result struct {
	Tool     string          `json:"tool"`
	Status   string          `json:"status"`
	Title    string          `json:"title"`
	Message  string          `json:"message"`
	Data     json.RawMessage `json:"data,omitempty"`
	Replayed bool            `json:"replayed,omitempty"`
}

type Service struct {
	enabled     bool
	repo        Repository
	signer      ConfirmationSigner
	checkin     CheckinWriter
	drafts      DraftWriter
	knowledge   KnowledgeReviewWriter
	invitations InvitationWriter
	evaluations EvaluationDraftWriter
}

type Option func(*Service)

func WithKnowledgeReviews(writer KnowledgeReviewWriter) Option {
	return func(service *Service) { service.knowledge = writer }
}

func WithInvitations(writer InvitationWriter) Option {
	return func(service *Service) { service.invitations = writer }
}

func WithEvaluationDrafts(writer EvaluationDraftWriter) Option {
	return func(service *Service) { service.evaluations = writer }
}

func NewService(enabled bool, repo Repository, signer ConfirmationSigner, checkin CheckinWriter, drafts DraftWriter, options ...Option) *Service {
	service := &Service{enabled: enabled, repo: repo, signer: signer, checkin: checkin, drafts: drafts}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) Execute(ctx context.Context, req ExecuteRequest) (*Result, error) {
	if s == nil || !s.enabled {
		return nil, ErrActionDisabled
	}
	if s.repo == nil || s.signer == nil || req.UserID <= 0 || strings.TrimSpace(req.SessionID) == "" || !confirmation.SupportedTool(req.Tool) {
		return nil, ErrInvalidConfirmation
	}
	claims, parameters, err := s.signer.Verify(req.Token, req.UserID, req.SessionID, req.Tool, req.Parameters)
	if err != nil {
		if errors.Is(err, confirmation.ErrExpiredToken) {
			return nil, ErrExpiredConfirmation
		}
		return nil, ErrInvalidConfirmation
	}
	if err := authorize(req, parameters); err != nil {
		return nil, err
	}
	tokenHash := confirmation.TokenDigest(req.Token)
	now := time.Now().UTC()
	record := &Record{
		TokenHash: tokenHash, UserID: req.UserID, SessionID: req.SessionID, ToolName: req.Tool,
		ParametersDigest: claims.ParametersDigest, Status: "executing", ResultJSON: "{}",
		ExpiresAt: time.Unix(claims.ExpiresAtUnix, 0).UTC(), CreatedAt: now,
	}
	existing, claimed, err := s.repo.Claim(ctx, record)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return replayResult(existing)
	}

	result := s.executeClaimed(ctx, req, tokenHash, parameters)
	resultJSON, _ := json.Marshal(result)
	status, errorCode := "succeeded", ""
	if result.Status != "success" {
		status, errorCode = "failed", "business_operation_failed"
	}
	if err := s.repo.Complete(ctx, tokenHash, status, string(resultJSON), errorCode, time.Now().UTC()); err != nil {
		return nil, err
	}
	return result, nil
}

// Recover restores a previously presented action proposal after a page reload.
// The persisted actions_json deliberately strips the confirmation token for
// privacy, so a reloaded card cannot be confirmed directly. If the proposal was
// already confirmed, Recover returns the stored result (keyed by the canonical
// parameter digest) so the outcome can be shown again; otherwise it re-signs a
// fresh, short-lived token for the same proposal so the user can still confirm.
func (s *Service) Recover(ctx context.Context, req RecoverRequest) (*RecoverResult, error) {
	if s == nil || !s.enabled {
		return nil, ErrActionDisabled
	}
	if s.repo == nil || s.signer == nil || req.UserID <= 0 || strings.TrimSpace(req.SessionID) == "" || !confirmation.SupportedTool(req.Tool) || len(req.Parameters) == 0 {
		return nil, ErrInvalidConfirmation
	}
	canonical, err := confirmation.CanonicalParameters(req.Parameters)
	if err != nil {
		return nil, ErrInvalidConfirmation
	}
	digest := confirmation.ParametersDigest(canonical)
	record, err := s.repo.FindLatest(ctx, req.UserID, req.SessionID, req.Tool, digest)
	if err != nil {
		return nil, err
	}
	if record != nil {
		result, err := replayResult(record)
		if err != nil {
			return nil, err
		}
		return &RecoverResult{Result: result}, nil
	}
	proposal, err := s.signer.Propose(req.UserID, req.SessionID, req.Tool, req.Title, req.Summary, canonical)
	if err != nil {
		return nil, err
	}
	return &RecoverResult{Proposal: proposal}, nil
}

func (s *Service) executeClaimed(ctx context.Context, req ExecuteRequest, tokenHash string, parameters json.RawMessage) *Result {
	switch req.Tool {
	case confirmation.ToolClockIn:
		if s.checkin == nil {
			return failureResult(req.Tool, "上班打卡失败", "打卡服务暂时不可用")
		}
		item, err := s.checkin.ClockIn(ctx, req.UserID, "agent-confirm-"+tokenHash)
		if err != nil {
			if errors.Is(err, domaincheckin.ErrAlreadyClockedIn) {
				return failureResult(req.Tool, "上班打卡未执行", "今天已经完成上班打卡")
			}
			return failureResult(req.Tool, "上班打卡失败", "未能完成上班打卡")
		}
		return successResult(req.Tool, "上班打卡成功", "已记录当前账号的上班打卡", item)
	case confirmation.ToolClockOut:
		if s.checkin == nil {
			return failureResult(req.Tool, "下班打卡失败", "打卡服务暂时不可用")
		}
		item, err := s.checkin.ClockOut(ctx, req.UserID, "agent-confirm-"+tokenHash)
		if err != nil {
			if errors.Is(err, domaincheckin.ErrNotClockedIn) {
				return failureResult(req.Tool, "下班打卡未执行", "今天尚未完成上班打卡")
			}
			return failureResult(req.Tool, "下班打卡失败", "未能完成下班打卡")
		}
		return successResult(req.Tool, "下班打卡成功", "已记录当前账号的下班打卡", item)
	case confirmation.ToolDraftAnnouncement:
		if s.drafts == nil {
			return failureResult(req.Tool, "公告草稿创建失败", "公告草稿服务暂时不可用")
		}
		var input confirmation.DraftAnnouncementParameters
		if err := decodeParameters(parameters, &input); err != nil {
			return failureResult(req.Tool, "公告草稿创建失败", "提议参数无效")
		}
		item, err := s.drafts.Create(ctx, req.UserID, announcementdraftapp.CreateRequest{
			Title: input.Title, Content: input.Content, Pinned: input.Pinned,
			Source:       map[string]any{"generated_by": "agent", "confirmation_hash": tokenHash, "note": input.SourceNote},
			RecipientIDs: input.RecipientIDs, EmailRequested: false,
		})
		if err != nil {
			return failureResult(req.Tool, "公告草稿创建失败", "未能保存公告草稿")
		}
		return successResult(req.Tool, "公告草稿已创建", "草稿已保存，仍需人工审核后才能发布", item)
	case confirmation.ToolReviewKnowledge:
		if s.knowledge == nil {
			return failureResult(req.Tool, "知识审核失败", "知识审核服务暂时不可用")
		}
		var input confirmation.KnowledgeReviewParameters
		if err := decodeParameters(parameters, &input); err != nil {
			return failureResult(req.Tool, "知识审核失败", "提议参数无效")
		}
		reviewerName := strings.TrimSpace(req.ActorName)
		if reviewerName == "" {
			reviewerName = "Agent 确认操作"
		}
		if input.Decision == "approve" {
			if err := s.knowledge.ApproveItem(ctx, input.ItemID, req.UserID, reviewerName); err != nil {
				return failureResult(req.Tool, "知识条目批准失败", "条目可能已被处理或当前状态不允许批准")
			}
			return successResult(req.Tool, "知识条目已批准", "待审核条目已通过审核", map[string]any{"item_id": input.ItemID, "decision": input.Decision, "status": "approved"})
		}
		if _, err := s.knowledge.RejectItem(ctx, input.ItemID, req.UserID, reviewerName, input.Reason); err != nil {
			return failureResult(req.Tool, "知识条目拒绝失败", "条目可能已被处理或当前状态不允许拒绝")
		}
		return successResult(req.Tool, "知识条目已拒绝", "待审核条目已拒绝并保留审核记录", map[string]any{"item_id": input.ItemID, "decision": input.Decision, "status": "rejected"})
	case confirmation.ToolGenerateInvite:
		if s.invitations == nil {
			return failureResult(req.Tool, "邀请码生成失败", "邀请码服务暂时不可用")
		}
		var input confirmation.InvitationParameters
		if err := decodeParameters(parameters, &input); err != nil {
			return failureResult(req.Tool, "邀请码生成失败", "提议参数无效")
		}
		item, err := s.invitations.GenerateCode(ctx, req.UserID, domaininvitation.InvitationType(input.Type))
		if err != nil {
			return failureResult(req.Tool, "邀请码生成失败", "未能生成邀请码，请检查权限后重试")
		}
		return successResult(req.Tool, "邀请码已生成", "请在有效期内使用；邀请码仅显示在此结果卡片中", map[string]any{
			"id": item.ID, "code": item.Code, "type": item.Type, "expires_at": item.ExpiresAt,
		})
	case confirmation.ToolSaveEvaluation:
		if s.evaluations == nil {
			return failureResult(req.Tool, "考评草稿保存失败", "考评服务暂时不可用")
		}
		var input confirmation.EvaluationDraftParameters
		if err := decodeParameters(parameters, &input); err != nil {
			return failureResult(req.Tool, "考评草稿保存失败", "提议参数无效")
		}
		var item *evaluationapp.DraftDTO
		var err error
		if input.Mode == "create" {
			item, err = s.evaluations.CreateDraft(ctx, req.UserID, evaluationapp.CreateDraftRequest{
				SnapshotID: input.SnapshotID, Title: input.Title, SummaryMarkdown: input.SummaryMarkdown,
				Audience: domainevaluation.DraftAudience(input.Audience), Model: "agent", PromptVersion: "agent-governed-v1",
			})
		} else {
			item, err = s.evaluations.UpdateDraft(ctx, input.DraftID, req.UserID, evaluationapp.UpdateDraftRequest{Title: input.Title, SummaryMarkdown: input.SummaryMarkdown})
		}
		if err != nil || item == nil {
			return failureResult(req.Tool, "考评草稿保存失败", "未能保存草稿；快照或草稿状态可能已变化")
		}
		return successResult(req.Tool, "考评草稿已保存", "仅保存为草稿，尚未审核、发布或通知成员", map[string]any{
			"id": item.ID, "snapshot_id": item.SnapshotID, "period_id": item.PeriodID,
			"status": item.Status, "audience": item.Audience, "title": item.Title,
		})
	default:
		return failureResult(req.Tool, "操作失败", "不支持的操作")
	}
}

func authorize(req ExecuteRequest, parameters json.RawMessage) error {
	isManager := req.Role == "captain" || req.Role == "super_admin"
	switch req.Tool {
	case confirmation.ToolDraftAnnouncement, confirmation.ToolReviewKnowledge, confirmation.ToolGenerateInvite, confirmation.ToolSaveEvaluation:
		if !isManager {
			return shared.ErrForbidden
		}
	}
	switch req.Tool {
	case confirmation.ToolReviewKnowledge:
		var input confirmation.KnowledgeReviewParameters
		if decodeParameters(parameters, &input) != nil || input.ItemID <= 0 || input.Decision != "approve" && input.Decision != "reject" || input.Decision == "reject" && strings.TrimSpace(input.Reason) == "" || len([]rune(input.Reason)) > 512 {
			return ErrInvalidConfirmation
		}
	case confirmation.ToolGenerateInvite:
		var input confirmation.InvitationParameters
		if decodeParameters(parameters, &input) != nil || input.Type != "member" && input.Type != "captain" {
			return ErrInvalidConfirmation
		}
		if input.Type == "captain" && req.Role != "super_admin" {
			return shared.ErrForbidden
		}
	case confirmation.ToolSaveEvaluation:
		var input confirmation.EvaluationDraftParameters
		if decodeParameters(parameters, &input) != nil || strings.TrimSpace(input.Title) == "" || len([]rune(input.Title)) > 256 || len([]rune(input.SummaryMarkdown)) > 12000 {
			return ErrInvalidConfirmation
		}
		if input.Mode == "create" {
			if input.SnapshotID <= 0 || input.DraftID != 0 || input.Audience != "captain" && input.Audience != "team" && input.Audience != "private" {
				return ErrInvalidConfirmation
			}
		} else if input.Mode != "update" || input.DraftID <= 0 || input.SnapshotID != 0 || input.Audience != "" {
			return ErrInvalidConfirmation
		}
	}
	return nil
}

func replayResult(record *Record) (*Result, error) {
	if record == nil {
		return nil, ErrInvalidConfirmation
	}
	if record.Status == "executing" {
		return &Result{Tool: record.ToolName, Status: "pending", Title: "操作正在处理", Message: "该确认已提交，请勿重复操作", Replayed: true}, nil
	}
	var result Result
	if err := json.Unmarshal([]byte(record.ResultJSON), &result); err != nil {
		return nil, fmt.Errorf("decode stored action result: %w", err)
	}
	result.Replayed = true
	return &result, nil
}

func decodeParameters(data json.RawMessage, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func successResult(toolName, title, message string, data any) *Result {
	payload, _ := json.Marshal(data)
	return &Result{Tool: toolName, Status: "success", Title: title, Message: message, Data: payload}
}

func failureResult(toolName, title, message string) *Result {
	return &Result{Tool: toolName, Status: "failed", Title: title, Message: message}
}
