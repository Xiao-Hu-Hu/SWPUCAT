package user

type UserDomainService struct {
	repo Repository
}

func NewUserDomainService(repo Repository) *UserDomainService {
	return &UserDomainService{repo: repo}
}

func (s *UserDomainService) ValidateTransfer(captainID, targetID int64) (*User, *User, error) {
	captain, err := s.repo.FindByID(nil, captainID)
	if err != nil {
		return nil, nil, ErrUserNotFound
	}
	if !captain.IsCaptain() {
		return nil, nil, ErrNotCaptain
	}

	target, err := s.repo.FindByID(nil, targetID)
	if err != nil {
		return nil, nil, ErrUserNotFound
	}

	return captain, target, nil
}
