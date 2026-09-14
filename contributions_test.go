package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	graphql "github.com/hasura/go-graphql-client"
)

func TestContributionDiscoveryPaginatesBeforeRanking(t *testing.T) {
	for _, releases := range []bool{false, true} {
		t.Run(fmt.Sprintf("releases=%t", releases), func(t *testing.T) {
			var repoPages, prPages int
			var retriedPage bool
			var retriedPRPage bool
			histories := map[string]int{}
			metadataQueries := map[string]int{}
			releaseQueries := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Query     string
					Variables map[string]interface{}
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				query := strings.ReplaceAll(req.Query, " ", "")
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(query, "repositories("):
					if req.Variables["after"] == "repos-next" && !retriedPage {
						retriedPage = true
						http.Error(w, "bad gateway", http.StatusBadGateway)
						return
					}
					repoPages++
					if repoPages == 1 {
						if req.Variables["after"] != nil {
							t.Error("first repository cursor must be null")
						}
						fmt.Fprint(w, `{"data":{"viewer":{"id":"USERID","login":"octocat","repositories":{"pageInfo":{"hasNextPage":true,"endCursor":"repos-next"},"edges":[{"node":{"nameWithOwner":"example/old"}},{"node":{"nameWithOwner":"example/no-commits"}}]}}}}`)
					} else {
						if repoPages != 2 || req.Variables["after"] != "repos-next" {
							t.Error("unexpected repository pagination")
						}
						fmt.Fprint(w, `{"data":{"viewer":{"id":"USERID","login":"octocat","repositories":{"pageInfo":{"hasNextPage":false},"edges":[{"node":{"nameWithOwner":"example/new"}}]}}}}`)
					}
				case strings.Contains(query, "pullRequests("):
					if req.Variables["after"] == "prs-next" && !retriedPRPage {
						retriedPRPage = true
						http.Error(w, "bad gateway", http.StatusBadGateway)
						return
					}
					if !strings.Contains(query, "pullRequests(first:20,after:$after)") || strings.Contains(query, "stargazers") || strings.Contains(query, "description") || strings.Contains(query, "orderBy") {
						t.Errorf("unexpected PR query: %s", query)
					}
					prPages++
					if prPages == 1 {
						if req.Variables["after"] != nil {
							t.Error("first PR cursor must be null")
						}
						fmt.Fprint(w, `{"data":{"viewer":{"pullRequests":{"pageInfo":{"hasNextPage":true,"endCursor":"prs-next"},"nodes":[{"state":"MERGED","repository":{"nameWithOwner":"example/new"}},{"state":"OPEN","repository":{"nameWithOwner":"upstream/project"}},{"state":"CLOSED","repository":{"nameWithOwner":"example/unmerged"}}]}}}}`)
					} else {
						if prPages != 2 || req.Variables["after"] != "prs-next" {
							t.Error("unexpected PR pagination")
						}
						fmt.Fprint(w, `{"data":{"viewer":{"pullRequests":{"pageInfo":{"hasNextPage":false},"nodes":[{"state":"MERGED","repository":{"nameWithOwner":"upstream/project"}},{"state":"MERGED","repository":{"nameWithOwner":"upstream/project"}},{"state":"MERGED","repository":{"nameWithOwner":"example/private"}},{"state":"MERGED","repository":{"nameWithOwner":"example/fork"}},{"state":"MERGED","repository":{"nameWithOwner":"octocat/octocat"}}]}}}}`)
					}
				case strings.Contains(query, "history("):
					name := req.Variables["name"].(string)
					histories[name]++
					if !reflect.DeepEqual(req.Variables["author"], map[string]interface{}{"id": "USERID"}) {
						t.Errorf("unexpected author: %v", req.Variables["author"])
					}
					nodes := `[]`
					if date := map[string]string{"old": "2026-07-01T00:00:00Z", "new": "2026-09-10T00:00:00Z", "project": "2026-09-11T00:00:00Z"}[name]; date != "" {
						nodes = fmt.Sprintf(`[{"authoredDate":%q}]`, date)
					}
					fmt.Fprintf(w, `{"data":{"repository":{"defaultBranchRef":{"target":{"history":{"nodes":%s}}}}}}`, nodes)
				case strings.Contains(query, "releases("):
					name := req.Variables["name"].(string)
					releaseQueries[name]++
					date := map[string]string{"old": "2026-09-12T00:00:00Z", "new": "2026-09-10T00:00:00Z", "project": "2026-09-11T00:00:00Z"}[name]
					fmt.Fprintf(w, `{"data":{"repository":{"releases":{"nodes":[{"tagName":"v1","publishedAt":%q}]}}}}`, date)
				case strings.Contains(query, "repository(owner:$owner,name:$name)"):
					name := req.Variables["name"].(string)
					metadataQueries[name]++
					fmt.Fprintf(w, `{"data":{"repository":{"nameWithOwner":%q,"isPrivate":%t,"isFork":%t}}}`, req.Variables["owner"].(string)+"/"+name, name == "private", name == "fork")
				default:
					t.Errorf("unexpected query: %s", query)
					http.Error(w, "unexpected query", 400)
				}
			}))
			defer server.Close()
			oldClient := gitHubClient
			gitHubClient = newGitHubGraphQLClient(server.URL, server.Client(), 0)
			t.Cleanup(func() { gitHubClient = oldClient })
			var names []string
			want := []string{"upstream/project", "example/new"}
			if releases {
				for _, repo := range recentReleases(2) {
					names = append(names, repo.Name)
				}
				// Release recency is independent of contribution recency.
				want = []string{"example/old", "upstream/project"}
				if !reflect.DeepEqual(releaseQueries, map[string]int{"old": 1, "new": 1, "project": 1}) {
					t.Errorf("release queries: %v", releaseQueries)
				}
			} else {
				for _, contribution := range recentContributions(2) {
					names = append(names, contribution.Repo.Name)
				}
			}
			if !reflect.DeepEqual(names, want) {
				t.Errorf("got %v, want %v", names, want)
			}
			if repoPages != 2 || prPages != 2 || !retriedPage || !retriedPRPage {
				t.Errorf("pages: repositories=%d PRs=%d", repoPages, prPages)
			}
			if !reflect.DeepEqual(histories, map[string]int{"old": 1, "new": 1, "project": 1, "no-commits": 1}) {
				t.Errorf("history queries: %v", histories)
			}
			if !reflect.DeepEqual(metadataQueries, map[string]int{"project": 1, "private": 1, "fork": 1, "octocat": 1}) {
				t.Errorf("metadata queries: %v", metadataQueries)
			}
		})
	}
}

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
		case strings.Contains(query, "pullRequests("):
			fmt.Fprint(w, `{"data":{"viewer":{"pullRequests":{"nodes":[]}}}}`)
		case strings.Contains(query, "repositories("):
			repositoryQueries++
			if strings.Contains(query, "repositoriesContributedTo") {
				t.Fatalf("expected query to avoid repositoriesContributedTo, got %s", req.Query)
			}
			if strings.Contains(query, "commitContributionsByRepository") {
				t.Fatalf("expected query to avoid commitContributionsByRepository, got %s", req.Query)
			}
			if !strings.Contains(query, "repositories(first:20,after:$after,affiliations:[OWNER,COLLABORATOR,ORGANIZATION_MEMBER],privacy:PUBLIC,isFork:false,orderBy:{field:PUSHED_AT,direction:DESC})") {
				t.Fatalf("expected viewer repositories query, got %s", req.Query)
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
										"stargazerCount": 0
									}
								},
								{
									"cursor": "private",
									"node": {
										"nameWithOwner": "example/private",
										"url": "https://github.com/example/private",
										"description": "Private repo",
										"isPrivate": true,
										"stargazerCount": 0
									}
								},
								{
									"cursor": "older",
									"node": {
										"nameWithOwner": "example/older",
										"url": "https://github.com/example/older",
										"description": "Older repo",
										"isPrivate": false,
										"stargazerCount": 1
									}
								},
								{
									"cursor": "newer",
									"node": {
										"nameWithOwner": "example/newer",
										"url": "https://github.com/example/newer",
										"description": "Newer repo",
										"isPrivate": false,
										"stargazerCount": 2
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

func TestRecentContributionQueriesUseTimeScalars(t *testing.T) {
	repositoriesQuery, err := graphql.ConstructQuery(&recentContributionRepositoriesQuery{}, map[string]interface{}{
		"after": (*graphql.String)(nil),
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
		var req struct{ Query string }
		json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "pullRequests(") {
			fmt.Fprint(w, `{"data":{"viewer":{"pullRequests":{"nodes":[]}}}}`)
			return
		}
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
									"stargazerCount": 1
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
