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

func TestRecentContributionsQueriesRecentRepositoriesAndCommitHistory(t *testing.T) {
	var repositoryQueries int
	var historyQueries int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}

		query := strings.ReplaceAll(req.Query, " ", "")
		switch {
		case strings.Contains(query, "repositories("):
			repositoryQueries++
			if strings.Contains(query, "repositoriesContributedTo") {
				t.Fatalf("expected query to avoid repositoriesContributedTo, got %s", req.Query)
			}
			if strings.Contains(query, "commitContributionsByRepository") {
				t.Fatalf("expected query to avoid commitContributionsByRepository, got %s", req.Query)
			}
			if !strings.Contains(query, "repositories(first:$maxRepositories,affiliations:[OWNER,COLLABORATOR,ORGANIZATION_MEMBER],privacy:PUBLIC,isFork:false,orderBy:{field:PUSHED_AT,direction:DESC})") {
				t.Fatalf("expected viewer repositories query, got %s", req.Query)
			}
			if got := req.Variables["maxRepositories"]; got != float64(7) {
				t.Fatalf("unexpected maxRepositories variable: %#v", got)
			}

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"data": {
					"viewer": {
						"id": "USERID",
						"login": "octocat",
						"repositories": {
							"edges": [
								{
									"cursor": "profile",
									"node": {
										"nameWithOwner": "octocat/octocat",
										"url": "https://github.com/octocat/octocat",
										"description": "Profile",
										"isPrivate": false,
										"stargazers": {"totalCount": 0}
									}
								},
								{
									"cursor": "private",
									"node": {
										"nameWithOwner": "example/private",
										"url": "https://github.com/example/private",
										"description": "Private repo",
										"isPrivate": true,
										"stargazers": {"totalCount": 0}
									}
								},
								{
									"cursor": "older",
									"node": {
										"nameWithOwner": "example/older",
										"url": "https://github.com/example/older",
										"description": "Older repo",
										"isPrivate": false,
										"stargazers": {"totalCount": 1}
									}
								},
								{
									"cursor": "newer",
									"node": {
										"nameWithOwner": "example/newer",
										"url": "https://github.com/example/newer",
										"description": "Newer repo",
										"isPrivate": false,
										"stargazers": {"totalCount": 2}
									}
								}
							]
						}
					}
				}
			}`)
		case strings.Contains(query, "history(first:1,author:$author)"):
			historyQueries++
			author, ok := req.Variables["author"].(map[string]interface{})
			if !ok {
				t.Fatalf("expected author variable object, got %#v", req.Variables["author"])
			}
			if got := author["id"]; got != "USERID" {
				t.Fatalf("unexpected author ID: %#v", got)
			}

			authoredDateByRepo := map[interface{}]string{
				"older": "2026-07-28T06:00:00Z",
				"newer": "2026-07-28T07:00:00Z",
			}
			authoredDate, ok := authoredDateByRepo[req.Variables["name"]]
			if !ok {
				t.Fatalf("unexpected history lookup variables: %#v", req.Variables)
			}

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{
				"data": {
					"repository": {
						"defaultBranchRef": {
							"target": {
								"history": {
									"nodes": [
										{"authoredDate": %q}
									]
								}
							}
						}
					}
				}
			}`, authoredDate)
		default:
			t.Fatalf("unexpected GraphQL query: %s", req.Query)
		}
	}))
	defer server.Close()

	oldClient := gitHubClient
	oldUsername := username
	gitHubClient = newGitHubGraphQLClient(server.URL, server.Client(), 0)
	username = "octocat"
	t.Cleanup(func() {
		gitHubClient = oldClient
		username = oldUsername
	})

	contributions := recentContributions(2)
	if len(contributions) != 2 {
		t.Fatalf("expected 2 contributions, got %d", len(contributions))
	}
	if contributions[0].Repo.Name != "example/newer" {
		t.Fatalf("unexpected first repo: %+v", contributions[0].Repo)
	}
	if contributions[1].Repo.Name != "example/older" {
		t.Fatalf("unexpected second repo: %+v", contributions[1].Repo)
	}
	if repositoryQueries != 1 {
		t.Fatalf("expected 1 viewer repositories query, got %d", repositoryQueries)
	}
	if historyQueries != 2 {
		t.Fatalf("expected 2 commit history queries, got %d", historyQueries)
	}
}

