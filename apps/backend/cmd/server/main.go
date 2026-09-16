package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/assignment"
	"github.com/Satyajeet-Das/ai-scribe/internal/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	httpPlatform "github.com/Satyajeet-Das/ai-scribe/internal/platform/http"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/handler"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/job"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/redis"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

const DefaultShutdownTimeout = 30 * time.Second

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic("failed to load configuration: " + err.Error())
	}

	loggerService := logger.NewLoggerService(cfg.Observability)
	defer loggerService.Shutdown()

	log := logger.NewLoggerWithService(cfg.Observability, loggerService)
	log.Info().Str("env", cfg.Primary.Env).Msg("starting AI Exam Scribe modular monolith")

	db, err := database.New(cfg, &log, loggerService)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize PostgreSQL database connection")
	}
	defer db.Close()

	if cfg.Primary.Env != "local" {
		if err := database.Migrate(context.Background(), &log, cfg); err != nil {
			log.Fatal().Err(err).Msg("failed to apply database migrations")
		}
	}

	redisClient := redis.New(cfg, &log, loggerService)
	defer redisClient.Close()

	jobService := job.NewJobService(&log, cfg)
	jobService.InitHandlers(cfg, &log)
	go func() {
		if err := jobService.Start(); err != nil {
			log.Error().Err(err).Msg("background job worker failed to start")
		}
	}()
	defer jobService.Stop()

	// -------------------------------------------------------------------------
	// Dependency Injection & Domain Composition
	// -------------------------------------------------------------------------
	_ = auth.NewService(cfg.Auth.SecretKey)
	authMiddleware := auth.NewMiddleware(&log)

	userRepo := user.NewRepository(db.Pool)
	_ = user.NewService(userRepo, &log)

	examRepo := exam.NewRepository(db.Pool)
	examService := exam.NewService(examRepo, &log)
	examHandler := exam.NewHandler(examService)

	questionRepo := question.NewRepository(db.Pool)
	questionService := question.NewService(questionRepo, &log)
	questionHandler := question.NewHandler(questionService)

	assignmentRepo := assignment.NewRepository(db.Pool)
	assignmentService := assignment.NewService(assignmentRepo, &log)
	assignmentHandler := assignment.NewHandler(assignmentService)

	sessionRepo := session.NewRepository(db.Pool)
	sessionService := session.NewService(sessionRepo, &log)
	sessionHandler := session.NewHandler(sessionService)

	answerRepo := answer.NewRepository(db.Pool)
	answerService := answer.NewService(answerRepo, &log)
	answerHandler := answer.NewHandler(answerService)

	// -------------------------------------------------------------------------
	// HTTP Platform Routing & Transport
	// -------------------------------------------------------------------------
	middlewares := middleware.NewMiddlewares(cfg, &log, loggerService)
	healthHandler := handler.NewHealthHandler(cfg.Primary.Env, db, redisClient, loggerService)
	openAPIHandler := handler.NewOpenAPIHandler()

	router := httpPlatform.NewRouter(httpPlatform.RouterParams{
		Middlewares:    middlewares,
		HealthHandler:  healthHandler,
		OpenAPIHandler: openAPIHandler,
		Logger:         &log,
	})

	v1 := router.Group("/api/v1")
	examHandler.RegisterRoutes(v1, authMiddleware.RequireAuth)
	questionHandler.RegisterRoutes(v1, authMiddleware.RequireAuth)
	assignmentHandler.RegisterRoutes(v1, authMiddleware.RequireAuth)
	sessionHandler.RegisterRoutes(v1, authMiddleware.RequireAuth)
	answerHandler.RegisterRoutes(v1, authMiddleware.RequireAuth)

	srv := httpPlatform.NewServer(&cfg.Server, router, &log)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatal().Err(err).Msg("server terminated unexpectedly")
		}
	}()

	// -------------------------------------------------------------------------
	// Graceful Shutdown
	// -------------------------------------------------------------------------
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	<-ctx.Done()
	stop()

	log.Info().Msg("received interrupt signal, initiating graceful shutdown")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal().Err(err).Msg("server forced to shutdown")
	}

	log.Info().Msg("server exited cleanly")
}
