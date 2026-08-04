package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	graphql "github.com/hasura/go-graphql-client"
)

var recentPullRequestsQuery struct {
	User struct {
		Login        graphql.String
		PullRequests struct {
			TotalCount graphql.Int
			Edges      []struct {
				Cursor graphql.String
				Node   qlPullRequest
			}
		} `graphql:"pullRequests(first: $count, orderBy: {field: CREATED_AT, direction: DESC})"`
	} `graphql:"user(login:$username)"`
}

var recentReposQuery struct {
	User struct {
		Login        graphql.String
		Repositories struct {
			TotalCount graphql.Int
			Edges      []struct {
				Cursor graphql.String
				Node   qlRepository
			}
		} `graphql:"repositories(first: $count, privacy: PUBLIC, isFork: $isFork, ownerAffiliations: OWNER, orderBy: {field: CREATED_AT, direction: DESC})"`
	} `graphql:"user(login:$username)"`
}

type recentReleasesQuery struct {
	Viewer struct {
		Login        graphql.String
		Repositories struct {
			Edges []struct {
				Cursor graphql.String
				Node   qlRepository
			}
		} `graphql:"repositories(first: $count, affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER], privacy: PUBLIC, orderBy: {field: PUSHED_AT, direction: DESC})"`
	}
}

type recentReleaseQuery struct {
	Repository struct {
		Releases qlRelease `graphql:"releases(first: 10, orderBy: {field: CREATED_AT, direction: DESC})"`
	} `graphql:"repository(owner:$owner, name:$name)"`
}

var repoQuery struct {
	Repository struct {
		Description   graphql.String
		NameWithOwner graphql.String
		IsPrivate     graphql.Boolean
		URL           graphql.String
		Stargazers    struct {
			TotalCount graphql.Int
		}
		Releases qlRelease `graphql:"releases(last: 1)"`
	} `graphql:"repository(owner:$owner, name:$name)"`
}

func recentPullRequests(count int) []PullRequest {
	// fmt.Printf("Finding recently created pullRequests...\n")

	var pullRequests []PullRequest
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count + 1), // +1 in case we encounter the meta-repo itself
	}
	err := gitHubClient.Query(context.Background(), &recentPullRequestsQuery, variables)
	if err != nil {
		panic(err)
	}

	for _, v := range recentPullRequestsQuery.User.PullRequests.Edges {
		// ignore meta-repo
		if string(v.Node.Repository.NameWithOwner) == fmt.Sprintf("%s/%s", username, username) {
			continue
		}
		if v.Node.Repository.IsPrivate {
			continue
		}

		pullRequests = append(pullRequests, pullRequestFromQL(v.Node))
		if len(pullRequests) == count {
			break
		}
	}

	// fmt.Printf("Found %d pullRequests!\n", len(pullRequests))
	return pullRequests
}

func recentRepos(count int) []Repo {
	// fmt.Printf("Finding recently created repos...\n")

	var repos []Repo
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count + 1), // +1 in case we encounter the meta-repo itself
		"isFork":   graphql.Boolean(false),
	}
	err := gitHubClient.Query(context.Background(), &recentReposQuery, variables)
	if err != nil {
		panic(err)
	}

	for _, v := range recentReposQuery.User.Repositories.Edges {
		// ignore meta-repo
		if string(v.Node.NameWithOwner) == fmt.Sprintf("%s/%s", username, username) {
			continue
		}

		repos = append(repos, repoFromQL(v.Node))
		if len(repos) == count {
			break
		}
	}

	// fmt.Printf("Found %d repos!\n", len(repos))
	return repos
}

func recentForks(count int) []Repo {
	// fmt.Printf("Finding recently created repos...\n")

	var repos []Repo
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count + 1), // +1 in case we encounter the meta-repo itself
		"isFork":   graphql.Boolean(true),
	}
	err := gitHubClient.Query(context.Background(), &recentReposQuery, variables)
	if err != nil {
		panic(err)
	}

	for _, v := range recentReposQuery.User.Repositories.Edges {
		// ignore meta-repo
		if string(v.Node.NameWithOwner) == fmt.Sprintf("%s/%s", username, username) {
			continue
		}

		repos = append(repos, repoFromQL(v.Node))
		if len(repos) == count {
			break
		}
	}

	// fmt.Printf("Found %d repos!\n", len(repos))
	return repos
}

func recentReleases(count int) []Repo {
	// fmt.Printf("Finding recent releases...\n")

	if count <= 0 {
		return nil
	}

	var repos []Repo
	var query recentReleasesQuery
	variables := map[string]interface{}{
		"count": graphql.Int(recentReleaseRepositoryLimit(count)),
	}
	err := gitHubClient.Query(context.Background(), &query, variables)
	if err != nil {
		panic(fmt.Errorf("querying recent release repository candidates: %w", err))
	}

	for _, v := range query.Viewer.Repositories.Edges {
		r := repoFromQL(v.Node)
		release, ok := recentRelease(string(v.Node.NameWithOwner))
		if !ok {
			continue
		}

		r.LastRelease = release
		repos = append(repos, r)
	}

	sort.Slice(repos, func(i, j int) bool {
		if repos[i].LastRelease.PublishedAt.Equal(repos[j].LastRelease.PublishedAt) {
			return repos[i].Stargazers > repos[j].Stargazers
		}
		return repos[i].LastRelease.PublishedAt.After(repos[j].LastRelease.PublishedAt)
	})

	// fmt.Printf("Found %d repos!\n", len(repos))
	if len(repos) > count {
		return repos[:count]
	}
	return repos
}

func recentRelease(nameWithOwner string) (Release, bool) {
	owner, name, ok := strings.Cut(nameWithOwner, "/")
	if !ok {
		return Release{}, false
	}

	var query recentReleaseQuery
	variables := map[string]interface{}{
		"owner": graphql.String(owner),
		"name":  graphql.String(name),
	}
	err := gitHubClient.Query(context.Background(), &query, variables)
	if err != nil {
		panic(fmt.Errorf("querying recent releases for %s: %w", nameWithOwner, err))
	}

	for _, rel := range query.Repository.Releases.Nodes {
		if rel.IsPrerelease || rel.IsDraft {
			continue
		}
		if rel.TagName == "" || rel.PublishedAt.IsZero() {
			continue
		}
		return Release{
			Name:        string(rel.Name),
			TagName:     string(rel.TagName),
			PublishedAt: rel.PublishedAt,
			URL:         string(rel.URL),
		}, true
	}

	return Release{}, false
}

func recentReleaseRepositoryLimit(count int) int {
	if count <= 0 {
		return 0
	}

	limit := count + 10
	if limit > 20 {
		return 20
	}
	return limit
}

func repo(owner, name string) Repo {
	variables := map[string]interface{}{
		"owner": graphql.String(owner),
		"name":  graphql.String(name),
	}
	err := gitHubClient.Query(context.Background(), &repoQuery, variables)
	if err != nil {
		panic(err)
	}
	repo := repoQuery.Repository
	return Repo{
		Name:        string(repo.NameWithOwner),
		URL:         string(repo.URL),
		Description: string(repo.Description),
		Stargazers:  int(repo.Stargazers.TotalCount),
		IsPrivate:   bool(repo.IsPrivate),
		LastRelease: releaseFromQL(repo.Releases),
	}
}
