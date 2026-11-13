package ui_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/server/ui"
)

const bundle = "assets/brain-graph.bundle.js"

// sums parses SHA256SUMS: one "<hex>  <name>" line per file.
func sums(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(read(t, "assets/SHA256SUMS")))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		sum, name, ok := strings.Cut(line, "  ")
		if !ok {
			t.Fatalf("SHA256SUMS line %q: want \"<sha256>  <file>\"", line)
		}
		out[name] = sum
	}
	return out
}

func TestFilesHoldTheContractFiles(t *testing.T) {
	for _, name := range []string{"shell.html", "app.js", "graph.js", bundle, "assets/SHA256SUMS"} {
		if _, err := fs.Stat(ui.Files(), name); err != nil {
			t.Errorf("Files() lacks %s: %v", name, err)
		}
	}
}

// TestBundleMatchesSHA256SUMS fails on a hand edit of the bundle: make bundle
// writes the bundle and its sum together, so only a rebuild keeps them equal.
func TestBundleMatchesSHA256SUMS(t *testing.T) {
	want := sums(t)
	if len(want) == 0 {
		t.Fatal("SHA256SUMS is empty; run make bundle")
	}
	for name, sum := range want {
		got := sha256.Sum256([]byte(read(t, "assets/"+name)))
		if hex.EncodeToString(got[:]) != sum {
			t.Errorf(
				"assets/%s sha256 = %x, SHA256SUMS says %s; rebuild with make bundle",
				name,
				got,
				sum,
			)
		}
	}
	if _, ok := want["brain-graph.bundle.js"]; !ok {
		t.Errorf("SHA256SUMS does not list brain-graph.bundle.js")
	}
}

func TestBundleSetsBrainSigma(t *testing.T) {
	src := read(t, bundle)
	if !strings.Contains(src, "BrainSigma") {
		t.Errorf("%s does not set window.BrainSigma", bundle)
	}
}
