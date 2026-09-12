package mcpserver

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

// indexInfoInput is the typed input for the csl_index_info tool: it takes no
// parameters beyond the response format.
type indexInfoInput struct {
	formatParam
}

// semanticInfo describes the semantic index inside csl_index_info output.
type semanticInfo struct {
	Built        bool `json:"built"         jsonschema:"true when at least one per-repo vector store exists"`
	Stores       int  `json:"stores"        jsonschema:"number of per-repo vector stores"`
	Chunks       int  `json:"chunks"        jsonschema:"total embedded chunks across all stores"`
	ModelPresent bool `json:"model_present" jsonschema:"true when Ollama is reachable and the configured embedding model is pulled; when false run: ollama pull unclemusclez/jina-embeddings-v2-base-code:f16"`
}

// indexInfoOutput is the typed output of the csl_index_info tool.
type indexInfoOutput struct {
	ReposIndexed    int          `json:"repos_indexed"               jsonschema:"repos tracked in the index state"`
	DirtyRepos      int          `json:"dirty_repos"                 jsonschema:"repos whose working tree was dirty when last indexed"`
	Shards          int          `json:"shards"                      jsonschema:"zoekt shard files on disk"`
	CorruptShards   int          `json:"corrupt_shards"              jsonschema:"shards that failed validation; run csl doctor to repair"`
	IndexSizeBytes  int64        `json:"index_size_bytes"            jsonschema:"total size of all zoekt shards"`
	NewestIndexedAt string       `json:"newest_indexed_at,omitempty" jsonschema:"most recent per-repo index time (RFC3339)"`
	OldestIndexedAt string       `json:"oldest_indexed_at,omitempty" jsonschema:"least recent per-repo index time (RFC3339); a very old value means some repo isn't being reindexed"`
	DaemonRunning   bool         `json:"daemon_running"              jsonschema:"true when the search daemon answered a ping; false means the next query pays daemon startup"`
	Semantic        semanticInfo `json:"semantic"`
}

// lsInput is the typed input for the csl_ls tool.
type lsInput struct {
	Repo      string `json:"repo"                jsonschema:"case-insensitive regex or substring matched against the org/repo name; must resolve to exactly one repo"`
	Path      string `json:"path,omitempty"      jsonschema:"directory relative to the repo root to list (default: the root)"`
	Glob      string `json:"glob,omitempty"      jsonschema:"filter entries by base-name glob (e.g. '*.go'); matched against the file or directory name, not the full path"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"walk the whole subtree instead of one level; directories are omitted, only files are returned"`
	formatParam
}

// lsEntry is one file or directory in the csl_ls result.
type lsEntry struct {
	Path  string `json:"path"            jsonschema:"path relative to the repo root"`
	Dir   bool   `json:"dir,omitempty"   jsonschema:"true for directories"`
	Bytes int64  `json:"bytes,omitempty" jsonschema:"file size; omitted for directories"`
}

// lsOutput is the typed output of the csl_ls tool.
type lsOutput struct {
	Repo           string    `json:"repo"                      jsonschema:"the resolved repo name (canonical org/repo form)"`
	Path           string    `json:"path"                      jsonschema:"the listed directory relative to the repo root ('.' for the root)"`
	Entries        []lsEntry `json:"entries"`
	Total          int       `json:"total"                     jsonschema:"number of entries returned"`
	Truncated      bool      `json:"truncated"                 jsonschema:"true if the listing was capped; narrow with path/glob"`
	TotalAvailable int       `json:"total_available,omitempty" jsonschema:"entries available before the cap (only set when truncated)"`
}

func registerInfoTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_index_info",
		Description: "Global health of the csl search index in one call: repo count, shard count and size, corrupt shards, " +
			"newest/oldest per-repo index times, whether the search daemon is running, and semantic index status (stores, chunks, model presence). " +
			"Use to answer 'is the index healthy', 'is semantic search ready', or 'why is search slow' before falling back to per-repo csl_repo_info calls. " +
			"Reads state from disk and pings the daemon; it doesn't scan repos, so it returns in well under a second.",
	}, withFormat(handleIndexInfo, nil))

	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_ls",
		Description: "List files and directories inside a locally checked-out repo. " +
			"Use to answer 'what files are in internal/web/' without shelling out to ls or find. " +
			"Lists one directory level by default; set recursive=true to walk the whole subtree (files only). " +
			"Filter with glob (matched against base names, e.g. '*.go'). " +
			"Reads the filesystem directly (always current, .git excluded) and caps output at 500 entries; truncated=true with total_available when capped.",
	}, withFormat(handleLs, renderLsText))
}

