package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// Vault is a handle to a brain vault on disk. Operations preserve atomicity
// for single-file writes (tmp + rename) and best-effort rollback for
// multi-file operations like Promote.
type Vault struct {
	Root string
}

// Open returns a Vault rooted at the given directory. Errors if the directory
// does not exist.
func Open(root string) (*Vault, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("open vault %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vault root %s is not a directory", root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Vault{Root: abs}, nil
}

// TierDir returns the absolute directory path for a tier.
func (v *Vault) TierDir(t Tier) string {
	return filepath.Join(v.Root, t.Dir())
}

// WriteOptions bundles the inputs needed to create a new note in a tier.
type WriteOptions struct {
	Tier     Tier
	Title    string
	Body     string
	Tags     []string
	Aliases  []string
	Type     string // note | source | moc | output (defaults based on tier)
	Status   string // raw | validated | archived (defaults based on tier)
	Source   string // optional URL
	Evidence string // optional trust tier: A | B | C | D | mixed — empty for non-empirical
	Filename string // optional; defaults to Slug(Title) + ".md" (YYYY-MM-DD- prefix for outputs)
	// Set assigns frontmatter keys after the tier defaults, so it can
	// override them. Typed keys decode into their fields; any other key is
	// stored as is (see markdown.Frontmatter.Set).
	Set []markdown.Field
	// Unset removes frontmatter keys after Set is applied.
	Unset []string
	Now   time.Time
}

// Write creates a new note. Returns the absolute path of the created file.
// Errors with ErrExists if the target already exists. Outputs always carry
// the meta/output tag, whatever Set and Unset say.
func (v *Vault) Write(opts WriteOptions) (string, error) {
	if opts.Title == "" {
		return "", fmt.Errorf("title is required")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	fm := markdown.Frontmatter{
		Title:    opts.Title,
		Aliases:  opts.Aliases,
		Tags:     opts.Tags,
		Type:     opts.Type,
		Status:   opts.Status,
		Source:   opts.Source,
		Evidence: opts.Evidence,
		Created:  opts.Now.UTC().Format("2006-01-02"),
		Updated:  opts.Now.UTC().Format("2006-01-02"),
	}
	if fm.Type == "" {
		fm.Type = defaultType(opts.Tier)
	}
	if fm.Status == "" {
		fm.Status = defaultStatus(opts.Tier)
	}
	if err := applyFields(&fm, opts.Set, opts.Unset); err != nil {
		return "", err
	}
	// Outputs are synthesis artifacts that cite many notes — including them
	// as graph nodes turns every output into a hub by audit-trail design.
	// Tag them meta/output so the user can filter outputs out of the graph
	// while preserving the wikilink-based audit trail.
	if opts.Tier == TierOutputs && !fm.HasTag("meta/output") {
		fm.Tags = append([]string{"meta/output"}, fm.Tags...)
	}

	filename := opts.Filename
	if filename == "" {
		slug := Slug(opts.Title)
		if opts.Tier == TierOutputs {
			slug = opts.Now.UTC().Format("2006-01-02") + "-" + slug
		}
		filename = slug + ".md"
	}
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}

	dir := v.TierDir(opts.Tier)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir tier %s: %w", dir, err)
	}
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%w: %s", ErrExists, v.Rel(path))
	}

	content, err := markdown.Render(fm, []byte(ensureTrailingNewline(opts.Body)))
	if err != nil {
		return "", err
	}
	if err := atomicWrite(path, content, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ReadFrontmatter reads the note at the absolute path and parses its
// frontmatter and body. A missing note is ErrNotFound.
func (v *Vault) ReadFrontmatter(path string) (markdown.Frontmatter, []byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return markdown.Frontmatter{}, nil, fmt.Errorf("%w: %s", ErrNotFound, v.Rel(path))
	}
	if err != nil {
		return markdown.Frontmatter{}, nil, err
	}
	fm, body, err := markdown.ParseFrontmatter(data)
	if err != nil {
		return markdown.Frontmatter{}, nil, fmt.Errorf("%s: %w", v.Rel(path), err)
	}
	return fm, body, nil
}

