package main

import (
	"time"

	graphql "github.com/hasura/go-graphql-client"
)

// Contribution represents a contribution to a repo.
type Contribution struct {
	OccurredAt time.Time
	Repo       Repo
}

// Gist represents a gist.
type Gist struct {
	Name        string
	Description string
	URL         string
	CreatedAt   time.Time
}

// Star represents a star/favorite event.
type Star struct {
	StarredAt time.Time
	Repo      Repo
}

// PullRequest represents a pull request.
type PullRequest struct {
	Title     string
	URL       string
	State     string
	CreatedAt time.Time
	Repo      Repo
}

// Release represents a release.
type Release struct {
	Name        string
	TagName     string
	PublishedAt time.Time
	URL         string
}

// Repo represents a git repo.
type Repo struct {
	Name        string
	URL         string
	Description string
	IsPrivate   bool
	Stargazers  int
	LastRelease Release
}

// Sponsor represents a sponsor.
type Sponsor struct {
	User      User
	CreatedAt time.Time
}

// User represents a SCM user.
type User struct {
	Login     string
	Name      string
	AvatarURL string
	URL       string
}

type qlGist struct {
	Name        graphql.String
	Description graphql.String
	URL         graphql.String
	CreatedAt   time.Time
}

type qlPullRequest struct {
	URL        graphql.String
	Title      graphql.String
	State      graphql.String
	CreatedAt  time.Time
	Repository qlRepository
}

type qlRelease struct {
	Nodes []struct {
		Name         graphql.String
		TagName      graphql.String
		PublishedAt  time.Time
		URL          graphql.String
		IsPrerelease graphql.Boolean
		IsDraft      graphql.Boolean
	}
}

type qlRepository struct {
	NameWithOwner graphql.String
	URL           graphql.String
	Description   graphql.String
	IsPrivate     graphql.Boolean
	Stargazers    struct {
		TotalCount graphql.Int
	}
}

type qlUser struct {
	Login     graphql.String
	Name      graphql.String
	AvatarURL graphql.String
	URL       graphql.String
}

func gistFromQL(gist qlGist) Gist {
	return Gist{
		Name:        string(gist.Name),
		Description: string(gist.Description),
		URL:         string(gist.URL),
		CreatedAt:   gist.CreatedAt,
	}
}

func pullRequestFromQL(pullRequest qlPullRequest) PullRequest {
	return PullRequest{
		Title:     string(pullRequest.Title),
		URL:       string(pullRequest.URL),
		State:     string(pullRequest.State),
		CreatedAt: pullRequest.CreatedAt,
		Repo:      repoFromQL(pullRequest.Repository),
	}
}

func releaseFromQL(release qlRelease) Release {
	return Release{
		Name:        string(release.Nodes[0].Name),
		TagName:     string(release.Nodes[0].TagName),
		PublishedAt: release.Nodes[0].PublishedAt,
		URL:         string(release.Nodes[0].URL),
	}
}

func repoFromQL(repo qlRepository) Repo {
	return Repo{
		Name:        string(repo.NameWithOwner),
		URL:         string(repo.URL),
		Description: string(repo.Description),
		Stargazers:  int(repo.Stargazers.TotalCount),
		IsPrivate:   bool(repo.IsPrivate),
	}
}

func userFromQL(user qlUser) User {
	return User{
		Login:     string(user.Login),
		Name:      string(user.Name),
		AvatarURL: string(user.AvatarURL),
		URL:       string(user.URL),
	}
}
