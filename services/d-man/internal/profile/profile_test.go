package profile

import "testing"

func TestRender(t *testing.T) {
	const home = "/home/u"
	tests := []struct {
		name      string
		base      string
		blocklist []byte
		want      string
	}{
		{
			name: "no block list renders the base alone",
			base: "suffix = \"this\"\n",
			want: "suffix = \"this\"\n",
		},
		{
			name:      "block list goes above the base",
			base:      "[[route]]\nname = \"csl\"\nport = 7424\n",
			blocklist: []byte("blocklist = [\"a.example\"]\n"),
			want:      "blocklist = [\"a.example\"]\n[[route]]\nname = \"csl\"\nport = 7424\n",
		},
		{
			name:      "a block list without a final newline does not run into the base",
			base:      "suffix = \"this\"\n",
			blocklist: []byte("blocklist = []"),
			want:      "blocklist = []\nsuffix = \"this\"\n",
		},
		{
			name: "games_dir ~/ becomes home in either quote style and any spacing",
			base: "games_dir = \"~/g\"\n  games_dir='~/g'\n",
			want: "games_dir = \"/home/u/g\"\n  games_dir='/home/u/g'\n",
		},
		{
			name: "only a games_dir key line is rewritten",
			base: "# games_dir = \"~/g\"\nother = \"~/g\"\ngames_dir = \"/abs/~/g\"\n",
			want: "# games_dir = \"~/g\"\nother = \"~/g\"\ngames_dir = \"/abs/~/g\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(Render([]byte(tt.base), tt.blocklist, home))
			if got != tt.want {
				t.Errorf("Render =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestCheckName(t *testing.T) {
	for _, name := range []string{"work", "personal", "focus-2", "a"} {
		if err := CheckName(name); err != nil {
			t.Errorf("CheckName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range []string{"", "Work", "../x", "a/b", "-x", "x-", "a.b", "*"} {
		if err := CheckName(name); err == nil {
			t.Errorf("CheckName(%q) = nil, want an error", name)
		}
	}
}
