package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/mcpserver"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/replicate"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/cloud"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// runShared serves the store of several machines on --bind: no spool to
// drain, records from the replicate loops of local serves at /api/sync,
// the MCP tools at /mcp, and a key on every request that reads a record.
// It holds no checkout, so no query reads its disk.
func runShared(ctx context.Context, table trace.Prices) error {
	keys, err := readKeys(flagKeysFile)
	if err != nil {
		return err
	}
	st, where, err := openSharedStore()
	if err != nil {
		return err
	}
	defer closeStore(st)

	q := &query.Service{Store: st, Prices: table, NoCheckouts: true}
	srv := server.New(st, q, buildinfo.Get())
	tools, err := sharedMCP(srv, st)
	if err != nil {
		return err
	}
	log.Printf("moondb: shared instance serving %s on %s", where, flagBind)
	return serveOn(ctx, flagBind, srv.SharedHandler(server.SharedConfig{Keys: keys, MCP: tools}))
}

// sharedDataDir is where a shared instance keeps its log: --data-dir, else
// the workdir.
func sharedDataDir() string {
	if flagDataDir != "" {
		return flagDataDir
	}
	return flagWorkdir
}

// openSharedStore opens the backend --backend names and describes it for
// the log. The bucket's token source lives as long as the process, so the
// copy of the log on Close still has credentials after a signal.
func openSharedStore() (store.Store, string, error) {
	dir := sharedDataDir()
	if flagBackend == "local" {
		st, err := local.Open(dir)
		if err != nil {
			return nil, "", err
		}
		return st, "the local store " + dir, nil
	}
	b, err := cloud.NewBucket(context.Background(), flagGCSBucket)
	if err != nil {
		return nil, "", err
	}
	st, err := cloud.Open(cloud.Config{Dir: dir, Bucket: b})
	if err != nil {
		return nil, "", err
	}
	return st, fmt.Sprintf("the cloud store %s with bucket %s", dir, b.Name()), nil
}

// sharedMCP builds the MCP tools a shared instance mounts at /mcp. They
// call the instance's own API in process, after /mcp has checked the
// caller's key. The transport is stateless, so no session lives in the
// process, and moondb_doctor checks the one thing that can be down there:
// the store.
func sharedMCP(srv *server.Server, st store.Store) (http.Handler, error) {
	tools, err := mcpserver.New(buildinfo.Get().Version, mcpserver.Config{
		Port:    flagPort,
		BaseURL: flagBaseURL,
		API:     srv.APIHandler(),
		Checks: func(context.Context) []doctor.Check {
			return []doctor.Check{{Name: "store-reachable", Run: st.Ping}}
		},
	})
	if err != nil {
		return nil, err
	}
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return tools },
		&mcp.StreamableHTTPOptions{Stateless: true},
	), nil
}

// readKeys reads the keys file a shared instance accepts keys from.
func readKeys(path string) (server.Keys, error) {
	f, err := os.Open(path)
	if err != nil {
		return server.Keys{}, fmt.Errorf("keys file: %w", err)
	}
	defer f.Close()
	keys, err := server.ParseKeys(f)
	if err != nil {
		return server.Keys{}, fmt.Errorf("keys file %s: %w", path, err)
	}
	return keys, nil
}

// startReplicate starts copying st to the instance --replicate-to names,
// for as long as ctx lives. Without --replicate-to it does nothing.
func startReplicate(ctx context.Context, st store.Store) error {
	if flagReplicateTo == "" {
		return nil
	}
	key, err := readKeyFile(flagReplicateKeyFile)
	if err != nil {
		return err
	}
	r, err := replicate.New(replicate.Config{
		Dir:    flagWorkdir,
		Chunks: st.Chunks(),
		Target: flagReplicateTo,
		Key:    key,
		State:  filepath.Join(flagWorkdir, moondb.ReplicateCursorFile),
	})
	if err != nil {
		return err
	}
	log.Printf("moondb: replicating to %s", flagReplicateTo)
	go r.Run(ctx)
	return nil
}

// readKeyFile reads a key from its file, surrounding whitespace dropped.
func readKeyFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("replicate key file: %w", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		return "", errors.New("replicate key file " + path + " is empty")
	}
	return key, nil
}
