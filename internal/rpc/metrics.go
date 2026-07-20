package rpc

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/IgliHoxha/dropcrate/internal/metrics"
)

func unaryMetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		metrics.ObserveGRPC(info.FullMethod, status.Code(err).String(), time.Since(start).Seconds())
		return resp, err
	}
}

func streamMetricsInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		metrics.ObserveGRPC(info.FullMethod, status.Code(err).String(), time.Since(start).Seconds())
		return err
	}
}
