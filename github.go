package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	graphql "github.com/hasura/go-graphql-client"
	"golang.org/x/oauth2"
)

const (
	gitHubGraphQLEndpoint = "https://api.github.com/graphql"
	gitHubMaxRetries      = 4
	gitHubRetryBaseDelay  = time.Second
)

func newGitHubClient(token string) *graphql.Client {
	var httpClient graphql.Doer = http.DefaultClient
	if token != "" {
		httpClient = oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		))
	}

	return newGitHubGraphQLClient(gitHubGraphQLEndpoint, httpClient, gitHubRetryBaseDelay)
}

func newGitHubGraphQLClient(endpoint string, httpClient graphql.Doer, retryBaseDelay time.Duration) *graphql.Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return graphql.NewClient(endpoint, &loggingGitHubClient{client: httpClient},
		graphql.WithRetry(gitHubMaxRetries),
		graphql.WithRetryBaseDelay(retryBaseDelay),
		graphql.WithRetryHTTPStatus([]int{
			http.StatusInternalServerError,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		}),
	)
}

type loggingGitHubClient struct {
	client   graphql.Doer
	requests atomic.Uint64
}

func (c *loggingGitHubClient) Do(req *http.Request) (*http.Response, error) {
	id := c.requests.Add(1)
	start := time.Now()
	slog.Info("GitHub HTTP request started", "request", id, "method", req.Method)
	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("GitHub HTTP request failed", "request", id, "duration", time.Since(start), "error_type", fmt.Sprintf("%T", err))
		return resp, err
	}
	slog.Info("GitHub HTTP response", "request", id, "duration", time.Since(start),
		"status", resp.StatusCode, "github_request_id", resp.Header.Get("X-GitHub-Request-Id"),
		"rate_limit", resp.Header.Get("X-RateLimit-Limit"), "rate_remaining", resp.Header.Get("X-RateLimit-Remaining"),
		"rate_reset", resp.Header.Get("X-RateLimit-Reset"), "retry_after", resp.Header.Get("Retry-After"))
	return resp, nil
}

func queryGitHub(ctx context.Context, operation string, query interface{}, variables map[string]interface{}) error {
	start := time.Now()
	// Only log known, non-secret query inputs. Never log HTTP headers or bodies.
	inputs := make(map[string]interface{})
	for _, key := range []string{"username", "owner", "name", "count", "after", "isFork", "author"} {
		if value, ok := variables[key]; ok {
			inputs[key] = value
		}
	}
	encoded, _ := json.Marshal(inputs)
	shape, err := graphql.ConstructQuery(query, variables)
	if err != nil {
		return err
	}
	slog.Info("GitHub query started", "operation", operation, "variables", string(encoded), "query", shape)
	err = gitHubClient.Query(ctx, query, variables)
	if err != nil {
		slog.Error("GitHub query failed", "operation", operation, "duration", time.Since(start), "error_type", fmt.Sprintf("%T", err))
	} else {
		slog.Info("GitHub query completed", "operation", operation, "duration", time.Since(start))
	}
	return err
}
