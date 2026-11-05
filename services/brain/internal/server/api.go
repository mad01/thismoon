package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/check"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/stale"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Defaults of the list and search routes, the CLI's flag defaults.
const (
	defaultListLimit   = 100
	defaultSearchLimit = 50
	defaultHybridLimit = 20
)

// Search modes of /api/search.
const (
	modeLiteral = "literal"
	modeHybrid  = "hybrid"
)

type vaultsView struct {
	Vaults []vaultEntry `json:"vaults"`
}

type vaultEntry struct {
	Name    string `json:"name"`
	Shared  bool   `json:"shared"`
	Default bool   `json:"default"`
	// Notes counts the vault's notes in the four tiers; 0 for a vault serve
	// cannot read.
	Notes int `json:"notes"`
}

func (s *Server) handleVaults(w http.ResponseWriter, _ *http.Request) {
	m, err := s.current()
	if err != nil {
		writeError(w, err)
		return
	}
	counts := map[*vault.Vault]int{}
	for _, n := range m.sn.Notes() {
		if n.Tier() != "" {
			counts[n.Ref.Vault]++
		}
	}
	out := vaultsView{Vaults: []vaultEntry{}}
	for _, v := range s.lib.Set().Vaults() {
		out.Vaults = append(out.Vaults, vaultEntry{
			Name: v.Name, Shared: v.Shared, Default: v.Default, Notes: counts[v],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type stateView struct {
	Key    string       `json:"key"`
	Vaults []vaultPrint `json:"vaults"`
}

// handleState answers the UI's poll from the fingerprint memo alone; it
// never rebuilds the model.
func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	prints := s.fingerprints()
	writeJSON(w, http.StatusOK, stateView{Key: layoutKey(prints, false), Vaults: prints})
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := vault.ListOptions{Tag: q.Get("tag"), Status: q.Get("status")}
	var err error
	if opts.Limit, err = positiveInt(q.Get("limit"), "limit", defaultListLimit); err != nil {
		writeError(w, err)
		return
	}
	if q.Get("tier") == "" {
		writeError(w, badRequest("tier is required: inbox, notes, outputs or MOCs"))
		return
	}
	if opts.Tier, err = vault.ParseTier(q.Get("tier")); err != nil {
		writeError(w, err)
		return
	}
	m, err := s.current()
	if err != nil {
		writeError(w, err)
		return
	}
	set, err := m.scope(q.Get("vault"))
	if err != nil {
		writeError(w, err)
		return
	}
	listing, err := set.List(opts)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

type noteView struct {
	Ref         string                `json:"ref"`
	Vault       string                `json:"vault"`
	Path        string                `json:"path"`
	Frontmatter vault.FrontmatterView `json:"frontmatter"`
	Body        string                `json:"body"`
	SHA         string                `json:"sha"`
	Status      string                `json:"status"`
	Banner      string                `json:"banner"`
	Links       []vault.NoteLink      `json:"links"`
}

// handleNote returns a note as stored, with each wikilink resolved by the
// graph's resolver, so the browser renders links without resolving them.
func (s *Server) handleNote(w http.ResponseWriter, r *http.Request) {
	m, ref, err := s.locate(r)
	if err != nil {
		writeError(w, err)
		return
	}
	note, err := ref.Vault.Read(ref.Path, 0, 0)
	if err != nil {
		writeError(w, err)
		return
	}
	fm, body, err := ref.Vault.ReadFrontmatter(ref.Abs())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, noteView{
		Ref:         ref.String(),
		Vault:       ref.Vault.Name,
		Path:        ref.Path,
		Frontmatter: note.Frontmatter,
		Body:        string(body),
		SHA:         note.SHA,
		Status:      vault.StatusOf(ref.Path, fm),
		Banner:      note.Banner,
		Links:       m.sn.Links(ref.Vault, body),
	})
}

func (s *Server) handleBacklinks(w http.ResponseWriter, r *http.Request) {
	m, ref, err := s.locate(r)
	if err != nil {
		writeError(w, err)
		return
	}
	links, err := m.set.Backlinks(ref)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// locate resolves the ref parameter in the current model's vaults.
func (s *Server) locate(r *http.Request) (*model, vault.Ref, error) {
	name := strings.TrimSpace(r.URL.Query().Get("ref"))
	if name == "" {
		return nil, vault.Ref{}, badRequest("ref is required: vault:path, id:<ulid> or a bare path")
	}
	m, err := s.current()
	if err != nil {
		return nil, vault.Ref{}, err
	}
	ref, err := m.set.Locate(name)
	if err != nil {
		return nil, vault.Ref{}, err
	}
	return m, ref, nil
}

// searchParams is /api/search's query, parsed.
type searchParams struct {
	text           string
	mode           string
	vault          string
	tier           vault.Tier
	includeRevoked bool
	limit          int
}

func parseSearch(r *http.Request) (searchParams, error) {
	q := r.URL.Query()
	p := searchParams{text: q.Get("q"), mode: q.Get("mode"), vault: q.Get("vault")}
	if strings.TrimSpace(p.text) == "" {
		return p, badRequest("q is required")
	}
	switch p.mode {
	case "":
		p.mode = modeLiteral
	case modeLiteral, modeHybrid:
	default:
		return p, badRequest("mode must be %s or %s, got %q", modeLiteral, modeHybrid, p.mode)
	}
	var err error
	if p.includeRevoked, err = flag01(q.Get("include_revoked"), "include_revoked"); err != nil {
		return p, err
	}
	fallback := defaultSearchLimit
	if p.mode == modeHybrid {
		fallback = defaultHybridLimit
	}
	if p.limit, err = positiveInt(q.Get("limit"), "limit", fallback); err != nil {
		return p, err
	}
	if t := q.Get("tier"); t != "" {
		if p.tier, err = vault.ParseTier(t); err != nil {
			return p, err
		}
	}
	return p, nil
}

// handleSearch runs brain search or brain hybrid-search. Both refresh a
// stale index first, through library and under the vault's index lock.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	p, err := parseSearch(r)
	if err != nil {
		writeError(w, err)
		return
	}
	m, err := s.current()
	if err != nil {
		writeError(w, err)
		return
	}
	set, err := m.scope(p.vault)
	if err != nil {
		writeError(w, err)
		return
	}
	if p.mode == modeHybrid {
		res, err := s.lib.HybridSearch(r.Context(), set, library.HybridQuery{
			Text:           p.text,
			Tier:           string(p.tier),
			K:              p.limit,
			IncludeRevoked: p.includeRevoked,
			Rerank:         s.cfg.Semantic.RerankDefault,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}
	res, err := s.lib.Search(r.Context(), set, search.SearchOptions{
		Pattern:        p.text,
		Tier:           p.tier,
		Limit:          p.limit,
		IncludeRevoked: p.includeRevoked,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleStale is brain stale without --drift: Queue.Drift is false.
func (s *Server) handleStale(w http.ResponseWriter, _ *http.Request) {
	m, err := s.current()
	if err != nil {
		writeError(w, err)
		return
	}
	q, err := s.staleQueue(m, stale.DefaultLimit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

// staleQueue builds the stale queue over every tier note of the model that
// is not revoked, as brain stale does, showing limit notes per group.
func (s *Server) staleQueue(m *model, limit int) (stale.Queue, error) {
	var notes []*vault.NoteView
	for _, n := range m.sn.Notes() {
		if n.Tier() != "" && !lifecycle.Revoked(n.Status()) {
			notes = append(notes, n)
		}
	}
	return stale.Build(stale.Options{Snapshot: m.sn, Notes: notes, Now: s.now(), Limit: limit})
}

// handleCheck is brain check over every vault serve can read.
func (s *Server) handleCheck(w http.ResponseWriter, _ *http.Request) {
	m, err := s.current()
	if err != nil {
		writeError(w, err)
		return
	}
	report, err := check.Run(check.Options{Set: m.set, Now: s.now()})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

type promotionsView struct {
	Promotions []promotion `json:"promotions"`
}

// promotion is a private note waiting on its shared pull request.
type promotion struct {
	Vault    string `json:"vault"`
	Path     string `json:"path"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	SharedPR string `json:"shared_pr"`
	StatusAt string `json:"status_at"`
}

func (s *Server) handlePromotions(w http.ResponseWriter, _ *http.Request) {
	m, err := s.current()
	if err != nil {
		writeError(w, err)
		return
	}
	out := promotionsView{Promotions: []promotion{}}
	for _, n := range m.sn.Notes() {
		if n.Status() != lifecycle.Promoting {
			continue
		}
		f := lifecycle.Read(n.Frontmatter)
		out.Promotions = append(out.Promotions, promotion{
			Vault:    n.Ref.Vault.Name,
			Path:     n.Ref.Path,
			ID:       n.Frontmatter.ID,
			Title:    n.Frontmatter.Title,
			SharedPR: f.SharedPR,
			StatusAt: f.StatusAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// positiveInt parses an optional count parameter; empty is fallback.
func positiveInt(value, name string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, badRequest("%s must be a positive integer, got %q", name, value)
	}
	return n, nil
}

// flag01 parses an optional 0 or 1 parameter; empty is false.
func flag01(value, name string) (bool, error) {
	switch value {
	case "", "0":
		return false, nil
	case "1":
		return true, nil
	}
	return false, badRequest("%s must be 0 or 1, got %q", name, value)
}