func handleIndexInfo(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ indexInfoInput,
) (*mcp.CallToolResult, indexInfoOutput, error) {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return nil, indexInfoOutput{}, fmt.Errorf("resolve index dir: %w", err)
	}

	state, err := search.LoadState(indexDir)
	if err != nil {
		return nil, indexInfoOutput{}, fmt.Errorf("load index state: %w", err)
	}

	out := indexInfoOutput{ReposIndexed: len(state.Repos)}

	var newest, oldest time.Time
	for _, rs := range state.Repos {
		if rs.Dirty {
			out.DirtyRepos++
		}
		if newest.IsZero() || rs.IndexedAt.After(newest) {
			newest = rs.IndexedAt
		}
		if oldest.IsZero() || rs.IndexedAt.Before(oldest) {
			oldest = rs.IndexedAt
		}
	}
	if !newest.IsZero() {
		out.NewestIndexedAt = newest.Format(time.RFC3339)
		out.OldestIndexedAt = oldest.Format(time.RFC3339)
	}

	shards, corrupted, err := search.ValidateShards(indexDir)
	if err != nil {
		return nil, indexInfoOutput{}, fmt.Errorf("validate shards: %w", err)
	}
	out.Shards = len(shards)
	out.CorruptShards = len(corrupted)
	for _, sh := range shards {
		out.IndexSizeBytes += sh.Size
	}

	out.DaemonRunning = daemon.Ping(daemon.DefaultSocketPath()) == nil

	if semDir, err := semantic.DefaultSemanticIndexDir(); err == nil {
		if ix, err := semantic.OpenIndex(semDir); err == nil {
			out.Semantic.Stores = ix.Stores()
			out.Semantic.Chunks = ix.Len()
			out.Semantic.Built = ix.Stores() > 0
		}
	}
	out.Semantic.ModelPresent = semantic.NewDefaultEmbedder().CheckModel(ctx) == nil

	return nil, out, nil
}

const maxLsEntries = 500

func handleLs(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in lsInput,
) (*mcp.CallToolResult, lsOutput, error) {
	if strings.TrimSpace(in.Repo) == "" {
		return nil, lsOutput{}, fmt.Errorf("repo is required")
	}
	if in.Glob != "" {
		if _, err := filepath.Match(in.Glob, "probe"); err != nil {
			return nil, lsOutput{}, fmt.Errorf("invalid glob %q: %w", in.Glob, err)
		}
	}

	matched, err := resolveRepo(in.Repo)
	if err != nil {
		return nil, lsOutput{}, err
	}

	relDir := filepath.Clean(in.Path)
	if relDir == "" || relDir == "." {
		relDir = "."
	}
	if relDir == ".." || strings.HasPrefix(relDir, "../") || filepath.IsAbs(relDir) {
		return nil, lsOutput{}, fmt.Errorf("path must stay inside the repo, got %q", in.Path)
	}
	absDir := filepath.Join(matched.Path, relDir)

	var entries []lsEntry
	if in.Recursive {
		entries, err = lsRecursive(matched.Path, absDir, in.Glob)
	} else {
		entries, err = lsSingleLevel(matched.Path, absDir, in.Glob)
	}
	if err != nil {
		return nil, lsOutput{}, err
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		return entries[i].Path < entries[j].Path
	})

	out := lsOutput{Repo: matched.Name, Path: relDir}
	if len(entries) > maxLsEntries {
		out.Truncated = true
		out.TotalAvailable = len(entries)
		entries = entries[:maxLsEntries]
	}
	out.Entries = entries
	out.Total = len(entries)
	return nil, out, nil
}

// lsSingleLevel lists the direct children of absDir.
func lsSingleLevel(repoRoot, absDir, glob string) ([]lsEntry, error) {
	dirEntries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", absDir, err)
	}
	entries := make([]lsEntry, 0, len(dirEntries))
	for _, e := range dirEntries {
		if e.Name() == ".git" {
			continue
		}
		if glob != "" {
			if ok, _ := filepath.Match(glob, e.Name()); !ok {
				continue
			}
		}
		rel, err := filepath.Rel(repoRoot, filepath.Join(absDir, e.Name()))
		if err != nil {
			continue
		}
		entry := lsEntry{Path: rel, Dir: e.IsDir()}
		if !e.IsDir() {
			if info, err := e.Info(); err == nil {
				entry.Bytes = info.Size()
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// lsRecursive walks the subtree under absDir and returns files only.
func lsRecursive(repoRoot, absDir, glob string) ([]lsEntry, error) {
	var entries []lsEntry
	err := filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if glob != "" {
			if ok, _ := filepath.Match(glob, d.Name()); !ok {
				return nil
			}
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		entry := lsEntry{Path: rel}
		if info, infoErr := d.Info(); infoErr == nil {
			entry.Bytes = info.Size()
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", absDir, err)
	}
	return entries, nil
}
