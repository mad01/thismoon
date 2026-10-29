package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// serveFlags is every serve flag checkServeFlags reads, so a case can set
// them all and put them back.
type serveFlags struct {
	shared                               bool
	bind, backend, dataDir, bucket, keys string
	replicateTo, replicateKey            string
}

func setServeFlags(t *testing.T, f serveFlags) {
	t.Helper()
	was := serveFlags{
		flagShared, flagBind, flagBackend, flagDataDir, flagGCSBucket,
		flagKeysFile, flagReplicateTo, flagReplicateKeyFile,
	}
	t.Cleanup(func() { applyServeFlags(was) })
	if f.backend == "" {
		f.backend = moondb.DefaultBackend
	}
	applyServeFlags(f)
}

func applyServeFlags(f serveFlags) {
	flagShared, flagBind, flagBackend, flagDataDir = f.shared, f.bind, f.backend, f.dataDir
	flagGCSBucket, flagKeysFile = f.bucket, f.keys
	flagReplicateTo, flagReplicateKeyFile = f.replicateTo, f.replicateKey
}

func TestCheckServeFlags(t *testing.T) {
	cases := []struct {
		name    string
		flags   serveFlags
		wantErr string
	}{
		{"plain local serve", serveFlags{}, ""},
		{"local replicating", serveFlags{replicateTo: "https://m", replicateKey: "/k"}, ""},
		{"replicating without a key", serveFlags{replicateTo: "https://m"}, "--replicate-key-file"},
		{"local with a bind", serveFlags{bind: "0.0.0.0:7433"}, "--bind is for --shared"},
		{"local on the cloud", serveFlags{backend: "cloud"}, "--backend cloud is for --shared"},
		{"shared without a bind", serveFlags{shared: true, keys: "/k"}, "explicit --bind"},
		{"shared without keys", serveFlags{shared: true, bind: ":7433"}, "--keys-file"},
		{
			"shared with a bare port",
			serveFlags{shared: true, bind: "7433", keys: "/k"},
			"host:port",
		},
		{"shared on local disk", serveFlags{shared: true, bind: ":7433", keys: "/k"}, ""},
		{"cloud without a bucket", serveFlags{
			shared: true, bind: ":7433", keys: "/k",
			backend: "cloud",
		}, "--gcs-bucket"},
		{"cloud with a bucket", serveFlags{
			shared: true, bind: ":7433", keys: "/k",
			backend: "cloud", bucket: "b",
		}, ""},
		{"an unknown backend", serveFlags{
			shared: true, bind: ":7433", keys: "/k",
			backend: "s3",
		}, "unknown --backend"},
		{"shared replicating", serveFlags{
			shared: true, bind: ":7433", keys: "/k",
			replicateTo: "https://m",
		}, "--replicate-to is for a local serve"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setServeFlags(t, c.flags)
			err := checkServeFlags()
			switch {
			case c.wantErr == "" && err != nil:
				t.Errorf("checkServeFlags = %v, want nil", err)
			case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
				t.Errorf("checkServeFlags = %v, want an error naming %q", err, c.wantErr)
			}
		})
	}
}

// bearer signs every request with a key, the way an MCP client registered
// against a shared instance does.
type bearer struct {
	key  string
	base http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+b.key)
	return b.base.RoundTrip(clone)
}

// TestSharedMCPOverHTTP builds a shared instance as runShared does and
// calls a tool at /mcp with a key: the tool reads the instance's own store
// in process. Without the key, /mcp refuses the connection.
func TestSharedMCPOverHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := local.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.PutSession(ctx, store.Session{
		ID: "sess-shared", Title: "from the laptop",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("k1"))
	keys, err := server.ParseKeys(strings.NewReader(hex.EncodeToString(sum[:])))
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(st, &query.Service{Store: st, NoCheckouts: true}, buildinfo.Info{})
	tools, err := sharedMCP(srv, st)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.SharedHandler(server.SharedConfig{Keys: keys, MCP: tools}))
	t.Cleanup(ts.Close)

	session, err := connect(ctx, ts.URL+"/mcp", "k1")
	if err != nil {
		t.Fatalf("connect with the key: %v", err)
	}
	defer func() { _ = session.Close() }()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "moondb_sessions", Arguments: map[string]any{"since": "24h"},
	})
	if err != nil || res.IsError {
		t.Fatalf("moondb_sessions = %s, %v", toolText(res), err)
	}
	if text := toolText(res); !strings.Contains(text, "sess-shared") {
		t.Errorf("moondb_sessions = %s, want the stored session", text)
	}
	if s, err := connect(ctx, ts.URL+"/mcp", "wrong"); err == nil {
		_ = s.Close()
		t.Error("connect with a wrong key succeeded, want a refusal")
	}
}

func connect(ctx context.Context, url, key string) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "moondb-test", Version: "test"}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: url,
		HTTPClient: &http.Client{
			Transport: bearer{key: key, base: http.DefaultTransport},
		},
		DisableStandaloneSSE: true,
	}, nil)
}

func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
