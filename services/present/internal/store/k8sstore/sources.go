package k8sstore

import (
	"context"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// Sources live inside the page object, so each save or delete is one
// read-modify-write that leaves the version alone.

func (s *Store) SaveDoc(ctx context.Context, id string, doc []byte) error {
	_, err := s.mutate(ctx, id, func(rec *record) { rec.Doc = doc; rec.Page.HasDoc = true })
	return err
}

func (s *Store) LoadDoc(ctx context.Context, id string) ([]byte, error) {
	rec, _, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec.Doc == nil {
		return nil, store.ErrNotFound
	}
	return rec.Doc, nil
}

func (s *Store) HasDoc(ctx context.Context, id string) bool {
	rec, _, err := s.get(ctx, id)
	return err == nil && rec.Doc != nil
}

func (s *Store) DeleteDoc(ctx context.Context, id string) error {
	_, err := s.mutate(ctx, id, func(rec *record) { rec.Doc = nil; rec.Page.HasDoc = false })
	return err
}

func (s *Store) SaveGraphSource(ctx context.Context, id string, src []byte) error {
	_, err := s.mutate(ctx, id, func(rec *record) { rec.GraphSource = src })
	return err
}

func (s *Store) LoadGraphSource(ctx context.Context, id string) ([]byte, error) {
	rec, _, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec.GraphSource == nil {
		return nil, store.ErrNotFound
	}
	return rec.GraphSource, nil
}

func (s *Store) HasGraphSource(ctx context.Context, id string) bool {
	rec, _, err := s.get(ctx, id)
	return err == nil && rec.GraphSource != nil
}

func (s *Store) DeleteGraphSource(ctx context.Context, id string) error {
	_, err := s.mutate(ctx, id, func(rec *record) { rec.GraphSource = nil })
	return err
}
