package semantic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/cslignore"
	"github.com/mad01/thismoon/services/csl/internal/grammar"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// indexDirName is the semantic index's directory inside csl's state
// directory, holding one vector store per repo.
const indexDirName = "semantic-index"

// maxFileSize caps the per-file size considered for semantic indexing (1MB).
const maxFileSize = 1 << 20

// embedBatch is the maximum number of chunks embedded per Embed call.
const embedBatch = 32

// IndexStats summarizes one semantic indexing pass over a repo.
type IndexStats struct {
	FilesScanned   int
	FilesEmbedded  int
	FilesSkipped   int
	ChunksEmbedded int
}

// IndexProgress is emitted during IndexRepoSemantic for each file processed.
type IndexProgress struct {
	FilesTotal int
	FilesDone  int
	Current    string
}

// IndexOption configures IndexRepoSemantic.
type IndexOption func(*indexOpts)

type indexOpts struct {
	onProgress func(IndexProgress)
}

// WithProgress sets a callback invoked after each file is processed.
func WithProgress(fn func(IndexProgress)) IndexOption {
	return func(o *indexOpts) { o.onProgress = fn }
}

// DefaultSemanticIndexDir returns the semantic index directory under csl's
// state directory, beside the lexical one search.DefaultIndexDir resolves.
func DefaultSemanticIndexDir() (string, error) {
	return csl.StatePath(indexDirName)
}

// StorePathForRepo returns the per-repo store file path under indexDir. The
// repo name's slashes are replaced so it is a single filesystem-safe segment.
func StorePathForRepo(indexDir string, repo finder.Repo) string {
	name := strings.ReplaceAll(repo.Name, "/", "_")
	return filepath.Join(indexDir, name+".gob")
}

// SemanticStatePath returns the fingerprint state file path under indexDir.
// It is kept separate from the lexical index's state.
func SemanticStatePath(indexDir string) string {
	return filepath.Join(indexDir, "state.json")
}

// IndexRepoSemantic embeds a repo's source into its per-repo vector store,
// incrementally: unchanged files (same content hash) keep their existing
// vectors, changed/new files are re-embedded, and vanished files are pruned.
func IndexRepoSemantic(
	ctx context.Context,
	indexDir string,
	repo finder.Repo,
	emb Embedder,
	opts ...IndexOption,
) (IndexStats, error) {
	var o indexOpts
	for _, fn := range opts {
		fn(&o)
	}

	storePath := StorePathForRepo(indexDir, repo)
	store, err := LoadStore(storePath)
	if err != nil {
		return IndexStats{}, err
	}
	if store.Dim() != emb.Dim() || store.ChunkerVersion() != chunkerVersion {
		// Model swap (dim change) or chunking behavior change: the per-file
		// content hashes would wrongly skip every unchanged file, so drop the
		// store and re-embed the repo from scratch.
		store = NewStore(emb.Dim())
	}
	store.SetRepoPath(repo.Path)

	total := 0
	if o.onProgress != nil {
		total = countSourceFiles(repo.Path)
	}

	var stats IndexStats
	seen := make(map[string]bool)
	existing := store.Files()

	walkErr := walkSourceFiles(repo.Path, func(rel string, content []byte) error {
		stats.FilesScanned++
		seen[rel] = true
		hash := hashContent(content)
		if existing[rel] == hash {
			stats.FilesSkipped++
		} else if err := embedFile(ctx, store, emb, repo.Name, rel, hash, content, &stats); err != nil {
			return err
		}
		if o.onProgress != nil {
			o.onProgress(IndexProgress{
				FilesTotal: total,
				FilesDone:  stats.FilesScanned,
				Current:    rel,
			})
		}
		return nil
	})
	if walkErr != nil {
		return stats, walkErr
	}

	for path := range existing {
		if !seen[path] {
			store.DeleteFile(path)
		}
	}
	if err := store.Save(storePath); err != nil {
		return stats, err
	}
	saveFingerprint(indexDir, repo) // best-effort; non-git repos are skipped
	return stats, nil
}

