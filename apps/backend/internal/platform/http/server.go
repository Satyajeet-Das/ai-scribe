package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
)

type Server struct {
	httpServer *http.Server
	logger     *zerolog.Logger
	cfg        *config.ServerConfig
}

func NewServer(cfg *config.ServerConfig, handler http.Handler, logger *zerolog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         ":" + cfg.Port,
			Handler:      handler,
			ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
			IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
		},
		logger: logger,
		cfg:    cfg,
	}
}

func (s *Server) Start() error {
	s.logger.Info().
		Str("port", s.cfg.Port).
		Msg("starting HTTP server")

	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server listen error: %w", err)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info().Msg("shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}
