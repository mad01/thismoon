package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestClaudeSourcesFor pins where Claude Code looks for each settings file:
// the shared project file in the session directory, the project local file
// at the main checkout root with the session directory's legacy file below
// it, and managed settings on top, drop-ins after managed-settings.json.
// ~/.claude/settings.local.json is a source only for a session started in
// the home directory itself.
func TestClaudeSourcesFor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	user := filepath.Join(home, ".claude", "settings.json")
	repo, sub := "/r", "/r/sub"
	tests := []struct {
		name      string
		cwd, root string
		want      []SettingsSource
	}{
		{"no session directory", "", "", []SettingsSource{{ScopeUser, user}}},
		{"outside a repository", "/p", "", []SettingsSource{
			{ScopeUser, user},
			{ScopeProject, "/p/.claude/settings.json"},
			{ScopeLocal, "/p/.claude/settings.local.json"},
		}},
		{"at the repository root", repo, repo, []SettingsSource{
			{ScopeUser, user},
			{ScopeProject, "/r/.claude/settings.json"},
			{ScopeLocal, "/r/.claude/settings.local.json"},
		}},
		{"in a subdirectory", sub, repo, []SettingsSource{
			{ScopeUser, user},
			{ScopeProject, "/r/sub/.claude/settings.json"},
			{ScopeLocal, "/r/sub/.claude/settings.local.json"},
			{ScopeLocal, "/r/.claude/settings.local.json"},
		}},
		{"repository root is home", sub, home, []SettingsSource{
			{ScopeUser, user},
			{ScopeProject, "/r/sub/.claude/settings.json"},
			{ScopeLocal, "/r/sub/.claude/settings.local.json"},
		}},
		{"session in home", home, "", []SettingsSource{
			{ScopeUser, user},
			{ScopeLocal, filepath.Join(home, ".claude", "settings.local.json")},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ClaudeSources{User: user}.For(tt.cwd, tt.root)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("For(%q, %q) =\n %v\nwant\n %v", tt.cwd, tt.root, got, tt.want)
			}
		})
	}
}

// TestClaudeSourcesForManaged: managed-settings.json first, then every
// visible *.json drop-in in name order, all above the user and project files.
func TestClaudeSourcesForManaged(t *testing.T) {
	managed := t.TempDir()
	drops := filepath.Join(managed, "managed-settings.d")
	if err := os.MkdirAll(drops, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"20-b.json", "10-a.json", ".hidden.json", "notes.txt"} {
		writeFile(t, drops, name, "{}")
	}
	got, err := ClaudeSources{ManagedDir: managed}.For("", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []SettingsSource{
		{ScopeManaged, filepath.Join(managed, "managed-settings.json")},
		{ScopeManaged, filepath.Join(drops, "10-a.json")},
		{ScopeManaged, filepath.Join(drops, "20-b.json")},
	}
	if !slices.Equal(got, want) {
		t.Errorf("For = %v, want %v", got, want)
	}
}

// autoMemoryCase is one ResolveAutoMemory run: the content of each source in
// precedence order (user, project, local, managed; "" leaves it missing) and
// the session's CLAUDE_CODE_DISABLE_AUTO_MEMORY.
type autoMemoryCase struct {
	name                          string
	user, project, local, managed string
	env                           string
	wantDir                       string
	wantScope                     string // the source the directory came from
	wantOff                       bool
	wantErr                       bool
}

func runAutoMemoryCase(t *testing.T, tt autoMemoryCase) {
	t.Helper()
	dir := t.TempDir()
	var sources []SettingsSource
	for _, s := range []struct{ scope, content string }{
		{ScopeUser, tt.user},
		{ScopeProject, tt.project},
		{ScopeLocal, tt.local},
		{ScopeManaged, tt.managed},
	} {
		path := filepath.Join(dir, s.scope+".json")
		if s.content != "" {
			writeFile(t, dir, s.scope+".json", s.content)
		}
		sources = append(sources, SettingsSource{s.scope, path})
	}
	getenv := func(key string) string {
		if key == EnvDisableAutoMemory {
			return tt.env
		}
		return ""
	}
	got, err := ResolveAutoMemory(sources, getenv)
	if (err != nil) != tt.wantErr {
		t.Fatalf("error = %v, want error %v", err, tt.wantErr)
	}
	if tt.wantErr {
		return
	}
	if got.Directory != tt.wantDir || got.Source.Scope != tt.wantScope {
		t.Errorf("directory %q from %q, want %q from %q",
			got.Directory, got.Source.Scope, tt.wantDir, tt.wantScope)
	}
	if (got.Off != "") != tt.wantOff {
		t.Errorf("Off = %q, want off %v", got.Off, tt.wantOff)
	}
}

// TestResolveAutoMemoryPrecedence: the highest source that sets
// autoMemoryDirectory wins (managed, local, project, user), a key set to ""
// counts as set, and a source belt cannot read as JSON is an error, because
// belt then cannot tell what Claude Code resolved.
func TestResolveAutoMemoryPrecedence(t *testing.T) {
	dir := func(d string) string { return `{"autoMemoryDirectory": "` + d + `"}` }
	for _, tt := range []autoMemoryCase{
		{name: "no sources"},
		{name: "no key", user: `{"model": "opus"}`},
		{name: "user only", user: dir("~/u"), wantDir: "~/u", wantScope: ScopeUser},
		{
			name: "project over user", user: dir("~/u"), project: dir("~/p"),
			wantDir: "~/p", wantScope: ScopeProject,
		},
		{
			name: "local over project", user: dir("~/u"), project: dir("~/p"), local: dir("~/l"),
			wantDir: "~/l", wantScope: ScopeLocal,
		},
		{
			name: "managed over local", local: dir("~/l"), managed: dir("~/m"),
			wantDir: "~/m", wantScope: ScopeManaged,
		},
		{
			name: "local without the key", user: dir("~/u"), local: `{}`,
			wantDir: "~/u", wantScope: ScopeUser,
		},
		{
			name: "empty local value counts as set", user: dir("~/u"), local: dir(""),
			wantScope: ScopeLocal,
		},
		{name: "broken local", user: dir("~/u"), local: `{not json`, wantErr: true},
		{name: "not a string", user: `{"autoMemoryDirectory": 7}`, wantErr: true},
		{name: "enabled not a bool", user: `{"autoMemoryEnabled": "no"}`, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) { runAutoMemoryCase(t, tt) })
	}
}

