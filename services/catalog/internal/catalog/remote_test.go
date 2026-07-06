package catalog

import "testing"

func TestOriginURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config string
		want   string
		ok     bool
	}{
		{
			name: "ssh origin",
			config: "[core]\n\trepositoryformatversion = 0\n" +
				"[remote \"origin\"]\n\turl = git@github.com:mad01/dotfiles.git\n\tfetch = +refs/heads/*\n",
			want: "git@github.com:mad01/dotfiles.git",
			ok:   true,
		},
		{
			name: "https origin after another remote",
			config: "[remote \"upstream\"]\n\turl = git@github.com:other/dotfiles.git\n" +
				"[remote \"origin\"]\n\turl = https://github.com/mad01/thismoon.git\n",
			want: "https://github.com/mad01/thismoon.git",
			ok:   true,
		},
		{name: "no origin", config: "[remote \"upstream\"]\n\turl = git@github.com:other/x.git\n", want: "", ok: false},
		{name: "empty", config: "", want: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := originURL(tt.config)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("originURL() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestRemoteToHostSlug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		remote     string
		host, slug string
		ok         bool
	}{
		{"scp ssh", "git@github.com:mad01/dotfiles.git", "github.com", "mad01/dotfiles", true},
		{"scp ssh no suffix", "git@github.com:mad01/dotfiles", "github.com", "mad01/dotfiles", true},
		{"ssh url", "ssh://git@git.example.com/team/repo.git", "git.example.com", "team/repo", true},
		{"https", "https://github.com/mad01/thismoon.git", "github.com", "mad01/thismoon", true},
		{"https no suffix trailing slash", "https://github.com/mad01/thismoon/", "github.com", "mad01/thismoon", true},
		{"garbage", "not-a-remote", "", "", false},
		{"empty", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host, slug, ok := remoteToHostSlug(tt.remote)
			if host != tt.host || slug != tt.slug || ok != tt.ok {
				t.Fatalf("remoteToHostSlug(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.remote, host, slug, ok, tt.host, tt.slug, tt.ok)
			}
		})
	}
}

func TestRepoWebURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		host, slug, sub string
		want            string
	}{
		{"root empty", "github.com", "mad01/thismoon", "", "https://github.com/mad01/thismoon"},
		{"root dot", "github.com", "mad01/thismoon", ".", "https://github.com/mad01/thismoon"},
		{"subdir", "github.com", "mad01/dotfiles", "present", "https://github.com/mad01/dotfiles/tree/HEAD/present"},
		{"nested subdir", "github.com", "mad01/dotfiles", "recipes/d-man", "https://github.com/mad01/dotfiles/tree/HEAD/recipes/d-man"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := repoWebURL(tt.host, tt.slug, tt.sub); got != tt.want {
				t.Fatalf("repoWebURL(%q,%q,%q) = %q, want %q", tt.host, tt.slug, tt.sub, got, tt.want)
			}
		})
	}
}

func TestRepoURLFor(t *testing.T) {
	t.Parallel()
	cfg := "[remote \"origin\"]\n\turl = git@github.com:mad01/dotfiles.git\n"
	tests := []struct {
		name                 string
		config, root, source string
		want                 string
	}{
		{"root file", cfg, "/repos/dotfiles", "/repos/dotfiles/service-info.yaml", "https://github.com/mad01/dotfiles"},
		{"component subdir", cfg, "/repos/dotfiles", "/repos/dotfiles/present/service-info.yaml", "https://github.com/mad01/dotfiles/tree/HEAD/present"},
		{"trailing slash root", cfg, "/repos/dotfiles/", "/repos/dotfiles/present/service-info.yaml", "https://github.com/mad01/dotfiles/tree/HEAD/present"},
		{"no remote", "", "/repos/dotfiles", "/repos/dotfiles/service-info.yaml", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RepoURLFor(tt.config, tt.root, tt.source); got != tt.want {
				t.Fatalf("RepoURLFor() = %q, want %q", got, tt.want)
			}
		})
	}
}
