package user

import (
	"context"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/domain/user"
)

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(hashedPassword, password string) error
}

type JWTService interface {
	GenerateAccessToken(userID int64, role string) (string, int64, error)
	GenerateRefreshToken(userID int64) (string, error)
	ParseRefreshToken(tokenString string) (int64, error)
}

type UserApplicationService struct {
	userRepo  user.Repository
	hasher    PasswordHasher
	jwtSvc    JWTService
	publisher shared.EventPublisher
}

func NewUserApplicationService(
	userRepo user.Repository,
	hasher PasswordHasher,
	jwtSvc JWTService,
	publisher shared.EventPublisher,
) *UserApplicationService {
	return &UserApplicationService{
		userRepo:  userRepo,
		hasher:    hasher,
		jwtSvc:    jwtSvc,
		publisher: publisher,
	}
}

func (s *UserApplicationService) Register(ctx context.Context, req RegisterRequest) (*LoginResponse, error) {
	username, err := user.NewUsername(req.Username)
	if err != nil {
		return nil, err
	}
	nickname, err := user.NewNickname(req.Nickname)
	if err != nil {
		return nil, err
	}

	exists, err := s.userRepo.ExistsByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, user.ErrUsernameExists
	}

	hashedPassword, err := s.hasher.Hash(req.Password)
	if err != nil {
		return nil, err
	}

	u := user.NewUser(username, nickname, user.PasswordHash(hashedPassword))
	if err := s.userRepo.Create(ctx, u); err != nil {
		return nil, err
	}

	s.publisher.Publish(user.NewUserRegistered(u.ID, string(u.Username), string(u.Nickname)))

	return s.generateTokens(u)
}

func (s *UserApplicationService) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	u, err := s.userRepo.FindByUsername(ctx, user.Username(req.Username))
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

func (s *UserApplicationService) TransferCaptain(ctx context.Context, operatorID, targetID int64) error {
	operator, err := s.userRepo.FindByID(ctx, operatorID)
	if err != nil {
		return err
	}
	target, err := s.userRepo.FindByID(ctx, targetID)
	if err != nil {
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

func (s *UserApplicationService) ListMembers(ctx context.Context) ([]MemberDTO, error) {
	users, err := s.userRepo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]MemberDTO, 0, len(users))
	for _, u := range users {
		result = append(result, MemberDTO{
			ID:           u.ID,
			Nickname:     string(u.Nickname),
			Username:     string(u.Username),
			Role:         string(u.Role),
			JoinedAt:     u.JoinedAt.Format("2006-01-02"),
			CheckinCount: u.CheckinCount,
		})
	}
	return result, nil
}

func (s *UserApplicationService) generateTokens(u *user.User) (*LoginResponse, error) {
	accessToken, expiresIn, err := s.jwtSvc.GenerateAccessToken(u.ID, string(u.Role))
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
		ID:           u.ID,
		Username:     string(u.Username),
		Nickname:     string(u.Nickname),
		Role:         string(u.Role),
		JoinedAt:     u.JoinedAt.Format("2006-01-02"),
		CheckinCount: u.CheckinCount,
	}
}
