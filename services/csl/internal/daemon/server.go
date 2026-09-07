package daemon

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sourcegraph/zoekt"
	zoektsearch "github.com/sourcegraph/zoekt/search"
	"google.golang.org/grpc"
	"gopkg.in/natefinch/lumberjack.v2"

	pb "github.com/mad01/thismoon/services/csl/internal/daemon/proto"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

type searchServer struct {
	pb.UnimplementedSearchDaemonServer
	searcher zoekt.Searcher
	indexDir string
	semIndex *semantic.Index   // nil when no semantic index is loaded
	embedder semantic.Embedder // nil when the embedding model is unavailable
	cancel   context.CancelFunc
	idle     *time.Timer
	idleMu   sync.Mutex
	idleDur  time.Duration
}

func (s *searchServer) resetIdle() {
	s.idleMu.Lock()
	defer s.idleMu.Unlock()
	if s.idle != nil {
		s.idle.Reset(s.idleDur)
	}
}

func (s *searchServer) Search(
	ctx context.Context,
	req *pb.SearchRequest,
) (*pb.SearchResponse, error) {
	s.resetIdle()
	start := time.Now()

	opts := search.SearchOptions{
		Pattern:       req.GetPattern(),
		RepoFilter:    req.GetRepoFilter(),
		FileFilter:    req.GetFileFilter(),
		Lang:          req.GetLang(),
		CaseSensitive: req.GetCaseSensitive(),
		Limit:         int(req.GetLimit()),
		ContextLines:  int(req.GetContextLines()),
		OutputMode:    req.GetOutputMode(),
		Offset:        int(req.GetOffset()),
	}

	matches, err := search.SearchWith(ctx, s.searcher, opts, req.GetRepoNames())
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	log.Printf("rpc search: pattern=%q matches=%d took=%s",
		req.GetPattern(), len(matches), time.Since(start).Round(time.Millisecond))

	resp := &pb.SearchResponse{
		Matches: make([]*pb.Match, len(matches)),
	}
	for i, m := range matches {
		resp.Matches[i] = &pb.Match{
			Repo:     m.Repo,
			RepoPath: m.RepoPath,
			File:     m.File,
			Line:     int32(m.Line),
			Column:   int32(m.Column),
			Text:     m.Text,
			Before:   m.Before,
			After:    m.After,
		}
	}
	return resp, nil
}

func (s *searchServer) Count(ctx context.Context, req *pb.CountRequest) (*pb.CountResponse, error) {
	s.resetIdle()
	start := time.Now()

	opts := search.CountOptions{
		Pattern:    req.GetPattern(),
		RepoFilter: req.GetRepoFilter(),
		Lang:       req.GetLang(),
		GroupBy:    req.GetGroupBy(),
	}

	results, total, err := search.CountWith(ctx, s.searcher, opts)
	if err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}
	log.Printf("rpc count: pattern=%q total=%d took=%s",
		req.GetPattern(), total, time.Since(start).Round(time.Millisecond))

	resp := &pb.CountResponse{
		Total:   int32(total),
		Results: make([]*pb.CountResult, len(results)),
	}
	for i, r := range results {
		resp.Results[i] = &pb.CountResult{
			Group: r.Group,
			Count: int32(r.Count),
		}
	}
	return resp, nil
}

func (s *searchServer) Validate(
	_ context.Context,
	req *pb.ValidateRequest,
) (*pb.ValidateResponse, error) {
	s.resetIdle()

	info := search.ValidateQuery(req.GetPattern())
	return &pb.ValidateResponse{
		Valid:  info.Valid,
		Parsed: info.Parsed,
		Error:  info.Error,
		Hint:   info.Hint,
	}, nil
}

func (s *searchServer) Ping(_ context.Context, _ *pb.PingRequest) (*pb.PingResponse, error) {
	s.resetIdle()
	return &pb.PingResponse{}, nil
}

func (s *searchServer) Shutdown(
	_ context.Context,
	_ *pb.ShutdownRequest,
) (*pb.ShutdownResponse, error) {
	s.resetIdle()
	s.cancel()
	return &pb.ShutdownResponse{}, nil
}

