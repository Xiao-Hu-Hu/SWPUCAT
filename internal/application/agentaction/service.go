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
	domaincheckin "SWPUCAT/internal/domain/checkin"
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

type ExecuteRequest struct {
	UserID     int64
	Role       string
	SessionID  string
	Tool       string
	Token      string
	Parameters json.RawMessage
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
	enabled bool
	repo    Repository
	signer  ConfirmationVerifier
	checkin CheckinWriter
	drafts  DraftWriter
}

func NewService(enabled bool, repo Repository, signer ConfirmationVerifier, checkin CheckinWriter, drafts DraftWriter) *Service {
	return &Service{enabled: enabled, repo: repo, signer: signer, checkin: checkin, drafts: drafts}
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
	if req.Tool == confirmation.ToolDraftAnnouncement && req.Role != "captain" && req.Role != "super_admin" {
		return nil, shared.ErrForbidden
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
	default:
		return failureResult(req.Tool, "操作失败", "不支持的操作")
	}
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
