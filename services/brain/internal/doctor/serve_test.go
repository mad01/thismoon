package doctor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/server"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

func statuses(checks []Check) map[string]string {
	out := map[string]string{}
	for _, c := range checks {
		out[c.Name] = c.Status
	}
	return out
}

func byName(checks []Check, name string) Check {
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	return Check{}
}

// fakeServe answers /healthz and /api/doctor with view, or doctorStatus
// when it is not 200.
func fakeServe(t *testing.T, view server.DoctorView, doctorStatus int) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /version", buildinfo.Info{Commit: "abc1234"}.Handler())
	mux.HandleFunc("GET /api/doctor", func(w http.ResponseWriter, _ *http.Request) {
		if doctorStatus != http.StatusOK {
			w.WriteHeader(doctorStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(view)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// serveFixture isolates brain and returns a config of one vault and the
// view of a serve that sees it as the CLI does.
func serveFixture(t *testing.T) (*config.Config, server.DoctorView) {
	t.Helper()
	testhome.Isolate(t)
	root := t.TempDir()
	cfg := &config.Config{Vaults: []config.Vault{{Name: "personal", Path: root}}}
	state, err := brain.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	view := server.DoctorView{
		StateDir:          state,
		LayoutDirWritable: true,
		Config:            "/cfg.yaml",
		Vaults: []server.DoctorVault{
			{Name: "personal", Path: root, Resolved: root, Readable: true},
		},
	}
	return cfg, view
}

func TestServeChecksSkipWhenNothingListens(t *testing.T) {
	cfg, _ := serveFixture(t)
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	checks := checkServe(context.Background(), dead.URL, cfg)
	for name, status := range statuses(checks) {
		if status != StatusSkip {
			t.Errorf("%s = %s, want skip", name, status)
		}
	}
	if len(checks) != 4 ||
		!strings.Contains(byName(checks, checkServeReachable).Detail, "brain serve") {
		t.Errorf("checks = %+v", checks)
	}
}

func TestServeChecks(t *testing.T) {
	cfg, healthy := serveFixture(t)
	tests := []struct {
		name    string
		view    func(server.DoctorView) server.DoctorView
		cfg     *config.Config
		status  int
		want    map[string]string
		wantFix map[string]string
	}{
		{
			name: "healthy",
			want: map[string]string{checkServeVaults: StatusOK, checkServeStateDir: StatusOK},
		},
		{
			name: "vault unreadable under the sandbox",
			view: func(v server.DoctorView) server.DoctorView {
				v.Vaults = []server.DoctorVault{{
					Name: "personal", Path: v.Vaults[0].Path, Resolved: v.Vaults[0].Path,
					Error: "operation not permitted",
				}}
				return v
			},
			want:    map[string]string{checkServeVaults: StatusFail, checkServeStateDir: StatusOK},
			wantFix: map[string]string{checkServeVaults: brain.ServeSandboxLocal},
		},
		{
			name: "vault unknown to serve",
			view: func(v server.DoctorView) server.DoctorView {
				v.Vaults = []server.DoctorVault{}
				return v
			},
			want:    map[string]string{checkServeVaults: StatusFail, checkServeStateDir: StatusOK},
			wantFix: map[string]string{checkServeVaults: brain.ConfigEnv},
		},
		{
			name: "no config here",
			cfg:  &config.Config{},
			want: map[string]string{checkServeVaults: StatusOK, checkServeStateDir: StatusOK},
		},
		{
			name: "state dir differs",
			view: func(v server.DoctorView) server.DoctorView {
				v.StateDir = "/elsewhere/brain"
				return v
			},
			want:    map[string]string{checkServeVaults: StatusOK, checkServeStateDir: StatusFail},
			wantFix: map[string]string{checkServeStateDir: "XDG_STATE_HOME"},
		},
		{
			name: "layout dir not writable",
			view: func(v server.DoctorView) server.DoctorView {
				v.LayoutDirWritable = false
				return v
			},
			want:    map[string]string{checkServeVaults: StatusOK, checkServeStateDir: StatusFail},
			wantFix: map[string]string{checkServeStateDir: "filesystem.allowWrite"},
		},
		{
			name:   "serve without /api/doctor",
			status: http.StatusNotFound,
			want:   map[string]string{checkServeVaults: StatusFail, checkServeStateDir: StatusSkip},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			view := healthy
			if tc.view != nil {
				view = tc.view(view)
			}
			status := tc.status
			if status == 0 {
				status = http.StatusOK
			}
			c := cfg
			if tc.cfg != nil {
				c = tc.cfg
			}
			checks := checkServe(context.Background(), fakeServe(t, view, status), c)
			got := statuses(checks)
			if got[checkServeReachable] != StatusOK {
				t.Errorf("%s = %s, want ok", checkServeReachable, got[checkServeReachable])
			}
			// The test binary carries no commit, so the skew probe skips.
			if got[checkVersionSkew] != StatusSkip {
				t.Errorf("%s = %s, want skip", checkVersionSkew, got[checkVersionSkew])
			}
			for name, want := range tc.want {
				if got[name] != want {
					t.Errorf("%s = %s, want %s (%+v)", name, got[name], want, byName(checks, name))
				}
			}
			for name, want := range tc.wantFix {
				if fix := byName(checks, name).Fix; !strings.Contains(fix, want) {
					t.Errorf("%s fix %q lacks %q", name, fix, want)
				}
			}
		})
	}
}

func TestServeChecksWithoutConfig(t *testing.T) {
	_, view := serveFixture(t)
	checks := checkServe(context.Background(), fakeServe(t, view, http.StatusOK), nil)
	if got := byName(checks, checkServeVaults).Status; got != StatusSkip {
		t.Errorf("%s = %s, want skip without a config", checkServeVaults, got)
	}
}

func TestServeReachableFailsOnABadAnswer(t *testing.T) {
	serveFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	checks := checkServe(context.Background(), srv.URL, nil)
	if c := byName(checks, checkServeReachable); c.Status != StatusFail || c.Fix == "" {
		t.Errorf("%s = %+v, want a failure with a fix", checkServeReachable, c)
	}
}

// TestRunAgainstServe runs brain doctor against a real brain serve whose
// sandbox cannot read one of the vaults, simulated with mode 000.
func TestRunAgainstServe(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode 000 directory")
	}
	home := testhome.Isolate(t)
	personal, team := t.TempDir(), t.TempDir()
	cfgPath := filepath.Join(home, "config.yaml")
	cfgText := "vaults:\n  - {name: personal, path: " + personal + "}\n" +
		"  - {name: team, path: " + team + ", shared: true}\n"
	if err := os.WriteFile(cfgPath, []byte(cfgText), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	s, err := server.New(server.Options{Config: cfg, ConfigPath: cfgPath})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(brain.PortEnv, u.Port())
	if err := os.Chmod(team, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(team, 0o755) })

	report := Run(context.Background(), cfgPath)
	got := statuses(report.Checks)
	if got[checkServeReachable] != StatusOK || got[checkServeStateDir] != StatusOK {
		t.Errorf("statuses = %v, want serve reachable and sharing the state dir", got)
	}
	vaults := byName(report.Checks, checkServeVaults)
	resolved, err := filepath.EvalSymlinks(team)
	if err != nil {
		t.Fatal(err)
	}
	if vaults.Status != StatusFail || !strings.Contains(vaults.Detail, "team") ||
		!strings.Contains(vaults.Fix, `"`+resolved+`"`) ||
		!strings.Contains(vaults.Fix, brain.ServeSandboxLocal) {
		t.Errorf("%s = %+v, want the team vault's resolved path %s in the fix",
			checkServeVaults, vaults, resolved)
	}
}
