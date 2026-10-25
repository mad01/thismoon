package cli

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"

	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/config"
	"github.com/mad01/thismoon/services/speak/internal/provider"
)

// loadConfig resolves the provider config from the root flags.
func loadConfig() (config.Config, error) {
	return config.Load(config.Options{
		Path:     flagConfig,
		Provider: flagProvider,
		TTSURL:   flagTTSURL,
	})
}

// activeProvider builds the provider every subcommand synthesizes through. A
// config that cannot load, or an active block that cannot be used, becomes
// a provider that fails every synthesis with the reason: serve and mcp stay
// up and say what is wrong on every surface instead of refusing to start or
// quietly using another provider. The reason is also logged to stderr.
func activeProvider(ctx context.Context) *provider.Provider {
	cfg, err := loadConfig()
	if err != nil {
		log.Print(configLoadMessage(err, os.Getenv("SRT_RUN")))
		name := flagProvider
		if name == "" {
			name = "config"
		}
		return provider.Broken(name, err)
	}
	active := cfg.ActiveProvider()
	if active.Problem != "" {
		log.Printf("speak: provider %q cannot be used: %s", active.Name, active.Problem)
	}
	return provider.New(ctx, active)
}

// configLoadMessage is the startup log line for a config that cannot load.
// label is the sandbox label srt-run sets as SRT_RUN, empty outside one.
// srt checks the resolved path, so inside a sandbox a permission error
// usually means the config is a symlink whose target the settings do not
// allow, and the line names the machine-local expansion layer that takes
// the target.
func configLoadMessage(err error, label string) string {
	msg := "speak: " + err.Error()
	if label == "" || !errors.Is(err, fs.ErrPermission) {
		return msg
	}
	return msg + " (inside the " + label + " sandbox: if the config is a symlink, " +
		"add its target to allowRead in ~/.config/srt/" + label + ".local.json, then restart)"
}

// describeSource says where the config came from, for logs and `speak config`.
func describeSource(cfg config.Config) string {
	if cfg.Found {
		return cfg.Path
	}
	return cfg.Path + " (not found: using the implicit " + speak.DefaultProvider + " provider)"
}
