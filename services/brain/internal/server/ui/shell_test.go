package ui_test

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// TestShellLoadsScriptsInOrder pins the load order docs/serve.md relies on:
// webkit, then the bundle, then graph.js, then app.js.
func TestShellLoadsScriptsInOrder(t *testing.T) {
	shell := read(t, "shell.html")
	order := []string{
		`src="/webkit/boot.js"`,
		`href="/webkit/webkit.css"`,
		`src="/webkit/webkit.js"`,
		`src="/assets/brain-graph.bundle.js"`,
		`src="/graph.js"`,
		`src="/app.js"`,
	}
	last := -1
	for _, s := range order {
		i := strings.Index(shell, s)
		if i < 0 {
			t.Fatalf("shell.html lacks %s", s)
		}
		if i < last {
			t.Errorf("shell.html loads %s out of order", s)
		}
		last = i
	}
	for _, s := range []string{`<wk-header brand="brain"`, `id="app"`} {
		if !strings.Contains(shell, s) {
			t.Errorf("shell.html lacks %s", s)
		}
	}
}

// jsList reads `var NAME = [...];` out of graph.js.
func jsList(t *testing.T, src, name string) []string {
	t.Helper()
	m := regexp.MustCompile(`var ` + name + ` = (\[[^\]\n]*\]);`).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("graph.js has no one-line `var %s = [...]`", name)
	}
	var out []string
	if err := json.Unmarshal([]byte(m[1]), &out); err != nil {
		t.Fatalf("graph.js %s is not a JSON list: %v", name, err)
	}
	return out
}

// TestGraphListsMatchGo keeps graph.js's tiers and statuses in step with the
// Go lists, so a new status gets a colour and a filter entry.
func TestGraphListsMatchGo(t *testing.T) {
	src := read(t, "graph.js")
	if got := jsList(t, src, "STATUSES"); !slices.Equal(got, lifecycle.Statuses) {
		t.Errorf("graph.js STATUSES = %v, lifecycle.Statuses = %v", got, lifecycle.Statuses)
	}
	var tiers []string
	for _, tier := range vault.AllTiers {
		tiers = append(tiers, string(tier))
	}
	if got := jsList(t, src, "TIERS"); !slices.Equal(got, tiers) {
		t.Errorf("graph.js TIERS = %v, vault.AllTiers = %v", got, tiers)
	}
}
