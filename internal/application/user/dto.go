package user

type RegisterRequest struct {
	Username         string `json:"username" validate:"required,min=3,max=32,alphanum"`
	Nickname         string `json:"nickname" validate:"required,min=1,max=64"`
	Password         string `json:"password" validate:"required,min=6,max=128"`
	Email            string `json:"email" validate:"required,email"`
	VerificationCode string `json:"verification_code" validate:"required,len=6"`
	InvitationCode   string `json:"invitation_code" validate:"required,len=6"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type SendCodeRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type LoginResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresIn    int64   `json:"expires_in"`
	User         UserDTO `json:"user"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type UserDTO struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	StudentID     string `json:"student_id,omitempty"`
	Email         string `json:"email,omitempty"`
	Nickname      string `json:"nickname"`
	Avatar        string `json:"avatar,omitempty"`
	TechDirection string `json:"tech_direction,omitempty"`
	Role          string `json:"role"`
	JoinedAt      string `json:"joined_at"`
	CheckinCount  int64  `json:"checkin_count"`
}

type ChangePasswordRequest struct {
	OldPassword      string `json:"old_password" validate:"required"`
	NewPassword      string `json:"new_password" validate:"required,min=6,max=128"`
	VerificationCode string `json:"verification_code" validate:"required,len=6"`
}

type TransferCaptainRequest struct {
	TargetUserID int64 `json:"target_user_id" validate:"required,min=1"`
}

type MemberDTO struct {
	ID            int64  `json:"id"`
	Nickname      string `json:"nickname"`
	Username      string `json:"username"`
	StudentID     string `json:"student_id,omitempty"`
	Avatar        string `json:"avatar,omitempty"`
	TechDirection string `json:"tech_direction,omitempty"`
	Role          string `json:"role"`
	JoinedAt      string `json:"joined_at"`
	CheckinCount  int64  `json:"checkin_count"`
}
