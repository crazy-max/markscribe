package main

import (
	"context"

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
	err := gitHubClient.Query(context.Background(), &viewerQuery, nil)
	if err != nil {
		return "", err
	}

	return string(viewerQuery.Viewer.Login), nil
}

func recentFollowers(count int) []User {
	// fmt.Printf("Finding recent followers...\n")

	var users []User
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count),
	}
	err := gitHubClient.Query(context.Background(), &recentFollowersQuery, variables)
	if err != nil {
		panic(err)
	}

	// fmt.Printf("%+v\n", query)
	for _, v := range recentFollowersQuery.User.Followers.Edges {
		users = append(users, userFromQL(v.Node))
	}

	// fmt.Printf("Found %d recent followers!\n", len(users))
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
