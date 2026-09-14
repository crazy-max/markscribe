package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log/slog"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/KyleBanks/goodreads"
	graphql "github.com/hasura/go-graphql-client"
)

var (
	gitHubClient    *graphql.Client
	goodReadsClient *goodreads.Client
	goodReadsID     string
	username        string

	write = flag.String("write", "", "write output to")
)

func main() {
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Println("Usage: markscribe [template]")
		os.Exit(1)
	}

	start := time.Now()
	slog.Info("Reading template", "path", flag.Args()[0])
	tplIn, err := ioutil.ReadFile(flag.Args()[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Can't read file:", err)
		os.Exit(1)
	}

	slog.Info("Parsing template", "bytes", len(tplIn))
	tpl, err := template.New("tpl").Funcs(template.FuncMap{
		/* GitHub */
		"recentContributions": recentContributions,
		"recentPullRequests":  recentPullRequests,
		"recentRepos":         recentRepos,
		"recentForks":         recentForks,
		"recentReleases":      recentReleases,
		"followers":           recentFollowers,
		"recentStars":         recentStars,
		"gists":               gists,
		"sponsors":            sponsors,
		"repo":                repo,
		/* RSS */
		"rss": rssFeed,
		/* GoodReads */
		"goodReadsReviews":          goodReadsReviews,
		"goodReadsCurrentlyReading": goodReadsCurrentlyReading,
		/* Literal.club */
		"literalClubCurrentlyReading": literalClubCurrentlyReading,
		/* Utils */
		"humanize": humanized,
		"reverse":  reverse,
		"now":      time.Now,
		"contains": strings.Contains,
		"toLower":  strings.ToLower,
	}).Parse(string(tplIn))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Can't parse template:", err)
		os.Exit(1)
	}

	gitHubToken := os.Getenv("GITHUB_TOKEN")
	goodReadsToken := os.Getenv("GOODREADS_TOKEN")
	goodReadsID = os.Getenv("GOODREADS_USER_ID")
	slog.Info("Initializing API clients", "github_authenticated", gitHubToken != "", "goodreads_authenticated", goodReadsToken != "")

	gitHubClient = newGitHubClient(gitHubToken)
	goodReadsClient = goodreads.NewClient(goodReadsToken)

	if len(gitHubToken) > 0 {
		username, err = getUsername()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Can't retrieve GitHub profile:", err)
			os.Exit(1)
		}
		slog.Info("GitHub profile resolved", "username", username)
	}

	w := os.Stdout
	if len(*write) > 0 {
		slog.Info("Opening output file", "path", *write)
		f, err := os.Create(*write)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Can't create:", err)
			os.Exit(1)
		}
		defer f.Close() //nolint: errcheck
		w = f
	}

	slog.Info("Rendering template", "output", w.Name())
	err = tpl.Execute(w, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Can't render template:", err)
		os.Exit(1)
	}
	slog.Info("Template rendered", "output", w.Name(), "duration", time.Since(start))
}
