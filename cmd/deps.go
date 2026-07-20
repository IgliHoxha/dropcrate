package cmd

import (
	"context"
	"database/sql"
	"log/slog"
	"os"

	"github.com/IgliHoxha/dropcrate/internal/auth"
	"github.com/IgliHoxha/dropcrate/internal/cache"
	"github.com/IgliHoxha/dropcrate/internal/config"
	"github.com/IgliHoxha/dropcrate/internal/database"
	"github.com/IgliHoxha/dropcrate/internal/events"
	"github.com/IgliHoxha/dropcrate/internal/files"
	"github.com/IgliHoxha/dropcrate/internal/service"
	"github.com/IgliHoxha/dropcrate/internal/storage"
)

// deps bundles the constructed application dependencies shared by commands.
type deps struct {
	cfg   config.Config
	db    *sql.DB
	cache *cache.MetadataCache
	store *storage.S3Storage
	svc   *service.Service
	auth  *auth.Authenticator
	log   *slog.Logger
}

// buildDeps loads config and constructs the database, object store, cache, and
// service. The returned cleanup closes the resources and must always be called.
func buildDeps(ctx context.Context) (*deps, func(), error) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}

	db, err := database.Open(ctx, cfg.MySQLDSN)
	if err != nil {
		return nil, nil, err
	}

	store, err := storage.NewS3(ctx, cfg.S3)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	metaCache := cache.New(cfg.Redis, cfg.DefaultTTL)
	if err := metaCache.Ping(ctx); err != nil {
		_ = metaCache.Close()
		_ = db.Close()
		return nil, nil, err
	}

	// Domain-event publisher: real Kafka producer when brokers are configured,
	// otherwise a no-op so the rest of the app is unaffected.
	var publisher events.Publisher = events.Nop{}
	if cfg.Kafka.Enabled() {
		publisher = events.NewKafka(cfg.Kafka.Brokers, cfg.Kafka.TopicPrefix, log)
		log.Info("kafka events enabled", "brokers", cfg.Kafka.Brokers, "topic_prefix", cfg.Kafka.TopicPrefix)
	}

	authn := auth.New(cfg.APIKeys)
	if authn.Enabled() {
		log.Info("api-key authentication enabled", "keys", len(cfg.APIKeys))
	}

	repo := files.NewRepository(db)
	svc := service.New(repo, store, metaCache, publisher, cfg.DefaultTTL, cfg.MaxUploadBytes)

	cleanup := func() {
		_ = publisher.Close()
		_ = metaCache.Close()
		_ = db.Close()
	}

	return &deps{cfg: cfg, db: db, cache: metaCache, store: store, svc: svc, auth: authn, log: log}, cleanup, nil
}
