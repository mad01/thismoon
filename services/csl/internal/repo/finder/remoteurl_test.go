package finder

import "testing"

func TestFileURL(t *testing.T) {
	tests := []struct {
		name string
		repo Repo
		file string
		line int
		want string
	}{
		{
			name: "github with line",
			repo: Repo{Name: "mad01/thismoon", Host: "github.com"},
			file: "internal/cli/root.go",
			line: 12,
			want: "https://github.com/mad01/thismoon/blob/HEAD/internal/cli/root.go#L12",
		},
		{
			name: "github enterprise host",
			repo: Repo{Name: "team/service", Host: "git.example.com"},
			file: "main.go",
			line: 1,
			want: "https://git.example.com/team/service/blob/HEAD/main.go#L1",
		},
		{
			name: "zero line omits fragment",
			repo: Repo{Name: "mad01/thismoon", Host: "github.com"},
			file: "README.md",
			line: 0,
			want: "https://github.com/mad01/thismoon/blob/HEAD/README.md",
		},
		{
			name: "leading slash trimmed",
			repo: Repo{Name: "mad01/thismoon", Host: "github.com"},
			file: "/internal/web/server.go",
			line: 5,
			want: "https://github.com/mad01/thismoon/blob/HEAD/internal/web/server.go#L5",
		},
		{
			name: "missing host returns empty",
			repo: Repo{Name: "mad01/thismoon", Host: ""},
			file: "main.go",
			line: 1,
			want: "",
		},
		{
			name: "missing name returns empty",
			repo: Repo{Name: "", Host: "github.com"},
			file: "main.go",
			line: 1,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FileURL(tt.repo, tt.file, tt.line)
			if got != tt.want {
				t.Errorf("FileURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
