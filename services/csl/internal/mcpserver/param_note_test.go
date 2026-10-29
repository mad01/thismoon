package mcpserver

import (
	"reflect"
	"strings"
	"testing"
)

func TestParamNameNote(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{query: "output_mode", want: true},
		{query: "output_mode context_lines", want: true},
		{query: "content", want: true},
		{query: "repo", want: true},
		{query: "output_mode foo", want: false},
		{query: "retry backoff", want: false},
		{query: "", want: false},
	}
	for _, tt := range tests {
		got := paramNameNote(tt.query)
		if (got != "") != tt.want {
			t.Errorf("paramNameNote(%q) = %q, want note=%v", tt.query, got, tt.want)
		}
		if got != "" && !strings.Contains(got, "parameter names") {
			t.Errorf("note %q does not say these are parameter names", got)
		}
	}
}

// TestSearchParamWordsCoverInputTags keeps the parameter-name trap in step
// with csl_search's input schema: every json tag of searchInput, including
// the embedded formatParam, is a word the trap recognises.
func TestSearchParamWordsCoverInputTags(t *testing.T) {
	var walk func(rt reflect.Type)
	walk = func(rt reflect.Type) {
		for i := range rt.NumField() {
			f := rt.Field(i)
			if f.Anonymous {
				walk(f.Type)
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if !searchParamWords[name] {
				t.Errorf("searchInput tag %q is missing from searchParamWords", name)
			}
		}
	}
	walk(reflect.TypeOf(searchInput{}))
}
