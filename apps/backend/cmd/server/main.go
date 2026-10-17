//go:generate go run ./cmd/openapi-gen

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/assignment"
	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	jwtadapter "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth/adapters/jwt"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/database"
	httpPlatform "github.com/Satyajeet-Das/ai-scribe/internal/platform/http"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/handler"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/middleware"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/job"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/openapi"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/redis"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/recording"
	"github.com/Satyajeet-Das/ai-scribe/internal/grading"
	"github.com/Satyajeet-Das/ai-scribe/internal/review"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
	"github.com/Satyajeet-Das/ai-scribe/internal/voice"
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

	if err := database.Migrate(context.Background(), &log, cfg); err != nil {
		log.Fatal().Err(err).Msg("failed to apply database migrations")
	}

	redisClient := redis.New(cfg, &log, loggerService)
	defer redisClient.Close()

	jobService := job.NewJobService(&log, cfg)
	defer jobService.Stop()

	// -------------------------------------------------------------------------
	// Dependency Injection & Domain Composition
	// -------------------------------------------------------------------------
	userRepo := user.NewRepository(db.Pool)
	userService := user.NewService(userRepo, &log)
	studentHandler := user.NewHandler(userService)

	// Auth platform setup (pluggable JWT provider)
	accessDuration := 15 * time.Minute
	if cfg.Auth.AccessTokenDuration > 0 {
		accessDuration = time.Duration(cfg.Auth.AccessTokenDuration) * time.Second
	}
	refreshDuration := 7 * 24 * time.Hour
	if cfg.Auth.RefreshTokenDuration > 0 {
		refreshDuration = time.Duration(cfg.Auth.RefreshTokenDuration) * time.Second
	}
	issuer := "ai-scribe"
	if cfg.Auth.Issuer != "" {
		issuer = cfg.Auth.Issuer
	}

	jwtConfig := jwtadapter.Config{
		SecretKey:            cfg.Auth.SecretKey,
		AccessTokenDuration:  accessDuration,
		RefreshTokenDuration: refreshDuration,
		Issuer:               issuer,
	}

	jwtAdapter := jwtadapter.NewAdapter(jwtConfig, userRepo, redisClient, &log)
	authProvider := jwtAdapter

	authMiddleware := platformauth.NewMiddleware(authProvider, &log)

	authRepo := auth.NewRepository(db.Pool)
	authService := auth.NewService(userRepo, authRepo, jwtAdapter, jwtConfig, &log)
	authHandler := auth.NewHandler(authService, cfg.Primary.Env == "production")

	examRepo := exam.NewRepository(db.Pool)
	examService := exam.NewService(examRepo, &log)
	examHandler := exam.NewHandler(examService)

	questionRepo := question.NewRepository(db.Pool)
	questionService := question.NewService(questionRepo, examService, db, &log)
	questionHandler := question.NewHandler(questionService)

	assignmentRepo := assignment.NewRepository(db.Pool)
	assignmentService := assignment.NewService(assignmentRepo, examService, userRepo, &log)
	assignmentHandler := assignment.NewHandler(assignmentService)

	sessionRepo := session.NewRepository(db.Pool)
	sessionCache := session.NewRedisCache(redisClient)
	sessionService := session.NewService(sessionRepo, assignmentService, examService, questionService, sessionCache, db, jobService, &log)
	sessionHandler := session.NewHandler(sessionService)

	jobService.InitHandlers(cfg, &log, sessionService)
	go func() {
		if err := jobService.Start(); err != nil {
			log.Error().Err(err).Msg("background job worker failed to start")
		}
	}()

	answerRepo := answer.NewRepository(db.Pool)
	answerService := answer.NewService(answerRepo, sessionService, questionService, &log)
	answerHandler := answer.NewHandler(answerService)

	// Teacher response review: read-only, owner/admin only.
	reviewService := review.NewService(review.NewRepository(db.Pool), examService, review.DefaultPolicy(), &log)
	reviewHandler := review.NewHandler(reviewService)

	// Teacher marking of those papers: the only place marks are written.
	gradingService := grading.NewService(grading.NewRepository(db.Pool), examService, &log)
	gradingHandler := grading.NewHandler(gradingService)

	// Where retained answer audio lives. Nil when there is none to read or write.
	audioStore := openAudioStore(context.Background(), cfg.Recording, &log)
	if cfg.Recording.Enabled && audioStore == nil {
		log.Fatal().Msg("recording is enabled but the audio store could not be opened")
	}

	// Listening to a student's recorded answers. This stays available when new
	// recording is switched off, so audio that was already kept can still be
	// checked. Every playback is audited before any audio is sent.
	if audioStore != nil {
		reviewHandler.WithPlayback(review.EnablePlayback(reviewService, audioStore, audit.NewRecorder(db.Pool)))
		log.Info().Str("store", audioStore.Name()).Msg("recording playback for teachers enabled")
	}

	// Deleting audio when its retention period ends, and tidying the store.
	// Off unless switched on; it only acts on retention dates that already exist.
	janitorCtx, stopJanitor := context.WithCancel(context.Background())
	janitorDone := make(chan struct{})
	if cfg.Recording.Cleanup.Enabled && audioStore != nil {
		janitor := recording.NewJanitor(recording.NewJanitorRepository(db.Pool), audioStore, audit.NewRecorder(db.Pool),
			recording.JanitorOptions{
				Interval:         cfg.Recording.Cleanup.Interval,
				BatchSize:        cfg.Recording.Cleanup.BatchSize,
				OrphanGrace:      cfg.Recording.Cleanup.OrphanGrace,
				UnconfirmedGrace: cfg.Recording.Cleanup.UnconfirmedGrace,
				DeleteOrphans:    cfg.Recording.Cleanup.DeleteOrphans,
			}, &log)
		go func() {
			defer close(janitorDone)
			janitor.Run(janitorCtx)
		}()
		log.Info().
			Dur("interval", cfg.Recording.Cleanup.Interval).
			Bool("delete_orphans", cfg.Recording.Cleanup.DeleteOrphans).
			Msg("recording cleanup is ENABLED")
	} else {
		close(janitorDone)
		if cfg.Recording.Enabled && !cfg.Recording.Cleanup.Enabled {
			log.Warn().Msg("recording is on but cleanup is off: dictations that students do not confirm will keep their audio until you enable SCRIBE_RECORDING.CLEANUP.ENABLED")
		}
		if cfg.Recording.Cleanup.Enabled {
			log.Warn().Msg("recording cleanup is enabled but there is no audio store; nothing will be cleaned")
		}
	}

	// Voice pipeline: WebSocket transport, STT, intent classification, TTS.
	// It reaches exam state only through the session and answer services.
	voiceProviders, err := voice.BuildProviders(cfg.Voice)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize voice providers")
	}

	// Retention of dictated answers is off unless explicitly enabled: it is an
	// institutional decision, not something the system starts doing by itself.
	var voiceRecorder voice.Recorder
	if cfg.Recording.Enabled {
		voiceRecorder = recording.NewService(recording.NewRepository(db.Pool), audioStore, recording.Options{
			RetentionDays: cfg.Recording.RetentionDays,
			SaveTimeout:   cfg.Recording.SaveTimeout,
		}, &log)
		log.Info().
			Str("store", audioStore.Name()).
			Int("retention_days", cfg.Recording.RetentionDays).
			Msg("answer audio retention is ENABLED")
	}

	voiceOrigins := cfg.Voice.AllowedOrigins
	if len(voiceOrigins) == 0 {
		voiceOrigins = cfg.Server.CORSAllowedOrigins
	}
	voiceService := voice.NewService(voice.Deps{
		Config:         cfg.Voice,
		AllowedOrigins: voiceOrigins,
		Auth:           authProvider,
		Sessions:       sessionService,
		Answers:        answerService,
		STT:            voiceProviders.STT,
		TTS:            voiceProviders.TTS,
		Classifier:     voiceProviders.Classifier,
		Recorder:       voiceRecorder,
		Metrics:        voice.NewMetrics(loggerService.GetApplication()),
		Logger:         &log,
	})
	log.Info().
		Bool("enabled", cfg.Voice.Enabled).
		Str("stt", voiceProviders.STT.Name()).
		Str("tts", voiceProviders.TTS.Name()).
		Str("intent", voiceProviders.Classifier.Name()).
		Msg("voice pipeline configured")

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

	// Rate limiter for login endpoint (5 attempts per minute per IP)
	loginRateLimiter := middlewares.RateLimit.WithRedis(redisClient, &log).Limit(5, 1*time.Minute)

	v1 := router.Group("/api/v1")
	authHandler.RegisterRoutes(v1, authMiddleware.RequireAuth, loginRateLimiter)
	studentHandler.RegisterRoutes(v1, authMiddleware.RequireAuth, authMiddleware.RequireRole(platformauth.RoleTeacher, platformauth.RoleAdmin))
	studentHandler.RegisterRoutes(router.Group(""), authMiddleware.RequireAuth, authMiddleware.RequireRole(platformauth.RoleTeacher, platformauth.RoleAdmin))
	examHandler.RegisterRoutes(v1, authMiddleware.RequireAuth, authMiddleware.RequireRole(platformauth.RoleTeacher, platformauth.RoleAdmin))
	questionHandler.RegisterRoutes(v1, authMiddleware.RequireAuth)
	assignmentHandler.RegisterRoutes(v1, authMiddleware.RequireAuth)
	sessionHandler.RegisterRoutes(v1, authMiddleware.RequireAuth)
	answerHandler.RegisterRoutes(v1, authMiddleware.RequireAuth)
	reviewHandler.RegisterRoutes(v1, authMiddleware.RequireAuth, authMiddleware.RequireRole(platformauth.RoleTeacher, platformauth.RoleAdmin))
	gradingHandler.RegisterRoutes(v1, authMiddleware.RequireAuth, authMiddleware.RequireRole(platformauth.RoleTeacher, platformauth.RoleAdmin))
	voiceService.RegisterRoutes(v1)

	// In development, auto-sync OpenAPI spec and watch for Go source changes
	if cfg.Primary.Env != "production" {
		if err := openapi.SyncOpenAPISpec(router.Routes(), "static/openapi.json"); err != nil {
			log.Warn().Err(err).Msg("failed to sync openapi spec on startup")
		} else {
			log.Info().Msg("synchronized OpenAPI documentation from Go codebase")
		}

		openapiWatcher := openapi.NewWatcher(
			func() []*echo.Route { return router.Routes() },
			"static/openapi.json",
			[]string{"internal", "cmd"},
			&log,
		)
		openapiWatcher.Start()
		defer openapiWatcher.Stop()
	}

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

	// http.Server.Shutdown does not touch hijacked connections, so WebSockets
	// are closed first, each with a close frame and a bounded wait.
	voiceCtx, voiceCancel := context.WithTimeout(shutdownCtx, cfg.Voice.ShutdownGrace)
	if err := voiceService.Shutdown(voiceCtx); err != nil {
		log.Warn().Err(err).Msg("voice connections did not all close gracefully")
	}
	voiceCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal().Err(err).Msg("server forced to shutdown")
	}

	// Let a cleanup run in progress stop between steps rather than mid-delete.
	stopJanitor()
	select {
	case <-janitorDone:
	case <-shutdownCtx.Done():
		log.Warn().Msg("recording cleanup did not stop in time")
	}

	log.Info().Msg("server exited cleanly")
}

