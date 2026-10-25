package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/pflag"

	"github.com/mad01/thismoon/services/moondb"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name         string
		file         string // "" writes no file
		wantEnabled  bool
		wantExcludes []string
		wantPort     int
		wantErr      bool
	}{
		{
			name:         "no file is the zero config",
			wantEnabled:  true,
			wantExcludes: moondb.DefaultExcludes,
		},
		{
			name:         "empty table",
			file:         "[capture]\n",
			wantEnabled:  true,
			wantExcludes: moondb.DefaultExcludes,
		},
		{
			name:         "off",
			file:         "[capture]\nenabled = false\n",
			wantEnabled:  false,
			wantExcludes: moondb.DefaultExcludes,
		},
		{
			name:         "excludes replace the defaults",
			file:         "[capture]\nexclude = [\"*.secret\"]\n",
			wantEnabled:  true,
			wantExcludes: []string{"*.secret"},
		},
		{
			name:         "top-level keys",
			file:         "port = 9000\nworkdir = \"/tmp/m\"\n",
			wantEnabled:  true,
			wantExcludes: moondb.DefaultExcludes,
			wantPort:     9000,
		},
		{name: "broken file is an error", file: "[capture\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), configFile)
			if tt.file != "" {
				if err := os.WriteFile(path, []byte(tt.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			c, err := loadConfig(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got := c.Capture.enabled(); got != tt.wantEnabled {
				t.Errorf("enabled() = %v, want %v", got, tt.wantEnabled)
			}
			if got := c.Capture.excludes(); !slices.Equal(got, tt.wantExcludes) {
				t.Errorf("excludes() = %v, want %v", got, tt.wantExcludes)
			}
			if c.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", c.Port, tt.wantPort)
			}
		})
	}
}

func TestDefaultConfigPathHonorsXDG(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	if got, want := defaultConfigPath(), filepath.Join(root, "moondb", configFile); got != want {
		t.Errorf("defaultConfigPath() = %q, want %q", got, want)
	}
}

// TestApplyConfigPrecedence pins flag, then env, then file, then default.
func TestApplyConfigPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      string
		wantPort int
	}{
		{name: "file beats default", wantPort: 9000},
		{name: "env beats file", env: "9100", wantPort: 7433},
		{
			name:     "flag beats env and file",
			args:     []string{"--port=9200"},
			env:      "9100",
			wantPort: 9200,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagPort = moondb.DefaultPort
			if tt.env != "" {
				t.Setenv("MOONDB_PORT", tt.env)
			}
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.IntVar(&flagPort, "port", flagPort, "")
			flags.String("workdir", "", "")
			flags.String("base-url", "", "")
			if err := flags.Parse(tt.args); err != nil {
				t.Fatal(err)
			}
			applyConfig(flags, config{Port: 9000})
			if flagPort != tt.wantPort {
				t.Errorf("flagPort = %d, want %d", flagPort, tt.wantPort)
			}
		})
	}
}
