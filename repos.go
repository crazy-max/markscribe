package main

import (
	"context"
	"fmt"
	"log/slog"
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

type recentReleaseQuery struct {
	Repository struct {
		Releases qlRelease `graphql:"releases(first: 10, orderBy: {field: CREATED_AT, direction: DESC})"`
	} `graphql:"repository(owner:$owner, name:$name)"`
}

var repoQuery struct {
	Repository struct {
		Description    graphql.String
		NameWithOwner  graphql.String
		IsPrivate      graphql.Boolean
		URL            graphql.String
		StargazerCount graphql.Int
		Releases       qlRelease `graphql:"releases(last: 1)"`
	} `graphql:"repository(owner:$owner, name:$name)"`
}

func recentPullRequests(count int) []PullRequest {
	defer logOperation("recentPullRequests", "count", count)()

	var pullRequests []PullRequest
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count + 1), // +1 in case we encounter the meta-repo itself
	}
	err := queryGitHub(context.Background(), "recentPullRequests", &recentPullRequestsQuery, variables)
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
	slog.Info("Results selected", "kind", "pull requests", "items", len(pullRequests))
	return pullRequests
}

func recentRepos(count int) []Repo {
	defer logOperation("recentRepos", "count", count)()

	var repos []Repo
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count + 1), // +1 in case we encounter the meta-repo itself
		"isFork":   graphql.Boolean(false),
	}
	err := queryGitHub(context.Background(), "recentRepos", &recentReposQuery, variables)
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
	slog.Info("Results selected", "kind", "repositories", "items", len(repos))
	return repos
}

func recentForks(count int) []Repo {
	defer logOperation("recentForks", "count", count)()

	var repos []Repo
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count + 1), // +1 in case we encounter the meta-repo itself
		"isFork":   graphql.Boolean(true),
	}
	err := queryGitHub(context.Background(), "recentForks", &recentReposQuery, variables)
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
	slog.Info("Results selected", "kind", "repositories", "items", len(repos))
	return repos
}

func recentReleases(count int) []Repo {
	defer logOperation("recentReleases", "count", count)()

	if count <= 0 {
		return nil
	}

	var repos []Repo
	for _, contribution := range contributedRepositories() {
		r := contribution.Repo
		release, ok := recentRelease(r.Name)
		if !ok {
			slog.Info("Release repository skipped", "repository", r.Name, "reason", "no eligible release")
			continue
		}

		r.LastRelease = release
		slog.Info("Release selected", "repository", r.Name, "tag", release.TagName, "published_at", release.PublishedAt)
		repos = append(repos, r)
	}

	sort.Slice(repos, func(i, j int) bool {
		if repos[i].LastRelease.PublishedAt.Equal(repos[j].LastRelease.PublishedAt) {
			return repos[i].Stargazers > repos[j].Stargazers
		}
		return repos[i].LastRelease.PublishedAt.After(repos[j].LastRelease.PublishedAt)
	})
	if len(repos) > count {
		slog.Info("Results selected", "kind", "repositories", "items", len(repos[:count]))
		return repos[:count]
	}
	slog.Info("Results selected", "kind", "repositories", "items", len(repos))
	return repos
}

func recentRelease(nameWithOwner string) (Release, bool) {
	defer logOperation("recentRelease", "repository", nameWithOwner)()
	owner, name, ok := strings.Cut(nameWithOwner, "/")
	if !ok {
		return Release{}, false
	}

	var query recentReleaseQuery
	variables := map[string]interface{}{
		"owner": graphql.String(owner),
		"name":  graphql.String(name),
	}
	err := queryGitHub(context.Background(), "recentRelease", &query, variables)
	if err != nil {
		panic(fmt.Errorf("querying recent releases for %s: %w", nameWithOwner, err))
	}

	for _, rel := range query.Repository.Releases.Nodes {
		if rel.IsPrerelease || rel.IsDraft {
			slog.Info("Release skipped", "repository", nameWithOwner, "tag", rel.TagName, "draft", rel.IsDraft, "prerelease", rel.IsPrerelease)
			continue
		}
		if rel.TagName == "" || rel.PublishedAt.IsZero() {
			slog.Info("Release skipped", "repository", nameWithOwner, "reason", "missing tag or publication date")
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

func repo(owner, name string) Repo {
	defer logOperation("repo", "owner", owner, "name", name)()
	variables := map[string]interface{}{
		"owner": graphql.String(owner),
		"name":  graphql.String(name),
	}
	err := queryGitHub(context.Background(), "repo", &repoQuery, variables)
	if err != nil {
		panic(err)
	}
	repo := repoQuery.Repository
	return Repo{
		Name:        string(repo.NameWithOwner),
		URL:         string(repo.URL),
		Description: string(repo.Description),
		Stargazers:  int(repo.StargazerCount),
		IsPrivate:   bool(repo.IsPrivate),
		LastRelease: releaseFromQL(repo.Releases),
	}
}
