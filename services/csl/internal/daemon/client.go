package daemon

import (
	"context"
	"errors"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/services/csl"
	pb "github.com/mad01/thismoon/services/csl/internal/daemon/proto"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// ErrDaemonNotRunning indicates the search daemon is unreachable.
var ErrDaemonNotRunning = errors.New("daemon: search daemon is not running")

// notRunning is the one wrap point every failed daemon RPC goes through: the
// sentinel plus the agentdoc hint pointing at 'csl doctor'. errors.Is against
// ErrDaemonNotRunning still matches, so fallback decisions are unaffected.
func notRunning() error {
	return agentdoc.Hint(ErrDaemonNotRunning, csl.Facts())
}

// isConnectionError returns true if the error indicates the daemon is
// unreachable (as opposed to a real application-level error).
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return true
	}
	switch st.Code() {
	case codes.Unavailable, codes.Unimplemented:
		return true
	default:
		return false
	}
}

// dial connects to the daemon's Unix socket with a timeout.
func dial(socketPath string) (pb.SearchDaemonClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, err
	}
	return pb.NewSearchDaemonClient(conn), conn, nil
}

// SearchVia sends a search request to the daemon.
func SearchVia(
	ctx context.Context,
	socketPath string,
	opts search.SearchOptions,
	repoNames map[string]string,
) ([]search.Match, error) {
	client, conn, err := dial(socketPath)
	if err != nil {
		return nil, notRunning()
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.Search(ctx, &pb.SearchRequest{
		Pattern:       opts.Pattern,
		RepoFilter:    opts.RepoFilter,
		FileFilter:    opts.FileFilter,
		Lang:          opts.Lang,
		CaseSensitive: opts.CaseSensitive,
		Limit:         int32(opts.Limit),
		ContextLines:  int32(opts.ContextLines),
		OutputMode:    opts.OutputMode,
		RepoNames:     repoNames,
		Offset:        int32(opts.Offset),
	})
	if err != nil {
		if isConnectionError(err) {
			return nil, notRunning()
		}
		return nil, err
	}

	matches := make([]search.Match, len(resp.Matches))
	for i, m := range resp.Matches {
		matches[i] = search.Match{
			Repo:     m.Repo,
			RepoPath: m.RepoPath,
			File:     m.File,
			Line:     int(m.Line),
			Column:   int(m.Column),
			Text:     m.Text,
			Before:   m.Before,
			After:    m.After,
		}
	}
	return matches, nil
}

// CountVia sends a count request to the daemon.
func CountVia(
	ctx context.Context,
	socketPath string,
	opts search.CountOptions,
) ([]search.CountResult, int, error) {
	client, conn, err := dial(socketPath)
	if err != nil {
		return nil, 0, notRunning()
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.Count(ctx, &pb.CountRequest{
		Pattern:    opts.Pattern,
		RepoFilter: opts.RepoFilter,
		Lang:       opts.Lang,
		GroupBy:    opts.GroupBy,
	})
	if err != nil {
		if isConnectionError(err) {
			return nil, 0, notRunning()
		}
		return nil, 0, err
	}

	results := make([]search.CountResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = search.CountResult{
			Group: r.Group,
			Count: int(r.Count),
		}
	}
	return results, int(resp.Total), nil
}

// ValidateVia sends a validate request to the daemon.
func ValidateVia(socketPath string, pattern string) (search.QueryInfo, error) {
	client, conn, err := dial(socketPath)
	if err != nil {
		return search.QueryInfo{}, notRunning()
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Validate(ctx, &pb.ValidateRequest{
		Pattern: pattern,
	})
	if err != nil {
		if isConnectionError(err) {
			return search.QueryInfo{}, notRunning()
		}
		return search.QueryInfo{}, err
	}

	return search.QueryInfo{
		Valid:  resp.Valid,
		Parsed: resp.Parsed,
		Error:  resp.Error,
		Hint:   resp.Hint,
	}, nil
}

// SemanticSearchVia sends a semantic search request to the daemon. The response
// carries Available=false (not an error) when the daemon has no semantic index
// or embedding model loaded, so callers can decide what to surface.
func SemanticSearchVia(
	socketPath string,
	req *pb.SemanticSearchRequest,
) (*pb.SemanticSearchResponse, error) {
	client, conn, err := dial(socketPath)
	if err != nil {
		return nil, notRunning()
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.SemanticSearch(ctx, req)
	if err != nil {
		if isConnectionError(err) {
			return nil, notRunning()
		}
		return nil, err
	}
	return resp, nil
}

// Ping checks if the daemon is alive.
func Ping(socketPath string) error {
	client, conn, err := dial(socketPath)
	if err != nil {
		return notRunning()
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Ping(ctx, &pb.PingRequest{})
	if err != nil {
		if isConnectionError(err) {
			return notRunning()
		}
		return err
	}
	return nil
}

// Shutdown sends a shutdown request to the daemon.
func Shutdown(socketPath string) error {
	client, conn, err := dial(socketPath)
	if err != nil {
		return notRunning()
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Shutdown(ctx, &pb.ShutdownRequest{})
	if err != nil {
		return notRunning()
	}
	return nil
}

// EnsureDaemon ensures the daemon is running, starting it if needed.
// Returns nil if the daemon is ready, ErrDaemonNotRunning if it cannot be started.
func EnsureDaemon(indexDir, socketPath string) error {
	if err := Ping(socketPath); err == nil {
		return nil
	}

	if err := StartBackground(); err != nil {
		return notRunning()
	}

	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if err := Ping(socketPath); err == nil {
			return nil
		}
	}
	return notRunning()
}
