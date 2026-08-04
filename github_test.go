package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubGraphQLClientRetriesTransientStatus(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"viewer":{"login":"octocat"}}}`)
	}))
	defer server.Close()

	client := newGitHubGraphQLClient(server.URL, server.Client(), 0)

	var query struct {
		Viewer struct {
			Login string
		}
	}
	if err := client.Query(context.Background(), &query, nil); err != nil {
		t.Fatal(err)
	}
	if query.Viewer.Login != "octocat" {
		t.Fatalf("unexpected login: %q", query.Viewer.Login)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestGitHubGraphQLClientDoesNotRetryPermanentStatus(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "bad credentials", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newGitHubGraphQLClient(server.URL, server.Client(), 0)

	var query struct {
		Viewer struct {
			Login string
		}
	}
	if err := client.Query(context.Background(), &query, nil); err == nil {
		t.Fatal("expected query to fail")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}
