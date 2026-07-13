package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/services/pr/internal/config"
	"github.com/mad01/thismoon/services/pr/internal/github"
	"github.com/mad01/thismoon/services/pr/internal/store"
)

// Both pages are static chrome-only shells; their bodies are rendered in the
// browser from JSON (docs/adr/0005). The list shell + appJS fetch GET /api/prs; the
// detail shell + detailJS fetch GET /api/pr/{host}/{owner}/{repo}/{number}.

//go:embed shell.html
var shellHTML []byte

//go:embed app.js
var appJS []byte

//go:embed detail_shell.html
var detailShellHTML []byte

//go:embed detail.js
var detailJS []byte

type Options struct {
	Port       int
	ConfigPath string
	Workdir    string
	Version    string
}

func Serve(opts Options) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	storePath := filepath.Join(opts.Workdir, "store.json")
	st, err := store.Load(storePath)
	if err != nil {
		return fmt.Errorf("load store: %w", err)
	}
	cached := st.Snapshot()
	log.Printf("pr: loaded %d cached repos from %s", len(cached), storePath)

	client := github.NewClient()
	poller := newPoller(cfg, client, st)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go poller.run(ctx)

	mux := newMux(poller, client, cfg, opts.Version)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		log.Printf("%s %s", r.Method, r.URL.Path)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", opts.Port)
	log.Printf("pr: serving on http://%s (poll %s, %d repos)",
		addr, cfg.PollInterval.Duration, len(cfg.AllRepos()))
	return http.ListenAndServe(addr, handler)
}

func newMux(
	poller *Poller,
	client *github.Client,
	cfg *config.Config,
	version string,
) *http.ServeMux {
	mux := http.NewServeMux()
	webkit.Mount(mux)

	// The page serves a static chrome-only shell; app.js fetches GET /api/prs
	// and builds the dashboard in the browser.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(shellHTML)
	})

	mux.HandleFunc("GET /app.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		// no-cache so a pr rebuild's app.js is picked up on the next load.
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(appJS)
	})

	// The detail page is a static chrome-only shell; detail.js reads the PR
	// coordinates from the URL and fetches GET /api/pr/{...} to build the body.
	mux.HandleFunc("GET /pr/{host}/{owner}/{repo}/{number}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(detailShellHTML)
	})

	mux.HandleFunc("GET /detail.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(detailJS)
	})

	mux.HandleFunc("GET /api/pr/{host}/{owner}/{repo}/{number}", handlePRDetail(client))

	mux.HandleFunc("POST /api/approve", handleApprove(client))
	mux.HandleFunc("POST /api/merge", handleMerge(client, poller))
	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, _ *http.Request) {
		poller.Refresh(context.Background())
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/prs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(poller.Snapshot())
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": version})
	})

	return mux
}

// handlePRDetail does the live GitHub fetch for one PR and returns its detail as
// JSON. The markdown body and unified diff are rendered to HTML here (the heavy
// content transforms stay server-side); everything else is raw data the browser
// shapes for display.
func handlePRDetail(client *github.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.PathValue("host")
		owner := r.PathValue("owner")
		repo := r.PathValue("repo")
		num, err := strconv.Atoi(r.PathValue("number"))
		if err != nil {
			http.Error(w, "invalid PR number", http.StatusBadRequest)
			return
		}

		ref := github.RepoRef{Owner: owner, Name: repo, Host: host}
		ctx := r.Context()

		// Fetch PR first (need head SHA for checks).
		pr, err := client.GetPR(ctx, ref, num)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		// Fetch diff, files, reviews, checks in parallel.
		var (
			diff    string
			files   []github.PRFile
			reviews []github.Review
			checks  *github.CheckSuite
		)
		var wg sync.WaitGroup
		wg.Add(4)
		go func() { defer wg.Done(); diff, _ = client.GetDiff(ctx, ref, num) }()
		go func() { defer wg.Done(); files, _ = client.GetFiles(ctx, ref, num) }()
		go func() { defer wg.Done(); reviews, _ = client.GetReviews(ctx, ref, num) }()
		go func() { defer wg.Done(); checks, _ = client.GetChecks(ctx, ref, pr.Head.SHA) }()
		wg.Wait()

		fileViews := make([]fileJSON, 0, len(files))
		for _, f := range files {
			fileViews = append(fileViews, fileJSON{
				Filename:  f.Filename,
				Status:    f.Status,
				Additions: f.Additions,
				Deletions: f.Deletions,
			})
		}

		reviewViews := make([]reviewJSON, 0, len(reviews))
		for _, rv := range reviews {
			if rv.State == "PENDING" {
				continue
			}
			reviewViews = append(reviewViews, reviewJSON{
				Author:      rv.User.Login,
				State:       rv.State,
				Body:        rv.Body,
				SubmittedAt: rv.SubmittedAt,
			})
		}

		cfgRepo := config.Repo{Owner: owner, Name: repo, Host: host}
		detail := prDetailJSON{
			Number:       pr.Number,
			Title:        pr.Title,
			Author:       pr.User.Login,
			HTMLURL:      pr.HTMLURL,
			Draft:        pr.Draft,
			Additions:    pr.Additions,
			Deletions:    pr.Deletions,
			ChangedFiles: pr.ChangedFiles,
			HeadRef:      pr.Head.Ref,
			BaseRef:      pr.Base.Ref,
			CreatedAt:    pr.CreatedAt,
			UpdatedAt:    pr.UpdatedAt,
			Repo: repoJSON{
				Name:    cfgRepo.FullName(),
				Host:    host,
				HTMLURL: cfgRepo.HTMLURL(),
			},
			BodyHTML: renderMarkdown(pr.Body),
			DiffHTML: renderDiffHTML(parseDiff(diff)),
			Files:    fileViews,
			Reviews:  reviewViews,
			Checks:   buildChecks(checks),
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(detail)
	}
}
