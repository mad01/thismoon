package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Frontmatter is the CONTEXT.md contract: the machine-readable metadata that
// makes an item findable by ticket, topic, status, or repo.
type Frontmatter struct {
	Key     string    `yaml:"key"`
	Ticket  string    `yaml:"ticket,omitempty"`
	Topic   string    `yaml:"topic,omitempty"`
	Status  string    `yaml:"status"`
	Created time.Time `yaml:"created"`
	Updated time.Time `yaml:"updated"`
	LastCwd string    `yaml:"last_cwd,omitempty"`
	Repos   []string  `yaml:"repos,omitempty"`
}

// Item is one work item: its frontmatter plus the markdown body (a "Where I am"
// snapshot rewritten each checkpoint and an append-only "Log").
type Item struct {
	FM   Frontmatter
	Body string
}

// CheckpointInput carries one checkpoint's content. Empty fields are skipped.
type CheckpointInput struct {
	Ticket string // set on first creation
	Topic  string
	Where  string // replaces the "Where I am" snapshot
	Note   string // prepended as a new "Log" entry
	Repo   string // repo this checkpoint touched ("" = none / not in a repo)
	Cwd    string // recorded as last_cwd
}

var keyRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Sanitize turns an arbitrary key into a safe directory name. Ticket ids
// (ABC-1234) pass through unchanged; free text is slugified.
func Sanitize(key string) string {
	k := keyRe.ReplaceAllString(strings.TrimSpace(key), "-")
	k = strings.Trim(k, "-")
	if k == "" {
		return "untitled"
	}
	return k
}

// Load reads and parses an item's CONTEXT.md.
func (s *Store) Load(key string) (*Item, error) {
	b, err := os.ReadFile(s.contextPath(key))
	if err != nil {
		return nil, err
	}
	return parseItem(b)
}

func parseItem(b []byte) (*Item, error) {
	text := string(b)
	if !strings.HasPrefix(text, "---\n") {
		return nil, fmt.Errorf("missing frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return nil, fmt.Errorf("unterminated frontmatter")
	}
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(text[4:4+end]), &fm); err != nil {
		return nil, err
	}
	body := strings.TrimPrefix(text[4+end+5:], "\n")
	return &Item{FM: fm, Body: body}, nil
}

// Render returns the full CONTEXT.md bytes (frontmatter + body).
func (it *Item) Render() ([]byte, error) {
	fmBytes, err := yaml.Marshal(it.FM)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(fmBytes) + "---\n\n" + it.Body), nil
}

// Checkpoint creates the item if missing, then applies in to its CONTEXT.md,
// optional per-repo note, frontmatter, and the git history.
func (s *Store) Checkpoint(key string, in CheckpointInput) (*Item, error) {
	if err := s.ensureGit(); err != nil {
		return nil, err
	}
	now := s.now()

	var it *Item
	if s.Exists(key) {
		var err error
		if it, err = s.Load(key); err != nil {
			return nil, err
		}
	} else {
		it = &Item{
			FM: Frontmatter{
				Key: key, Ticket: in.Ticket, Topic: in.Topic,
				Status: "active", Created: now,
			},
			Body: fmt.Sprintf("# %s\n\n## Where I am\n\n_Just started._\n\n## Log\n", key),
		}
	}

	it.FM.Updated = now
	if in.Cwd != "" {
		it.FM.LastCwd = in.Cwd
	}
	if in.Repo != "" && !contains(it.FM.Repos, in.Repo) {
		it.FM.Repos = append(it.FM.Repos, in.Repo)
	}
	if in.Where != "" {
		it.Body = replaceSection(it.Body, "## Where I am", in.Where)
	}
	if in.Note != "" {
		it.Body = prependLog(it.Body, now, in.Repo, in.Note)
	}

	if err := s.writeItem(it); err != nil {
		return nil, err
	}
	if in.Repo != "" && in.Note != "" {
		if err := s.appendRepoNote(key, in.Repo, now, in.Note); err != nil {
			return nil, err
		}
	}
	if err := s.commit(fmt.Sprintf("worklog: checkpoint %s", key)); err != nil {
		return nil, err
	}
	return it, nil
}

func (s *Store) writeItem(it *Item) error {
	if err := os.MkdirAll(s.itemDir(it.FM.Key), 0o755); err != nil {
		return err
	}
	b, err := it.Render()
	if err != nil {
		return err
	}
	return os.WriteFile(s.contextPath(it.FM.Key), b, 0o644)
}

func (s *Store) appendRepoNote(key, repo string, t time.Time, note string) error {
	p := s.repoPath(key, repo)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	header := fmt.Sprintf("### %s\n\n%s\n\n", t.Format("2006-01-02 15:04"), note)
	existing, _ := os.ReadFile(p)
	if len(existing) == 0 {
		existing = []byte(fmt.Sprintf("# %s — %s\n\n", key, repo))
	}
	return os.WriteFile(p, append(existing, []byte(header)...), 0o644)
}

// SetStatus updates an item's status and commits.
func (s *Store) SetStatus(key, status string) (*Item, error) {
	if err := s.ensureGit(); err != nil {
		return nil, err
	}
	it, err := s.Load(key)
	if err != nil {
		return nil, err
	}
	it.FM.Status = status
	it.FM.Updated = s.now()
	if err := s.writeItem(it); err != nil {
		return nil, err
	}
	if err := s.commit(fmt.Sprintf("worklog: %s -> %s", key, status)); err != nil {
		return nil, err
	}
	return it, nil
}

// RepoNote returns the markdown of an item's per-repo note file.
func (s *Store) RepoNote(key, repo string) (string, error) {
	b, err := os.ReadFile(s.repoPath(key, repo))
	return string(b), err
}

// replaceSection swaps the content under header (up to the next "## " heading
// or end of doc), keeping the heading line itself.
func replaceSection(body, header, content string) string {
	idx := strings.Index(body, header+"\n")
	if idx < 0 {
		return body + "\n" + header + "\n\n" + content + "\n"
	}
	start := idx + len(header) + 1
	rest := body[start:]
	next := strings.Index(rest, "\n## ")
	tail := ""
	if next >= 0 {
		tail = rest[next:]
	}
	return body[:start] + "\n" + content + "\n" + tail
}

// prependLog inserts a new entry directly under the "## Log" heading.
func prependLog(body string, t time.Time, repo, note string) string {
	scope := repo
	if scope == "" {
		scope = "(no repo)"
	}
	entry := fmt.Sprintf("\n### %s — %s\n\n%s\n", t.Format("2006-01-02 15:04"), scope, note)
	idx := strings.Index(body, "## Log\n")
	if idx < 0 {
		return body + "\n## Log\n" + entry
	}
	at := idx + len("## Log\n")
	return body[:at] + entry + body[at:]
}
