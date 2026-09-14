package main

import (
	"context"
	"log/slog"

	graphql "github.com/hasura/go-graphql-client"
)

var viewerQuery struct {
	Viewer struct {
		Login graphql.String
	}
}

var recentFollowersQuery struct {
	User struct {
		Login     graphql.String
		Followers struct {
			TotalCount graphql.Int
			Edges      []struct {
				Cursor graphql.String
				Node   qlUser
			}
		} `graphql:"followers(first: $count)"`
	} `graphql:"user(login:$username)"`
}

func getUsername() (string, error) {
	err := queryGitHub(context.Background(), "getUsername", &viewerQuery, nil)
	if err != nil {
		return "", err
	}

	return string(viewerQuery.Viewer.Login), nil
}

func recentFollowers(count int) []User {
	defer logOperation("recentFollowers", "count", count)()

	var users []User
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count),
	}
	err := queryGitHub(context.Background(), "recentFollowers", &recentFollowersQuery, variables)
	if err != nil {
		panic(err)
	}
	for _, v := range recentFollowersQuery.User.Followers.Edges {
		users = append(users, userFromQL(v.Node))
	}
	slog.Info("Results selected", "kind", "followers", "items", len(users))
	return users
}

/*
{
  user(login: "muesli") {
    login
    followers(first: 10) {
      totalCount
      edges {
        cursor
        node {
          id
          avatarUrl
          login
		  name
		  url
        }
      }
    }
  }
}
*/
