package k8sstore

import (
	"context"
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// startCache runs st's page cache until the test ends and waits for it to
// sync. Cleanup waits for RunCache to return, so nothing it starts outlives
// the test.
func startCache(t *testing.T, st *Store) *logCollector {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var logs logCollector
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunCache(ctx, st, logs.logf)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	waitFor(t, st.cache.synced)
	return &logs
}

func fakeCachedFixture(t *testing.T) *fixture {
	t.Helper()
	f := fakeFixture(t)
	startCache(t, f.st)
	return f
}

// waitForMeta polls GetMeta for id until ok accepts what it returns.
func waitForMeta(t *testing.T, st *Store, id string, ok func(store.Page, error) bool) {
	t.Helper()
	waitFor(t, func() bool { return ok(st.GetMeta(context.Background(), id)) })
}

// runCacheConformance checks that a running page cache follows the
// namespace: GetMeta sees creates, updates, and deletes once the watch
// delivers them, and the sweeper judges expiry from the cache. The fake
// client and the kind cluster both run it.
func runCacheConformance(t *testing.T, newFixture func(t *testing.T) *fixture) {
	t.Helper()
	t.Run("GetMetaFollowsTheWatch", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		p, err := f.st.Create(ctx, store.Draft{
			Title: "T", Content: "<p>v1</p>", Graph: "cy.init();",
			References: []store.Reference{{Title: "r", URL: "https://r"}},
			Doc:        []byte(`{"sections":[]}`),
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		waitForMeta(t, f.st, p.ID, func(m store.Page, err error) bool {
			return err == nil && m.Version == 1
		})
		m, err := f.st.GetMeta(ctx, p.ID)
		if err != nil {
			t.Fatalf("GetMeta: %v", err)
		}
		if m.Content != "" || m.Graph != "" || m.References != nil {
			t.Fatalf("cached meta carries bodies: %+v", m)
		}
		if m.Title != "T" || !m.HasGraph || !m.HasRefs || !m.HasDoc {
			t.Fatalf("cached meta lost metadata: %+v", m)
		}

		content := "<p>v2</p>"
		if _, err := f.st.Update(ctx, p.ID, store.Patch{Content: &content}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		waitForMeta(t, f.st, p.ID, func(m store.Page, err error) bool {
			return err == nil && m.Version == 2
		})

		if err := f.st.Delete(ctx, p.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		waitForMeta(t, f.st, p.ID, func(_ store.Page, err error) bool {
			return errors.Is(err, store.ErrNotFound)
		})
	})

	t.Run("SweepJudgesFromTheCache", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		past, future := f.now.Add(-time.Hour), f.now.Add(time.Hour)
		gone, err := f.st.Create(ctx, store.Draft{
			Title: "Gone", Content: "<p>x</p>", Ephemeral: true, ExpiresAt: &past,
		})
		if err != nil {
			t.Fatal(err)
		}
		live, err := f.st.Create(ctx, store.Draft{
			Title: "Live", Content: "<p>x</p>", Ephemeral: true, ExpiresAt: &future,
		})
		if err != nil {
			t.Fatal(err)
		}
		kept, err := f.st.Create(ctx, store.Draft{Title: "Kept", Content: "<p>x</p>"})
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{gone.ID, live.ID, kept.ID} {
			waitForMeta(t, f.st, id, func(_ store.Page, err error) bool { return err == nil })
		}

		n, err := f.st.SweepExpired(ctx, f.now)
		if err != nil || n != 1 {
			t.Fatalf("SweepExpired = %d, %v; want 1, nil", n, err)
		}
		if _, err := f.st.Get(ctx, gone.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("expired page: err = %v, want ErrNotFound", err)
		}
		for _, id := range []string{live.ID, kept.ID} {
			if _, err := f.st.Get(ctx, id); err != nil {
				t.Errorf("page %s swept or unreadable: %v", id, err)
			}
		}
	})
}

func TestFakeClientCacheConformance(t *testing.T) {
	runCacheConformance(t, fakeCachedFixture)
}

// TestCachedGetMetaSkipsAPIServer pins the reason the cache exists: once it
// has synced, the version poll every open tab makes costs no API request.
func TestCachedGetMetaSkipsAPIServer(t *testing.T) {
	client := newFakeClient()
	f := fixtureOn(t, client)
	ctx := context.Background()
	p, err := f.st.Create(ctx, store.Draft{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	startCache(t, f.st)

	before := len(client.Actions())
	for range 3 {
		if m, err := f.st.GetMeta(ctx, p.ID); err != nil || m.Version != 1 {
			t.Fatalf("GetMeta = %+v, %v", m, err)
		}
	}
	for _, a := range client.Actions()[before:] {
		if a.GetVerb() == "get" {
			t.Fatalf("cached GetMeta reached the API server: %v", a)
		}
	}
}

// TestCachedSweepDoesNotList pins the sweeper's switch to the cache: once
// it has synced, a sweep lists nothing from the API server.
func TestCachedSweepDoesNotList(t *testing.T) {
	client := newFakeClient()
	f := fixtureOn(t, client)
	startCache(t, f.st)

	before := len(client.Actions())
	if _, err := f.st.SweepExpired(context.Background(), f.now); err != nil {
		t.Fatal(err)
	}
	for _, a := range client.Actions()[before:] {
		if a.GetVerb() == "list" {
			t.Fatalf("cached sweep listed from the API server: %v", a)
		}
	}
}

// TestSweepDeletesAtTheResourceVersionItJudged pins the delete precondition:
// the sweeper names the resourceVersion it saw the page at, so the API
// server refuses the delete if the page has changed since.
func TestSweepDeletesAtTheResourceVersionItJudged(t *testing.T) {
	for _, cached := range []bool{false, true} {
		name := "FromTheAPIServer"
		if cached {
			name = "FromTheCache"
		}
		t.Run(name, func(t *testing.T) {
			client := newFakeClient()
			f := fixtureOn(t, client)
			ctx := context.Background()
			exp := f.now.Add(-time.Hour)
			p, err := f.st.Create(ctx, store.Draft{
				Title: "Gone", Content: "<p>x</p>", Ephemeral: true, ExpiresAt: &exp,
			})
			if err != nil {
				t.Fatal(err)
			}
			// The fake tracker keeps resourceVersions beside objects rather
			// than in them, so stamp one the sweeper can see.
			const seen = "42"
			u, err := client.Resource(GVR).Namespace(testNamespace).
				Get(ctx, p.ID, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			u.SetResourceVersion(seen)
			if err := client.Tracker().Update(GVR, u, testNamespace); err != nil {
				t.Fatal(err)
			}
			if cached {
				startCache(t, f.st)
			}

			var got *metav1.Preconditions
			client.PrependReactor("delete", "pages",
				func(a k8stesting.Action) (bool, runtime.Object, error) {
					got = a.(k8stesting.DeleteAction).GetDeleteOptions().Preconditions
					return false, nil, nil
				})
			n, err := f.st.SweepExpired(ctx, f.now)
			if err != nil || n != 1 {
				t.Fatalf("SweepExpired = %d, %v; want 1, nil", n, err)
			}
			if got == nil || got.ResourceVersion == nil || *got.ResourceVersion != seen {
				t.Fatalf("delete preconditions = %+v, want resourceVersion %s", got, seen)
			}
		})
	}
}

// TestSweepSkipsPageChangedSinceSeen covers the refused delete: the API
// server answers Conflict because the page moved on, and the sweeper
// leaves it for the next sweep instead of failing or counting it.
func TestSweepSkipsPageChangedSinceSeen(t *testing.T) {
	client := newFakeClient()
	f := fixtureOn(t, client)
	ctx := context.Background()
	exp := f.now.Add(-time.Hour)
	p, err := f.st.Create(ctx, store.Draft{
		Title: "Moved on", Content: "<p>x</p>", Ephemeral: true, ExpiresAt: &exp,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.PrependReactor("delete", "pages", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(GVR.GroupResource(), p.ID,
			errors.New("the object has been modified"))
	})

	n, err := f.st.SweepExpired(ctx, f.now)
	if err != nil || n != 0 {
		t.Fatalf("SweepExpired = %d, %v; want 0, nil", n, err)
	}
	if _, err := f.st.Get(ctx, p.ID); err != nil {
		t.Fatalf("page gone after a refused delete: %v", err)
	}
}

// TestCachedGetMetaIsDetached guards the cache against its readers: editing
// the page GetMeta returned must not change what the next read sees.
func TestCachedGetMetaIsDetached(t *testing.T) {
	f := fakeFixture(t)
	ctx := context.Background()
	exp := f.now.Add(time.Hour)
	p, err := f.st.Create(ctx, store.Draft{
		Title: "T", Content: "<p>x</p>", Ephemeral: true, ExpiresAt: &exp,
	})
	if err != nil {
		t.Fatal(err)
	}
	startCache(t, f.st)

	first, err := f.st.GetMeta(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	*first.ExpiresAt = first.ExpiresAt.Add(-48 * time.Hour)
	again, err := f.st.GetMeta(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !again.ExpiresAt.Equal(exp) {
		t.Fatalf("cached expiry = %v after a caller edit, want %v", again.ExpiresAt, exp)
	}
}

// TestRunCacheLogsStartAndSync pins the two lines an operator looks for in
// pod logs: the cache started, and it holds the namespace's pages.
func TestRunCacheLogsStartAndSync(t *testing.T) {
	f := fakeFixture(t)
	if _, err := f.st.Create(context.Background(), store.Draft{
		Title: "T", Content: "<p>x</p>",
	}); err != nil {
		t.Fatal(err)
	}
	logs := startCache(t, f.st)
	waitFor(t, func() bool { return len(logs.snapshot()) == 2 })

	lines := logs.snapshot()
	if want := "present: page cache watching pages in " + testNamespace; lines[0] != want {
		t.Errorf("first log line = %q, want %q", lines[0], want)
	}
	if want := "present: page cache synced pages=1"; lines[1] != want {
		t.Errorf("sync log line = %q, want %q", lines[1], want)
	}
}
