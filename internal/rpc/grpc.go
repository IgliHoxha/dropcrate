package rpc

import (
	"log/slog"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/IgliHoxha/dropcrate/internal/auth"
	pb "github.com/IgliHoxha/dropcrate/internal/rpc/dropcratev1"
	"github.com/IgliHoxha/dropcrate/internal/service"
)

// NewGRPC builds a *grpc.Server with the FileService, the standard gRPC health
// service, and server reflection registered. Reflection lets tools such as
// grpcurl explore the API without a compiled proto on hand; the health service
// backs load-balancer and orchestrator probes. When authn is enabled, Upload
// and Delete require a valid bearer key.
func NewGRPC(svc *service.Service, authn *auth.Authenticator, log *slog.Logger) *grpc.Server {
	// Metrics wrap auth so failed-auth calls are still counted. The OTel stats
	// handler emits spans (a no-op until tracing is configured).
	g := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(unaryMetricsInterceptor(), unaryAuthInterceptor(authn)),
		grpc.ChainStreamInterceptor(streamMetricsInterceptor(), streamAuthInterceptor(authn)),
	)
	pb.RegisterFileServiceServer(g, NewServer(svc, log))

	hs := health.NewServer()
	healthpb.RegisterHealthServer(g, hs)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus(pb.FileService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)

	reflection.Register(g)
	return g
}
