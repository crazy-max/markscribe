package main

import (
	"context"
	"log/slog"
	"time"

	graphql "github.com/hasura/go-graphql-client"
)

var recentStarsQuery struct {
	User struct {
		Login graphql.String
		Stars struct {
			TotalCount graphql.Int
			Edges      []struct {
				Cursor    graphql.String
				StarredAt time.Time
				Node      qlRepository
			}
		} `graphql:"starredRepositories(first: $count, after:$after, orderBy: {field: STARRED_AT, direction: DESC})"`
	} `graphql:"user(login:$username)"`
}

func recentStars(count int) []Star {
	defer logOperation("recentStars", "count", count)()
	var starredRepos []Star
	var after *graphql.String

outer:
	for {
		variables := map[string]interface{}{
			"username": graphql.String(username),
			"count":    graphql.Int(count),
			"after":    after,
		}
		err := queryGitHub(context.Background(), "recentStars", &recentStarsQuery, variables)
		if err != nil {
			panic(err)
		}

		for _, v := range recentStarsQuery.User.Stars.Edges {
			if v.Node.IsPrivate {
				continue
			}
			starredRepos = append(starredRepos, Star{
				StarredAt: v.StarredAt,
				Repo:      repoFromQL(v.Node),
			})
			if len(starredRepos) >= count {
				break outer
			}
			after = graphql.NewString(v.Cursor)
		}
	}

	slog.Info("Results selected", "kind", "starred repositories", "items", len(starredRepos))

	return starredRepos
}

/*
{
	viewer {
		login
		starredRepositories(first: 3, orderBy: {field: STARRED_AT, direction: DESC}) {
			totalCount
			edges {
				cursor
				starredAt
				node {
					nameWithOwner
					url
					description
				}
			}
		}
	}
}
*/
