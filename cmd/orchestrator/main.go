package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/neandrson/go_calc_final/internal/app"
	"github.com/neandrson/go_calc_final/internal/config"
	"github.com/neandrson/go_calc_final/internal/repositories/agent"
	appRepo "github.com/neandrson/go_calc_final/internal/repositories/app"
	"github.com/neandrson/go_calc_final/internal/repositories/expression"
	"github.com/neandrson/go_calc_final/internal/repositories/queue"
	"github.com/neandrson/go_calc_final/internal/repositories/subExpression"
	"github.com/neandrson/go_calc_final/internal/repositories/user"
	"github.com/neandrson/go_calc_final/internal/services/auth"
	"github.com/neandrson/go_calc_final/internal/services/orchestrator"
	log "github.com/sirupsen/logrus"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Add this line for logging filename and line number!
	log.SetReportCaller(true)

	// Only log the debug severity or above.
	log.SetLevel(log.DebugLevel)
}

// Start инициализирует и запускает оркестратор
func Start() {
	cfg := config.MustLoad()
	dataSourceName := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable",
		cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DbName, cfg.Postgres.User, cfg.Postgres.Password)
	log.Printf(dataSourceName)

	expressionRepo, err := expression.NewPostgresRepository(dataSourceName)
	fmt.Println(expressionRepo, err)
	if err != nil {
		log.Fatalf("Не удалось подключиться к postgres: %v", err)
		return
	}
	subExpressionRepo, err := subExpression.NewPostgresRepository(dataSourceName)
	if err != nil {
		log.Fatalf("Не удалось подключиться к postgres: %v", err)
		return
	}
	agentRepo, err := agent.NewPostgresRepository(dataSourceName)
	if err != nil {
		log.Fatalf("Не удалось подключиться к агенту postgres: %v", err)
		return
	}

	expressionsQueueRepo, err := queue.NewRabbitMQRepository(cfg.UrlRabbit, cfg.Queue.NameQueueWithTasks)
	if err != nil {
		log.Fatalf("Не удалось запустить очередь: %v", err)
	}
	calculationsQueueRepository, err := queue.NewRabbitMQRepository(cfg.UrlRabbit, cfg.Queue.NameQueueWithFinishedTasks)
	if err != nil {
		log.Fatalf("Не удалось запустить очередь: %v", err)
	}
	heartbeatsQueueRepository, err := queue.NewRabbitMQRepository(cfg.UrlRabbit, cfg.Queue.NameQueueWithHeartbeats)
	if err != nil {
		log.Fatalf("Не удалось запустить очередь: %v", err)
	}
	rpcQueueRepository, err := queue.NewRabbitMQRepository(cfg.UrlRabbit, cfg.Queue.NameQueueWithRPC)
	if err != nil {
		log.Fatalf("Не удалось запустить очередь: %v", err)
	}
	userRepository, err := user.NewPostgresRepository(dataSourceName)
	if err != nil {
		log.Fatalf("Не удалось запустить очередь: %v", err)
	}
	appRepository, err := appRepo.NewPostgresRepository(dataSourceName)
	if err != nil {
		log.Fatalf("Не удалось запустить очередь: %v", err)
	}

	ctx := context.Background()
	logSlog := slog.New(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)

	newOrchestrator := orchestrator.NewOrchestrator(ctx, expressionRepo, subExpressionRepo, expressionsQueueRepo,
		calculationsQueueRepository, heartbeatsQueueRepository, rpcQueueRepository, agentRepo, cfg.RetrySubExpressionTimout)
	newAuth := auth.New(logSlog, userRepository, userRepository, appRepository, cfg.TokenTTL)

	// Регистрация хендлеров
	application := app.New(logSlog, newOrchestrator, appRepository, newAuth, cfg.GRPC.Port, cfg.CalculationTimeouts, cfg.TokenTTL)
	go func() {
		application.GRPCServer.MustRun()
	}()
	// Graceful shutdown

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	<-stop

	application.GRPCServer.Stop()
	log.Info("Сервер остановлен")

}

func main() {
	Start()
}
