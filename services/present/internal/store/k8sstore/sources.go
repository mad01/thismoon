package k8sstore

import (
	"context"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// Sources live inside the page object, so a read is one get and a write is
// part of the Edit that bumps the version.

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

func (s *Store) LoadDeckSource(ctx context.Context, id string) ([]byte, error) {
	rec, _, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec.DeckSource == nil {
		return nil, store.ErrNotFound
	}
	return rec.DeckSource, nil
}

func (s *Store) HasDeckSource(ctx context.Context, id string) bool {
	rec, _, err := s.get(ctx, id)
	return err == nil && rec.DeckSource != nil
}
