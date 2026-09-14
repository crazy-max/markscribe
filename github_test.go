package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestGitHubGraphQLExecutionErrorRetries(t *testing.T) {
	const internal = `{"message":"Something went wrong while executing your query on 2026-09-14T11:30:04Z. Please include REQUEST-ID when reporting this issue.","extensions":{}}`
	for _, tt := range []struct {
		name     string
		errors   string
		recover  bool
		attempts int
	}{
		{name: "transient execution error", errors: internal, recover: true, attempts: 2},
		{name: "persistent execution error", errors: internal, attempts: gitHubMaxRetries + 1},
		{name: "permission error", errors: `{"message":"Resource not accessible by integration","extensions":{"type":"FORBIDDEN"}}`, attempts: 1},
		{name: "validation error", errors: `{"message":"Field does not exist","extensions":{"code":"undefinedField"}}`, attempts: 1},
		{name: "mixed errors", errors: internal + `,{"message":"Could not resolve to a Repository","extensions":{"type":"NOT_FOUND"}}`, attempts: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var attempts int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				w.Header().Set("Content-Type", "application/json")
				if tt.recover && attempts > 1 {
					fmt.Fprint(w, `{"data":{"viewer":{"login":"octocat"}}}`)
					return
				}
				fmt.Fprintf(w, `{"data":null,"errors":[%s]}`, tt.errors)
			}))
			defer server.Close()
			client := newGitHubGraphQLClient(server.URL, server.Client(), 0)
			var query struct{ Viewer struct{ Login string } }
			err := client.Query(context.Background(), &query, nil)
			if (err == nil) != tt.recover {
				t.Fatalf("unexpected result: %v", err)
			}
			if attempts != tt.attempts {
				t.Errorf("got %d attempts, want %d", attempts, tt.attempts)
			}
			if tt.recover && query.Viewer.Login != "octocat" {
				t.Errorf("unexpected login: %s", query.Viewer.Login)
			}
		})
	}
}

func TestGitHubGraphQLDoesNotRetryEmptyErrors(t *testing.T) {
	if retryGitHubGraphQLErrors(graphql.Errors{}) {
		t.Fatal("empty errors must not be retried")
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
