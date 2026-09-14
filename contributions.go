package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	graphql "github.com/hasura/go-graphql-client"
)

type contributionPageInfo struct {
	HasNextPage graphql.Boolean
	EndCursor   graphql.String
}

type contributionRepository struct {
	qlRepository
	IsFork graphql.Boolean
}

type gitHubCommitAuthor struct {
	ID graphql.ID `json:"id,omitempty"`
}

func (gitHubCommitAuthor) GetGraphQLType() string {
	return "CommitAuthor"
}

type recentContributionRepositoriesQuery struct {
	Viewer struct {
		ID           graphql.ID
		Login        graphql.String
		Repositories struct {
			PageInfo contributionPageInfo
			Edges    []struct {
				Cursor graphql.String
				Node   contributionRepository
			}
		} `graphql:"repositories(first: 20, after: $after, affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER], privacy: PUBLIC, isFork: false, orderBy: {field: PUSHED_AT, direction: DESC})"`
	}
}

type contributionPullRequestsQuery struct {
	Viewer struct {
		PullRequests struct {
			PageInfo contributionPageInfo
			Nodes    []struct {
				State      graphql.String
				Repository struct {
					NameWithOwner graphql.String
				}
			}
		} `graphql:"pullRequests(first: 20, after: $after)"`
	}
}

type contributionRepositoryQuery struct {
	Repository contributionRepository `graphql:"repository(owner: $owner, name: $name)"`
}

type recentContributionCommitFragment struct {
	History struct {
		Nodes []struct {
			AuthoredDate time.Time
		}
	} `graphql:"history(first: 1, author: $author)"`
}

type recentContributionCommitQuery struct {
	Repository struct {
		DefaultBranchRef struct {
			Target struct {
				recentContributionCommitFragment `graphql:"... on Commit"`
			}
		}
	} `graphql:"repository(owner:$owner, name:$name)"`
}

func recentContributions(count int) []Contribution {
	if count <= 0 {
		return nil
	}

	contributions := contributedRepositories()
	if len(contributions) > count {
		return contributions[:count]
	}
	return contributions
}

// Discover candidates without GitHub's expensive contribution aggregates. Keep
// history lookups separate, and rank only after every candidate page is read.
func contributedRepositories() []Contribution {
	var candidates []contributionRepository
	var author gitHubCommitAuthor
	var login string
	var after *graphql.String
	for {
		var query recentContributionRepositoriesQuery
		if err := gitHubClient.Query(context.Background(), &query, map[string]interface{}{"after": after}); err != nil {
			panic(fmt.Errorf("querying recent contribution repository candidates: %w", err))
		}
		author.ID = query.Viewer.ID
		login = string(query.Viewer.Login)
		for _, edge := range query.Viewer.Repositories.Edges {
			candidates = append(candidates, edge.Node)
		}
		page := query.Viewer.Repositories.PageInfo
		if !page.HasNextPage {
			break
		}
		after = graphql.NewString(page.EndCursor)
	}

	// Avoid filtering and resolving repository metadata inside the PR connection.
	// One repository can appear in thousands of PRs; resolve it only once.
	known := make(map[graphql.String]bool, len(candidates))
	for _, repo := range candidates {
		known[repo.NameWithOwner] = true
	}
	var upstream []graphql.String
	after = nil
	for {
		var query contributionPullRequestsQuery
		if err := gitHubClient.Query(context.Background(), &query, map[string]interface{}{"after": after}); err != nil {
			panic(fmt.Errorf("querying contribution pull request repositories: %w", err))
		}
		for _, pr := range query.Viewer.PullRequests.Nodes {
			name := pr.Repository.NameWithOwner
			if pr.State != "MERGED" || name == "" || known[name] {
				continue
			}
			known[name] = true
			upstream = append(upstream, name)
		}
		page := query.Viewer.PullRequests.PageInfo
		if !page.HasNextPage {
			break
		}
		after = graphql.NewString(page.EndCursor)
	}
	for _, nameWithOwner := range upstream {
		owner, name, _ := strings.Cut(string(nameWithOwner), "/")
		var query contributionRepositoryQuery
		variables := map[string]interface{}{
			"owner": graphql.String(owner),
			"name":  graphql.String(name),
		}
		if err := gitHubClient.Query(context.Background(), &query, variables); err != nil {
			panic(fmt.Errorf("querying contribution repository %s: %w", nameWithOwner, err))
		}
		candidates = append(candidates, query.Repository)
	}

	var contributions []Contribution
	seen := make(map[graphql.String]bool)
	for _, repo := range candidates {
		if bool(repo.IsPrivate) || bool(repo.IsFork) || string(repo.NameWithOwner) == login+"/"+login || seen[repo.NameWithOwner] {
			continue
		}
		seen[repo.NameWithOwner] = true

		occurredAt, ok := recentContributionOccurredAt(string(repo.NameWithOwner), author)
		if !ok {
			continue
		}

		contributions = append(contributions, Contribution{
			Repo:       repoFromQL(repo.qlRepository),
			OccurredAt: occurredAt,
		})
	}

	sort.Slice(contributions, func(i, j int) bool {
		return contributions[i].OccurredAt.After(contributions[j].OccurredAt)
	})

	return contributions
}

func recentContributionOccurredAt(nameWithOwner string, author gitHubCommitAuthor) (time.Time, bool) {
	owner, name, ok := strings.Cut(nameWithOwner, "/")
	if !ok {
		return time.Time{}, false
	}

	var query recentContributionCommitQuery
	variables := map[string]interface{}{
		"owner":  graphql.String(owner),
		"name":   graphql.String(name),
		"author": author,
	}
	err := gitHubClient.Query(context.Background(), &query, variables)
	if err != nil {
		panic(fmt.Errorf("querying recent contribution commits for %s: %w", nameWithOwner, err))
	}

	commits := query.Repository.DefaultBranchRef.Target.History.Nodes
	if len(commits) == 0 {
		return time.Time{}, false
	}
	return commits[0].AuthoredDate, true
}
