package main

import (
	"log/slog"
	"time"

	"github.com/mmcdole/gofeed"
)

// RSSEntry represents a single RSS entry.
type RSSEntry struct {
	Title       string
	URL         string
	PublishedAt time.Time
}

func rssFeed(url string, count int) []RSSEntry {
	defer logOperation("rssFeed", "count", count)()
	var r []RSSEntry

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(url)
	if err != nil {
		panic(err)
	}

	for _, v := range feed.Items {

		r = append(r, RSSEntry{
			Title:       v.Title,
			URL:         v.Link,
			PublishedAt: *v.PublishedParsed,
		})
		if len(r) == count {
			break
		}
	}

	slog.Info("Results selected", "kind", "RSS entries", "items", len(r))

	return r
}
