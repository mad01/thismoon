package github

import (
	"strconv"
	"time"
)

type PullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Draft     bool      `json:"draft"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	User      User      `json:"user"`
	Head      Ref       `json:"head"`
	Base      Ref       `json:"base"`

	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
	ChangedFiles int `json:"changed_files"`

	Merged   bool       `json:"merged"`
	MergedAt *time.Time `json:"merged_at"`

	Labels             []Label `json:"labels"`
	RequestedReviewers []User  `json:"requested_reviewers"`
	Mergeable          *bool   `json:"mergeable"`
	MergeableState     string  `json:"mergeable_state"`
}

type User struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

type Ref struct {
	Ref  string  `json:"ref"`
	SHA  string  `json:"sha"`
	Repo RefRepo `json:"repo"`
}

type RefRepo struct {
	FullName string `json:"full_name"`
	Archived bool   `json:"archived"`
}

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Review struct {
	ID          int       `json:"id"`
	User        User      `json:"user"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
	HTMLURL     string    `json:"html_url"`
}

type PRFile struct {
	Filename    string `json:"filename"`
	Status      string `json:"status"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Changes     int    `json:"changes"`
	Patch       string `json:"patch"`
	BlobURL     string `json:"blob_url"`
	ContentsURL string `json:"contents_url"`
}

type CheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type CheckSuite struct {
	CheckRuns  []CheckRun `json:"check_runs"`
	TotalCount int        `json:"total_count"`
}

type RepoRef struct {
	Owner string
	Name  string
	Host  string
}

func (r RepoRef) FullName() string { return r.Owner + "/" + r.Name }

func (r RepoRef) HTMLURL() string {
	return "https://" + r.Host + "/" + r.Owner + "/" + r.Name
}

func (r RepoRef) PRURL(number int) string {
	return r.HTMLURL() + "/pull/" + strconv.Itoa(number)
}
