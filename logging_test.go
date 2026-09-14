package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	graphql "github.com/hasura/go-graphql-client"
	"golang.org/x/oauth2"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &output
}

func TestGitHubLoggingPreservesRetriesAndRedactsSecrets(t *testing.T) {
	output := captureLogs(t)
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Error("authorization was not preserved")
		}
		if body, err := io.ReadAll(r.Body); err != nil || !strings.Contains(string(body), "viewer") {
			t.Errorf("request body was not preserved: %s, %v", body, err)
		}
		w.Header().Set("X-GitHub-Request-Id", "request-from-github")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		if attempts == 1 {
			http.Error(w, "secret-error-body", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"viewer":{"login":"secret-response-body"}}}`)
	}))
	defer server.Close()
	client := &http.Client{Transport: &oauth2.Transport{
		Source: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "secret-token"}),
		Base:   server.Client().Transport,
	}}
	previous := gitHubClient
	gitHubClient = newGitHubGraphQLClient(server.URL, client, 0)
	t.Cleanup(func() { gitHubClient = previous })
	var query struct {
		Viewer struct{ Login graphql.String }
	}
	err := queryGitHub(context.Background(), "test-query", &query, map[string]interface{}{
		"owner": graphql.String("example"), "password": graphql.String("secret-password"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if query.Viewer.Login != "secret-response-body" || attempts != 2 {
		t.Fatalf("response/retries changed: login=%s attempts=%d", query.Viewer.Login, attempts)
	}
	logs := output.String()
	for _, want := range []string{"GitHub query started", "test-query", "example", "request=1", "request=2", "status=502", "status=200", "request-from-github", "rate_remaining=4999", "GitHub query completed", "duration="} {
		if !strings.Contains(logs, want) {
			t.Errorf("missing %q in logs: %s", want, logs)
		}
	}
	for _, secret := range []string{"secret-token", "secret-password", "secret-error-body", "secret-response-body", "Authorization"} {
		if strings.Contains(logs, secret) {
			t.Errorf("logs leaked %q", secret)
		}
	}
}

func TestOperationLoggingPreservesPanic(t *testing.T) {
	output := captureLogs(t)
	failure := fmt.Errorf("secret-failure-detail")
	func() {
		defer func() {
			if got := recover(); got != failure {
				t.Errorf("panic changed: %v", got)
			}
		}()
		defer logOperation("failing-operation", "count", 5)()
		panic(failure)
	}()
	logs := output.String()
	if !strings.Contains(logs, "Operation failed") || strings.Contains(logs, "Operation completed") || strings.Contains(logs, failure.Error()) {
		t.Fatalf("unexpected failure logs: %s", logs)
	}
}

func TestOperationLoggingCompletes(t *testing.T) {
	output := captureLogs(t)
	func() { defer logOperation("successful-operation")() }()
	if logs := output.String(); !strings.Contains(logs, "Operation started") || !strings.Contains(logs, "Operation completed") {
		t.Fatalf("unexpected success logs: %s", logs)
	}
}

func TestCLILoggingKeepsMarkdownOnStdout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "template.tpl")
	if err := os.WriteFile(path, []byte("# Generated Markdown\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLILoggingHelper$")
	cmd.Env = append(os.Environ(), "MARKSCRIBE_LOGGING_TEST_TEMPLATE="+path, "GITHUB_TOKEN=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %s", err, stderr.String())
	}
	if stdout.String() != "# Generated Markdown\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Template rendered") || strings.Contains(stderr.String(), "# Generated Markdown") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCLILoggingHelper(t *testing.T) {
	path := os.Getenv("MARKSCRIBE_LOGGING_TEST_TEMPLATE")
	if path == "" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("markscribe", flag.ExitOnError)
	write = flag.String("write", "", "write output to")
	os.Args = []string{"markscribe", path}
	main()
	os.Exit(0)
}
