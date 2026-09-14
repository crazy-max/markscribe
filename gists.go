package main

import (
	"context"
	"log/slog"

	graphql "github.com/hasura/go-graphql-client"
)

var gistsQuery struct {
	User struct {
		Login graphql.String
		Gists struct {
			TotalCount graphql.Int
			Edges      []struct {
				Cursor graphql.String
				Node   qlGist
			}
		} `graphql:"gists(first: $count, orderBy: {field: CREATED_AT, direction: DESC})"`
	} `graphql:"user(login:$username)"`
}

func gists(count int) []Gist {
	defer logOperation("gists", "count", count)()

	var gists []Gist
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count),
	}
	err := queryGitHub(context.Background(), "gists", &gistsQuery, variables)
	if err != nil {
		panic(err)
	}
	for _, v := range gistsQuery.User.Gists.Edges {
		gists = append(gists, gistFromQL(v.Node))
	}
	slog.Info("Results selected", "kind", "gists", "items", len(gists))
	return gists
}

/*
{
  user(login: "muesli") {
    login
    gists(first: 100) {
      totalCount
      edges {
        cursor
        node {
		  name
		  description
		  url
		  createdAt
        }
      }
    }
  }
}
*/