// TestResolveAutoMemoryOff: auto memory is off through autoMemoryEnabled
// false at the highest source that sets it, or CLAUDE_CODE_DISABLE_AUTO_MEMORY
// (1 off, 0 on whatever the key says). Under
// permissions.blockReadsOutsideWorkingDirectories, from any source, Claude
// Code loads nothing from a directory a project file chose.
func TestResolveAutoMemoryOff(t *testing.T) {
	toU := `{"autoMemoryDirectory": "~/u"}`
	off := `{"autoMemoryEnabled": false}`
	on := `{"autoMemoryEnabled": true}`
	block := `{"permissions": {"blockReadsOutsideWorkingDirectories": true}}`
	for _, tt := range []autoMemoryCase{
		{name: "on by default", user: toU, wantDir: "~/u", wantScope: ScopeUser},
		{
			name: "disabled in user settings", user: `{"autoMemoryDirectory": "~/u", "autoMemoryEnabled": false}`,
			wantDir: "~/u", wantScope: ScopeUser, wantOff: true,
		},
		{
			name: "disabled for the project", user: toU, project: off,
			wantDir: "~/u", wantScope: ScopeUser, wantOff: true,
		},
		{
			name: "local turns it back on", user: toU, project: off, local: on,
			wantDir: "~/u", wantScope: ScopeUser,
		},
		{
			name: "managed turns it off", user: toU, local: on, managed: off,
			wantDir: "~/u", wantScope: ScopeUser, wantOff: true,
		},
		{name: "env 1", user: toU, env: "1", wantDir: "~/u", wantScope: ScopeUser, wantOff: true},
		{name: "env true", user: toU, env: "true", wantDir: "~/u", wantScope: ScopeUser, wantOff: true},
		{
			name: "env 0 overrides the key", user: toU, project: off, env: "0",
			wantDir: "~/u", wantScope: ScopeUser,
		},
		{
			name: "block with a project directory", user: block,
			project: `{"autoMemoryDirectory": "~/p"}`, wantDir: "~/p", wantScope: ScopeProject, wantOff: true,
		},
		{
			name: "block with a local directory", managed: block,
			local: `{"autoMemoryDirectory": "~/l"}`, wantDir: "~/l", wantScope: ScopeLocal, wantOff: true,
		},
		{
			name: "block with a user directory", user: toU, project: block,
			wantDir: "~/u", wantScope: ScopeUser,
		},
		{
			name: "block false is unset", user: `{"permissions": {"blockReadsOutsideWorkingDirectories": false}}`,
			project: `{"autoMemoryDirectory": "~/p"}`, wantDir: "~/p", wantScope: ScopeProject,
		},
	} {
		t.Run(tt.name, func(t *testing.T) { runAutoMemoryCase(t, tt) })
	}
}
