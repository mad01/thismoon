package k8sstore

import (
	"context"
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// cachedPage is what the page cache keeps for one Page object: the name,
// namespace, labels, and resourceVersion it was seen at, and the page
// without its content, graph, or references. An object that fails to
// decode keeps the error instead, so it surfaces on read rather than
// breaking the watch.
type cachedPage struct {
	metav1.ObjectMeta
	page store.Page
	err  error
}

// newCachedPage slims a Page object down to what the cache keeps.
func newCachedPage(u *unstructured.Unstructured) *cachedPage {
	c := &cachedPage{ObjectMeta: metav1.ObjectMeta{
		Name:            u.GetName(),
		Namespace:       u.GetNamespace(),
		Labels:          u.GetLabels(),
		ResourceVersion: u.GetResourceVersion(),
	}}
	rec, err := fromObject(u)
	c.page, c.err = withoutBodies(rec.Page), err
	return c
}

// snapshot returns the cached page with its pointer fields copied, so a
// caller that edits what it got cannot reach into the cache.
func (c *cachedPage) snapshot() store.Page {
	p := c.page
	if p.ExpiresAt != nil {
		t := *p.ExpiresAt
		p.ExpiresAt = &t
	}
	if p.Shared != nil {
		s := *p.Shared
		if s.ExpiresAt != nil {
			t := *s.ExpiresAt
			s.ExpiresAt = &t
		}
		p.Shared = &s
	}
	return p
}

// slimPage is the informer's transform. client-go may hand it an object it
// has already transformed, so a cachedPage passes through unchanged.
func slimPage(obj any) (any, error) {
	switch o := obj.(type) {
	case *cachedPage:
		return o, nil
	case *unstructured.Unstructured:
		return newCachedPage(o), nil
	default:
		return obj, nil
	}
}

// pageCache mirrors the namespace's Page objects in memory from one
// list-and-watch against the API server. It keeps metadata only: a page's
// bodies can reach a mebibyte, and every replica holds its own copy. It is
// read-only and runs no reconcile loop; writes still go to the API server.
type pageCache struct {
	informer cache.SharedIndexInformer
	ns       string
}

// newPageCache builds the informer from the dynamic client directly rather
// than through client-go's dynamicinformer package, which imports the typed
// informers for every built-in API group and with them all of k8s.io/api.
func newPageCache(client dynamic.Interface, ns string) (*pageCache, error) {
	pages := client.Resource(GVR).Namespace(ns)
	lw := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return pages.List(ctx, o)
		},
		WatchFuncWithContext: func(ctx context.Context, o metav1.ListOptions) (watch.Interface, error) {
			return pages.Watch(ctx, o)
		},
	}
	inf := cache.NewSharedIndexInformerWithOptions(
		cache.ToListWatcherWithWatchListSemantics(lw, client),
		&unstructured.Unstructured{},
		cache.SharedIndexInformerOptions{ObjectDescription: GVR.String()},
	)
	if err := inf.SetTransform(slimPage); err != nil {
		return nil, fmt.Errorf("k8sstore: page cache transform: %w", err)
	}
	return &pageCache{informer: inf, ns: ns}, nil
}

// run lists and watches until ctx is done, logging when it starts and
// again once it holds the namespace's pages. It returns only after its
// sync-watching goroutine has, so nothing logs after run is gone.
func (c *pageCache) run(ctx context.Context, logf func(string, ...any)) {
	logf("present: page cache watching pages in %s", c.ns)
	var wg sync.WaitGroup
	wg.Go(func() {
		if cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
			logf("present: page cache synced pages=%d", len(c.informer.GetStore().ListKeys()))
		}
	})
	c.informer.RunWithContext(ctx)
	wg.Wait()
}

// synced reports whether the cache has completed its first list, after
// which it is fit to answer reads. It is false before run is called.
func (c *pageCache) synced() bool { return c.informer.HasSynced() }

// get returns the cached page for id, or store.ErrNotFound when the cache
// holds no such object.
func (c *pageCache) get(id string) (store.Page, error) {
	obj, ok, err := c.informer.GetStore().GetByKey(c.ns + "/" + id)
	if err != nil {
		return store.Page{}, fmt.Errorf("page cache get %s: %w", id, err)
	}
	if !ok {
		return store.Page{}, store.ErrNotFound
	}
	cp, err := asCachedPage(obj)
	if err != nil {
		return store.Page{}, err
	}
	if cp.err != nil {
		return store.Page{}, cp.err
	}
	return cp.snapshot(), nil
}

// ephemeral returns every cached page labeled ephemeral, the same set the
// sweeper's label-selected list returns from the API server.
func (c *pageCache) ephemeral() ([]*cachedPage, error) {
	var out []*cachedPage
	for _, obj := range c.informer.GetStore().List() {
		cp, err := asCachedPage(obj)
		if err != nil {
			return nil, err
		}
		if cp.Labels[LabelEphemeral] != "true" {
			continue
		}
		if cp.err != nil {
			return nil, cp.err
		}
		out = append(out, cp)
	}
	return out, nil
}

func asCachedPage(obj any) (*cachedPage, error) {
	cp, ok := obj.(*cachedPage)
	if !ok {
		return nil, fmt.Errorf("k8sstore: page cache holds a %T", obj)
	}
	return cp, nil
}
