package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	graphql "github.com/hasura/go-graphql-client"
)

func TestRecentReleasesQueriesViewerRepositories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}

		query := strings.ReplaceAll(req.Query, " ", "")
		if strings.Contains(query, "repositoriesContributedTo") {
			t.Fatalf("expected query to avoid repositoriesContributedTo, got %s", req.Query)
		}
		if !strings.Contains(query, "repositories(first:$count,affiliations:[OWNER,COLLABORATOR,ORGANIZATION_MEMBER],privacy:PUBLIC,orderBy:{field:PUSHED_AT,direction:DESC})") {
			t.Fatalf("expected viewer repositories query, got %s", req.Query)
		}
		if got := req.Variables["count"]; got != float64(20) {
			t.Fatalf("unexpected count variable: %#v", got)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"data": {
				"viewer": {
					"repositories": {
						"edges": [
							{
								"cursor": "older",
								"node": {
									"nameWithOwner": "example/older",
									"url": "https://github.com/example/older",
									"description": "Older repo",
									"isPrivate": false,
									"stargazers": {"totalCount": 1},
									"releases": {
										"nodes": [
											{
												"name": "v1.0.0",
												"tagName": "v1.0.0",
												"publishedAt": "2026-07-28T06:00:00Z",
												"url": "https://github.com/example/older/releases/tag/v1.0.0",
												"isPrerelease": false,
												"isDraft": false
											}
										]
									}
								}
							},
							{
								"cursor": "newer",
								"node": {
									"nameWithOwner": "example/newer",
									"url": "https://github.com/example/newer",
									"description": "Newer repo",
									"isPrivate": false,
									"stargazers": {"totalCount": 2},
									"releases": {
										"nodes": [
											{
												"name": "v2.0.0-rc1",
												"tagName": "v2.0.0-rc1",
												"publishedAt": "2026-07-28T08:00:00Z",
												"url": "https://github.com/example/newer/releases/tag/v2.0.0-rc1",
												"isPrerelease": true,
												"isDraft": false
											},
											{
												"name": "v2.0.0",
												"tagName": "v2.0.0",
												"publishedAt": "2026-07-28T07:00:00Z",
												"url": "https://github.com/example/newer/releases/tag/v2.0.0",
												"isPrerelease": false,
												"isDraft": false
											}
										]
									}
								}
							}
						]
					}
				}
			}
		}`)
	}))
	defer server.Close()

	oldClient := gitHubClient
	gitHubClient = newGitHubGraphQLClient(server.URL, server.Client(), 0)
	t.Cleanup(func() {
		gitHubClient = oldClient
	})

	repos := recentReleases(2)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0].Name != "example/newer" {
		t.Fatalf("unexpected first repo: %+v", repos[0])
	}
	if repos[0].LastRelease.TagName != "v2.0.0" {
		t.Fatalf("unexpected first release: %+v", repos[0].LastRelease)
	}
	if repos[1].Name != "example/older" {
		t.Fatalf("unexpected second repo: %+v", repos[1])
	}
}

func TestRecentReleaseRepositoryLimit(t *testing.T) {
	tests := map[int]int{
		0:  0,
		1:  10,
		5:  50,
		10: 50,
	}

	for count, want := range tests {
		if got := recentReleaseRepositoryLimit(count); got != want {
			t.Fatalf("count %d: expected %d, got %d", count, want, got)
		}
	}
}

func TestRecentReleasesWrapsRepositoryQueryErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	oldClient := gitHubClient
	gitHubClient = newGitHubGraphQLClient(server.URL, server.Client(), 0)
	t.Cleanup(func() {
		gitHubClient = oldClient
	})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if !strings.Contains(fmt.Sprint(r), "querying recent release repository candidates") {
			t.Fatalf("expected repository query context, got %v", r)
		}
	}()

	recentReleases(1)
}

func TestRecentReleaseQueryAvoidsContributionAggregates(t *testing.T) {
	query, err := graphql.ConstructQuery(&recentReleasesQuery, map[string]interface{}{
		"count": graphql.Int(10),
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(query, "repositoriesContributedTo") {
		t.Fatalf("expected query to avoid repositoriesContributedTo, got %s", query)
	}
}

func TestRecentReleasesReturnsNilForNonPositiveCount(t *testing.T) {
	oldClient := gitHubClient
	gitHubClient = graphql.NewClient("http://127.0.0.1:1/graphql", nil)
	t.Cleanup(func() {
		gitHubClient = oldClient
	})

	if repos := recentReleases(0); repos != nil {
		t.Fatalf("expected nil repos, got %#v", repos)
	}
}
