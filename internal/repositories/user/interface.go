package user

import (
	"context"

	"github.com/neandrson/go_calc_final/internal/models"
)

type Repository interface {
	// Create создает пользователя
	Create(ctx context.Context, login string, passHash []byte) (int64, error)
	// Get возвращает пользователя по id
	Get(ctx context.Context, login string) (models.User, error)
}
