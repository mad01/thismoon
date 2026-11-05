package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/atomicfile"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/lockfile"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/noteid"
)

// Vault is a handle to a brain vault on disk. A single-file write is atomic
// (a uniquely named temporary file, renamed or linked into place), a new
// note never replaces an existing one, and a multi-file operation like
// Promote rolls back on a best-effort basis. Every change holds the vault's
// write lock when WriteLock is set.
type Vault struct {
	// Root is the absolute vault directory.
	Root string
	// Name is the vault's config name, the prefix of a qualified ref.
	Name string
	// Shared marks a team vault: Write, Edit and Promote refuse it with
	// ErrReadOnly.
	Shared bool
	// Default marks the vault a write targets when no vault is named.
	Default bool
	// WriteLock is the lock file Write, Edit, Promote and MigrateIDs hold,
	// so two brain processes never change the vault at once. Empty takes
	// no lock.
	WriteLock string
	// Committer records each change brain makes, under the write lock,
	// with the paths it touched: a git commit when the vault is a repo.
	// Nil records nothing.
	Committer Committer
}

// Committer records the files one brain change touched. gitvault.Repo is
// the one implementation: it commits exactly those paths and never pushes.
type Committer interface {
	Commit(message string, paths []string) error
}

// Change verbs: the word after "brain:" in the commit a change makes.
const (
	VerbWrite     = "write"
	VerbCapture   = "capture"
	VerbEdit      = "edit"
	VerbPromote   = "promote"
	VerbSupersede = "supersede"
	VerbRetract   = "retract"
	VerbWithdraw  = "withdraw"
	VerbFlag      = "flag"
	VerbKeep      = "keep"
	VerbArchive   = "archive"
	VerbMigrate   = "migrate"
	VerbShare     = "share"
	VerbUnshare   = "unshare"
	VerbRemove    = "remove"
)

// commit records a change through the Committer: the message is
// "brain: <subject>", then body when it is set. paths are vault-relative.
// A failed commit leaves the change on disk and says so.
func (v *Vault) commit(subject string, paths []string, body string) error {
	if v.Committer == nil || len(paths) == 0 {
		return nil
	}
	msg := "brain: " + subject
	if body != "" {
		msg += "\n\n" + body
	}
	if err := v.Committer.Commit(msg, paths); err != nil {
		return fmt.Errorf(
			"%s is changed on disk, but the git commit failed: %w "+
				"(commit it by hand, or set autocommit: false on vault %s)",
			paths[0], err, v.Name,
		)
	}
	return nil
}

// subjectOf is the commit subject of a change to rels: the verb and the
// first path, and how many more there are.
func subjectOf(verb string, rels []string) string {
	switch len(rels) {
	case 0:
		return verb
	case 1:
		return verb + " " + rels[0]
	}
	return fmt.Sprintf("%s %s and %d more", verb, rels[0], len(rels)-1)
}

// ContentSHA returns the hex sha256 of a note's bytes: the sha read reports
// and EditOptions.IfMatch compares.
func ContentSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// lockWrites takes the vault's write lock and returns its release.
func (v *Vault) lockWrites() (func() error, error) {
	if v.WriteLock == "" {
		return func() error { return nil }, nil
	}
	return lockfile.Hold(v.WriteLock)
}

// LockWrites takes the vault's write lock and returns its release, for a
// change brain makes outside the note methods, such as the pull and push
// of brain sync. The note methods take it themselves; holding it around
// one of them deadlocks.
func (v *Vault) LockWrites() (func() error, error) {
	return v.lockWrites()
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
	Status   string // a lifecycle status; defaults based on tier
	Source   string // optional URL
	Evidence string // optional trust tier: A | B | C | D | mixed — empty for non-empirical
	Filename string // optional; defaults to Slug(Title) + ".md" (YYYY-MM-DD- prefix for outputs)
	// Verb names the change in its commit: VerbWrite when empty,
	// VerbCapture for brain capture.
	Verb string
	// Set assigns frontmatter keys after the tier defaults, so it can
	// override them. Typed keys decode into their fields; any other key is
	// stored as is (see markdown.Frontmatter.Set).
	Set []markdown.Field
	// Unset removes frontmatter keys after Set is applied.
	Unset []string
	Now   time.Time
}

