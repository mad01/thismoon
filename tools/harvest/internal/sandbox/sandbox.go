// Package sandbox says how `harvest run` starts each step of a harvest:
// under srt (anthropics/sandbox-runtime, through the recipe's launcher script) or
// directly, and with which environment. Build is pure, so the argv and the
// environment of every step can be checked without starting anything; Run
// in run.go is the I/O shell around it.
//
// A step under srt gets a clean environment built from an allow-list, so no
// secret of the parent's reaches it by accident. Only the complete step gets
// the provider variables kit/llm reads, together with the provider's host as
// SRT_RUN_ALLOW_DOMAINS: the one step that talks to a model is the one that
// may reach the network. The claude CLI backend is the exception. Its login
// is in the macOS keychain, which srt does not let it reach, so its complete
// step runs outside the sandbox with the parent's environment. That is safe
// because kit/llm runs claude with no tools, no hooks and no settings: a pure
// completion.
package sandbox

import (
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/tools/harvest/internal/settings"
)

// The environment variables Build sets or reads besides the settings.
const (
	// envAllowDomains is the variable srt-run merges into the settings'
	// allowed hosts at launch.
	envAllowDomains = "SRT_RUN_ALLOW_DOMAINS"
	// envLiteLLMBaseURL is the proxy base URL kit/llm reads; the host the
	// complete step may reach comes from it.
	envLiteLLMBaseURL = "LITELLM_BASE_URL"
	// openRouterHost is the host kit/llm's OpenRouter backend calls.
	openRouterHost = "openrouter.ai"
	// defaultPath is the PATH a step gets when the parent has none.
	defaultPath = "/usr/bin:/bin"
)

// baseEnv names the variables every step under the allow-list gets. srt-run
// hands TMPDIR on to the child.
var baseEnv = []string{"HOME", "PATH", "TMPDIR"}

// Config is how the steps of one run start.
type Config struct {
	On bool // run the steps under srt; off runs them directly
	// Launcher is the script that runs its arguments under srt with
	// harvest's settings file (recipes/harvest/harvest-step.sh).
	Launcher string
	Self     string   // the harvest binary each step runs
	Environ  []string // the parent's environment, as KEY=VALUE pairs
}

// Step is one step process.
type Step struct {
	Name string   // prepare, complete or apply
	Args []string // the step's arguments after its name
	// Backend is the model backend the complete step must use, and empty for
	// the other steps. It decides whether the step gets the provider
	// variables and whether it runs confined.
	Backend string
}

// Command is the process Build describes: exec Path with Args under Env in
// Dir.
type Command struct {
	Path string
	Args []string
	Env  []string
	Dir  string
}

// Confined reports whether step runs under srt: the sandbox is on and the
// step is not the claude CLI's model call.
func (c Config) Confined(step Step) bool { return c.On && step.Backend != llm.Claude }

// Build returns the command that starts step. Every step runs `<self> step
// <name> <args...>` from /, so a working directory inside a read-denied tree
// cannot fail it. Under srt the command is the launcher followed by that
// whole command line.
func Build(cfg Config, step Step) Command {
	argv := append([]string{"step", step.Name}, step.Args...)
	c := Command{Path: cfg.Self, Args: argv, Env: env(cfg, step), Dir: "/"}
	if cfg.Confined(step) {
		c.Path = cfg.Launcher
		c.Args = append([]string{cfg.Self}, argv...)
	}
	return c
}

// env is the step's environment. Each step carries HARVEST_CHILD, so no
// session it might start harvests itself, and the complete step carries the
// backend the run selected, so it cannot pick another one.
func env(cfg Config, step Step) []string {
	own := []string{settings.EnvChild + "=1"}
	if step.Backend != "" {
		own = append(own, settings.EnvBackend+"="+step.Backend)
	}
	if step.Backend == llm.Claude {
		return append(without(cfg.Environ, own), own...)
	}

	parent := lookup(cfg.Environ)
	var out []string
	for _, name := range baseEnv {
		if v := parent[name]; v != "" {
			out = append(out, name+"="+v)
		} else if name == "PATH" {
			out = append(out, name+"="+defaultPath)
		}
	}
	for _, name := range settings.Names {
		if v := parent[name]; v != "" {
			out = append(out, name+"="+v)
		}
	}
	out = append(without(out, own), own...)
	if step.Backend == "" {
		return out
	}
	for _, name := range llm.ProviderEnv {
		if v := parent[name]; v != "" {
			out = append(out, name+"="+v)
		}
	}
	if cfg.Confined(step) {
		if host := ProviderHost(step.Backend, parent); host != "" {
			out = append(out, envAllowDomains+"="+host)
		}
	}
	return out
}

// ProviderHost is the host a backend's model call goes to, as host or
// host:port: the LiteLLM base URL without its scheme, userinfo, path, query
// and fragment, or OpenRouter's host. It is "" for any other backend and for
// a LiteLLM backend with no base URL.
func ProviderHost(backend string, env map[string]string) string {
	switch backend {
	case llm.LiteLLM:
		h := env[envLiteLLMBaseURL]
		if _, rest, ok := strings.Cut(h, "://"); ok {
			h = rest
		}
		if i := strings.IndexAny(h, "/?#"); i >= 0 {
			h = h[:i]
		}
		if i := strings.LastIndex(h, "@"); i >= 0 {
			h = h[i+1:]
		}
		return h
	case llm.OpenRouter:
		return openRouterHost
	default:
		return ""
	}
}

// lookup turns KEY=VALUE pairs into a map; a later pair wins, as exec's own
// handling of duplicates does.
func lookup(environ []string) map[string]string {
	m := make(map[string]string, len(environ))
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

// without returns environ minus the pairs whose names the pairs in set
// carry, so a value set here never sits beside the parent's own.
func without(environ, set []string) []string {
	names := make([]string, 0, len(set))
	for _, kv := range set {
		k, _, _ := strings.Cut(kv, "=")
		names = append(names, k)
	}
	return slices.DeleteFunc(slices.Clone(environ), func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return slices.Contains(names, k)
	})
}
