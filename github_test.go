package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	graphql "github.com/hasura/go-graphql-client"
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

func TestGitHubQueryConstructionUsesTimeScalars(t *testing.T) {
	query, err := graphql.ConstructQuery(&recentContributionsQuery, map[string]interface{}{
		"username": graphql.String("octocat"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(query, "occurredAt") {
		t.Fatalf("expected query to include occurredAt, got %s", query)
	}
	if strings.Contains(query, "wall") || strings.Contains(query, "ext") || strings.Contains(query, "loc") {
		t.Fatalf("expected time.Time to be treated as a scalar, got %s", query)
	}
}