// Write creates a new note. Returns the absolute path of the created file.
// Content holding a secret fails with ErrSecretFound before anything is
// written; Edit, Promote and the status changes refuse it the same way.
// Errors with ErrExists if the target already exists, also when another
// writer creates it first. Outputs always carry the meta/output tag,
// whatever Set and Unset say. A note without an id after Set and Unset gets
// a fresh one, and its status gets its lifecycle defaults (expires_at for
// raw, review_by for validated) unless Set gave them. Write is Draft then
// Create; a caller that checks the draft first (library.Write looks for
// duplicates) calls the two itself.
func (v *Vault) Write(opts WriteOptions) (string, error) {
	d, err := v.Draft(opts)
	if err != nil {
		return "", err
	}
	return v.Create(d)
}

// Draft is a new note rendered and checked, not yet on disk.
type Draft struct {
	// Path is the absolute path the note goes to.
	Path        string
	Frontmatter markdown.Frontmatter
	Body        []byte
	content     []byte
	verb        string
}

// Content returns the note's bytes as Create writes them.
func (d Draft) Content() []byte { return d.content }

// Draft renders the note opts describes and runs every check Write makes
// before it touches disk: a shared vault is ErrReadOnly, a taken path
// ErrExists and a secret ErrSecretFound. Nothing is written.
func (v *Vault) Draft(opts WriteOptions) (Draft, error) {
	if err := v.writable(); err != nil {
		return Draft{}, err
	}
	if opts.Title == "" {
		return Draft{}, fmt.Errorf("title is required")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	fm, err := newFrontmatter(opts)
	if err != nil {
		return Draft{}, err
	}
	path := filepath.Join(v.TierDir(opts.Tier), noteFilename(opts))
	if _, err := os.Stat(path); err == nil {
		return Draft{}, fmt.Errorf("%w: %s", ErrExists, v.Rel(path))
	}
	body := []byte(ensureTrailingNewline(opts.Body))
	content, err := markdown.Render(fm, body)
	if err != nil {
		return Draft{}, err
	}
	if err := v.refuseSecrets(path, content); err != nil {
		return Draft{}, err
	}
	verb := opts.Verb
	if verb == "" {
		verb = VerbWrite
	}
	return Draft{Path: path, Frontmatter: fm, Body: body, content: content, verb: verb}, nil
}

// Create writes a draft under the vault's write lock and returns its path.
// It never replaces a note: one created since the draft is ErrExists.
func (v *Vault) Create(d Draft) (path string, err error) {
	if err := v.writable(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(d.Path), 0o755); err != nil {
		return "", fmt.Errorf("mkdir tier %s: %w", filepath.Dir(d.Path), err)
	}
	release, err := v.lockWrites()
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, release()) }()
	if err := v.create(d.Path, d.content); err != nil {
		return "", err
	}
	rel := v.Rel(d.Path)
	return d.Path, v.commit(subjectOf(d.verb, []string{rel}), []string{rel}, "")
}

// newFrontmatter builds a new note's frontmatter: the fields opts names,
// the tier's type and status defaults, Set and Unset, an id and the
// lifecycle defaults.
func newFrontmatter(opts WriteOptions) (markdown.Frontmatter, error) {
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
		return markdown.Frontmatter{}, err
	}
	if fm.ID == "" {
		fm.ID = noteid.New(opts.Now)
	}
	lifecycle.ApplyDefaults(&fm, opts.Now)
	// Outputs are synthesis artifacts that cite many notes; as graph nodes
	// every output would be a hub by audit-trail design. Tag them
	// meta/output so the graph can filter them out while the wikilink
	// audit trail stays.
	if opts.Tier == TierOutputs && !fm.HasTag("meta/output") {
		fm.Tags = append([]string{"meta/output"}, fm.Tags...)
	}
	return fm, nil
}

// noteFilename is the file a new note gets: opts.Filename, else the title's
// slug, date-prefixed in outputs.
func noteFilename(opts WriteOptions) string {
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
	return filename
}

// create writes a new note at path, failing with ErrExists when one is
// there already.
func (v *Vault) create(path string, content []byte) error {
	err := atomicfile.Create(path, content, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%w: %s", ErrExists, v.Rel(path))
	}
	return err
}

// ReadFrontmatter reads the note at the absolute path and parses its
// frontmatter and body. A missing note is ErrNotFound.
func (v *Vault) ReadFrontmatter(path string) (markdown.Frontmatter, []byte, error) {
	_, fm, body, err := v.readNote(path)
	return fm, body, err
}

// readNote reads the note at the absolute path: its bytes, frontmatter and
// body. A missing note is ErrNotFound.
func (v *Vault) readNote(path string) ([]byte, markdown.Frontmatter, []byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, markdown.Frontmatter{}, nil, fmt.Errorf("%w: %s", ErrNotFound, v.Rel(path))
	}
	if err != nil {
		return nil, markdown.Frontmatter{}, nil, err
	}
	fm, body, err := markdown.ParseFrontmatter(data)
	if err != nil {
		return nil, markdown.Frontmatter{}, nil, fmt.Errorf("%s: %w", v.Rel(path), err)
	}
	return data, fm, body, nil
}

