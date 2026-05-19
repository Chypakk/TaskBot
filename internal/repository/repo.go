package repository

import (
	"context"
	"tg_sheduler/internal/domain"
)

type TaskRepository interface {
	Create(ctx context.Context, task *domain.Task) error
	GetByID(ctx context.Context, id int) (*domain.Task, error)
	GetByUserID(ctx context.Context, userID int) ([]*domain.Task, error)
	Update(ctx context.Context, task *domain.Task) error
	Delete(ctx context.Context, id int) error
}

type NotificationRepository interface {
	CreateBatch(ctx context.Context, notifs []domain.Notification) error
	GetNotifications(ctx context.Context) ([]*domain.Notification, error)
	MarkAsSent(ctx context.Context, id int) error
	DeleteByTaskID(ctx context.Context, taskID int) error
}

type UserRepository interface{
	CreateUser(ctx context.Context, user *domain.User) (*domain.User, error)
	//получение пользователя по ID телеграмма
	GetUserByTGID(ctx context.Context, id int) (*domain.User, error)
	GetUserByID(ctx context.Context, id int) (*domain.User, error)
}