package openapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
)

// Watcher monitors Go files in the codebase and regenerates openapi.json on changes.
type Watcher struct {
	routesProvider func() []*echo.Route
	openapiPath    string
	watchDirs      []string
	logger         *zerolog.Logger
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// NewWatcher creates a new OpenAPI watcher.
func NewWatcher(routesProvider func() []*echo.Route, openapiPath string, watchDirs []string, logger *zerolog.Logger) *Watcher {
	return &Watcher{
		routesProvider: routesProvider,
		openapiPath:    openapiPath,
		watchDirs:      watchDirs,
		logger:         logger,
	}
}

// Start begins background monitoring of Go files.
func (w *Watcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.wg.Add(1)

	go func() {
		defer w.wg.Done()

		// Get initial highest mod time
		lastMod := w.getLatestGoModTime()

		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentMod := w.getLatestGoModTime()
				if currentMod.After(lastMod) {
					lastMod = currentMod

					// Debounce to allow multiple file saves to settle
					time.Sleep(300 * time.Millisecond)

					routes := w.routesProvider()
					if err := SyncOpenAPISpec(routes, w.openapiPath); err != nil {
						if w.logger != nil {
							w.logger.Error().Err(err).Msg("[openapi] failed to auto-update openapi.json")
						}
					} else {
						if w.logger != nil {
							w.logger.Info().
								Str("component", "openapi").
								Msg("[openapi] documentation updated automatically from Go code changes")
						}
					}
				}
			}
		}
	}()
}

// Stop stops the watcher gracefully.
func (w *Watcher) Stop() {
	if w.cancel != nil {
		w.cancel()
		w.wg.Wait()
	}
}

func (w *Watcher) getLatestGoModTime() time.Time {
	var latest time.Time

	for _, dir := range w.watchDirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(path, ".go") {
				if info.ModTime().After(latest) {
					latest = info.ModTime()
				}
			}
			return nil
		})
	}

	return latest
}
