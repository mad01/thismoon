package trace

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb"
)

func TestPromptLine(t *testing.T) {
	long := strings.Repeat("ab ", 60)
	cases := []struct{ name, prompt, want string }{
		{"first line", "fix the bug\nand more", "fix the bug"},
		{"blank lines first", "\n  \n  fix it  \n", "fix it"},
		{
			"tag line skipped",
			"<teammate-message teammate_id=\"lead\" summary=\"x\">\nbuild it\n",
			"build it",
		},
		{"inline tags dropped", "<command-name>/review</command-name> now", "/review now"},
		{"spaces collapsed", "a \t  b", "a b"},
		{"only tags", "<a>\n</a>\n", ""},
		{
			"long",
			long,
			string([]rune(strings.TrimSpace(long))[:moondb.SessionPromptLineRunes]) + "…",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PromptLine(c.prompt); got != c.want {
				t.Errorf("PromptLine(%q) = %q, want %q", c.prompt, got, c.want)
			}
		})
	}
}

// TestFormatTotalsCost: totals with no cost read as no cost when nothing
// was billed, and as unknown when a model has no price.
func TestFormatTotalsCost(t *testing.T) {
	cases := []struct {
		name string
		t    Totals
		want string
	}{
		{"nothing billed", Totals{}, "-"},
		{"unpriced", Totals{Unpriced: []string{"m"}}, "unknown"},
		{"priced", Totals{Cost: new(1.5)}, "$1.50"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FormatTotalsCost(c.t); got != c.want {
				t.Errorf("FormatTotalsCost = %q, want %q", got, c.want)
			}
		})
	}
}