// EditOptions replaces the body and/or merges frontmatter fields. At most one
// of Body / Append may be set. FrontmatterPatch is a partial Frontmatter that
// overlays the existing one (non-zero fields win). Set and Unset then assign
// and remove individual keys, in that order.
type EditOptions struct {
	Body             string
	Append           string
	FrontmatterPatch *markdown.Frontmatter
	Set              []markdown.Field
	Unset            []string
	Now              time.Time
}

// Edit modifies an existing note. Body, Append, and FrontmatterPatch can each
// be set independently. Updates frontmatter.Updated to Now (defaults to
// time.Now()).
func (v *Vault) Edit(path string, opts EditOptions) error {
	if opts.Body != "" && opts.Append != "" {
		return fmt.Errorf("edit: body and append are mutually exclusive")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	fm, body, err := v.ReadFrontmatter(path)
	if err != nil {
		return err
	}
	if opts.FrontmatterPatch != nil {
		fm = mergeFrontmatter(fm, *opts.FrontmatterPatch)
	}
	if err := applyFields(&fm, opts.Set, opts.Unset); err != nil {
		return err
	}
	switch {
	case opts.Body != "":
		body = []byte(ensureTrailingNewline(opts.Body))
	case opts.Append != "":
		buf := make([]byte, 0, len(body)+len(opts.Append)+1)
		buf = append(buf, body...)
		if len(buf) > 0 && buf[len(buf)-1] != '\n' {
			buf = append(buf, '\n')
		}
		buf = append(buf, []byte(ensureTrailingNewline(opts.Append))...)
		body = buf
	}
	fm.Touch(opts.Now)
	content, err := markdown.Render(fm, body)
	if err != nil {
		return err
	}
	return atomicWrite(path, content, 0o644)
}

// PromoteOptions controls a raw→validated move.
type PromoteOptions struct {
	TargetTier Tier   // default TierNotes
	NewTitle   string // if set, rename (and re-slug) the note
	MOC        string // if set, append a bullet to this MOC path (relative to vault or absolute)
	Now        time.Time
}

// PromoteResult describes the outcome of a Promote call.
type PromoteResult struct {
	OldPath          string
	NewPath          string
	BacklinkRewrites []string // paths of files whose wikilinks were rewritten
	MOCAppended      string   // path of the MOC appended to, if any
}

// Promote moves a note from its current tier (must be TierInbox) to
// TargetTier (default TierNotes). Updates frontmatter.Status to "validated",
// rewrites [[oldStem]] wikilinks across the vault to point at the new stem,
// optionally appends to a MOC. Best-effort rollback on failure.
func (v *Vault) Promote(path string, opts PromoteOptions) (PromoteResult, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	target := opts.TargetTier
	if target == "" {
		target = TierNotes
	}

	fm, body, err := v.ReadFrontmatter(path)
	if err != nil {
		return PromoteResult{}, err
	}

	fromTier, err := TierOf(v.Root, path)
	if err != nil {
		return PromoteResult{}, err
	}
	if fromTier != TierInbox {
		return PromoteResult{}, fmt.Errorf("promote: source must be in inbox/, got %s", fromTier)
	}

	// Validate moc target up front so a bad target (batch-provenance index)
	// rejects the call before any move happens — no half-done state.
	if opts.MOC != "" {
		mocAbs := opts.MOC
		if !filepath.IsAbs(mocAbs) {
			mocAbs = filepath.Join(v.Root, mocAbs)
		}
		isBatch, err := isBatchIndexMOC(mocAbs)
		if err != nil {
			return PromoteResult{}, fmt.Errorf("check moc target %s: %w", mocAbs, err)
		}
		if isBatch {
			return PromoteResult{}, fmt.Errorf(
				"moc target %s is a batch-provenance index (filename matches research-index-* / kickoff-*, or has tag meta/index); pass a topical MOC or omit moc",
				mocAbs,
			)
		}
	}

	oldStem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	title := fm.Title
	if opts.NewTitle != "" {
		title = opts.NewTitle
		fm.Title = opts.NewTitle
	}
	if title == "" {
		title = oldStem
	}
	newStem := Slug(title)
	newPath := filepath.Join(v.TierDir(target), newStem+".md")
	if _, err := os.Stat(newPath); err == nil {
		return PromoteResult{}, fmt.Errorf("%w: promote target %s", ErrExists, v.Rel(newPath))
	}

	fm.Status = "validated"
	fm.Touch(opts.Now)
	content, err := markdown.Render(fm, body)
	if err != nil {
		return PromoteResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return PromoteResult{}, err
	}
	// Write new file, delete old only after success.
	if err := atomicWrite(newPath, content, 0o644); err != nil {
		return PromoteResult{}, err
	}

	// Rewrite backlinks across the vault.
	rewrites, rewriteErr := rewriteWikilinkTarget(v.Root, oldStem, newStem)
	if rewriteErr != nil {
		// Roll back the new file; a failed rollback is reported with the cause.
		rollbackErr := os.Remove(newPath)
		return PromoteResult{}, errors.Join(
			fmt.Errorf("rewrite backlinks: %w", rewriteErr),
			rollbackErr,
		)
	}

	if err := os.Remove(path); err != nil {
		// Try to roll back rewrites? Not worth it for best-effort; leave a warning.
		return PromoteResult{}, fmt.Errorf(
			"remove old path %s after writing %s: %w",
			path,
			newPath,
			err,
		)
	}

	res := PromoteResult{
		OldPath:          path,
		NewPath:          newPath,
		BacklinkRewrites: rewrites,
	}

	if opts.MOC != "" {
		mocPath := opts.MOC
		if !filepath.IsAbs(mocPath) {
			mocPath = filepath.Join(v.Root, mocPath)
		}
		if err := appendMOCBullet(mocPath, title, newStem, opts.Now); err != nil {
			return res, fmt.Errorf("append to MOC %s: %w", mocPath, err)
		}
		res.MOCAppended = mocPath
	}
	return res, nil
}

func defaultType(t Tier) string {
	switch t {
	case TierOutputs:
		return "output"
	case TierMOCs:
		return "moc"
	default:
		return "note"
	}
}

func defaultStatus(t Tier) string {
	switch t {
	case TierInbox:
		return "raw"
	case TierNotes, TierMOCs:
		return "validated"
	case TierOutputs:
		return "validated"
	}
	return "raw"
}

func mergeFrontmatter(base, patch markdown.Frontmatter) markdown.Frontmatter {
	if patch.Title != "" {
		base.Title = patch.Title
	}
	if len(patch.Aliases) > 0 {
		base.Aliases = patch.Aliases
	}
	if len(patch.Tags) > 0 {
		base.Tags = patch.Tags
	}
	if patch.Type != "" {
		base.Type = patch.Type
	}
	if patch.Status != "" {
		base.Status = patch.Status
	}
	if patch.Source != "" {
		base.Source = patch.Source
	}
	if patch.Evidence != "" {
		base.Evidence = patch.Evidence
	}
	if patch.ID != "" {
		base.ID = patch.ID
	}
	return base
}

// applyFields assigns set, then removes unset, on fm.
func applyFields(fm *markdown.Frontmatter, set []markdown.Field, unset []string) error {
	for _, f := range set {
		if err := fm.Set(f.Key, f.Value); err != nil {
			return err
		}
	}
	for _, key := range unset {
		fm.Unset(key)
	}
	return nil
}

func ensureTrailingNewline(s string) string {
	if s == "" {
		return ""
	}
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
