package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	PollInterval Duration `toml:"poll_interval"`
	Sources      []Source `toml:"source"`
}

type Source struct {
	Host  string   `toml:"host"`
	Owner string   `toml:"owner"`
	Repos []string `toml:"repos"`
}

type Repo struct {
	Owner string
	Name  string
	Host  string
}

func (r Repo) FullName() string { return r.Owner + "/" + r.Name }

func (r Repo) HTMLURL() string {
	return fmt.Sprintf("https://%s/%s/%s", r.Host, r.Owner, r.Name)
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func Load(path string) (*Config, error) {
	path = expandTilde(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.PollInterval.Duration == 0 {
		cfg.PollInterval.Duration = 5 * time.Minute
	}
	for i := range cfg.Sources {
		if cfg.Sources[i].Host == "" {
			cfg.Sources[i].Host = "github.com"
		}
	}
	return &cfg, nil
}

func (c *Config) AllRepos() []Repo {
	var repos []Repo
	for _, s := range c.Sources {
		for _, name := range s.Repos {
			repos = append(repos, Repo{
				Owner: s.Owner,
				Name:  name,
				Host:  s.Host,
			})
		}
	}
	return repos
}

func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
