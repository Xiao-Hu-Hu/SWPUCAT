package user

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"SWPUCAT/internal/domain/invitation"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"

	"golang.org/x/crypto/bcrypt"
)

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(hashedPassword, password string) error
}

type JWTService interface {
	GenerateAccessToken(userID int64, username, role, studentID string) (string, int64, error)
	GenerateRefreshToken(userID int64) (string, error)
	ParseRefreshToken(tokenString string) (int64, error)
}

type EmailService interface {
	SendVerificationCode(to string, code string) error
}

type VerificationCodeRepository interface {
	Create(ctx context.Context, email string, code string, expiresAt time.Time) error
	FindValidCode(ctx context.Context, email string, code string) (bool, error)
}

type InvitationCodeRepository interface {
	FindByCode(ctx context.Context, code string) (*invitation.InvitationCode, error)
	Update(ctx context.Context, code *invitation.InvitationCode) error
}

type UserApplicationService struct {
	userRepo     user.Repository
	hasher       PasswordHasher
	jwtSvc       JWTService
	publisher    shared.EventPublisher
	emailSvc     EmailService
	codeRepo     VerificationCodeRepository
	invitationRepo InvitationCodeRepository
}

func NewUserApplicationService(
	userRepo user.Repository,
	hasher PasswordHasher,
	jwtSvc JWTService,
	publisher shared.EventPublisher,
	emailSvc EmailService,
	codeRepo VerificationCodeRepository,
	invitationRepo InvitationCodeRepository,
) *UserApplicationService {
	return &UserApplicationService{
		userRepo:       userRepo,
		hasher:         hasher,
		jwtSvc:         jwtSvc,
		publisher:      publisher,
		emailSvc:       emailSvc,
		codeRepo:       codeRepo,
		invitationRepo: invitationRepo,
	}
}

func (s *UserApplicationService) SendVerificationCode(ctx context.Context, email string) error {
	code := generateCode()
	expiresAt := time.Now().Add(2 * time.Minute)

	if err := s.codeRepo.Create(ctx, email, code, expiresAt); err != nil {
		log.Printf("[ERROR] Failed to save verification code: %v", err)
		return err
	}

	log.Printf("[INFO] Sending verification code %s to %s", code, email)
	if err := s.emailSvc.SendVerificationCode(email, code); err != nil {
		log.Printf("[ERROR] Failed to send email to %s: %v", email, err)
		return err
	}

	log.Printf("[INFO] Verification code sent successfully to %s", email)
	return nil
}

func generateCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}

func (s *UserApplicationService) verifyCode(ctx context.Context, email, code string) error {
	valid, err := s.codeRepo.FindValidCode(ctx, email, code)
	if err != nil || !valid {
		return fmt.Errorf("验证码无效或已过期")
	}
	return nil
}

func (s *UserApplicationService) Register(ctx context.Context, req RegisterRequest) (*LoginResponse, error) {
	studentID, err := user.NewStudentID(req.Username)
	if err != nil {
		return nil, err
	}
	nickname, err := user.NewNickname(req.Nickname)
	if err != nil {
		return nil, err
	}

	// Verify email code
	valid, err := s.codeRepo.FindValidCode(ctx, req.Email, req.VerificationCode)
	if err != nil || !valid {
		return nil, fmt.Errorf("invalid verification code")
	}

	// Validate invitation code
	invCode, err := s.invitationRepo.FindByCode(ctx, req.InvitationCode)
	if err != nil {
		return nil, fmt.Errorf("invalid invitation code")
	}
	if !invCode.IsValid() {
		if invCode.Used {
			return nil, invitation.ErrCodeAlreadyUsed
		}
		return nil, invitation.ErrCodeExpired
	}

	// Determine role based on invitation type
	var role user.Role
	switch invCode.Type {
	case invitation.TypeCaptain:
		role = user.RoleCaptain
	case invitation.TypeMember:
		role = user.RoleMember
	default:
		return nil, fmt.Errorf("invalid invitation type")
	}

	exists, err := s.userRepo.ExistsByStudentID(ctx, studentID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, user.ErrStudentIDExists
	}

	hashedPassword, err := s.hasher.Hash(req.Password)
	if err != nil {
		return nil, err
	}

	u := user.NewUserWithStudentID(studentID, req.Email, nickname, user.PasswordHash(hashedPassword))
	u.Role = role

	if err := s.userRepo.Create(ctx, u); err != nil {
		return nil, err
	}

	// Mark invitation code as used
	if err := invCode.Use(u.ID); err != nil {
		return nil, err
	}
	if err := s.invitationRepo.Update(ctx, invCode); err != nil {
		return nil, err
	}

	s.publisher.Publish(user.NewUserRegistered(u.ID, string(u.Username), string(u.Nickname)))

	return s.generateTokens(u)
}

