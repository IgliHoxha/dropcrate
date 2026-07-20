// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings for the service.
type Config struct {
	HTTPAddr       string
	GRPCAddr       string
	PublicBaseURL  string
	MySQLDSN       string
	MaxUploadBytes int64
	DefaultTTL     time.Duration
	SweepInterval  time.Duration
	SweepBatch     int
	APIKeys        []string
	// DownloadSigningKey, when set, makes download links HMAC-signed and
	// expiring; empty leaves downloads open by id.
	DownloadSigningKey string
	DownloadURLTTL     time.Duration
	Redis              RedisConfig
	S3                 S3Config
	Kafka              KafkaConfig
}

// KafkaConfig configures the optional domain-event publisher. Event publishing
// is disabled unless Brokers is non-empty.
type KafkaConfig struct {
	Brokers     []string
	TopicPrefix string
}

// Enabled reports whether a broker is configured.
func (k KafkaConfig) Enabled() bool { return len(k.Brokers) > 0 }

// RedisConfig configures the metadata cache.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// S3Config configures the S3-compatible object store (MinIO, AWS S3, ...).
type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

// Load reads configuration from environment variables, applying sane
// local-development defaults so the service boots against docker-compose
// without any extra setup.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:       getenv("HTTP_ADDR", ":8080"),
		GRPCAddr:       getenv("GRPC_ADDR", ":9090"),
		PublicBaseURL:  getenv("PUBLIC_BASE_URL", "http://localhost:8080"),
		MySQLDSN:       getenv("MYSQL_DSN", "app:secret@tcp(127.0.0.1:3306)/app?parseTime=true&multiStatements=true"),
		MaxUploadBytes: getenvInt64("MAX_UPLOAD_BYTES", 100<<20), // 100 MiB
		SweepBatch:     int(getenvInt64("SWEEP_BATCH", 100)),
		APIKeys:        getenvList("API_KEYS", nil),
		Redis: RedisConfig{
			Addr:     getenv("REDIS_ADDR", "127.0.0.1:6379"),
			Password: getenv("REDIS_PASSWORD", ""),
			DB:       int(getenvInt64("REDIS_DB", 0)),
		},
		S3: S3Config{
			Endpoint:  getenv("S3_ENDPOINT", "127.0.0.1:9000"),
			AccessKey: getenv("S3_ACCESS_KEY", "guest"),
			SecretKey: getenv("S3_SECRET_KEY", "supersecret"),
			Bucket:    getenv("S3_BUCKET", "dropcrate"),
			Region:    getenv("S3_REGION", "us-east-1"),
			UseSSL:    getenvBool("S3_USE_SSL", false),
		},
		Kafka: KafkaConfig{
			Brokers:     getenvList("KAFKA_BROKERS", nil),
			TopicPrefix: getenv("KAFKA_TOPIC_PREFIX", "dropcrate."),
		},
	}

	ttl, err := time.ParseDuration(getenv("DEFAULT_TTL", "168h")) // 7 days
	if err != nil {
		return Config{}, fmt.Errorf("invalid DEFAULT_TTL: %w", err)
	}
	cfg.DefaultTTL = ttl

	sweep, err := time.ParseDuration(getenv("SWEEP_INTERVAL", "15m"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid SWEEP_INTERVAL: %w", err)
	}
	cfg.SweepInterval = sweep

	cfg.DownloadSigningKey = getenv("DOWNLOAD_SIGNING_KEY", "")
	urlTTL, err := time.ParseDuration(getenv("DOWNLOAD_URL_TTL", "1h"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid DOWNLOAD_URL_TTL: %w", err)
	}
	cfg.DownloadURLTTL = urlTTL

	return cfg, nil
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getenvInt64(key string, def int64) int64 {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func getenvBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// getenvList reads a comma-separated env var into a slice, trimming whitespace
// and dropping empty entries. An unset or empty var yields def.
func getenvList(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
