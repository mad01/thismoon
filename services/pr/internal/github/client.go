package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Client struct {
	ghBin string
}

func NewClient() *Client {
	return &Client{ghBin: resolveGH()}
}

func resolveGH() string {
	if p, err := exec.LookPath("gh"); err == nil {
		return p
	}
	for _, p := range []string{
		"/opt/homebrew/bin/gh",
		"/usr/local/bin/gh",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "gh"
}

func (c *Client) ListPRs(ctx context.Context, repo RepoRef) ([]PullRequest, error) {
	out, err := c.ghAPI(
		ctx,
		repo.Host,
		fmt.Sprintf("repos/%s/%s/pulls?state=open&per_page=100", repo.Owner, repo.Name),
	)
	if err != nil {
		return nil, fmt.Errorf("list PRs for %s: %w", repo.FullName(), err)
	}
	var prs []PullRequest
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse PRs for %s: %w", repo.FullName(), err)
	}
	return prs, nil
}

func (c *Client) GetPR(ctx context.Context, repo RepoRef, number int) (*PullRequest, error) {
	out, err := c.ghAPI(
		ctx,
		repo.Host,
		fmt.Sprintf("repos/%s/%s/pulls/%d", repo.Owner, repo.Name, number),
	)
	if err != nil {
		return nil, err
	}
	var pr PullRequest
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) GetDiff(ctx context.Context, repo RepoRef, number int) (string, error) {
	out, err := c.ghAPIWithHeaders(ctx, repo.Host,
		fmt.Sprintf("repos/%s/%s/pulls/%d", repo.Owner, repo.Name, number),
		"Accept: application/vnd.github.v3.diff")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (c *Client) GetFiles(ctx context.Context, repo RepoRef, number int) ([]PRFile, error) {
	out, err := c.ghAPI(
		ctx,
		repo.Host,
		fmt.Sprintf("repos/%s/%s/pulls/%d/files?per_page=100", repo.Owner, repo.Name, number),
	)
	if err != nil {
		return nil, err
	}
	var files []PRFile
	if err := json.Unmarshal(out, &files); err != nil {
		return nil, err
	}
	return files, nil
}

func (c *Client) GetReviews(ctx context.Context, repo RepoRef, number int) ([]Review, error) {
	out, err := c.ghAPI(
		ctx,
		repo.Host,
		fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", repo.Owner, repo.Name, number),
	)
	if err != nil {
		return nil, err
	}
	var reviews []Review
	if err := json.Unmarshal(out, &reviews); err != nil {
		return nil, err
	}
	return reviews, nil
}

func (c *Client) GetChecks(ctx context.Context, repo RepoRef, sha string) (*CheckSuite, error) {
	out, err := c.ghAPI(
		ctx,
		repo.Host,
		fmt.Sprintf("repos/%s/%s/commits/%s/check-runs", repo.Owner, repo.Name, sha),
	)
	if err != nil {
		return nil, err
	}
	var suite CheckSuite
	if err := json.Unmarshal(out, &suite); err != nil {
		return nil, err
	}
	return &suite, nil
}

func (c *Client) Approve(ctx context.Context, repo RepoRef, number int) error {
	body := `{"event":"APPROVE"}`
	_, err := c.ghAPISend(ctx, repo.Host, "POST",
		fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", repo.Owner, repo.Name, number),
		body)
	return err
}

func (c *Client) RequestChanges(
	ctx context.Context,
	repo RepoRef,
	number int,
	message string,
) error {
	payload := map[string]string{"event": "REQUEST_CHANGES", "body": message}
	b, _ := json.Marshal(payload)
	_, err := c.ghAPISend(ctx, repo.Host, "POST",
		fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", repo.Owner, repo.Name, number),
		string(b))
	return err
}

func (c *Client) MergePR(ctx context.Context, repo RepoRef, number int, method string) error {
	if method == "" {
		method = "squash"
	}
	payload := map[string]string{"merge_method": method}
	b, _ := json.Marshal(payload)
	_, err := c.ghAPISend(ctx, repo.Host, "PUT",
		fmt.Sprintf("repos/%s/%s/pulls/%d/merge", repo.Owner, repo.Name, number),
		string(b))
	return err
}

func (c *Client) ghAPI(ctx context.Context, host, path string) ([]byte, error) {
	return c.ghAPIWithHeaders(ctx, host, path)
}

func (c *Client) ghAPIWithHeaders(
	ctx context.Context,
	host, path string,
	headers ...string,
) ([]byte, error) {
	args := []string{"api", path}
	for _, h := range headers {
		args = append(args, "-H", h)
	}
	return c.runGH(ctx, host, args...)
}

// ghAPISend runs `gh api <path> -X <method>` with body piped to stdin.
func (c *Client) ghAPISend(ctx context.Context, host, method, path, body string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.ghBin, "api", path, "-X", method, "--input", "-")
	cmd.Stdin = strings.NewReader(body)
	cmd.Env = append(cmd.Environ(), "GH_HOST="+host)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh api %s %s: %s: %w", method, path, stderr.String(), err)
	}
	return stdout.Bytes(), nil
}

func (c *Client) runGH(ctx context.Context, host string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.ghBin, args...)
	cmd.Env = append(cmd.Environ(), "GH_HOST="+host)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %s: %s: %w", strings.Join(args, " "), stderr.String(), err)
	}
	return stdout.Bytes(), nil
}

type ReviewDecision struct {
	Number         int    `json:"number"`
	ReviewDecision string `json:"reviewDecision"`
}

func (c *Client) GetReviewDecisions(ctx context.Context, repo RepoRef) (map[int]string, error) {
	query := fmt.Sprintf(
		`{ repository(owner: %q, name: %q) { pullRequests(states: OPEN, first: 100) { nodes { number reviewDecision } } } }`,
		repo.Owner,
		repo.Name,
	)
	out, err := c.runGH(ctx, repo.Host, "api", "graphql", "-f", "query="+query)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Repository struct {
				PullRequests struct {
					Nodes []ReviewDecision `json:"nodes"`
				} `json:"pullRequests"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, err
	}
	m := make(map[int]string)
	for _, n := range resp.Data.Repository.PullRequests.Nodes {
		if n.ReviewDecision != "" {
			m[n.Number] = n.ReviewDecision
		}
	}
	return m, nil
}
