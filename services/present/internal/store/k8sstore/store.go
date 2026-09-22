// Package k8sstore keeps present pages as Page custom resources in one
// Kubernetes namespace. It is the store behind a shared instance: every
// replica talks to the API server directly, optimistic concurrency comes
// from resourceVersion, and a periodic sweeper on each replica deletes
// expired ephemeral pages. There is no controller and no leader.
package k8sstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// listPageSize bounds one List call; the store pages through the rest.
const listPageSize = 200

// Config configures a Store. Client and Namespace are required; Now
// defaults to time.Now and MaxBytes to present.MaxPageBytes.
type Config struct {
	Client    dynamic.Interface
	Namespace string
	Now       func() time.Time
	MaxBytes  int
}

// Store is the Kubernetes-backed page store.
type Store struct {
	client   dynamic.Interface
	ns       string
	now      func() time.Time
	maxBytes int
}

var _ store.Store = (*Store)(nil)

// New returns a Store over cfg.Client in cfg.Namespace.
func New(cfg Config) (*Store, error) {
	if cfg.Client == nil {
		return nil, errors.New("k8sstore: no client")
	}
	if cfg.Namespace == "" {
		return nil, errors.New("k8sstore: no namespace")
	}
	s := &Store{client: cfg.Client, ns: cfg.Namespace, now: cfg.Now, maxBytes: cfg.MaxBytes}
	if s.now == nil {
		s.now = time.Now
	}
	if s.maxBytes <= 0 {
		s.maxBytes = present.MaxPageBytes
	}
	return s, nil
}

// Namespace reports where the store keeps its pages.
func (s *Store) Namespace() string { return s.ns }

func (s *Store) pages() dynamic.ResourceInterface {
	return s.client.Resource(GVR).Namespace(s.ns)
}

// Create persists a new page. A draft without an id gets a fresh capability
// id; a name collision (astronomically unlikely) is retried once under a
// freshly minted id, whether the draft named the id or the store did, so
// the caller must read the id back from the returned page.
func (s *Store) Create(ctx context.Context, d store.Draft) (store.Page, error) {
	now := s.now().UTC()
	rec := record{
		Page: store.Page{
			ID:         d.ID,
			Title:      d.Title,
			Content:    d.Content,
			Graph:      d.Graph,
			References: d.References,
			Version:    1,
			HasGraph:   d.Graph != "",
			HasRefs:    len(d.References) > 0,
			HasDoc:     d.Doc != nil,
			CreatedAt:  now,
			UpdatedAt:  now,
			Author:     d.Author,
			Ephemeral:  d.Ephemeral,
			ExpiresAt:  d.ExpiresAt,
		},
		Doc:         d.Doc,
		GraphSource: d.GraphSource,
	}
	mint := rec.Page.ID == ""
	if !mint && !store.ValidID(rec.Page.ID) {
		return store.Page{}, fmt.Errorf("k8sstore: invalid page id %q", rec.Page.ID)
	}
	for attempt := 0; ; attempt++ {
		if mint {
			rec.Page.ID = store.NewSharedID()
		}
		obj, err := newObject(rec, s.maxBytes)
		if err != nil {
			return store.Page{}, err
		}
		_, err = s.pages().Create(ctx, obj, metav1.CreateOptions{})
		if err == nil {
			return rec.Page, nil
		}
		if apierrors.IsAlreadyExists(err) && attempt == 0 {
			// The id is taken. Mint a new one and try again, even when the
			// caller supplied it: on a shared instance the id is a
			// capability, so any unused one will do.
			mint = true
			continue
		}
		return store.Page{}, fmt.Errorf("create page %s: %w", rec.Page.ID, err)
	}
}

// Get loads a page by id. Returns store.ErrNotFound when it does not exist.
func (s *Store) Get(ctx context.Context, id string) (store.Page, error) {
	rec, _, err := s.get(ctx, id)
	return rec.Page, err
}

