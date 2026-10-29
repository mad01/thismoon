// Package export turns a stored page into one HTML file that opens anywhere,
// offline included, with no present server behind it. What is ours goes into
// the file: webkit's stylesheet and scripts, the page shell's stylesheet,
// app.js and viz.js, the rendered body, the page data app.js reads in place
// of the API, and the page's local images as data URIs. The third-party
// browser libraries load from a public npm CDN at the versions the asset
// cache holds, each pinned by an integrity hash of the cached file, and only
// the ones the page's blocks use. The fonts' latin faces are inlined. See
// docs/adr/0029-present-standalone-export.md.
package export

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/present/internal/images"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// The renditions an export can carry.
const (
	viewBrief = "brief"
	viewDeck  = "deck"
)

// Pages is the one store read an export makes.
type Pages interface {
	Get(ctx context.Context, id string) (store.Page, error)
}

// Exporter builds standalone files from a page store and the workdir that
// holds the asset cache (assets/) and the image store (images/).
type Exporter struct {
	pages   Pages
	workdir string
}

// New returns an Exporter over pages and workdir.
func New(pages Pages, workdir string) *Exporter {
	return &Exporter{pages: pages, workdir: workdir}
}

// Request names the page to export. Deck asks for its slide deck; without
// it the brief is exported, or the deck when the page has no brief.
type Request struct {
	ID   string
	Deck bool
}

// Result is one exported file. Rendition is "brief" or "deck", Libraries
// the CDN URLs the file loads in order, and Notes what the file does
// without, such as fonts missing from the cache.
type Result struct {
	HTML      []byte
	Rendition string
	Libraries []string
	Notes     []string
}

// Export builds the standalone file for one page.
func (e *Exporter) Export(ctx context.Context, req Request) (Result, error) {
	p, err := e.pages.Get(ctx, req.ID)
	if err != nil {
		return Result{}, err
	}
	view, body, err := rendition(p, req.Deck)
	if err != nil {
		return Result{}, err
	}
	body, err = inlineImages(images.New(e.workdir).Dir(), body)
	if err != nil {
		return Result{}, err
	}
	body += referencesHTML(p.References)
	assets := filepath.Join(e.workdir, "assets")
	scripts, err := cdnScripts(assets, needs(body, p.Graph))
	if err != nil {
		return Result{}, err
	}
	fonts, fontNote, err := latinFonts(assets)
	if err != nil {
		return Result{}, err
	}
	doc, err := newDocument(parts{page: p, view: view, body: body, fonts: fonts, scripts: scripts})
	if err != nil {
		return Result{}, err
	}
	out, err := doc.render()
	if err != nil {
		return Result{}, err
	}
	return newResult(out, view, scripts, fontNote), nil
}

func newResult(out []byte, view string, scripts []cdnScript, notes ...string) Result {
	res := Result{HTML: out, Rendition: view}
	for _, s := range scripts {
		res.Libraries = append(res.Libraries, s.URL)
	}
	for _, n := range notes {
		if n != "" {
			res.Notes = append(res.Notes, n)
		}
	}
	return res
}

// rendition picks the body to export: the deck when asked for or when the
// page has no brief, the brief otherwise.
func rendition(p store.Page, deck bool) (string, string, error) {
	if deck && !p.HasDeck {
		return "", "", store.ErrNoDeck
	}
	if deck || (p.Content == "" && p.HasDeck) {
		return viewDeck, p.Deck, nil
	}
	return viewBrief, p.Content, nil
}

// inlineImages swaps every stored image the body shows (/img/<name>) for a
// data URI of the file in dir.
func inlineImages(dir, body string) (string, error) {
	for _, name := range images.Referenced(body) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", fmt.Errorf("export: image %s: %w", name, err)
		}
		body = strings.ReplaceAll(
			body,
			images.Served(name),
			dataURI(images.ContentType(name), data),
		)
	}
	return body, nil
}

// dataURI is data as a base64 data URI of the given media type.
func dataURI(mediaType string, data []byte) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// FileName is the default name of an exported file: <id>.html for the
// brief and <id>-deck.html for the deck.
func FileName(id, rendition string) string {
	if rendition == viewDeck {
		return id + "-deck.html"
	}
	return id + ".html"
}

// WriteFile writes an exported file to path through a temporary file in
// the same directory and a rename, so a reader never sees half of it. It
// creates the directory when it is missing.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("export: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".export-*.html")
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	// Removing fails harmlessly once the rename has moved the file.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() // the write error is the one to report
		return fmt.Errorf("export: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("export: write %s: %w", path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("export: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("export: %w", err)
	}
	return nil
}
