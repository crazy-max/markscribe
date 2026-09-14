package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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

type gitHubQueryLogKey struct{}

var gitHubQuerySequence atomic.Uint64

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
		graphql.WithRetryOnGraphQLError(retryGitHubGraphQLErrors),
		graphql.WithRetryHTTPStatus([]int{
			http.StatusInternalServerError,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		}),
	)
}

func retryGitHubGraphQLErrors(errs graphql.Errors) bool {
	if len(errs) == 0 {
		return false
	}
	logGitHubErrors(slog.Default(), errs)
	for _, err := range errs {
		// GitHub can return an internal execution failure with HTTP 200 and
		// no error code. Match its specific message, not arbitrary API errors.
		if !strings.HasPrefix(err.Message, "Something went wrong while executing your query") {
			return false
		}
	}
	slog.Warn("Retrying GitHub GraphQL execution failure", "errors", len(errs))
	return true
}

type loggingGitHubClient struct {
	client   graphql.Doer
	requests atomic.Uint64
}

func (c *loggingGitHubClient) Do(req *http.Request) (*http.Response, error) {
	id := c.requests.Add(1)
	start := time.Now()
	logger, ok := req.Context().Value(gitHubQueryLogKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}
	logger.Info("GitHub HTTP request started", "request", id, "method", req.Method)
	resp, err := c.client.Do(req)
	if err != nil {
		logger.Error("GitHub HTTP request failed", "request", id, "duration", time.Since(start), "error_type", fmt.Sprintf("%T", err))
		return resp, err
	}
	logger.Info("GitHub HTTP response", "request", id, "duration", time.Since(start),
		"status", resp.StatusCode, "github_request_id", resp.Header.Get("X-GitHub-Request-Id"),
		"rate_limit", resp.Header.Get("X-RateLimit-Limit"), "rate_remaining", resp.Header.Get("X-RateLimit-Remaining"),
		"rate_reset", resp.Header.Get("X-RateLimit-Reset"), "retry_after", resp.Header.Get("Retry-After"))
	return resp, nil
}

func queryGitHub(ctx context.Context, operation string, query interface{}, variables map[string]interface{}) error {
	start := time.Now()
	logger := slog.Default().With("operation", operation, "query_id", gitHubQuerySequence.Add(1))
	ctx = context.WithValue(ctx, gitHubQueryLogKey{}, logger)
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
	logger.Info("GitHub query started", "variables", string(encoded), "query", shape)
	err = gitHubClient.Query(ctx, query, variables)
	if err != nil {
		var graphqlErrors graphql.Errors
		if errors.As(err, &graphqlErrors) {
			logGitHubErrors(logger, graphqlErrors)
		}
		logger.Error("GitHub query failed", "duration", time.Since(start), "error_type", fmt.Sprintf("%T", err))
	} else {
		logger.Info("GitHub query completed", "duration", time.Since(start))
	}
	return err
}

func logGitHubErrors(logger *slog.Logger, errs graphql.Errors) {
	for i, err := range errs {
		// Client-generated errors can contain raw HTTP bodies. Only expand
		// server GraphQL errors, not transport or response-decoding failures.
		if err.Unwrap() != nil {
			continue
		}
		extensions, _ := json.Marshal(err.Extensions)
		path, _ := json.Marshal(err.Path)
		locations, _ := json.Marshal(err.Locations)
		logger.Error("GitHub GraphQL error", "error_index", i,
			"message", err.Message, "extensions", string(extensions),
			"path", string(path), "locations", string(locations))
	}
}
