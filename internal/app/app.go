package app

import (
	"log/slog"
	"time"

	grpcapp "github.com/neandrson/go_calc_final/internal/app/grpc"
	"github.com/neandrson/go_calc_final/internal/config"
	"github.com/neandrson/go_calc_final/internal/repositories/app"
	"github.com/neandrson/go_calc_final/internal/services/auth"
	"github.com/neandrson/go_calc_final/internal/services/orchestrator"
)

type App struct {
	GRPCServer *grpcapp.App
}

func New(
	log *slog.Logger,
	orchestrator orchestrator.IOrchestrator,
	appRepo app.Repository,
	auth auth.IOAuth,
	grpcPort int,
	timeouts config.CalculationTimeoutsConfig,
	tokenTTL time.Duration,
) *App {
	grpcServer := grpcapp.New(log, auth, orchestrator, appRepo, grpcPort, timeouts)
	return &App{
		GRPCServer: grpcServer,
	}
}
