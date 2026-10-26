package launch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixtures is shared with the layers tests, so run and explain agree on what
// the expansion layer may do.
const fixtures = "../../../../recipes/speak/testdata/srt-local"

// newHome lays out a scratch HOME: a stand-in srt under mise's shims dir, an
// empty pathbin, a TMPDIR, and base.json as the settings for label svc.
func newHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, d := range []string{".local/share/mise/shims", ".config/srt", "tmp", "pathbin"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExec(t, filepath.Join(home, ".local/share/mise/shims/srt"), "#!/bin/sh\nexit 99\n")
	copyFile(t, filepath.Join(fixtures, "base.json"), filepath.Join(home, ".config/srt/svc.json"))
	return home
}

func writeExec(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// input is a launch of `true` under svc with a clean environment and the
// runtime pinned to node, so a bun on the host can't change what the layer
// cases test. vars are KEY=value overrides; KEY= unsets.
func input(home string, vars ...string) Input {
	env := map[string]string{
		"HOME":            home,
		"TMPDIR":          filepath.Join(home, "tmp"),
		"PATH":            filepath.Join(home, "pathbin") + ":/usr/bin:/bin",
		"SRT_RUN_RUNTIME": "node",
	}
	for _, kv := range vars {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	return Input{
		Settings: filepath.Join(home, ".config/srt/svc.json"),
		Argv:     []string{"true"},
		Home:     home,
		Dir:      home,
		Env:      env,
	}
}

// settingsOf returns the --settings argument of a planned command.
func settingsOf(t *testing.T, c Command) string {
	t.Helper()
	i := slices.Index(c.Argv, "--settings")
	if i < 0 || i+1 >= len(c.Argv) {
		t.Fatalf("no --settings in %v", c.Argv)
	}
	return c.Argv[i+1]
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	var exit *ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("error %v carries no exit code", err)
	}
	return exit.Code
}

func hasEnv(c Command, kv string) bool { return slices.Contains(c.Env, kv) }

func hasVar(c Command, key string) bool {
	return slices.ContainsFunc(c.Env, func(e string) bool { return strings.HasPrefix(e, key+"=") })
}

func TestPlanPlain(t *testing.T) {
	home := newHome(t)
	c, err := Plan(input(home))
	if err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(home, ".local/share/mise/shims/srt")
	settings := filepath.Join(home, ".config/srt/svc.json")
	want := []string{srt, "--settings", settings, "--", "env", "SRT_RUN=svc", "true"}
	if c.Path != srt || !slices.Equal(c.Argv, want) || c.Runtime != "node" || c.Label != "svc" {
		t.Fatalf("Plan() = %+v, want exec %v", c, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".config/srt/merged")); err == nil {
		t.Error("a plain launch wrote a merged copy")
	}
	if !hasEnv(c, "CLAUDE_CODE_TMPDIR="+filepath.Join(home, "tmp")) {
		t.Errorf("CLAUDE_CODE_TMPDIR not set from TMPDIR: %v", c.Env)
	}
	wantPath := "PATH=" + filepath.Join(home, ".local/share/mise/shims") +
		":/usr/bin:/bin:/usr/sbin:/sbin:" + filepath.Join(home, "pathbin") + ":/usr/bin:/bin"
	if !hasEnv(c, wantPath) {
		t.Errorf("PATH not prefixed with the shims and system dirs: %v", c.Env)
	}
}

func TestPlanWidens(t *testing.T) {
	home := newHome(t)
	copyFile(
		t,
		filepath.Join(fixtures, "ok-widen.local.json"),
		filepath.Join(home, ".config/srt/svc.local.json"),
	)
	in := input(home, "SRT_RUN_ALLOW_DOMAINS=share.example.net,api.example.com", "SRT_DEBUG=1")
	var log bytes.Buffer
	in.Log = &log
	c, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	merged := filepath.Join(home, ".config/srt/merged/svc.json")
	if got := settingsOf(t, c); got != merged {
		t.Fatalf("srt was handed %s, want the merged copy %s", got, merged)
	}
	info, err := os.Stat(merged)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("merged copy is mode %o, want 600", info.Mode().Perm())
	}
	var m struct {
		Network struct {
			AllowedDomains    []string `json:"allowedDomains"`
			AllowMachLookup   []string `json:"allowMachLookup"`
			AllowUnixSockets  []string `json:"allowUnixSockets"`
			AllowLocalBinding bool     `json:"allowLocalBinding"`
		} `json:"network"`
		Filesystem struct {
			AllowRead  []string `json:"allowRead"`
			AllowWrite []string `json:"allowWrite"`
			DenyRead   []string `json:"denyRead"`
			DenyWrite  []string `json:"denyWrite"`
		} `json:"filesystem"`
		Weaker bool `json:"enableWeakerNetworkIsolation"`
	}
	data, _ := os.ReadFile(merged)
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		ok   bool
	}{
		{
			"allowedDomains",
			slices.Equal(
				m.Network.AllowedDomains,
				[]string{"*.example.org", "api.example.com", "share.example.net"},
			),
		},
		{
			"allowMachLookup",
			slices.Equal(
				m.Network.AllowMachLookup,
				[]string{"com.apple.SystemConfiguration.configd"},
			),
		},
		{
			"allowUnixSockets",
			slices.Equal(m.Network.AllowUnixSockets, []string{"/private/var/run/svc.sock"}),
		},
		{"allowLocalBinding", m.Network.AllowLocalBinding},
		{"enableWeakerNetworkIsolation", m.Weaker},
		{
			"allowRead",
			slices.Contains(m.Filesystem.AllowRead, "~/code") &&
				slices.Contains(m.Filesystem.AllowRead, "~/notes"),
		},
		{
			"allowWrite",
			slices.Contains(m.Filesystem.AllowWrite, "~/.cache/svc") &&
				slices.Contains(m.Filesystem.AllowWrite, "~/.local/share/svc"),
		},
		{"denyRead kept", slices.Equal(m.Filesystem.DenyRead, []string{"~", "~/code/**/.env"})},
		{"denyWrite kept", len(m.Filesystem.DenyWrite) == 0},
	}
	for _, ch := range checks {
		if !ch.ok {
			t.Errorf("merged copy: %s wrong:\n%s", ch.name, data)
		}
	}
	if hasVar(c, "SRT_RUN_ALLOW_DOMAINS") {
		t.Error("SRT_RUN_ALLOW_DOMAINS reached the child")
	}
	for _, line := range []string{"srt-run: svc allowedDomains: [", "srt-run: svc merged "} {
		if !strings.Contains(log.String(), line) {
			t.Errorf("SRT_DEBUG log lacks %q:\n%s", line, log.String())
		}
	}
}

