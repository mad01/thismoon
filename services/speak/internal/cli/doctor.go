package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/doctor"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/config"
	"github.com/mad01/thismoon/services/speak/internal/provider"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// synthesisProbeTimeout bounds the test synthesis. Generous on purpose: the
// local engine's first synthesis after a restart loads the model.
const synthesisProbeTimeout = 10 * time.Second

func init() {
	rootCmd.AddCommand(agentcli.DoctorCommand(speak.Facts(), doctorChecks))
}

// doctorChecks builds the check list at run time, so it probes the resolved
// --config/--provider/--port/--state-dir (env vars included) rather than
// the compile-time defaults. `speak doctor` and the speak_doctor MCP tool
// both run it, so a shell and an agent see the same diagnosis.
//
// The provider checks lead: the MCP tools speak through the provider alone,
// so the web surface's checks failing still leaves the tools able to talk.
func doctorChecks(ctx context.Context) []doctor.Check {
	cfg, loadErr := loadConfig()
	checks := []doctor.Check{configLoads(cfg, loadErr)}
	var p *provider.Provider
	if loadErr == nil {
		checks = append(checks, providerChecks(cfg)...)
		p = provider.New(ctx, cfg.ActiveProvider())
	} else {
		p = provider.Broken(cfg.Active, loadErr)
	}
	baseURL := serveBaseURL()
	return append(
		checks,
		engineReachable(p),
		synthesisWorks(p),
		voicesOffered(p),
		stateDirReadable(flagStateDir),
		doctor.ServiceReachable(baseURL),
		doctor.VersionSkew(baseURL),
	)
}

// configLoads reports whether the config file parses and names a block.
func configLoads(cfg config.Config, loadErr error) doctor.Check {
	return doctor.Check{
		Name: "config",
		Run: func(context.Context) error {
			switch {
			case loadErr != nil:
				return loadErr
			case !cfg.Found:
				return doctor.Skip("no file at " + cfg.Path + "; using the implicit " +
					speak.DefaultProvider + " provider")
			default:
				return nil
			}
		},
	}
}

// providerChecks lists every configured block. The active one fails when it
// cannot be used; an inactive one with a problem only notes it, since
// nothing uses it until it is selected.
func providerChecks(cfg config.Config) []doctor.Check {
	checks := make([]doctor.Check, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		name := "provider " + p.Name
		if p.Name == cfg.Active {
			name += " (active)"
		}
		checks = append(checks, doctor.Check{
			Name: name,
			Run: func(context.Context) error {
				switch {
				case p.Problem == "":
					return nil
				case p.Name == cfg.Active:
					return errors.New(p.Problem)
				default:
					return doctor.Skip("not active; " + p.Problem)
				}
			},
		})
	}
	return checks
}

// engineReachable pings the local engine: GET its root and count any HTTP
// response, even a 404, as reachable. It only proves a process is
// listening; synthesisWorks proves it can speak. A remote provider has no
// process of ours to ping.
func engineReachable(p *provider.Provider) doctor.Check {
	return doctor.Check{
		Name: "tts-engine-reachable",
		Run: func(ctx context.Context) error {
			checked, err := p.Ping(ctx)
			if !checked {
				return doctor.Skip("no local engine in use; tts-synthesis checks the provider")
			}
			return err
		},
	}
}

// synthesisWorks speaks a short test phrase through the active provider, the
// check a ping cannot make: an engine or provider that answers but cannot
// speak fails here with its own reason. A config problem skips (the config
// and provider checks report it), and so does an unreachable local engine,
// which tts-engine-reachable reports.
func synthesisWorks(p *provider.Provider) doctor.Check {
	return doctor.Check{
		Name: "tts-synthesis",
		Run: func(ctx context.Context) error {
			if p.Err() != nil {
				return doctor.Skip("the provider is not usable; see the config and provider checks")
			}
			ctx, cancel := context.WithTimeout(ctx, synthesisProbeTimeout)
			defer cancel()
			_, err := p.Synthesize(ctx, tts.Request{Text: "Ready."})
			te, ok := errors.AsType[*tts.Error](err)
			switch {
			case err == nil:
				return nil
			case ok && te.Kind == tts.KindNetwork && p.Config().Type == config.TypeLocal:
				return doctor.Skip("engine not reachable; see tts-engine-reachable")
			case ok:
				return fmt.Errorf("%s synthesis failed (%s): %s", te.Provider, te.Kind, te.Message)
			default:
				return fmt.Errorf("synthesis failed: %w", err)
			}
		},
	}
}

// voicesOffered checks the default voice is one the provider offers, and
// notes when the list had to fall back from discovery to a catalog.
func voicesOffered(p *provider.Provider) doctor.Check {
	return doctor.Check{
		Name: "voices",
		Run: func(context.Context) error {
			v := p.Voices()
			switch {
			case p.Err() != nil:
				return doctor.Skip("the provider is not usable")
			case strings.HasPrefix(v.Note, "default voice"):
				return errors.New(v.Note + "; set voice: on the block to one speak_voices lists")
			case v.Note != "":
				return doctor.Skip(
					fmt.Sprintf("%d voices from the %s: %s", len(v.List), v.Source, v.Note),
				)
			default:
				return nil
			}
		},
	}
}

// stateDirReadable is doctor.StoreReadable with one case carved out: the
// playback engine creates the state directory the first time it synthesizes
// audio, so on an install that has never played anything the directory is
// simply not there yet. That is not a fault to report — the shared check
// would call it "store not readable", which reads like a permissions or
// disk problem — so it skips with the path instead. Anything else that
// makes the directory unopenable still fails.
func stateDirReadable(path string) doctor.Check {
	check := doctor.StoreReadable(path)
	inner := check.Run
	check.Run = func(ctx context.Context) error {
		if path != "" {
			expanded, err := confdir.Expand(path)
			if err == nil {
				if _, statErr := os.Stat(expanded); errors.Is(statErr, fs.ErrNotExist) {
					return doctor.Skip("no playback state yet at " + expanded +
						"; created on the first speak_text or speak_file call")
				}
			}
		}
		return inner(ctx)
	}
	return check
}
