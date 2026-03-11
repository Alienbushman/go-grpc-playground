package server

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryLoggingInterceptor logs the outcome of every unary RPC.
//
// Log levels:
//   - Debug  — success (silent at the default Info level, opt-in via LOG_LEVEL=debug)
//   - Warn   — client errors: InvalidArgument, NotFound, AlreadyExists, etc.
//   - Error  — server errors: Internal, Unavailable, etc.
//
// Each log line includes: method, gRPC status code, and duration in ms.
// Error and Warn lines also include the status message.
func UnaryLoggingInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)

	code := status.Code(err)
	args := []any{
		"method", info.FullMethod,
		"code", code.String(),
		"duration_ms", time.Since(start).Milliseconds(),
	}

	switch {
	case err == nil:
		slog.DebugContext(ctx, "rpc ok", args...)
	case isClientError(code):
		slog.WarnContext(ctx, "rpc client error", append(args, "msg", status.Convert(err).Message())...)
	default:
		slog.ErrorContext(ctx, "rpc server error", append(args, "error", err)...)
	}

	return resp, err
}

// isClientError returns true for gRPC codes that represent caller mistakes
// rather than server-side failures.
func isClientError(c codes.Code) bool {
	switch c {
	case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
		codes.PermissionDenied, codes.Unauthenticated, codes.ResourceExhausted,
		codes.FailedPrecondition, codes.OutOfRange:
		return true
	}
	return false
}
