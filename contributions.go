package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	graphql "github.com/hasura/go-graphql-client"
)

const gitHubMaxContributionRepositories = 15

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
			Edges []struct {
				Cursor graphql.String
				Node   qlRepository
			}
		} `graphql:"repositories(first: $maxRepositories, affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER], privacy: PUBLIC, orderBy: {field: PUSHED_AT, direction: DESC})"`
	}
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

	var contributions []Contribution
	var query recentContributionRepositoriesQuery
	variables := map[string]interface{}{
		"maxRepositories": graphql.Int(recentContributionRepositoryLimit(count)),
	}
	err := gitHubClient.Query(context.Background(), &query, variables)
	if err != nil {
		panic(fmt.Errorf("querying recent contribution repository candidates: %w", err))
	}

	login := string(query.Viewer.Login)
	author := gitHubCommitAuthor{ID: query.Viewer.ID}
	for _, edge := range query.Viewer.Repositories.Edges {
		repo := edge.Node
		if string(repo.NameWithOwner) == login+"/"+login {
			continue
		}
		if repo.IsPrivate {
			continue
		}

		occurredAt, ok := recentContributionOccurredAt(string(repo.NameWithOwner), author)
		if !ok {
			continue
		}

		contributions = append(contributions, Contribution{
			Repo:       repoFromQL(repo),
			OccurredAt: occurredAt,
		})
	}

	sort.Slice(contributions, func(i, j int) bool {
		return contributions[i].OccurredAt.After(contributions[j].OccurredAt)
	})

	if len(contributions) > count {
		return contributions[:count]
	}
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

func recentContributionRepositoryLimit(count int) int {
	if count <= 0 {
		return 0
	}

	limit := count + 5
	if limit > gitHubMaxContributionRepositories {
		return gitHubMaxContributionRepositories
	}
	return limit
}
