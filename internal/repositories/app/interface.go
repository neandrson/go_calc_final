package app

import (
	"context"

	"github.com/neandrson/go_calc_final/internal/models"
)

type Repository interface {
	App(ctx context.Context, appID int) (models.App, error)
}
