package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	gh "github.com/google/go-github/v75/github"

	"github.com/mad01/thismoon/services/prs/internal/store"
)

// requestTimeout bounds each API request; a poll worker that hangs starves
// the whole cycle.
const requestTimeout = 30 * time.Second

// Client fetches open PRs from any number of GitHub hosts. One go-github
// client is built per host, authenticated with the TokenSource's minted
// token; a 401 drops both and retries once with a fresh token.
type Client struct {
	tokens *TokenSource

	mu      sync.Mutex
	perHost map[string]*gh.Client

	// apiBase maps a host to its REST base URL; "" means api.github.com.
	// A field rather than a function so tests can point a fake host at an
	// httptest server.
	apiBase func(host string) string
}

// NewClient returns a Client minting its per-host tokens from tokens.
func NewClient(tokens *TokenSource) *Client {
	return &Client{
		tokens:  tokens,
		perHost: map[string]*gh.Client{},
		apiBase: func(host string) string {
			if host == "github.com" {
				return ""
			}
			return "https://" + host + "/api/v3/"
		},
	}
}

// OpenPRs returns the open, non-draft pull requests of host/owner/name, each
// carrying the review decision computed from its reviews. Draft, closed, and
// merged PRs never leave this function.
func (c *Client) OpenPRs(ctx context.Context, host, owner, name string) ([]store.PR, error) {
	prs, err := c.openPRs(ctx, host, owner, name)
	if isUnauthorized(err) {
		// The cached token expired or was revoked; re-mint once.
		c.reset(host)
		prs, err = c.openPRs(ctx, host, owner, name)
	}
	return prs, err
}

func (c *Client) openPRs(ctx context.Context, host, owner, name string) ([]store.PR, error) {
	client, err := c.client(host)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	var out []store.PR
	opts := &gh.PullRequestListOptions{
		State:       "open",
		ListOptions: gh.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := client.PullRequests.List(ctx, owner, name, opts)
		if err != nil {
			return nil, fmt.Errorf("list pulls %s/%s/%s: %w", host, owner, name, err)
		}
		for _, pr := range page {
			if pr.GetDraft() || pr.GetState() != "open" {
				continue
			}
			decision, err := c.reviewDecision(ctx, client, owner, name, pr.GetNumber())
			if err != nil {
				return nil, fmt.Errorf(
					"list reviews %s/%s/%s#%d: %w", host, owner, name, pr.GetNumber(), err,
				)
			}
			out = append(out, toPR(host, owner+"/"+name, pr, decision))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

// reviewDecision fetches a PR's reviews and reduces them to the decision the
// dashboard cares about: each reviewer's latest APPROVED or CHANGES_REQUESTED
// stands (a dismissed review reads as DISMISSED and drops out), any standing
// CHANGES_REQUESTED wins over approvals, and no standing review at all is "".
func (c *Client) reviewDecision(
	ctx context.Context,
	client *gh.Client,
	owner, name string,
	number int,
) (string, error) {
	latest := map[string]string{}
	opts := &gh.ListOptions{PerPage: 100}
	for {
		reviews, resp, err := client.PullRequests.ListReviews(ctx, owner, name, number, opts)
		if err != nil {
			return "", err
		}
		for _, r := range reviews {
			state := r.GetState()
			if state != "APPROVED" && state != "CHANGES_REQUESTED" {
				continue
			}
			latest[r.GetUser().GetLogin()] = state
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	decision := ""
	for _, state := range latest {
		if state == "CHANGES_REQUESTED" {
			return "CHANGES_REQUESTED", nil
		}
		decision = "APPROVED"
	}
	return decision, nil
}

// toPR maps a go-github pull request onto the store model.
func toPR(host, repo string, pr *gh.PullRequest, decision string) store.PR {
	var labels []string
	for _, l := range pr.Labels {
		labels = append(labels, l.GetName())
	}
	return store.PR{
		Host:           host,
		Repo:           repo,
		Number:         pr.GetNumber(),
		Title:          pr.GetTitle(),
		URL:            pr.GetHTMLURL(),
		Author:         pr.GetUser().GetLogin(),
		CreatedAt:      pr.GetCreatedAt().Time,
		UpdatedAt:      pr.GetUpdatedAt().Time,
		Labels:         labels,
		ReviewDecision: decision,
	}
}

// client returns the cached go-github client for host, building one with a
// freshly minted token on first use.
func (c *Client) client(host string) (*gh.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if client, ok := c.perHost[host]; ok {
		return client, nil
	}
	token, err := c.tokens.Token(host)
	if err != nil {
		return nil, err
	}
	client := gh.NewClient(&http.Client{Timeout: requestTimeout}).WithAuthToken(token)
	if base := c.apiBase(host); base != "" {
		client, err = client.WithEnterpriseURLs(base, base)
		if err != nil {
			return nil, fmt.Errorf("enterprise base URL for %s: %w", host, err)
		}
	}
	c.perHost[host] = client
	return client, nil
}

// reset drops host's cached client and token so the next call re-mints.
func (c *Client) reset(host string) {
	c.mu.Lock()
	delete(c.perHost, host)
	c.mu.Unlock()
	c.tokens.Invalidate(host)
}

// isUnauthorized reports whether err is an HTTP 401 from the API.
func isUnauthorized(err error) bool {
	var ghErr *gh.ErrorResponse
	return errors.As(err, &ghErr) &&
		ghErr.Response != nil &&
		ghErr.Response.StatusCode == http.StatusUnauthorized
}
