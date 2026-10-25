package layers

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Write prints p as a table: one row per effective setting, with the layer
// it came from. Paths under home print in ~ form.
func (p Policy) Write(w io.Writer, home string) error {
	local := "none"
	if p.LocalPath != "" {
		local = tilde(p.LocalPath, home)
	}
	env := "unset"
	if p.Env != "" {
		env = p.Env
	}
	fmt.Fprintf(w, "srt policy for %s\n", p.Label)
	fmt.Fprintf(w, "  base   %s\n", tilde(p.BasePath, home))
	fmt.Fprintf(w, "  local  %s\n", local)
	fmt.Fprintf(w, "  env    SRT_RUN_ALLOW_DOMAINS=%s\n\n", env)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, e := range p.Entries {
		value := e.Value
		if value == "" {
			value = "(empty)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Key, value, e.Layer)
	}
	return tw.Flush()
}

func tilde(path, home string) string {
	if home != "" && strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}
