package rpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/IgliHoxha/dropcrate/internal/auth"
)

// protectedMethods require authentication when it is enabled. Reads (Download,
// GetMetadata) and the health/reflection services are intentionally left open,
// mirroring the HTTP transport where shareable download links stay usable.
var protectedMethods = map[string]bool{
	"/dropcrate.v1.FileService/Upload": true,
	"/dropcrate.v1.FileService/Delete": true,
}

// authorize returns an Unauthenticated error when a protected method is called
// without a valid bearer key. It is a no-op when auth is disabled.
func authorize(ctx context.Context, a *auth.Authenticator, fullMethod string) error {
	if a == nil || !a.Enabled() || !protectedMethods[fullMethod] {
		return nil
	}
	if !a.Valid(bearerFromContext(ctx)) {
		return status.Error(codes.Unauthenticated, "missing or invalid API key")
	}
	return nil
}

// bearerFromContext reads the bearer token from the request's authorization
// metadata, returning "" when absent or malformed.
func bearerFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return ""
	}
	return auth.BearerToken(vals[0])
}

func unaryAuthInterceptor(a *auth.Authenticator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := authorize(ctx, a, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func streamAuthInterceptor(a *auth.Authenticator) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := authorize(ss.Context(), a, info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}
