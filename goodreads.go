package main

import (
	"github.com/KyleBanks/goodreads/responses"
	"log/slog"
)

func goodReadsReviews(count int) []responses.Review {
	defer logOperation("goodReadsReviews", "count", count)()
	reviews, err := goodReadsClient.ReviewList(goodReadsID, "read", "date_read", "", "d", 1, count)
	if err != nil {
		panic(err)
	}
	slog.Info("Results selected", "kind", "Goodreads reviews", "items", len(reviews))
	return reviews
}

func goodReadsCurrentlyReading(count int) []responses.Review {
	defer logOperation("goodReadsCurrentlyReading", "count", count)()
	reviews, err := goodReadsClient.ReviewList(goodReadsID, "currently-reading", "date_updated", "", "d", 1, count)
	if err != nil {
		panic(err)
	}
	slog.Info("Results selected", "kind", "Goodreads reviews", "items", len(reviews))
	return reviews
}
