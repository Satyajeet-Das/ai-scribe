package job

import (
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/config"
)

type JobService struct {
	Client *asynq.Client
	server *asynq.Server
	logger *zerolog.Logger

	// docServer runs exam-document extraction. It is a server of its own, with
	// its own small worker pool, for two reasons: reading a document is slow and
	// heavy, so how many run at once on this machine is set exactly (not shared
	// with the ten workers that expire sessions and send email); and a backlog of
	// documents can never delay a session expiring on time. Nil unless document
	// ingestion is enabled.
	docServer *asynq.Server
	docJob    config.DocumentsJobConfig
	documents DocumentProcessor
}

func NewJobService(logger *zerolog.Logger, cfg *config.Config) *JobService {
	redisAddr := cfg.Redis.Address

	client := asynq.NewClient(asynq.RedisClientOpt{
		Addr: redisAddr,
	})

	server := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
		},
	)

	j := &JobService{
		Client: client,
		server: server,
		logger: logger,
	}

	if cfg.Documents.Enabled {
		j.docJob = cfg.Documents.Job
		j.docServer = asynq.NewServer(
			asynq.RedisClientOpt{Addr: redisAddr},
			asynq.Config{
				Concurrency:    cfg.Documents.Job.MaxConcurrent,
				Queues:         map[string]int{QueueDocuments: 1},
				RetryDelayFunc: documentRetryDelay,
				// Let a running extraction finish its current model call (or stop
				// cleanly) on shutdown; the document is then picked up again.
				ShutdownTimeout: 20 * time.Second,
			},
		)
	}
	return j
}

func (j *JobService) Start() error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskWelcome, j.handleWelcomeEmailTask)
	mux.HandleFunc(TaskExpireSession, j.handleExpireSessionTask)

	j.logger.Info().Msg("starting background job worker server")
	if err := j.server.Start(mux); err != nil {
		return err
	}

	if j.docServer != nil {
		docMux := asynq.NewServeMux()
		docMux.HandleFunc(TaskExtractDocument, j.handleExtractDocumentTask)
		j.logger.Info().Int("concurrency", j.docJob.MaxConcurrent).Msg("starting document extraction worker server")
		if err := j.docServer.Start(docMux); err != nil {
			return err
		}
	}

	return nil
}

func (j *JobService) Stop() {
	j.logger.Info().Msg("stopping background job server")
	j.server.Shutdown()
	if j.docServer != nil {
		j.docServer.Shutdown()
	}
	j.Client.Close()
}
