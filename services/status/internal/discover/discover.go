// Package discover finds t-man-managed launchd services and maps them to
// their d-man .this links. Discovery is a directory scan over the launchd
// plist dirs, so newly added services show up on the next poll cycle without
// any config.
package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"howett.net/plist"
)

// Service is one t-man-managed launchd job.
type Service struct {
	Label  string `json:"label"`
	Binary string `json:"binary"`
	Port   int    `json:"port,omitempty"` // 0 = no HTTP port; PID check
	Daemon bool   `json:"daemon"`         // LaunchDaemon (system) vs LaunchAgent (user)
	Link   string `json:"link,omitempty"` // http://<route>.this/ when a d-man route fronts the port
}

// HTTP reports whether the service is probed over HTTP.
func (s Service) HTTP() bool { return s.Port > 0 }

type tmanPlist struct {
	Label            string   `plist:"Label"`
	ProgramArguments []string `plist:"ProgramArguments"`
	TManMetadata     struct {
		ManagedBy string `plist:"ManagedBy"`
	} `plist:"TManMetadata"`
}

// DefaultDirs returns the launchd plist directories t-man writes to.
func DefaultDirs() (agents, daemons string) {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents"), "/Library/LaunchDaemons"
}

// Scan reads both plist directories, keeps t-man-managed jobs, and attaches
// .this links from the d-man routes file. A missing or unreadable routes file
// just means no links. Unparsable plists are skipped (the dirs are full of
// third-party agents).
func Scan(agentsDir, daemonsDir, routesPath string) ([]Service, error) {
	links := routeLinks(routesPath)
	var services []Service
	for _, dir := range []struct {
		path   string
		daemon bool
	}{{agentsDir, false}, {daemonsDir, true}} {
		entries, err := os.ReadDir(dir.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", dir.path, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".plist") {
				continue
			}
			svc, ok := parsePlist(filepath.Join(dir.path, e.Name()))
			if !ok {
				continue
			}
			svc.Daemon = dir.daemon
			svc.Link = links[svc.Port]
			services = append(services, svc)
		}
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Label < services[j].Label })
	return services, nil
}

func parsePlist(path string) (Service, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Service{}, false
	}
	var p tmanPlist
	if _, err := plist.Unmarshal(data, &p); err != nil {
		return Service{}, false
	}
	if p.TManMetadata.ManagedBy != "t-man" || p.Label == "" {
		return Service{}, false
	}
	svc := Service{Label: p.Label, Port: portFromArgs(p.ProgramArguments)}
	if len(p.ProgramArguments) > 0 {
		svc.Binary = p.ProgramArguments[0]
	}
	return svc, true
}

// portFromArgs extracts the port from "--port N" or "--port=N".
func portFromArgs(args []string) int {
	for i, a := range args {
		if a == "--port" && i+1 < len(args) {
			if p, err := strconv.Atoi(args[i+1]); err == nil {
				return p
			}
		}
		if v, ok := strings.CutPrefix(a, "--port="); ok {
			if p, err := strconv.Atoi(v); err == nil {
				return p
			}
		}
	}
	return 0
}

type routesFile struct {
	Suffix string `toml:"suffix"`
	Routes []struct {
		Name  string `toml:"name"`
		Port  int    `toml:"port"`
		Cname string `toml:"cname"`
	} `toml:"route"`
}

// routeLinks maps backend port -> http://<name>.<suffix>/ from d-man's
// routes.toml. CNAME aliases are skipped; the canonical route wins.
func routeLinks(path string) map[int]string {
	links := map[int]string{}
	if path == "" {
		return links
	}
	var rf routesFile
	if _, err := toml.DecodeFile(path, &rf); err != nil {
		return links
	}
	if rf.Suffix == "" {
		rf.Suffix = "this"
	}
	for _, r := range rf.Routes {
		if r.Cname != "" || r.Port == 0 {
			continue
		}
		if _, taken := links[r.Port]; !taken {
			links[r.Port] = fmt.Sprintf("http://%s.%s/", r.Name, rf.Suffix)
		}
	}
	return links
}