// embedFile chunks, embeds, and stores a single source file, updating stats.
func embedFile(
	ctx context.Context,
	store *Store,
	emb Embedder,
	repoName, rel, hash string,
	content []byte,
	stats *IndexStats,
) error {
	chunks, err := ChunkFile(repoName, rel, grammar.LangForPath(rel), content)
	if err != nil {
		return fmt.Errorf("chunk %s: %w", rel, err)
	}
	vecs, err := embedChunks(ctx, emb, chunks)
	if err != nil {
		return err
	}
	store.PutFile(rel, hash, chunks, vecs)
	stats.FilesEmbedded++
	stats.ChunksEmbedded += len(chunks)
	return nil
}

// embedChunks embeds every chunk's EmbedText in batches of embedBatch.
func embedChunks(ctx context.Context, emb Embedder, chunks []Chunk) ([][]float32, error) {
	out := make([][]float32, 0, len(chunks))
	for i := 0; i < len(chunks); i += embedBatch {
		end := i + embedBatch
		if end > len(chunks) {
			end = len(chunks)
		}
		texts := make([]string, 0, end-i)
		for _, c := range chunks[i:end] {
			texts = append(texts, c.EmbedText)
		}
		vecs, err := emb.Embed(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("embed batch: %w", err)
		}
		if len(vecs) != len(texts) {
			return nil, fmt.Errorf(
				"embedder returned %d vectors for %d texts",
				len(vecs),
				len(texts),
			)
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// skipExts lists file extensions that should never be semantically indexed.
var skipExts = map[string]bool{
	// images
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".ico": true, ".svg": true, ".webp": true, ".tiff": true, ".tif": true,
	".heic": true, ".icns": true,
	// fonts
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	// archives / compressed
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
	".zst": true, ".rar": true, ".7z": true, ".tgz": true,
	// compiled / binary
	".so": true, ".dylib": true, ".dll": true, ".exe": true, ".a": true,
	".o": true, ".obj": true, ".pyc": true, ".pyo": true, ".class": true,
	".jar": true, ".war": true, ".wasm": true, ".pcm": true,
	".swiftmodule": true, ".swiftdoc": true, ".swiftsourceinfo": true,
	// media / office
	".mp3": true, ".mp4": true, ".wav": true, ".avi": true, ".mov": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true,
	// data / serialized
	".bin": true, ".dat": true, ".db": true, ".sqlite": true, ".sqlite3": true,
	// ML model weights
	".safetensors": true, ".gguf": true, ".onnx": true, ".tflite": true,
	".pt": true, ".pth": true, ".ckpt": true, ".npy": true, ".npz": true,
	".mlmodel": true,
	// source maps / minified bundles
	".min.js": true, ".min.css": true, ".map": true,
	// editor/backup litter
	".backup": true, ".bak": true, ".orig": true, ".swp": true,
}

// skipDirSegments lists directory names whose contents are build output or
// vendored dependencies, never source worth embedding. Matched (lowercased)
// against every path segment, so committed trees like a Swift package's
// .build/ or a Go vendor/ dir are filtered even though git tracks them.
var skipDirSegments = map[string]bool{
	// Xcode / Swift
	".build": true, "deriveddata": true, "pods": true, "carthage": true,
	".swiftpm": true, "xcuserdata": true,
	// Java / Rust / general build output
	"target": true, ".gradle": true, "build": true, "dist": true, "out": true,
	// dependency trees
	"node_modules": true, "vendor": true, "__pycache__": true,
}

// skipDirSuffixes lists directory-name suffixes (lowercased) that mark
// machine-managed Xcode containers: asset catalogs (image sets plus
// Contents.json boilerplate) and project/workspace bundles (pbxproj,
// schemes, user state).
// Core ML compiled models and packages are directories too.
var skipDirSuffixes = []string{".xcassets", ".xcodeproj", ".xcworkspace", ".mlmodelc", ".mlpackage"}

// isSkippedPath reports whether any directory segment of the repo-relative
// path is build output, a vendored dependency tree, or a machine-managed
// Xcode container.
func isSkippedPath(rel string) bool {
	dir := path.Dir(filepath.ToSlash(rel))
	if dir == "." {
		return false
	}
	for seg := range strings.SplitSeq(dir, "/") {
		lower := strings.ToLower(seg)
		if skipDirSegments[lower] {
			return true
		}
		for _, suffix := range skipDirSuffixes {
			if strings.HasSuffix(lower, suffix) {
				return true
			}
		}
	}
	return false
}

// skipFileNames lists exact base names (lowercased) of machine-managed
// metadata: lockfiles and dependency manifests whose content is generated,
// not written.
var skipFileNames = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"cargo.lock": true, "go.sum": true, "package.resolved": true,
	"podfile.lock": true, "gemfile.lock": true, "composer.lock": true,
	"poetry.lock": true, "uv.lock": true, "gradle.lockfile": true,
	// tokenizer/model metadata shipped beside local ML model weights
	"tokenizer.json": true, "tokenizer_config.json": true, "vocab.json": true,
	"merges.txt": true, "special_tokens_map.json": true,
	"generation_config.json": true, "model.safetensors.index.json": true,
	// csl's own ignore file
	".cslignore": true,
}

// skipReasonForName classifies a repo-relative path against the name-based
// skip rules, returning a human-readable reason or "" when the name passes.
func skipReasonForName(path string) string {
	if isSkippedPath(path) {
		return "build output, vendored, or machine-managed tree"
	}
	lower := strings.ToLower(path)
	if skipFileNames[filepath.Base(lower)] {
		return "lockfile or model metadata"
	}
	if strings.HasSuffix(lower, ".min.js") || strings.HasSuffix(lower, ".min.css") {
		return "minified bundle"
	}
	if skipExts[strings.ToLower(filepath.Ext(path))] {
		return "binary or media extension"
	}
	return ""
}

// isSkippedFile reports whether a file should be excluded from semantic indexing.
func isSkippedFile(path string) bool {
	return skipReasonForName(path) != ""
}

// maxDataLineLen is the longest line still plausible in handwritten source.
// Machine-generated data files (tokenizer vocabularies, serialized model
// indexes, minified bundles shipped without a .min name) pack kilobytes onto
// one line; real code does not.
const maxDataLineLen = 2000

// looksLikeData sniffs the first 8KB for a single line longer than
// maxDataLineLen, the signature of serialized data rather than source.
func looksLikeData(content []byte) bool {
	n := min(len(content), 8192)
	lineStart := 0
	for i, b := range content[:n] {
		if b == '\n' {
			lineStart = i + 1
		} else if i-lineStart >= maxDataLineLen {
			return true
		}
	}
	return false
}

// looksLikeBinary sniffs the first 512 bytes for null bytes, a strong signal
// that the file is a compiled binary rather than text.
func looksLikeBinary(content []byte) bool {
	n := 512
	if len(content) < n {
		n = len(content)
	}
	for _, b := range content[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}

// rawGitFiles returns every git-tracked path under root, unfiltered, or an
// error when root is not a git work tree.
func rawGitFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range bytes.Split(out, []byte{0}) {
		if rel := string(entry); rel != "" {
			files = append(files, rel)
		}
	}
	return files, nil
}

// gitTrackedFiles returns the list of git-tracked files under root, filtered
// to exclude binary extensions and oversized files. Falls back to a filesystem
// walk for non-git directories.
func gitTrackedFiles(root string) ([]string, error) {
	raw, err := rawGitFiles(root)
	if err != nil {
		return fallbackWalkFiles(root)
	}
	ignore := cslignore.Load(root)
	var files []string
	for _, rel := range raw {
		if ignore.Match(rel) || isSkippedFile(rel) {
			continue
		}
		abs := filepath.Join(root, rel)
		info, err := os.Stat(abs)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileSize {
			continue
		}
		files = append(files, rel)
	}
	return files, nil
}

// fallbackWalkFiles walks the filesystem for non-git repos, applying the same
// name and size filters gitTrackedFiles does.
func fallbackWalkFiles(root string) ([]string, error) {
	raw, err := rawWalkFiles(root)
	if err != nil {
		return nil, err
	}
	ignore := cslignore.Load(root)
	var files []string
	for _, rel := range raw {
		if ignore.Match(rel) || isSkippedFile(rel) {
			continue
		}
		info, statErr := os.Stat(filepath.Join(root, rel))
		if statErr != nil || info.Size() > maxFileSize {
			continue
		}
		files = append(files, rel)
	}
	return files, nil
}

// walkSourceFiles iterates over git-tracked source files under root, reading
// each file and invoking fn. Binary files (by extension or content sniff) and
// oversized files are skipped.
func walkSourceFiles(root string, fn func(relPath string, content []byte) error) error {
	files, err := gitTrackedFiles(root)
	if err != nil {
		return fmt.Errorf("list files in %s: %w", root, err)
	}
	for _, rel := range files {
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		if looksLikeBinary(content) || looksLikeData(content) {
			continue
		}
		if err := fn(rel, content); err != nil {
			return err
		}
	}
	return nil
}

// FileDecision records whether one file would be semantically indexed, and
// the skip reason when it wouldn't.
type FileDecision struct {
	Path   string `json:"path"`
	Index  bool   `json:"index"`
	Reason string `json:"reason,omitempty"`
	Size   int64  `json:"size"`
}

// AuditRepoFiles classifies every tracked file under root with the same rules
// IndexRepoSemantic applies, without embedding anything. For non-git
// directories it walks the filesystem instead; there the pruned trees
// (node_modules, .build, ...) are not descended into, so their contents don't
// appear as individual decisions.
func AuditRepoFiles(root string) ([]FileDecision, error) {
	rels, err := rawGitFiles(root)
	if err != nil {
		if rels, err = rawWalkFiles(root); err != nil {
			return nil, fmt.Errorf("list files in %s: %w", root, err)
		}
	}
	ignore := cslignore.Load(root)
	out := make([]FileDecision, 0, len(rels))
	for _, rel := range rels {
		d := FileDecision{Path: rel}
		if ignore.Match(rel) {
			d.Reason = "matched .cslignore"
			out = append(out, d)
			continue
		}
		if reason := skipReasonForName(rel); reason != "" {
			d.Reason = reason
			out = append(out, d)
			continue
		}
		abs := filepath.Join(root, rel)
		info, statErr := os.Stat(abs)
		if statErr != nil || !info.Mode().IsRegular() {
			d.Reason = "not a regular file"
			out = append(out, d)
			continue
		}
		d.Size = info.Size()
		if info.Size() > maxFileSize {
			d.Reason = "over the 1MB size cap"
			out = append(out, d)
			continue
		}
		content, readErr := os.ReadFile(abs)
		switch {
		case readErr != nil:
			d.Reason = "unreadable"
		case looksLikeBinary(content):
			d.Reason = "binary content"
		case looksLikeData(content):
			d.Reason = "serialized data (over-long lines)"
		default:
			d.Index = true
		}
		out = append(out, d)
	}
	return out, nil
}

// rawWalkFiles walks the filesystem for non-git directories, pruning hidden
// and build/vendor trees but applying no per-file filters.
func rawWalkFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != root {
				return filepath.SkipDir
			}
			if skipDirSegments[strings.ToLower(base)] {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

// countSourceFiles returns the number of indexable source files under root.
func countSourceFiles(root string) int {
	files, err := gitTrackedFiles(root)
	if err != nil {
		return 0
	}
	return len(files)
}

// hashContent returns the hex sha256 of content.
func hashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}

// saveFingerprint records the repo's git fingerprint in the semantic index's
// own state file so the sync trigger can detect changed repos independently of
// the lexical index. Non-git repos (no fingerprint) are silently skipped.
func saveFingerprint(indexDir string, repo finder.Repo) {
	fp, err := search.Fingerprint(repo.Path)
	if err != nil {
		return
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		return
	}
	state.SetRepo(repo.Path, fp)
	_ = state.Save(indexDir)
}
