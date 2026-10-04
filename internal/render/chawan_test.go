//go:build unix

package render

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeBrowser writes an executable fake chawan script into dir and
// returns its path. The script body may contain the placeholder RECDIR,
// which is replaced with rec (an existing directory the fake records into).
func writeFakeBrowser(t *testing.T, dir, rec, body string) string {
	t.Helper()
	if err := os.MkdirAll(rec, 0o755); err != nil {
		t.Fatalf("creating record dir: %v", err)
	}
	script := strings.ReplaceAll(body, "RECDIR", rec)
	path := filepath.Join(dir, "chawan")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake browser: %v", err)
	}
	return path
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestLookPathMissing(t *testing.T) {
	c := New(Options{Binary: filepath.Join(t.TempDir(), "chawan")})
	_, err := c.LookPath()
	if err == nil {
		t.Fatal("expected an error for a missing binary")
	}
	if !strings.Contains(err.Error(), "browser") {
		t.Errorf("error should name the missing browser, got: %v", err)
	}
	if !strings.Contains(err.Error(), "chawan") {
		t.Errorf("error should mention chawan, got: %v", err)
	}
	if !strings.Contains(err.Error(), "https://chawan.net") {
		t.Errorf("error should include the install hint, got: %v", err)
	}
}

func TestLookPathFound(t *testing.T) {
	bin := writeFakeBrowser(t, t.TempDir(), filepath.Join(t.TempDir(), "rec"), "#!/bin/sh\nexit 0\n")
	c := New(Options{Binary: bin})
	path, err := c.LookPath()
	if err != nil {
		t.Fatalf("LookPath: %v", err)
	}
	if path != bin {
		t.Errorf("LookPath returned %q, want %q", path, bin)
	}
}

func TestShowHTMLPassesTempFile(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "rec")
	bin := writeFakeBrowser(t, dir, rec, `#!/bin/sh
printf '%s\n' "$#" > RECDIR/count
printf '%s\n' "$@" > RECDIR/args
cat "$1" > RECDIR/content
dirname "$1" > RECDIR/dir
exit 0
`)

	html := "<html><body><p>hello charss</p></body></html>"
	c := New(Options{Binary: bin})
	if err := c.ShowHTML(context.Background(), html); err != nil {
		t.Fatalf("ShowHTML: %v", err)
	}

	if got := readLines(t, filepath.Join(rec, "count")); len(got) != 1 || got[0] != "1" {
		t.Errorf("fake received %v args, want exactly 1", got)
	}
	args := readLines(t, filepath.Join(rec, "args"))
	if len(args) != 1 {
		t.Fatalf("fake received %d args, want 1: %q", len(args), args)
	}
	if filepath.Ext(args[0]) != ".html" {
		t.Errorf("fake received %q, want a .html path", args[0])
	}
	content, err := os.ReadFile(filepath.Join(rec, "content"))
	if err != nil {
		t.Fatalf("fake did not record content: %v", err)
	}
	if string(content) != html {
		t.Errorf("file content = %q, want %q", content, html)
	}
	// The temp dir must be gone after ShowHTML returns.
	tempDir := readLines(t, filepath.Join(rec, "dir"))[0]
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Errorf("temp dir %q still exists after ShowHTML returned (stat err: %v)", tempDir, err)
	}
}

func TestShowHTMLExitCode(t *testing.T) {
	bin := writeFakeBrowser(t, t.TempDir(), filepath.Join(t.TempDir(), "rec"), "#!/bin/sh\nexit 42\n")
	c := New(Options{Binary: bin})
	err := c.ShowHTML(context.Background(), "<p>x</p>")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want *ExitError, got %v", err)
	}
	if exitErr.Code != 42 {
		t.Errorf("ExitError.Code = %d, want 42", exitErr.Code)
	}
	if errors.Unwrap(exitErr) == nil {
		t.Errorf("ExitError should wrap the underlying error, got nil")
	}
}

func TestShowURLPassesURL(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "rec")
	bin := writeFakeBrowser(t, dir, rec, `#!/bin/sh
printf '%s\n' "$@" > RECDIR/args
exit 0
`)

	const url = "https://example.com/article.html"
	c := New(Options{Binary: bin, Args: []string{"-t", "sixel"}})
	if err := c.ShowURL(context.Background(), url); err != nil {
		t.Fatalf("ShowURL: %v", err)
	}

	args := readLines(t, filepath.Join(rec, "args"))
	want := []string{"-t", "sixel", url}
	if fmt.Sprint(args) != fmt.Sprint(want) {
		t.Errorf("fake received %q, want %q", args, want)
	}
}

func TestContextCancelKillsProcess(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "rec")
	bin := writeFakeBrowser(t, dir, rec, `#!/bin/sh
echo started > RECDIR/started
sleep 30
echo finished > RECDIR/finished
`)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Wait until the fake has actually started before cancelling,
		// so the test does not race process startup.
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(rec, "started")); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()

	c := New(Options{Binary: bin})
	start := time.Now()
	err := c.ShowURL(ctx, "https://example.com/article.html")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("ShowURL took %v to return after cancel", elapsed)
	}
	if _, err := os.Stat(filepath.Join(rec, "started")); err != nil {
		t.Fatalf("fake never started: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rec, "finished")); err == nil {
		t.Error("fake finished its sleep — the process group was not killed")
	}
}

func TestShowURLCanceledBeforeStart(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "rec")
	bin := writeFakeBrowser(t, dir, rec, `#!/bin/sh
echo ran > RECDIR/ran
`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := New(Options{Binary: bin})
	if err := c.ShowURL(ctx, "https://example.com/article.html"); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(rec, "ran")); err == nil {
		t.Error("fake browser ran despite a pre-cancelled context")
	}
}