func TestRecentContributionRepositoryLimit(t *testing.T) {
	tests := map[int]int{
		0:  0,
		1:  6,
		5:  10,
		10: 15,
		30: gitHubMaxContributionRepositories,
	}

	for count, want := range tests {
		if got := recentContributionRepositoryLimit(count); got != want {
			t.Fatalf("count %d: expected %d, got %d", count, want, got)
		}
	}
}

func TestRecentContributionQueriesUseTimeScalars(t *testing.T) {
	repositoriesQuery, err := graphql.ConstructQuery(&recentContributionRepositoriesQuery{}, map[string]interface{}{
		"maxRepositories": graphql.Int(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	compactRepositoriesQuery := strings.ReplaceAll(repositoriesQuery, " ", "")
	if strings.Contains(compactRepositoriesQuery, "repositoriesContributedTo") {
		t.Fatalf("expected query to avoid repositoriesContributedTo, got %s", repositoriesQuery)
	}
	if strings.Contains(compactRepositoriesQuery, "commitContributionsByRepository") {
		t.Fatalf("expected query to avoid commitContributionsByRepository, got %s", repositoriesQuery)
	}
	if !strings.Contains(compactRepositoriesQuery, "orderBy") {
		t.Fatalf("expected query to order repository candidates by pushed date, got %s", repositoriesQuery)
	}
	if !strings.Contains(compactRepositoriesQuery, "isFork:false") {
		t.Fatalf("expected query to exclude forks, got %s", repositoriesQuery)
	}

	commitQuery, err := graphql.ConstructQuery(&recentContributionCommitQuery{}, map[string]interface{}{
		"owner":  graphql.String("example"),
		"name":   graphql.String("repo"),
		"author": gitHubCommitAuthor{ID: "USERID"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(commitQuery, "authoredDate") {
		t.Fatalf("expected query to include authoredDate, got %s", commitQuery)
	}
	if !strings.Contains(commitQuery, "CommitAuthor!") {
		t.Fatalf("expected query to use CommitAuthor input, got %s", commitQuery)
	}
	if strings.Contains(commitQuery, "wall") || strings.Contains(commitQuery, "ext") || strings.Contains(commitQuery, "loc") {
		t.Fatalf("expected time.Time to be treated as a scalar, got %s", commitQuery)
	}
}

func TestRecentContributionsWrapsRepositoryQueryErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	oldClient := gitHubClient
	oldUsername := username
	gitHubClient = newGitHubGraphQLClient(server.URL, server.Client(), 0)
	username = "octocat"
	t.Cleanup(func() {
		gitHubClient = oldClient
		username = oldUsername
	})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if !strings.Contains(fmt.Sprint(r), "querying recent contribution repository candidates") {
			t.Fatalf("expected repository query context, got %v", r)
		}
	}()

	recentContributions(1)
}

func TestRecentContributionsWrapsCommitHistoryQueryErrors(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 1 {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"data": {
				"viewer": {
					"id": "USERID",
					"login": "octocat",
					"repositories": {
						"edges": [
							{
								"cursor": "repo",
								"node": {
									"nameWithOwner": "example/repo",
									"url": "https://github.com/example/repo",
									"description": "Example repo",
									"isPrivate": false,
									"stargazers": {"totalCount": 1}
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
	oldUsername := username
	gitHubClient = newGitHubGraphQLClient(server.URL, server.Client(), 0)
	username = "octocat"
	t.Cleanup(func() {
		gitHubClient = oldClient
		username = oldUsername
	})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if !strings.Contains(fmt.Sprint(r), "querying recent contribution commits for example/repo") {
			t.Fatalf("expected commit history query context, got %v", r)
		}
	}()

	recentContributions(1)
}