func (s *UserApplicationService) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	// Try to find by student ID first, then by username
	var u *user.User
	var err error

	if len(req.Username) == 12 && user.StudentID(req.Username) != "" {
		u, err = s.userRepo.FindByStudentID(ctx, user.StudentID(req.Username))
	}
	if u == nil {
		u, err = s.userRepo.FindByUsername(ctx, user.Username(req.Username))
	}
	if err != nil {
		return nil, user.ErrInvalidCredentials
	}

	if err := s.hasher.Verify(string(u.PasswordHash), req.Password); err != nil {
		return nil, user.ErrInvalidCredentials
	}

	return s.generateTokens(u)
}

func (s *UserApplicationService) RefreshToken(ctx context.Context, refreshToken string) (*LoginResponse, error) {
	userID, err := s.jwtSvc.ParseRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}

	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.generateTokens(u)
}

func (s *UserApplicationService) TransferCaptain(ctx context.Context, operatorID, targetID int64, verificationCode string) error {
	operator, err := s.userRepo.FindByID(ctx, operatorID)
	if err != nil {
		return err
	}
	target, err := s.userRepo.FindByID(ctx, targetID)
	if err != nil {
		return err
	}

	if err := s.verifyCode(ctx, operator.Email, verificationCode); err != nil {
		return err
	}

	if err := operator.TransferCaptainTo(target); err != nil {
		return err
	}

	if err := s.userRepo.Update(ctx, operator); err != nil {
		return err
	}
	if err := s.userRepo.Update(ctx, target); err != nil {
		return err
	}

	s.publisher.Publish(user.NewCaptainTransferred(operator.ID, target.ID))
	return nil
}

func (s *UserApplicationService) RemoveMember(ctx context.Context, operatorID, userID int64) error {
	target, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if target.IsCaptain() {
		return user.ErrCannotRemoveCaptain
	}

	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return err
	}

	s.publisher.Publish(user.NewMemberRemoved(userID, operatorID))
	return nil
}

func (s *UserApplicationService) GetUser(ctx context.Context, userID int64) (*UserDTO, error) {
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toUserDTO(u), nil
}

func (s *UserApplicationService) UpdateNickname(ctx context.Context, userID int64, nickname string) error {
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	u.Nickname = user.Nickname(nickname)
	return s.userRepo.Update(ctx, u)
}

func (s *UserApplicationService) UpdateAvatar(ctx context.Context, userID int64, avatarPath string) error {
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	u.Avatar = avatarPath
	return s.userRepo.Update(ctx, u)
}

func (s *UserApplicationService) UpdateTechDirection(ctx context.Context, userID int64, techDirection string) error {
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	u.TechDirection = techDirection
	return s.userRepo.Update(ctx, u)
}

func (s *UserApplicationService) ChangePassword(ctx context.Context, userID int64, req ChangePasswordRequest) error {
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
		return errors.New("旧密码不正确")
	}

	// Check if old and new password are the same
	if req.OldPassword == req.NewPassword {
		return errors.New("新密码不能与旧密码相同")
	}

	// Verify email code
	if err := s.verifyCode(ctx, string(u.Email), req.VerificationCode); err != nil {
		return err
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	u.PasswordHash = user.PasswordHash(string(hash))
	return s.userRepo.Update(ctx, u)
}

func (s *UserApplicationService) ListMembers(ctx context.Context) ([]MemberDTO, error) {
	users, err := s.userRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]MemberDTO, 0, len(users))
	for _, u := range users {
		result = append(result, MemberDTO{
			ID:            u.ID,
			Nickname:      string(u.Nickname),
			Username:      string(u.Username),
			StudentID:     string(u.StudentID),
			Avatar:        u.Avatar,
			TechDirection: u.TechDirection,
			Role:          string(u.Role),
			JoinedAt:      u.JoinedAt.Format("2006-01-02"),
			CheckinCount:  u.CheckinCount,
		})
	}
	return result, nil
}

func (s *UserApplicationService) generateTokens(u *user.User) (*LoginResponse, error) {
	accessToken, expiresIn, err := s.jwtSvc.GenerateAccessToken(u.ID, string(u.Nickname), string(u.Role), string(u.StudentID))
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.jwtSvc.GenerateRefreshToken(u.ID)
	if err != nil {
		return nil, err
	}
	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
		User:         *toUserDTO(u),
	}, nil
}

func toUserDTO(u *user.User) *UserDTO {
	return &UserDTO{
		ID:            u.ID,
		Username:      string(u.Username),
		StudentID:     string(u.StudentID),
		Email:         string(u.Email),
		Nickname:      string(u.Nickname),
		Avatar:        u.Avatar,
		TechDirection: u.TechDirection,
		Role:          string(u.Role),
		JoinedAt:      u.JoinedAt.Format("2006-01-02"),
		CheckinCount:  u.CheckinCount,
	}
}
