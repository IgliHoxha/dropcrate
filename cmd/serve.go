package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"

	"github.com/IgliHoxha/dropcrate/internal/api"
	"github.com/IgliHoxha/dropcrate/internal/rpc"
	"github.com/IgliHoxha/dropcrate/internal/sweeper"
	"github.com/IgliHoxha/dropcrate/internal/tracing"
	"github.com/IgliHoxha/dropcrate/internal/urlsign"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP and gRPC API servers",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runServe(cmd.Context())
	},
}

func runServe(ctx context.Context) error {
	d, cleanup, err := buildDeps(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	// Optional distributed tracing; a no-op unless an OTLP endpoint is set.
	shutdownTracing, err := tracing.Setup(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	// Readiness probe: every backing store must answer for the service to be
	// ready to receive traffic.
	ready := func(ctx context.Context) error {
		if err := d.db.PingContext(ctx); err != nil {
			return fmt.Errorf("mysql: %w", err)
		}
		if err := d.cache.Ping(ctx); err != nil {
			return fmt.Errorf("redis: %w", err)
		}
		if err := d.store.Ping(ctx); err != nil {
			return fmt.Errorf("s3: %w", err)
		}
		return nil
	}

	handler := api.New(d.svc, d.log, api.Options{
		BaseURL:        d.cfg.PublicBaseURL,
		MaxUploadBytes: d.cfg.MaxUploadBytes,
		Ready:          ready,
		Auth:           d.auth,
		Signer:         urlsign.New(d.cfg.DownloadSigningKey, d.cfg.DownloadURLTTL),
	}).Router()

	srv := &http.Server{
		Addr:              d.cfg.HTTPAddr,
		Handler:           otelhttp.NewHandler(handler, "http.server"),
		ReadHeaderTimeout: 10 * time.Second,
	}

	grpcSrv := rpc.NewGRPC(d.svc, d.auth, d.log)
	grpcLis, err := net.Listen("tcp", d.cfg.GRPCAddr)
	if err != nil {
		return err
	}

	// Stop serving and reaping when an OS signal arrives, then drain.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Background reaper for expired files.
	go sweeper.New(d.svc, d.cfg.SweepInterval, d.cfg.SweepBatch, d.log).Run(ctx)

	// One error channel for both servers; the first fatal error wins.
	errCh := make(chan error, 2)
	go func() {
		d.log.Info("http listening", "addr", d.cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	go func() {
		d.log.Info("grpc listening", "addr", d.cfg.GRPCAddr)
		if err := grpcSrv.Serve(grpcLis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		d.log.Info("shutting down")
		grpcSrv.GracefulStop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
