package daemon

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mad01/thismoon/services/csl/internal/search"
)

func TestIsConnectionError_Unavailable(t *testing.T) {
	err := status.Error(codes.Unavailable, "connection refused")
	if !isConnectionError(err) {
		t.Error("Unavailable should be a connection error")
	}
}

func TestIsConnectionError_Unimplemented(t *testing.T) {
	err := status.Error(codes.Unimplemented, "method not found")
	if !isConnectionError(err) {
		t.Error("Unimplemented should be a connection error")
	}
}

func TestIsConnectionError_Nil(t *testing.T) {
	if isConnectionError(nil) {
		t.Error("nil should not be a connection error")
	}
}

func TestIsConnectionError_NonGRPC(t *testing.T) {
	err := errors.New("some random error")
	if !isConnectionError(err) {
		t.Error("non-gRPC error should be treated as connection error (conservative)")
	}
}

func TestIsConnectionError_InvalidArgument(t *testing.T) {
	err := status.Error(codes.InvalidArgument, "bad query")
	if isConnectionError(err) {
		t.Error("InvalidArgument is a real error, not a connection error")
	}
}

func TestIsConnectionError_Internal(t *testing.T) {
	err := status.Error(codes.Internal, "server panic")
	if isConnectionError(err) {
		t.Error("Internal is a real error, not a connection error")
	}
}

func TestIsConnectionError_DeadlineExceeded(t *testing.T) {
	err := status.Error(codes.DeadlineExceeded, "timeout")
	if isConnectionError(err) {
		t.Error("DeadlineExceeded is a real error, not a connection error")
	}
}

func TestIsConnectionError_PermissionDenied(t *testing.T) {
	err := status.Error(codes.PermissionDenied, "unauthorized")
	if isConnectionError(err) {
		t.Error("PermissionDenied is a real error, not a connection error")
	}
}

func TestSearchVia_NotRunning(t *testing.T) {
	sockPath := "/tmp/nonexistent-test-socket-" + t.Name() + ".sock"
	_, err := SearchVia(context.Background(), sockPath, search.SearchOptions{Pattern: "x"}, nil)
	if !errors.Is(err, ErrDaemonNotRunning) {
		t.Errorf("expected ErrDaemonNotRunning for bad socket, got: %v", err)
	}
}

func TestCountVia_NotRunning(t *testing.T) {
	sockPath := "/tmp/nonexistent-test-socket-" + t.Name() + ".sock"
	_, _, err := CountVia(context.Background(), sockPath, search.CountOptions{Pattern: "x"})
	if !errors.Is(err, ErrDaemonNotRunning) {
		t.Errorf("expected ErrDaemonNotRunning for bad socket, got: %v", err)
	}
}

func TestValidateVia_NotRunning(t *testing.T) {
	sockPath := "/tmp/nonexistent-test-socket-" + t.Name() + ".sock"
	_, err := ValidateVia(sockPath, "hello")
	if !errors.Is(err, ErrDaemonNotRunning) {
		t.Errorf("expected ErrDaemonNotRunning for bad socket, got: %v", err)
	}
}
