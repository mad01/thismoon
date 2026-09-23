package k8sstore

import (
	"sync"

	"k8s.io/client-go/tools/cache"
)

// pageFeed fans the page cache's informer events out to whoever watches a
// page. Each watcher's channel holds one pending signal, so a burst of
// changes coalesces into one wakeup and a slow reader never blocks the
// informer.
type pageFeed struct {
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}

func newPageFeed() *pageFeed {
	return &pageFeed{subs: map[string]map[chan struct{}]struct{}{}}
}

// watch registers interest in page id. The returned stop releases it and
// is safe to call more than once.
func (f *pageFeed) watch(id string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	f.mu.Lock()
	if f.subs[id] == nil {
		f.subs[id] = map[chan struct{}]struct{}{}
	}
	f.subs[id][ch] = struct{}{}
	f.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			delete(f.subs[id], ch)
			if len(f.subs[id]) == 0 {
				delete(f.subs, id)
			}
		})
	}
}

// notify wakes every watcher of page id without blocking.
func (f *pageFeed) notify(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for ch := range f.subs[id] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// watchers reports how many watchers page id has.
func (f *pageFeed) watchers(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs[id])
}

// handler turns informer events into notifications: a page appearing, its
// version moving, or the page going away. An update that leaves the
// version alone, such as a source save, wakes no one, because an open tab
// has nothing to reload.
func (f *pageFeed) handler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: f.notifyObject,
		UpdateFunc: func(oldObj, newObj any) {
			if versionMoved(oldObj, newObj) {
				f.notifyObject(newObj)
			}
		},
		DeleteFunc: f.notifyObject,
	}
}

func (f *pageFeed) notifyObject(obj any) {
	if name, ok := pageName(obj); ok {
		f.notify(name)
	}
}

// versionMoved reports whether an update changed what an open tab shows.
// Anything it cannot read as two decoded pages counts as moved, so a
// watcher re-reads rather than miss a change.
func versionMoved(oldObj, newObj any) bool {
	o, okOld := oldObj.(*cachedPage)
	n, okNew := newObj.(*cachedPage)
	if !okOld || !okNew || o.err != nil || n.err != nil {
		return true
	}
	return o.page.Version != n.page.Version
}

// pageName names the page an informer event is about, including a deletion
// the informer only learned of on relist, which arrives as a tombstone.
func pageName(obj any) (string, bool) {
	if t, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		_, name, err := cache.SplitMetaNamespaceKey(t.Key)
		return name, err == nil
	}
	cp, ok := obj.(*cachedPage)
	if !ok {
		return "", false
	}
	return cp.Name, true
}