func (s *searchServer) SemanticSearch(
	ctx context.Context,
	req *pb.SemanticSearchRequest,
) (*pb.SemanticSearchResponse, error) {
	s.resetIdle()

	if s.semIndex == nil || s.embedder == nil {
		return &pb.SemanticSearchResponse{Available: false}, nil
	}

	start := time.Now()
	vec, err := s.embedder.EmbedQuery(ctx, req.GetQuery())
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	embedDur := time.Since(start)

	k := int(req.GetK())
	if k <= 0 {
		k = 10
	}

	hits := s.semIndex.Search(vec, k, semantic.Filter{
		Repos: req.GetRepos(),
		Langs: req.GetLangs(),
	})
	log.Printf("rpc semantic_search: query=%q hits=%d embed=%s took=%s",
		req.GetQuery(), len(hits),
		embedDur.Round(time.Millisecond), time.Since(start).Round(time.Millisecond))

	resp := &pb.SemanticSearchResponse{
		Available: true,
		Hits:      make([]*pb.SemanticHit, len(hits)),
	}
	for i, h := range hits {
		snippet, _ := semantic.ExpandHit(h, int(req.GetExpand())) // best-effort
		resp.Hits[i] = &pb.SemanticHit{
			Repo:      h.Repo,
			RepoPath:  h.RepoPath,
			File:      h.Path,
			Lang:      h.Lang,
			Kind:      h.Kind,
			StartLine: int32(h.StartLine),
			EndLine:   int32(h.EndLine),
			Score:     h.Score,
			Snippet:   snippet,
		}
	}
	return resp, nil
}

// Serve starts the gRPC daemon on a Unix socket. When semanticEnabled is true
// the daemon also loads the semantic index and embedding model at startup;
// otherwise it serves lexical search only.
func Serve(indexDir, socketPath string, idleTimeout time.Duration, semanticEnabled bool) error {
	// Set up rotating log writer so the daemon log does not grow unbounded.
	logWriter := &lumberjack.Logger{
		Filename:   DefaultLogPath(),
		MaxSize:    5, // MB
		MaxBackups: 1,
	}
	defer func() { _ = logWriter.Close() }()
	log.SetOutput(logWriter)

	searcher, err := zoektsearch.NewDirectorySearcher(indexDir)
	if err != nil {
		return fmt.Errorf("open index at %s: %w", indexDir, err)
	}

	pidPath := DefaultPIDPath()
	if err := RemoveStale(socketPath, pidPath); err != nil {
		searcher.Close()
		return fmt.Errorf("remove stale files: %w", err)
	}

	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		searcher.Close()
		return fmt.Errorf("listen on %s: %w", socketPath, err)
	}

	if err := WritePID(pidPath); err != nil {
		_ = lis.Close()
		searcher.Close()
		return fmt.Errorf("write pid: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	srv := &searchServer{
		searcher: searcher,
		indexDir: indexDir,
		cancel:   cancel,
		idleDur:  idleTimeout,
	}

	// Best-effort semantic load, only when opted in via config. The daemon must
	// keep serving lexical search even when the semantic index or embedding
	// model is unavailable.
	if semanticEnabled {
		loadSemantic(ctx, srv)
	}

	srv.idle = time.AfterFunc(idleTimeout, cancel)

	grpcServer := grpc.NewServer()
	pb.RegisterSearchDaemonServer(grpcServer, srv)

	// Handle SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	go func() { _ = grpcServer.Serve(lis) }()

	<-ctx.Done()

	grpcServer.GracefulStop()
	searcher.Close()
	if closer, ok := srv.embedder.(interface{ Close() error }); ok && closer != nil {
		_ = closer.Close()
	}

	// Cleanup socket and PID files.
	_ = os.Remove(socketPath)
	_ = os.Remove(pidPath)

	return nil
}

// loadSemantic best-effort loads the semantic index and embedding model onto
// srv. Any failure leaves the corresponding field nil and is logged — the
// daemon never fails over a missing or unbuilt semantic index.
func loadSemantic(ctx context.Context, srv *searchServer) {
	semDir, err := semantic.DefaultSemanticIndexDir()
	if err != nil {
		log.Printf("semantic: cannot resolve index dir: %v", err)
		return
	}
	idx, err := semantic.OpenIndex(semDir)
	if err != nil {
		log.Printf("semantic: open index %s failed: %v", semDir, err)
		return
	}
	if idx.Len() == 0 {
		log.Printf("semantic: no chunks indexed; semantic search disabled")
		return
	}
	srv.semIndex = idx

	emb := semantic.NewDefaultEmbedder()
	if err := emb.CheckModel(ctx); err != nil {
		log.Printf(
			"semantic: %d chunks across %d stores, embedder ready=false (%v)",
			idx.Len(),
			idx.Stores(),
			err,
		)
		return
	}
	srv.embedder = emb
	log.Printf(
		"semantic: %d chunks across %d stores, embedder ready=true (ollama model %s)",
		idx.Len(),
		idx.Stores(),
		emb.Model,
	)
}