// Every ok- fixture launches with the base denies intact; every refuse-
// fixture exits 1 before srt starts. The same files drive the layers tests.
func TestPlanSharedFixtures(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join(fixtures, "*.local.json"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("no fixtures under %s: %v", fixtures, err)
	}
	base, _ := os.ReadFile(filepath.Join(fixtures, "base.json"))
	var want struct {
		Filesystem struct {
			DenyRead []string `json:"denyRead"`
		} `json:"filesystem"`
	}
	if err := json.Unmarshal(base, &want); err != nil {
		t.Fatal(err)
	}
	for _, path := range cases {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			home := newHome(t)
			copyFile(t, path, filepath.Join(home, ".config/srt/svc.local.json"))
			c, err := Plan(input(home))
			merged := filepath.Join(home, ".config/srt/merged/svc.json")
			switch {
			case strings.HasPrefix(name, "ok-"):
				if err != nil {
					t.Fatalf("Plan() = %v, want a launch", err)
				}
				var got struct {
					Filesystem struct {
						DenyRead []string `json:"denyRead"`
					} `json:"filesystem"`
				}
				data, _ := os.ReadFile(settingsOf(t, c))
				if err := json.Unmarshal(data, &got); err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(got.Filesystem.DenyRead, want.Filesystem.DenyRead) {
					t.Errorf("a base deny changed: %v", got.Filesystem.DenyRead)
				}
			case strings.HasPrefix(name, "refuse-"):
				if err == nil || exitCode(t, err) != 1 {
					t.Fatalf("Plan() = %v, want exit 1", err)
				}
				if _, statErr := os.Stat(merged); statErr == nil {
					t.Error("a refused launch wrote a merged copy")
				}
			default:
				t.Fatalf("fixture %s must start with ok- or refuse-", name)
			}
		})
	}
}

