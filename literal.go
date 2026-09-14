package main

import (
	"log/slog"

	"github.com/crazy-max/markscribe/literal"
)

func literalClubCurrentlyReading(count int) []literal.Book {
	defer logOperation("literalClubCurrentlyReading", "count", count)()
	books, err := literal.CurrentlyReading()
	if err != nil {
		panic(err)
	}
	slog.Info("Literal books received", "items", len(books), "requested", count)
	if len(books) > count {
		return books[:count]
	}
	return books
}
