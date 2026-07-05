package server

import (
	"context"
	"log"
	"time"

	"github.com/mad01/thismoon/services/pr/internal/config"
	"github.com/mad01/thismoon/services/pr/internal/github"
	"github.com/mad01/thismoon/services/pr/internal/store"
)

type RepoPRs struct {
	Repo            config.Repo
	PRs             []store.CachedPR
	ReviewDecisions map[int]string
	Error           string
	Fetched         time.Time
}

type Poller struct {
	cfg    *config.Config
	client *github.Client
	store  *store.Store
}

func newPoller(cfg *config.Config, client *github.Client, st *store.Store) *Poller {
	return &Poller{cfg: cfg, client: client, store: st}
}

func (p *Poller) run(ctx context.Context) {
	p.poll(ctx)
	ticker := time.NewTicker(p.cfg.PollInterval.Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	validKeys := make(map[string]bool)
	for _, repo := range p.cfg.AllRepos() {
		key := repo.Host + "/" + repo.Owner + "/" + repo.Name
		validKeys[key] = true

		ref := github.RepoRef{Owner: repo.Owner, Name: repo.Name, Host: repo.Host}
		if ctx.Err() != nil {
			log.Printf("pr: poll: context canceled, skipping remaining repos")
			break
		}
		prs, err := p.client.ListPRs(ctx, ref)
		rs := &store.RepoState{FetchedAt: time.Now()}
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("pr: poll: context canceled, skipping remaining repos")
				break
			}
			log.Printf("pr: poll %s: %v", repo.FullName(), err)
			rs.Error = err.Error()
			if cached := p.store.Get(key); cached != nil && len(cached.PRs) > 0 {
				rs.PRs = cached.PRs
				rs.ReviewDecisions = cached.ReviewDecisions
			}
		} else if repoArchived(prs) {
			rs.Archived = true
			log.Printf("pr: poll %s: repo is archived, dropping %d open PRs", repo.FullName(), len(prs))
		} else {
			rs.PRs = toCached(prs)
			if decisions, err := p.client.GetReviewDecisions(ctx, ref); err == nil {
				rs.ReviewDecisions = decisions
			}
			log.Printf("pr: poll %s: %d open PRs", repo.FullName(), len(prs))
		}
		p.store.Put(key, rs)
	}
	p.store.PruneStale(validKeys)
	if err := p.store.Save(); err != nil {
		log.Printf("pr: save store: %v", err)
	}
}

// repoArchived reports whether the watched repo is archived. The pulls list
// payload carries the base repo's archived flag, so no extra API call is
// needed; a repo with zero open PRs never shows on the dashboard anyway.
func repoArchived(prs []github.PullRequest) bool {
	return len(prs) > 0 && prs[0].Base.Repo.Archived
}

func toCached(prs []github.PullRequest) []store.CachedPR {
	out := make([]store.CachedPR, len(prs))
	for i, pr := range prs {
		var labels []string
		for _, l := range pr.Labels {
			labels = append(labels, l.Name)
		}
		out[i] = store.CachedPR{
			Number:       pr.Number,
			Title:        pr.Title,
			State:        pr.State,
			Draft:        pr.Draft,
			HTMLURL:      pr.HTMLURL,
			Author:       pr.User.Login,
			CreatedAt:    pr.CreatedAt,
			UpdatedAt:    pr.UpdatedAt,
			HeadRef:      pr.Head.Ref,
			HeadSHA:      pr.Head.SHA,
			BaseRef:      pr.Base.Ref,
			Additions:    pr.Additions,
			Deletions:    pr.Deletions,
			ChangedFiles: pr.ChangedFiles,
			Labels:       labels,
			TicketID:     store.ExtractTicketID(pr.Title, pr.Head.Ref),
		}
	}
	return out
}

func (p *Poller) Snapshot() []RepoPRs {
	snap := p.store.Snapshot()
	repoMap := make(map[string]config.Repo)
	for _, r := range p.cfg.AllRepos() {
		repoMap[r.Host+"/"+r.Owner+"/"+r.Name] = r
	}
	out := make([]RepoPRs, 0, len(snap))
	for key, rs := range snap {
		repo, ok := repoMap[key]
		if !ok {
			continue
		}
		out = append(out, RepoPRs{
			Repo:            repo,
			PRs:             rs.PRs,
			ReviewDecisions: rs.ReviewDecisions,
			Error:           rs.Error,
			Fetched:         rs.FetchedAt,
		})
	}
	return out
}

func (p *Poller) Refresh(ctx context.Context) { p.poll(ctx) }