func TestPlanRefusalReasons(t *testing.T) {
	tests := []struct{ fixture, want string }{
		{"refuse-deny-key.local.json", "filesystem.denyRead is not allowed"},
		{"refuse-star.local.json", "which allows every host"},
		{"refuse-unknown-key.local.json", "key allowPty is not allowed"},
		{"refuse-invalid-json.local.json", "is not valid JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			home := newHome(t)
			copyFile(
				t,
				filepath.Join(fixtures, tt.fixture),
				filepath.Join(home, ".config/srt/svc.local.json"),
			)
			_, err := Plan(input(home))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Plan() = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestPlanRefusesEnvStar(t *testing.T) {
	home := newHome(t)
	_, err := Plan(input(home, "SRT_RUN_ALLOW_DOMAINS=ok.example.com,*:443"))
	if err == nil || exitCode(t, err) != 1 || !strings.Contains(err.Error(), "bare *") {
		t.Fatalf("Plan() = %v, want exit 1 on a bare * host", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".config/srt/merged")); statErr == nil {
		t.Error("a refused launch wrote a merged copy")
	}
}

func TestPlanSettingsAndLabel(t *testing.T) {
	home := newHome(t)
	abs := filepath.Join(home, ".config/srt/svc.json")

	in := input(home)
	in.Settings = "~/.config/srt/svc.json"
	in.Home = home
	c, err := Plan(in)
	if err != nil || settingsOf(t, c) != abs {
		t.Errorf("~ settings: %v, handed %q", err, settingsOf(t, c))
	}

	in = input(home)
	in.Settings = ".config/srt/svc.json"
	c, err = Plan(in)
	if err != nil || settingsOf(t, c) != abs {
		t.Errorf("relative settings: %v, handed %q", err, settingsOf(t, c))
	}

	c, err = Plan(input(home, "SRT_RUN_LABEL=other"))
	if err != nil || !slices.Contains(c.Argv, "SRT_RUN=other") {
		t.Errorf("SRT_RUN_LABEL: %v, argv %v", err, c.Argv)
	}

	_, err = Plan(input(home, "SRT_RUN_LABEL=../svc"))
	if err == nil || exitCode(t, err) != 2 {
		t.Errorf("a path as a label = %v, want exit 2", err)
	}

	_, err = Plan(Input{})
	if err == nil || exitCode(t, err) != 2 {
		t.Errorf("no arguments = %v, want exit 2", err)
	}
}

func TestPlanSrtMissing(t *testing.T) {
	home := newHome(t)
	if err := os.Remove(filepath.Join(home, ".local/share/mise/shims/srt")); err != nil {
		t.Fatal(err)
	}
	_, err := Plan(input(home))
	if err == nil || exitCode(t, err) != 127 || !strings.Contains(err.Error(), "srt not found") {
		t.Fatalf("Plan() = %v, want exit 127", err)
	}
}

// miseHome lays out home the way mise installs srt: the srt shim links to a
// stand-in mise, which answers `mise which srt` with npm's bin wrapper. The
// package's dist/cli.js sits behind a link into .mise, as aube lays it out.
func miseHome(t *testing.T) (home, cli string) {
	t.Helper()
	home = newHome(t)
	modules := filepath.Join(home, ".local/share/mise/installs/npm-srt/1/node_modules")
	pkg := filepath.Join(
		modules,
		".mise/sandbox-runtime/node_modules/@anthropic-ai/sandbox-runtime",
	)
	for _, d := range []string{filepath.Join(modules, ".bin"), filepath.Join(modules, "@anthropic-ai"), filepath.Join(pkg, "dist"), filepath.Join(home, "opt")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	shim := filepath.Join(home, ".local/share/mise/shims/srt")
	bin := filepath.Join(modules, ".bin/srt")
	if err := os.Rename(shim, bin); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.mise/sandbox-runtime/node_modules/@anthropic-ai/sandbox-runtime",
		filepath.Join(modules, "@anthropic-ai/sandbox-runtime")); err != nil {
		t.Fatal(err)
	}
	cli = filepath.Join(pkg, "dist/cli.js")
	if err := os.WriteFile(cli, []byte("// srt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mise := filepath.Join(home, "opt/mise")
	writeExec(
		t,
		mise,
		fmt.Sprintf(
			"#!/bin/sh\n[ \"$1 $2\" = \"which srt\" ] || exit 98\nprintf '%%s\\n' '%s'\n",
			bin,
		),
	)
	if err := os.Symlink(mise, shim); err != nil {
		t.Fatal(err)
	}
	return home, cli
}

func fakeBun(t *testing.T, path string) string {
	t.Helper()
	writeExec(t, path, "#!/bin/sh\nexit 0\n")
	return path
}

func sameFile(a, b string) bool {
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	return err == nil && os.SameFile(ia, ib)
}

func TestPlanRuntime(t *testing.T) {
	installs := ".local/share/mise/installs/bun/1.0.0/bin/bun"
	t.Run("installs-dir bun wins over PATH", func(t *testing.T) {
		home, cli := miseHome(t)
		want := fakeBun(t, filepath.Join(home, installs))
		fakeBun(t, filepath.Join(home, "pathbin/bun"))
		c, err := Plan(input(home, "SRT_RUN_RUNTIME="))
		if err != nil || c.Runtime != "bun" || c.Path != want {
			t.Fatalf("Plan() = %+v, %v; want bun %s", c, err, want)
		}
		wantArgv := []string{want, "--no-env-file", "--config=/dev/null", "--no-install"}
		if !slices.Equal(c.Argv[:4], wantArgv) || !sameFile(c.Argv[4], cli) ||
			!slices.Equal(
				c.Argv[5:],
				[]string{
					"--settings",
					filepath.Join(home, ".config/srt/svc.json"),
					"--",
					"env",
					"SRT_RUN=svc",
					"true",
				},
			) {
			t.Fatalf("bun argv %v, want %v + %s + srt's arguments", c.Argv, wantArgv, cli)
		}
	})
	t.Run("PATH bun when none installed", func(t *testing.T) {
		home, _ := miseHome(t)
		want := fakeBun(t, filepath.Join(home, "pathbin/bun"))
		c, err := Plan(input(home, "SRT_RUN_RUNTIME="))
		if err != nil || c.Runtime != "bun" || c.Path != want {
			t.Fatalf("Plan() = %+v, %v; want bun %s", c, err, want)
		}
	})
	t.Run("srt linked straight to cli.js", func(t *testing.T) {
		home := newHome(t)
		if err := os.Remove(filepath.Join(home, ".local/share/mise/shims/srt")); err != nil {
			t.Fatal(err)
		}
		cli := filepath.Join(home, "lib/sandbox-runtime/dist/cli.js")
		writeExec(t, cli, "// srt\n")
		if err := os.Symlink(cli, filepath.Join(home, "pathbin/srt")); err != nil {
			t.Fatal(err)
		}
		fakeBun(t, filepath.Join(home, installs))
		c, err := Plan(input(home, "SRT_RUN_RUNTIME="))
		if err != nil || c.Runtime != "bun" || !sameFile(c.Argv[4], cli) {
			t.Fatalf("Plan() = %+v, %v; want bun running %s", c, err, cli)
		}
	})
	t.Run("node forced with bun installed", func(t *testing.T) {
		home, _ := miseHome(t)
		fakeBun(t, filepath.Join(home, installs))
		c, err := Plan(input(home, "SRT_RUN_RUNTIME=node"))
		if err != nil || c.Runtime != "node" ||
			c.Path != filepath.Join(home, ".local/share/mise/shims/srt") {
			t.Fatalf("Plan() = %+v, %v; want node through the srt shim", c, err)
		}
	})
	t.Run("node when no bun anywhere", func(t *testing.T) {
		home, _ := miseHome(t)
		c, err := Plan(input(home, "SRT_RUN_RUNTIME="))
		if err != nil || c.Runtime != "node" {
			t.Fatalf("Plan() = %+v, %v; want node", c, err)
		}
	})
	t.Run("bun asked for but missing", func(t *testing.T) {
		home, _ := miseHome(t)
		_, err := Plan(input(home, "SRT_RUN_RUNTIME=bun"))
		if err == nil || exitCode(t, err) != 127 || !strings.Contains(err.Error(), "no bun under") {
			t.Fatalf("Plan() = %v, want exit 127", err)
		}
	})
	t.Run("bun asked for but cli.js missing", func(t *testing.T) {
		home, cli := miseHome(t)
		fakeBun(t, filepath.Join(home, installs))
		if err := os.Remove(cli); err != nil {
			t.Fatal(err)
		}
		_, err := Plan(input(home, "SRT_RUN_RUNTIME=bun"))
		if err == nil || exitCode(t, err) != 127 ||
			!strings.Contains(err.Error(), "cannot find srt's dist/cli.js") {
			t.Fatalf("Plan() = %v, want exit 127", err)
		}
	})
	t.Run("unknown runtime", func(t *testing.T) {
		home, _ := miseHome(t)
		_, err := Plan(input(home, "SRT_RUN_RUNTIME=deno"))
		if err == nil || exitCode(t, err) != 2 {
			t.Fatalf("Plan() = %v, want exit 2", err)
		}
	})
}