// openAudioStore returns the store holding retained answer audio, or nil when
// there is none to use.
//
// With recording enabled the store must work, and the caller treats nil as
// fatal. With recording off, a store is opened only if one was set up
// (an existing audio directory, or a configured bucket), so audio kept earlier
// stays playable; a problem is then logged and playback is simply unavailable.
func openAudioStore(ctx context.Context, cfg config.RecordingConfig, log *zerolog.Logger) audiostore.Store {
	switch cfg.Provider {
	case config.RecordingProviderS3:
		if !cfg.Enabled && !cfg.S3.Configured() {
			return nil
		}
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		store, err := audiostore.NewS3(ctx, audiostore.S3Config{
			Endpoint: cfg.S3.Endpoint, Bucket: cfg.S3.Bucket,
			AccessKey: cfg.S3.AccessKey, SecretKey: cfg.S3.SecretKey,
			Region: cfg.S3.Region, UseSSL: cfg.S3.UseSSL, CreateBucket: cfg.S3.CreateBucket,
		})
		if err != nil {
			log.Error().Err(err).Str("endpoint", cfg.S3.Endpoint).Str("bucket", cfg.S3.Bucket).
				Msg("audio store (s3) unavailable")
			return nil
		}
		return store

	default: // localfs
		if !cfg.Enabled {
			if info, err := os.Stat(cfg.Dir); err != nil || !info.IsDir() {
				return nil
			}
		}
		store, err := audiostore.NewLocalFS(cfg.Dir)
		if err != nil {
			log.Error().Err(err).Msg("audio store (localfs) unavailable")
			return nil
		}
		return store
	}
}