// get fetches the object and decodes it, keeping the object so a write
// can carry its resourceVersion back.
func (s *Store) get(ctx context.Context, id string) (record, *unstructured.Unstructured, error) {
	if !store.ValidID(id) {
		return record{}, nil, store.ErrNotFound
	}
	u, err := s.pages().Get(ctx, id, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return record{}, nil, store.ErrNotFound
	}
	if err != nil {
		return record{}, nil, fmt.Errorf("get page %s: %w", id, err)
	}
	rec, err := fromObject(u)
	if err != nil {
		return record{}, nil, err
	}
	return rec, u, nil
}

// mutate applies fn to the page under optimistic concurrency: on a
// resourceVersion conflict the read-modify-write runs again from a fresh
// read, so two replicas writing the same page never clobber each other.
func (s *Store) mutate(ctx context.Context, id string, fn func(rec *record)) (store.Page, error) {
	var out store.Page
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		rec, u, err := s.get(ctx, id)
		if err != nil {
			return err
		}
		fn(&rec)
		if err := setSpec(u, rec, s.maxBytes); err != nil {
			return err
		}
		if _, err := s.pages().Update(ctx, u, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update page %s: %w", id, err)
		}
		out = rec.Page
		return nil
	})
	return out, err
}

// Update applies patch, bumps the version, and stamps UpdatedAt.
func (s *Store) Update(ctx context.Context, id string, patch store.Patch) (store.Page, error) {
	return s.mutate(ctx, id, func(rec *record) {
		rec.Page.Apply(patch)
		rec.Page.Version++
		rec.Page.UpdatedAt = s.now().UTC()
	})
}

// Delete removes a page. Returns store.ErrNotFound when it does not exist.
func (s *Store) Delete(ctx context.Context, id string) error {
	if !store.ValidID(id) {
		return store.ErrNotFound
	}
	err := s.pages().Delete(ctx, id, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return store.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete page %s: %w", id, err)
	}
	return nil
}

// list pages through every object matching selector.
func (s *Store) list(ctx context.Context, selector string) ([]record, error) {
	var out []record
	cont := ""
	for {
		l, err := s.pages().List(ctx, metav1.ListOptions{
			Limit: listPageSize, Continue: cont, LabelSelector: selector,
		})
		if err != nil {
			return nil, fmt.Errorf("list pages: %w", err)
		}
		for i := range l.Items {
			rec, err := fromObject(&l.Items[i])
			if err != nil {
				return nil, err
			}
			out = append(out, rec)
		}
		cont = l.GetContinue()
		if cont == "" {
			return out, nil
		}
	}
}

// List returns every page fully loaded, newest update first.
func (s *Store) List(ctx context.Context) ([]store.Page, error) {
	recs, err := s.list(ctx, "")
	if err != nil {
		return nil, err
	}
	pages := make([]store.Page, 0, len(recs))
	for _, rec := range recs {
		pages = append(pages, rec.Page)
	}
	store.SortNewestFirst(pages)
	return pages, nil
}

// ListMeta returns every page without its content, graph, or references,
// newest update first.
func (s *Store) ListMeta(ctx context.Context) ([]store.Page, error) {
	pages, err := s.List(ctx)
	for i := range pages {
		pages[i].Content, pages[i].Graph, pages[i].References = "", "", nil
	}
	return pages, err
}

// SetShared records where the page was pushed, or clears it with nil. The
// version is untouched.
func (s *Store) SetShared(ctx context.Context, id string, info *store.SharedInfo) error {
	_, err := s.mutate(ctx, id, func(rec *record) { rec.Page.Shared = info })
	return err
}

// Ping lists one page, which exercises the client, the credentials, the
// namespace, and the CRD in one call.
func (s *Store) Ping(ctx context.Context) error {
	if _, err := s.pages().List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		return fmt.Errorf("list pages in %s: %w", s.ns, err)
	}
	return nil
}
