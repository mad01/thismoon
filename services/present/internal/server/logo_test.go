package server

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestEmbeddedLogoMatchesDocs keeps internal/server/logo.png a byte-identical
// copy of docs/assets/logo.png, so the mark a deck shows is the repo's.
func TestEmbeddedLogoMatchesDocs(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "assets", "logo.png"))
	if err != nil {
		t.Fatalf("read docs/assets/logo.png: %v", err)
	}
	if !bytes.Equal(src, logoPNG) {
		t.Fatal(
			"docs/assets/logo.png differs from internal/server/logo.png; copy the source file over",
		)
	}
	if !bytes.HasPrefix(logoPNG, []byte("\x89PNG")) {
		t.Error("embedded logo is not a PNG")
	}
}

// The logo is served from the binary in both modes, revalidated by a
// content ETag rather than pinned by an immutable cache header.
func TestLogoRoute(t *testing.T) {
	ts, _ := setup(t)
	shared := setupShared(t)
	for name, base := range map[string]string{"local": ts.URL, "shared": shared.ts.URL} {
		t.Run(name, func(t *testing.T) {
			resp, err := http.Get(base + "/logo.png")
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET /logo.png = %d, want 200", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
				t.Errorf("Content-Type = %q, want image/png", ct)
			}
			if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
				t.Errorf("Cache-Control = %q, want no-cache", cc)
			}
			etag := resp.Header.Get("ETag")
			if etag == "" {
				t.Fatal("no ETag")
			}
			if !bytes.Equal(body, logoPNG) {
				t.Errorf("body is %d bytes, want the %d embedded", len(body), len(logoPNG))
			}

			req, _ := http.NewRequest(http.MethodGet, base+"/logo.png", nil)
			req.Header.Set("If-None-Match", etag)
			resp, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNotModified {
				t.Errorf("GET with matching If-None-Match = %d, want 304", resp.StatusCode)
			}
		})
	}
}

// The deck chrome is wired across three files: the renderer's island class,
// the view code that reads it and builds the strip, and the shell's CSS for
// the strip. Pin the names together so one cannot drift from the others.
func TestDeckChromeWiring(t *testing.T) {
	app := string(appJS)
	for _, want := range []string{
		"deck-chrome", "/logo.png", "logo_position", "chrome.progress", "chrome.presenter",
		"chrome.footer", "deck-strip", "deck-dot", "deck-progress-fill", "deck-logo-corner",
		"slide ' + (i + 1) + ' of '",
	} {
		if !contains(app, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	shell := string(shellHTML)
	for _, want := range []string{
		".deck-strip", ".deck-dot.done", ".deck-dot.current", ".deck-progress-fill",
		".deck-logo-top-left", ".deck-logo-top-right", ".deck-bar.raised", ".deck.has-strip",
		"html.has-deck-strip.presenting .slide.active", ".brief-presenter",
	} {
		if !contains(shell, want) {
			t.Errorf("shell.html lacks %q", want)
		}
	}
}