// refuseUnclosed fails with ErrInvalidContent when data opens a
// frontmatter fence it never closes: rendering it would stack a second
// frontmatter block on top of the first.
func (v *Vault) refuseUnclosed(path string, data []byte) error {
	if markdown.Unclosed(data) {
		return fmt.Errorf("%w: %s: %w (add the closing --- line)",
			ErrInvalidContent, v.Rel(path), markdown.ErrUnclosed)
	}
	return nil
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
	// IfMatch, when set, is the sha the note had when the caller read it;
	// a note whose bytes changed since fails with ErrConflict.
	IfMatch string
	Now     time.Time
}

// Edit modifies an existing note and returns the sha of what it wrote.
// Body, Append, and FrontmatterPatch can each be set independently. Updates
// frontmatter.Updated to Now (defaults to time.Now()). An edit that changes
// the status records status_at and the new status's defaults. An edit that
// would change or remove the note's id fails with ErrImmutableID; a note
// without an id may be given one. With IfMatch set, a note that changed
// since the caller read it fails with ErrConflict and is left as it is.
func (v *Vault) Edit(path string, opts EditOptions) (sha string, err error) {
	if err := v.writable(); err != nil {
		return "", err
	}
	if opts.Body != "" && opts.Append != "" {
		return "", fmt.Errorf("edit: body and append are mutually exclusive")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	return v.rewrite(
		path,
		opts.IfMatch,
		VerbEdit,
		func(fm *markdown.Frontmatter, body []byte) ([]byte, error) {
			id, status := fm.ID, fm.Status
			if opts.FrontmatterPatch != nil {
				*fm = mergeFrontmatter(*fm, *opts.FrontmatterPatch)
			}
			if err := applyFields(fm, opts.Set, opts.Unset); err != nil {
				return nil, err
			}
			if id != "" && fm.ID != id {
				return nil, fmt.Errorf("%w: %s keeps id %s", ErrImmutableID, v.Rel(path), id)
			}
			lifecycle.Transition(fm, status, opts.Now)
			fm.Touch(opts.Now)
			return editedBody(body, opts), nil
		},
	)
}

// editedBody returns body with opts.Body replacing it or opts.Append added.
func editedBody(body []byte, opts EditOptions) []byte {
	switch {
	case opts.Body != "":
		return []byte(ensureTrailingNewline(opts.Body))
	case opts.Append != "":
		buf := make([]byte, 0, len(body)+len(opts.Append)+1)
		buf = append(buf, body...)
		if len(buf) > 0 && buf[len(buf)-1] != '\n' {
			buf = append(buf, '\n')
		}
		return append(buf, []byte(ensureTrailingNewline(opts.Append))...)
	}
	return body
}

// rewrite changes the note at path in place under the write lock. It
// refuses a note whose sha is no longer ifMatch (when set) with
// ErrConflict, lets change edit the frontmatter and return the new body,
// replaces the file atomically and commits it as verb. It returns the sha
// of what it wrote. The caller checks that the vault is writable.
func (v *Vault) rewrite(
	path, ifMatch, verb string,
	change func(fm *markdown.Frontmatter, body []byte) ([]byte, error),
) (sha string, err error) {
	return v.rewriteWith(path, ifMatch, verb, "", change)
}

// rewriteWith is rewrite with a body for the commit message.
func (v *Vault) rewriteWith(
	path, ifMatch, verb, commitBody string,
	change func(fm *markdown.Frontmatter, body []byte) ([]byte, error),
) (sha string, err error) {
	release, err := v.lockWrites()
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, release()) }()
	data, fm, body, err := v.readNote(path)
	if err != nil {
		return "", err
	}
	if err := v.refuseUnclosed(path, data); err != nil {
		return "", err
	}
	if got := ContentSHA(data); ifMatch != "" && got != ifMatch {
		return "", fmt.Errorf(
			"%w: %s has sha %s, not %s; read it again before editing",
			ErrConflict, v.Rel(path), got, ifMatch,
		)
	}
	if body, err = change(&fm, body); err != nil {
		return "", err
	}
	content, err := markdown.Render(fm, body)
	if err != nil {
		return "", err
	}
	if err := v.refuseSecrets(path, content); err != nil {
		return "", err
	}
	if err := atomicWrite(path, content, 0o644); err != nil {
		return "", err
	}
	rel := v.Rel(path)
	return ContentSHA(content), v.commit(subjectOf(verb, []string{rel}), []string{rel}, commitBody)
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
// TargetTier (default TierNotes). Updates frontmatter.Status to "validated"
// with its lifecycle defaults (status_at, review_by, no expires_at),
// points every wikilink in the vault that resolves to the note at its new
// stem or path (see linkRetarget), optionally appends to a MOC. The note keeps its id, or gets one when it
// has none. Best-effort rollback on failure.
func (v *Vault) Promote(path string, opts PromoteOptions) (res PromoteResult, err error) {
	if err := v.writable(); err != nil {
		return PromoteResult{}, err
	}
	// The lock spans the link rewrite too: no other brain write changes a
	// linking note between the read of its links and the rewrite.
	release, err := v.lockWrites()
	if err != nil {
		return PromoteResult{}, err
	}
	defer func() { err = errors.Join(err, release()) }()
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	target := opts.TargetTier
	if target == "" {
		target = TierNotes
	}

	data, fm, body, err := v.readNote(path)
	if err != nil {
		return PromoteResult{}, err
	}
	if err := v.refuseUnclosed(path, data); err != nil {
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
	oldTitle := fm.Title

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

	from := fm.Status
	fm.Status = lifecycle.Validated
	lifecycle.Transition(&fm, from, opts.Now)
	fm.Touch(opts.Now)
	if fm.ID == "" {
		fm.ID = noteid.New(opts.Now)
	}

	edits, err := v.promoteEdits(path, linkRetarget{
		oldTitle: oldTitle,
		newTitle: opts.NewTitle,
		aliases:  fm.Aliases,
		newStem:  newStem,
		newPath:  strings.TrimSuffix(v.Rel(newPath), ".md"),
	})
	if err != nil {
		return PromoteResult{}, err
	}
	if self, ok := edits[v.Rel(path)]; ok {
		if body, err = self.body(); err != nil {
			return PromoteResult{}, err
		}
		delete(edits, v.Rel(path))
	}
	content, err := markdown.Render(fm, body)
	if err != nil {
		return PromoteResult{}, err
	}
	if err := v.refuseSecrets(newPath, content); err != nil {
		return PromoteResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return PromoteResult{}, err
	}
	// Write new file, delete old only after success.
	if err := v.create(newPath, content); err != nil {
		return PromoteResult{}, err
	}

	rewrites, rewriteErr := v.writeEdits(edits)
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

	res = PromoteResult{
		OldPath:          path,
		NewPath:          newPath,
		BacklinkRewrites: rewrites,
	}

	var mocErr error
	if opts.MOC != "" {
		mocPath := opts.MOC
		if !filepath.IsAbs(mocPath) {
			mocPath = filepath.Join(v.Root, mocPath)
		}
		if err := appendMOCBullet(mocPath, title, newStem, opts.Now); err != nil {
			mocErr = fmt.Errorf("append to MOC %s: %w", mocPath, err)
		} else {
			res.MOCAppended = mocPath
		}
	}
	return res, errors.Join(mocErr, v.commitPromote(res))
}

// commitPromote commits a promote: the new note first, then the inbox file
// it left, the notes whose links it rewrote and the MOC it appended to.
func (v *Vault) commitPromote(res PromoteResult) error {
	rel := v.Rel(res.NewPath)
	paths := []string{rel, v.Rel(res.OldPath)}
	for _, p := range res.BacklinkRewrites {
		paths = append(paths, v.Rel(p))
	}
	if res.MOCAppended != "" {
		paths = append(paths, v.Rel(res.MOCAppended))
	}
	return v.commit(subjectOf(VerbPromote, []string{rel}), paths, "")
}

// writable refuses a shared vault. Shared notes change through a pull
// request against the vault's repo, never by a direct write.
func (v *Vault) writable() error {
	if v.Shared {
		return fmt.Errorf(
			"%w: %s is a shared vault; shared notes change through a pull request "+
				"(brain edit --pr opens one for an edit, brain promote --to %s shares a private note)",
			ErrReadOnly, v.Name, v.Name,
		)
	}
	return nil
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
	if t == TierInbox || t == "" {
		return lifecycle.Raw
	}
	return lifecycle.Validated
}

// StatusOf returns the effective status of the note at the vault-relative
// path rel: its status field, else the default of its tier.
func StatusOf(rel string, fm markdown.Frontmatter) string {
	if fm.Status != "" {
		return lifecycle.Normalize(fm.Status)
	}
	first, _, _ := strings.Cut(rel, "/")
	t, err := ParseTier(first)
	if err != nil {
		return ""
	}
	return defaultStatus(t)
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

// atomicWrite replaces the file at path with data, creating its directory.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicfile.Write(path, data, mode)
}
