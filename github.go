package main

import (
	"context"
	"net/http"
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
	return graphql.NewClient(endpoint, httpClient,
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
