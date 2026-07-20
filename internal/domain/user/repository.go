package user

import "context"

type Repository interface {
	Create(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id int64) (*User, error)
	FindByUsername(ctx context.Context, username Username) (*User, error)
	FindByStudentID(ctx context.Context, studentID StudentID) (*User, error)
	FindAll(ctx context.Context) ([]*User, error)
	FindByIDs(ctx context.Context, ids []int64) ([]*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context) (int64, error)
	ExistsByUsername(ctx context.Context, username Username) (bool, error)
	ExistsByStudentID(ctx context.Context, studentID StudentID) (bool, error)
}
