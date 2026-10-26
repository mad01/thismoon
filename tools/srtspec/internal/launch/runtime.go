package launch

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runtime is what runs srt's JavaScript. srt's own process stays up beside
// the command for its whole life, hosting the proxies that enforce
// allowedDomains, so the runtime's footprint is paid once per sandboxed
// process: bun holds about 48 MB resident against node's 83 MB.
type runtime struct {
	name string // bun or node
	bun  string // the bun binary, when name is bun
	cli  string // srt's dist/cli.js, when name is bun: the npm bin wrapper would exec node
}

// pickRuntime applies SRT_RUN_RUNTIME: unset means bun when one is
// installed, else node. bun fails closed when asked for but absent, or when
// srt's cli.js cannot be found behind the srt on PATH.
func pickRuntime(home string, env map[string]string, srt, label string) (runtime, error) {
	want := env["SRT_RUN_RUNTIME"]
	switch want {
	case "", "bun", "node":
	default:
		return runtime{}, usage("SRT_RUN_RUNTIME must be bun or node, not %q", want)
	}
	bun := ""
	if want != "node" {
		bun = findBun(home, env["PATH"])
	}
	if want == "" {
		want = "node"
		if bun != "" {
			want = "bun"
		}
	}
	if want == "node" {
		return runtime{name: "node"}, nil
	}
	if bun == "" {
		return runtime{}, missing(
			"SRT_RUN_RUNTIME=bun but no bun under %s or on PATH",
			filepath.Join(home, ".local/share/mise/installs/bun"),
		)
	}
	cli := findSrtCLI(srt, env)
	if cli == "" {
		return runtime{}, missing(
			"cannot find srt's dist/cli.js behind %s (tried mise which srt); refusing to launch %s under bun (SRT_RUN_RUNTIME=node runs the srt wrapper)",
			srt,
			label,
		)
	}
	return runtime{name: "bun", bun: bun, cli: cli}, nil
}

// findBun returns the last bun under mise's installs dir (its latest link
// when mise keeps one), else the first on path, else "".
func findBun(home, path string) string {
	matches, _ := filepath.Glob(filepath.Join(home, ".local/share/mise/installs/bun/*/bin/bun"))
	found := ""
	for _, m := range matches {
		if executable(m) {
			found = m
		}
	}
	if found == "" {
		found, _ = lookPath("bun", path)
	}
	return found
}

// findSrtCLI follows the srt on PATH to the package's dist/cli.js. That srt
// is normally a mise shim, a link to the mise binary, so mise names the bin
// its pin selects. That bin is npm's: a link to dist/cli.js, or a wrapper
// script in node_modules/.bin beside the package. Returns "" when the file
// is not there.
func findSrtCLI(srt string, env map[string]string) string {
	bin := srt
	if target, err := os.Readlink(bin); err == nil {
		target = resolveLink(bin, target)
		if filepath.Base(target) == "mise" {
			which, err := miseWhich(target, env)
			if err != nil {
				return ""
			}
			bin = which
		}
	}
	bin = followLinks(bin, 40)
	cli := bin
	if filepath.Base(bin) != "cli.js" {
		dir := filepath.Dir(bin)
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		cli = filepath.Join(dir, "..", "@anthropic-ai", "sandbox-runtime", "dist", "cli.js")
	}
	info, err := os.Stat(cli)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return cli
}

// miseWhich asks mise which srt its pin selects, in this cwd as the shim
// would.
func miseWhich(mise string, env map[string]string) (string, error) {
	cmd := exec.Command(mise, "which", "srt")
	cmd.Env = environ(env)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "", errors.New("mise which srt printed nothing")
	}
	return s, nil
}

func resolveLink(link, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(filepath.Dir(link), target)
}

// followLinks resolves a chain of symlinks, giving up after hops.
func followLinks(p string, hops int) string {
	for range hops {
		target, err := os.Readlink(p)
		if err != nil {
			return p
		}
		p = resolveLink(p, target)
	}
	return p
}
