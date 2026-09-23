package k8sstore

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// signaled waits for a signal on ch.
func signaled(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("no signal after %s", what)
	}
}

// signaledUntil waits for signals on ch until, after one of them, cond
// holds. The informer runs its handlers after updating its cache and
// asynchronously, so a watcher can still receive a late signal for an
// earlier event; a signal means "re-read", not "this change".
func signaledUntil(t *testing.T, ch <-chan struct{}, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-ch:
			if cond() {
				return
			}
		case <-deadline:
			t.Fatalf("no signal after %s showed it", what)
		}
	}
}

// quiet asserts ch holds no pending signal right now.
func quiet(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("signal after %s, want none", what)
	default:
	}
}

// TestWatchPageFollowsTheWatch runs the feed against a running cache: a
// watcher hears a version bump and a delete as the informer delivers them.
func TestWatchPageFollowsTheWatch(t *testing.T) {
	f := fakeCachedFixture(t)
	ctx := context.Background()
	p, err := f.st.Create(ctx, store.Draft{Title: "T", Content: "<p>1</p>"})
	if err != nil {
		t.Fatal(err)
	}
	waitForMeta(t, f.st, p.ID, func(_ store.Page, err error) bool { return err == nil })
	changed, stop := f.st.WatchPage(p.ID)
	defer stop()

	content := "<p>2</p>"
	if _, err := f.st.Update(ctx, p.ID, store.Patch{Content: &content}); err != nil {
		t.Fatal(err)
	}
	signaledUntil(t, changed, "a version bump", func() bool {
		m, err := f.st.GetMeta(ctx, p.ID)
		return err == nil && m.Version == 2
	})

	if err := f.st.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	signaledUntil(t, changed, "a delete", func() bool {
		_, err := f.st.GetMeta(ctx, p.ID)
		return errors.Is(err, store.ErrNotFound)
	})
}

// TestFeedSkipsUpdatesThatKeepTheVersion pins the filter: a source save or
// a share record changes the object but not what a tab shows, so it wakes
// no one. The handler is driven directly, so the check is not a race with
// the informer.
func TestFeedSkipsUpdatesThatKeepTheVersion(t *testing.T) {
	feed := newPageFeed()
	changed, stop := feed.watch("p1")
	defer stop()
	h := feed.handler()
	v1 := &cachedPage{ObjectMeta: metav1.ObjectMeta{Name: "p1"}, page: store.Page{Version: 1}}
	v1again := &cachedPage{ObjectMeta: metav1.ObjectMeta{Name: "p1"}, page: store.Page{
		Version: 1, HasDoc: true,
	}}
	v2 := &cachedPage{ObjectMeta: metav1.ObjectMeta{Name: "p1"}, page: store.Page{Version: 2}}

	h.OnUpdate(v1, v1again)
	quiet(t, changed, "an update that kept the version")
	h.OnUpdate(v1, v2)
	signaled(t, changed, "an update that moved the version")
	h.OnAdd(v1, false)
	signaled(t, changed, "an add")
	h.OnDelete(cache.DeletedFinalStateUnknown{Key: "present-test/p1", Obj: nil})
	signaled(t, changed, "a tombstone delete")
}

// TestFeedCoalescesAndReleases pins the channel contract: a burst leaves one
// pending signal rather than blocking the informer, other pages' changes do
// not leak in, and stop releases the watch for good.
func TestFeedCoalescesAndReleases(t *testing.T) {
	feed := newPageFeed()
	changed, stop := feed.watch("p1")
	feed.notify("p1")
	feed.notify("p1")
	feed.notify("p2")
	signaled(t, changed, "a burst")
	quiet(t, changed, "draining the one coalesced signal")

	stop()
	stop() // a second call must not panic
	if n := feed.watchers("p1"); n != 0 {
		t.Fatalf("watchers after stop = %d, want 0", n)
	}
	feed.notify("p1")
	quiet(t, changed, "a notify after stop")
}

func TestVersionMoved(t *testing.T) {
	page := func(v int) *cachedPage { return &cachedPage{page: store.Page{Version: v}} }
	broken := &cachedPage{err: errors.New("undecodable")}
	for _, tc := range []struct {
		name     string
		old, new any
		want     bool
	}{
		{"same version", page(3), page(3), false},
		{"new version", page(3), page(4), true},
		{"undecodable new object", page(3), broken, true},
		{"undecodable old object", broken, page(3), true},
		{"foreign object", page(3), "not a page", true},
	} {
		if got := versionMoved(tc.old, tc.new); got != tc.want {
			t.Errorf("%s: versionMoved = %v, want %v", tc.name, got, tc.want)
		}
	}
}
