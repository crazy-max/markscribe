package main

import (
	"context"
	"log/slog"
	"time"

	graphql "github.com/hasura/go-graphql-client"
)

var sponsorsQuery struct {
	User struct {
		Login                    graphql.String
		SponsorshipsAsMaintainer struct {
			TotalCount graphql.Int
			Edges      []struct {
				Cursor graphql.String
				Node   struct {
					CreatedAt     time.Time
					SponsorEntity struct {
						Typename     graphql.String `graphql:"__typename"`
						User         qlUser         `graphql:"... on User"`
						Organization qlUser         `graphql:"... on Organization"`
					}
				}
			}
		} `graphql:"sponsorshipsAsMaintainer(first: $count, orderBy: {field: CREATED_AT, direction: DESC})"`
	} `graphql:"user(login:$username)"`
}

func sponsors(count int) []Sponsor {
	defer logOperation("sponsors", "count", count)()

	var sponsors []Sponsor
	variables := map[string]interface{}{
		"username": graphql.String(username),
		"count":    graphql.Int(count),
	}
	err := queryGitHub(context.Background(), "sponsors", &sponsorsQuery, variables)
	if err != nil {
		panic(err)
	}

	for _, v := range sponsorsQuery.User.SponsorshipsAsMaintainer.Edges {
		switch v.Node.SponsorEntity.Typename {
		case "User":
			sponsors = append(sponsors, Sponsor{
				User:      userFromQL(v.Node.SponsorEntity.User),
				CreatedAt: v.Node.CreatedAt,
			})
		case "Organization":
			sponsors = append(sponsors, Sponsor{
				User:      userFromQL(v.Node.SponsorEntity.Organization),
				CreatedAt: v.Node.CreatedAt,
			})
		}
	}
	slog.Info("Results selected", "kind", "sponsors", "items", len(sponsors))
	return sponsors
}

/*
{
  user(login: "muesli") {
    login
    sponsorshipsAsMaintainer(first: 100) {
      totalCount
      edges {
        cursor
        node {
          createdAt
          sponsorEntity {
            __typename
            ... on User {
              login
              name
              avatarUrl
              url
            }
            ... on Organization {
              login
              name
              avatarUrl
              url
            }
          }
        }
      }
    }
  }
}
*/
