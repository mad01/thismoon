package settings

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// defaults is what Load returns under HOME when nothing else is set.
func defaults(home string) Settings {
	return Settings{
		StateDir: home + "/.local/state/harvest",
		MinBytes: DefaultMinBytes,
		MaxBytes: DefaultMaxBytes,
		Skill:    home + "/.claude/skills/harvest/SKILL.md",
		Sandbox:  true,
		Launcher: home + "/code/bin/harvest-step",
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    func() Settings
		wantErr string
	}{
		{
			name: "defaults under HOME",
			env:  map[string]string{"HOME": "/home/u"},
			want: func() Settings { return defaults("/home/u") },
		},
		{
			name: "every setting set",
			env: map[string]string{
				"HOME": "/home/u", EnvStateDir: "/var/h/", EnvMinBytes: "0", EnvMaxBytes: "50",
				EnvSkill: "~/skill.md", EnvBackend: "litellm", EnvModel: "m", EnvEffort: "low",
				EnvSandbox: "off", EnvLauncher: "/bin/harvest-step",
			},
			want: func() Settings {
				return Settings{
					StateDir: "/var/h", MinBytes: 0, MaxBytes: 50, Skill: "/home/u/skill.md",
					Backend: "litellm", Model: "m", Effort: "low", Sandbox: false,
					Launcher: "/bin/harvest-step",
				}
			},
		},
		{
			name: "tilde in the state dir expands",
			env:  map[string]string{"HOME": "/home/u", EnvStateDir: "~/h"},
			want: func() Settings {
				s := defaults("/home/u")
				s.StateDir = "/home/u/h"
				return s
			},
		},
		{
			name: "sandbox on by name",
			env:  map[string]string{"HOME": "/home/u", EnvSandbox: "on"},
			want: func() Settings { return defaults("/home/u") },
		},
		{
			name:    "sandbox value that is neither on nor off",
			env:     map[string]string{"HOME": "/h", EnvSandbox: "false"},
			wantErr: `HARVEST_SANDBOX="false" must be on or off`,
		},
		{
			name:    "relative skill file",
			env:     map[string]string{"HOME": "/h", EnvSkill: "SKILL.md"},
			wantErr: "HARVEST_SKILL must be an absolute path",
		},
		{
			name:    "no HOME for the default",
			env:     map[string]string{},
			wantErr: "HOME is not set",
		},
		{
			name:    "relative state dir",
			env:     map[string]string{EnvStateDir: "state"},
			wantErr: "must be an absolute path",
		},
		{
			name:    "min bytes not a number",
			env:     map[string]string{"HOME": "/h", EnvMinBytes: "4k"},
			wantErr: "HARVEST_MIN_BYTES=\"4k\" is not a whole number",
		},
		{
			name:    "max bytes zero",
			env:     map[string]string{"HOME": "/h", EnvMaxBytes: "0"},
			wantErr: "HARVEST_MAX_BYTES=0 is below 1",
		},
		{
			name:    "min bytes negative",
			env:     map[string]string{"HOME": "/h", EnvMinBytes: "-1"},
			wantErr: "HARVEST_MIN_BYTES=-1 is below 0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Load(env(tc.env))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Load() error = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if want := tc.want(); got != want {
				t.Errorf("Load() = %+v, want %+v", got, want)
			}
		})
	}
}
