package server

import (
	"encoding/json"
	"net/http"

	"github.com/mad01/thismoon/services/pr/internal/github"
)

type actionRequest struct {
	Host    string `json:"host"`
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	Message string `json:"message,omitempty"`
	Method  string `json:"method,omitempty"`
}

func decodeAction(r *http.Request) (actionRequest, github.RepoRef, error) {
	var req actionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, github.RepoRef{}, err
	}
	ref := github.RepoRef{Owner: req.Owner, Name: req.Repo, Host: req.Host}
	return req, ref, nil
}

func handleApprove(client *github.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ref, err := decodeAction(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := client.Approve(r.Context(), ref, req.Number); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleMerge(client *github.Client, poller *Poller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ref, err := decodeAction(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method := req.Method
		if method == "" {
			method = "squash"
		}
		if err := client.MergePR(r.Context(), ref, req.Number, method); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		go poller.Refresh(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}
}
