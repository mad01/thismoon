package mcpserver

import (
	"errors"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/server"
	"github.com/mad01/thismoon/services/present/internal/sharedclient"
	"github.com/mad01/thismoon/services/present/internal/store"
)

var sharedIDSuffix = regexp.MustCompile(`/p/[0-9a-f]{32}$`)

func TestShareToolRegisteredOnlyLocallyWithASharer(t *testing.T) {
	// The tool never talks to the instance at registration, so a dead
	// address is enough here.
	sharer := sharedclient.New("http://127.0.0.1:1", "k")

	s, err := New("test", Config{
		Workdir: t.TempDir(), Port: 7423, Checks: noChecks, Sharer: sharer,
	})
	if err != nil {
		t.Fatalf("New(local): %v", err)
	}
	got := toolNames(t, s)
	want := []string{
		"present_create", "present_read", "present_source", "present_update",
		"present_list", "present_open", "present_share", "present_doctor",
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("local tools with sharer = %v, want %v", got, want)
	}
	mcptest.VerifyToolAnnotations(t, s)

	shared, err := New("test", Config{
		Workdir: t.TempDir(), Mode: ModeShared, Checks: noChecks, Sharer: sharer,
	})
	if err != nil {
		t.Fatalf("New(shared): %v", err)
	}
	if slices.Contains(toolNames(t, shared), "present_share") {
		t.Error("a shared instance must not offer present_share even with a sharer configured")
	}
}

// sharedInstance starts a real shared-mode HTTP server over its own store.
func sharedInstance(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(st, server.Options{
		Mode: server.ModeShared, Workdir: dir, Info: buildinfo.Info{Version: "test"},
	}).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestShareToolPushesAndRecordsTheLink(t *testing.T) {
	ts := sharedInstance(t)
	local, err := store.NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	h := &handlers{
		store:   local,
		mode:    ModeLocal,
		baseURL: "http://localhost:7423",
		now:     func() time.Time { return now },
		open:    func(string) error { return nil },
		sharer:  sharedclient.New(ts.URL, author.NewKey()),
	}
	ctx := t.Context()

	_, created, err := h.handleCreate(ctx, nil, createInput{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, out, err := h.handleShare(ctx, nil, shareInput{ID: created.ID, Ephemeral: true})
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	if !strings.HasPrefix(out.URL, ts.URL) || !sharedIDSuffix.MatchString(out.URL) {
		t.Errorf("url = %q, want %s/p/<32 hex>", out.URL, ts.URL)
	}
	if !out.Ephemeral {
		t.Error("ephemeral share must report ephemeral")
	}
	if out.ExpiresAt == "" {
		t.Error("ephemeral share must report an expiry")
	} else if _, err := time.Parse(time.RFC3339, out.ExpiresAt); err != nil {
		t.Errorf("expires_at %q is not RFC3339: %v", out.ExpiresAt, err)
	}
	if want := now.Format(time.RFC3339); out.SharedAt != want {
		t.Errorf("shared_at = %q, want the fixed clock %q", out.SharedAt, want)
	}

	p, err := h.store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("local page after share: %v", err)
	}
	if p.Shared == nil {
		t.Fatal("local page carries no shared record")
	}
	if p.Shared.URL != out.URL || !p.Shared.Ephemeral || !p.Shared.SharedAt.Equal(now) {
		t.Errorf("shared record = %+v, want url %q ephemeral at %v", p.Shared, out.URL, now)
	}
	if p.Shared.ExpiresAt == nil || p.Shared.ExpiresAt.UTC().Format(time.RFC3339) != out.ExpiresAt {
		t.Errorf("shared record expiry = %v, want %q", p.Shared.ExpiresAt, out.ExpiresAt)
	}
	if !strings.HasSuffix(out.URL, "/p/"+p.Shared.ID) {
		t.Errorf("shared record id %q does not match url %q", p.Shared.ID, out.URL)
	}

	_, _, err = h.handleShare(ctx, nil, shareInput{ID: store.NewID()})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("share of a missing page: err = %v, want ErrNotFound", err)
	}
}
