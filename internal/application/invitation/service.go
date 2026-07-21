package invitation

import (
	"context"
	"fmt"
	"SWPUCAT/internal/domain/invitation"
	"SWPUCAT/internal/domain/user"
)

type InvitationDTO struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	Type      string `json:"type"`
	CreatorID int64  `json:"creator_id"`
	Used      bool   `json:"used"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

type InvitationService struct {
	invitationRepo invitation.Repository
	userRepo       user.Repository
}

func NewInvitationService(
	invitationRepo invitation.Repository,
	userRepo user.Repository,
) *InvitationService {
	return &InvitationService{
		invitationRepo: invitationRepo,
		userRepo:       userRepo,
	}
}

func (s *InvitationService) GenerateCode(ctx context.Context, creatorID int64, codeType invitation.InvitationType) (*InvitationDTO, error) {
	// Get creator
	creator, err := s.userRepo.FindByID(ctx, creatorID)
	if err != nil {
		return nil, fmt.Errorf("creator not found")
	}

	// Check permissions
	if codeType == invitation.TypeCaptain && !creator.Role.IsSuperAdmin() {
		return nil, fmt.Errorf("only super admin can generate captain invitation codes")
	}
	if codeType == invitation.TypeMember && !creator.Role.IsSuperAdmin() && !creator.Role.IsCaptain() {
		return nil, fmt.Errorf("only super admin or captain can generate member invitation codes")
	}

	code, err := invitation.NewInvitationCode(codeType, creatorID)
	if err != nil {
		return nil, err
	}

	if err := s.invitationRepo.Create(ctx, code); err != nil {
		return nil, err
	}

	return toDTO(code), nil
}

func (s *InvitationService) ValidateCode(ctx context.Context, code string) (*invitation.InvitationCode, error) {
	invCode, err := s.invitationRepo.FindByCode(ctx, code)
	if err != nil {
		return nil, invitation.ErrCodeNotFound
	}

	if !invCode.IsValid() {
		if invCode.Used {
			return nil, invitation.ErrCodeAlreadyUsed
		}
		return nil, invitation.ErrCodeExpired
	}

	return invCode, nil
}

func (s *InvitationService) UseCode(ctx context.Context, code string, userID int64) error {
	invCode, err := s.ValidateCode(ctx, code)
	if err != nil {
		return err
	}

	if err := invCode.Use(userID); err != nil {
		return err
	}

	return s.invitationRepo.Update(ctx, invCode)
}

func (s *InvitationService) GetMyCodes(ctx context.Context, creatorID int64) ([]InvitationDTO, error) {
	codes, err := s.invitationRepo.FindByCreatorID(ctx, creatorID)
	if err != nil {
		return nil, err
	}

	result := make([]InvitationDTO, 0, len(codes))
	for _, code := range codes {
		result = append(result, *toDTO(code))
	}
	return result, nil
}

func toDTO(code *invitation.InvitationCode) *InvitationDTO {
	return &InvitationDTO{
		ID:        code.ID,
		Code:      code.Code,
		Type:      string(code.Type),
		CreatorID: code.CreatorID,
		Used:      code.Used,
		ExpiresAt: code.ExpiresAt.Format("2006-01-02 15:04:05"),
		CreatedAt: code.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
