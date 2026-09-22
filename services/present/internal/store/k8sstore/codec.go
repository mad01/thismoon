package k8sstore

import (
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// pageSpec is the Page resource's spec as the API server stores it. Times
// are RFC 3339 strings: the converter treats them as plain strings and the
// CRD schema validates the format. Sources are JSON text kept opaque.
type pageSpec struct {
	Title       string      `json:"title"`
	Content     string      `json:"content"`
	Graph       string      `json:"graph,omitempty"`
	Doc         string      `json:"doc,omitempty"`
	GraphSource string      `json:"graphSource,omitempty"`
	References  []reference `json:"references,omitempty"`
	Version     int64       `json:"version"`
	Author      string      `json:"author,omitempty"`
	// Ephemeral is written even when false: it is a printer column, and an
	// omitted field renders as a blank cell in kubectl get pages rather
	// than as the "kept until deleted" it means.
	Ephemeral bool        `json:"ephemeral"`
	ExpiresAt string      `json:"expiresAt,omitempty"`
	CreatedAt string      `json:"createdAt"`
	UpdatedAt string      `json:"updatedAt"`
	Shared    *sharedInfo `json:"shared,omitempty"`
}

type reference struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type sharedInfo struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Ephemeral bool   `json:"ephemeral,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	SharedAt  string `json:"sharedAt"`
}

// record is a page as the store handles it: the Page plus the two sources
// the Store interface exposes through separate methods.
type record struct {
	Page        store.Page
	Doc         []byte
	GraphSource []byte
}

// specOf builds the spec for a record.
func specOf(rec record) pageSpec {
	p := rec.Page
	spec := pageSpec{
		Title:       p.Title,
		Content:     p.Content,
		Graph:       p.Graph,
		Doc:         string(rec.Doc),
		GraphSource: string(rec.GraphSource),
		Version:     int64(p.Version),
		Author:      p.Author,
		Ephemeral:   p.Ephemeral,
		ExpiresAt:   formatTime(p.ExpiresAt),
		CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:   p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	for _, r := range p.References {
		spec.References = append(spec.References, reference{Title: r.Title, URL: r.URL})
	}
	if s := p.Shared; s != nil {
		spec.Shared = &sharedInfo{
			ID:        s.ID,
			URL:       s.URL,
			Ephemeral: s.Ephemeral,
			ExpiresAt: formatTime(s.ExpiresAt),
			SharedAt:  s.SharedAt.UTC().Format(time.RFC3339Nano),
		}
	}
	return spec
}

// setSpec writes rec into u's spec and labels, refusing a page over
// maxBytes before anything reaches the API server.
func setSpec(u *unstructured.Unstructured, rec record, maxBytes int) error {
	spec := specOf(rec)
	raw, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("encode page: %w", err)
	}
	if len(raw) > maxBytes {
		return fmt.Errorf("%w: %d bytes, limit %d", store.ErrTooLarge, len(raw), maxBytes)
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		return fmt.Errorf("convert page: %w", err)
	}
	u.Object["spec"] = m
	labels := u.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	if rec.Page.Ephemeral {
		labels[LabelEphemeral] = "true"
	} else {
		delete(labels, LabelEphemeral)
	}
	u.SetLabels(labels)
	return nil
}

// newObject builds a fresh Page object for rec, named by its id.
func newObject(rec record, maxBytes int) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": group + "/" + version,
		"kind":       kind,
		"metadata":   map[string]any{"name": rec.Page.ID},
	}}
	if err := setSpec(u, rec, maxBytes); err != nil {
		return nil, err
	}
	return u, nil
}

// fromObject reads a Page object back into a record.
func fromObject(u *unstructured.Unstructured) (record, error) {
	raw, found, err := unstructured.NestedMap(u.Object, "spec")
	if err != nil || !found {
		return record{}, fmt.Errorf("page %s: missing spec", u.GetName())
	}
	var spec pageSpec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec); err != nil {
		return record{}, fmt.Errorf("page %s: decode spec: %w", u.GetName(), err)
	}
	p := store.Page{
		ID:        u.GetName(),
		Title:     spec.Title,
		Content:   spec.Content,
		Graph:     spec.Graph,
		Version:   int(spec.Version),
		HasGraph:  spec.Graph != "",
		HasRefs:   len(spec.References) > 0,
		HasDoc:    spec.Doc != "",
		Author:    spec.Author,
		Ephemeral: spec.Ephemeral,
	}
	for _, r := range spec.References {
		p.References = append(p.References, store.Reference{Title: r.Title, URL: r.URL})
	}
	if p.CreatedAt, err = parseTime(spec.CreatedAt); err != nil {
		return record{}, fmt.Errorf("page %s: createdAt: %w", p.ID, err)
	}
	if p.UpdatedAt, err = parseTime(spec.UpdatedAt); err != nil {
		return record{}, fmt.Errorf("page %s: updatedAt: %w", p.ID, err)
	}
	if p.ExpiresAt, err = parseOptionalTime(spec.ExpiresAt); err != nil {
		return record{}, fmt.Errorf("page %s: expiresAt: %w", p.ID, err)
	}
	if s := spec.Shared; s != nil {
		p.Shared = &store.SharedInfo{ID: s.ID, URL: s.URL, Ephemeral: s.Ephemeral}
		if p.Shared.ExpiresAt, err = parseOptionalTime(s.ExpiresAt); err != nil {
			return record{}, fmt.Errorf("page %s: shared.expiresAt: %w", p.ID, err)
		}
		if p.Shared.SharedAt, err = parseTime(s.SharedAt); err != nil {
			return record{}, fmt.Errorf("page %s: shared.sharedAt: %w", p.ID, err)
		}
	}
	rec := record{Page: p}
	if spec.Doc != "" {
		rec.Doc = []byte(spec.Doc)
	}
	if spec.GraphSource != "" {
		rec.GraphSource = []byte(spec.GraphSource)
	}
	return rec, nil
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func parseOptionalTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := parseTime(s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
