package image

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const stdoutMarker = "SIXEL-OUT-MARKER"

// writeFakeChafa creates a fake chafa executable in dir that records its
// argv and stdin next to the script, and prints stdoutMarker to stdout.
func writeFakeChafa(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "chafa")
	script := `#!/bin/sh
d=$(dirname "$0")
printf '%s\n' "$@" > "$d/args"
cat > "$d/stdin"
printf '` + stdoutMarker + `'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake chafa: %v", err)
	}
	return path
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestConvert_ArgsStdinStdout(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeChafa(t, dir)
	data := []byte("fake-image-bytes-\x00\x01\x02")

	r := New(Options{Format: FormatSixel, Columns: 80, MaxColors: 256, Binary: fake})
	out, err := r.Convert(context.Background(), data)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if string(out) != stdoutMarker {
		t.Errorf("stdout = %q, want %q", out, stdoutMarker)
	}

	args := readLines(t, filepath.Join(dir, "args"))
	want := []string{"--format", "sixel", "--size", "80", "--colors", "256", "-"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %v, want %v (mismatch at %d)", args, want, i)
		}
	}

	got, err := os.ReadFile(filepath.Join(dir, "stdin"))
	if err != nil {
		t.Fatalf("reading stdin capture: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("stdin = %q, want %q", got, data)
	}
}

func TestConvert_DefaultFormatIsSixel(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeChafa(t, dir)

	r := New(Options{Binary: fake}) // zero-value Format
	if _, err := r.Convert(context.Background(), []byte("img")); err != nil {
		t.Fatalf("Convert: %v", err)
	}

	args := readLines(t, filepath.Join(dir, "args"))
	var format string
	for i, a := range args {
		if a == "--format" && i+1 < len(args) {
			format = args[i+1]
		}
	}
	if format != "sixel" {
		t.Errorf("default --format = %q, want sixel", format)
	}
}

func TestConvert_OmitsOptionalFlagsWhenZero(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeChafa(t, dir)

	r := New(Options{Binary: fake}) // Columns=0, MaxColors=0
	if _, err := r.Convert(context.Background(), []byte("img")); err != nil {
		t.Fatalf("Convert: %v", err)
	}

	args := readLines(t, filepath.Join(dir, "args"))
	for _, a := range args {
		if a == "--size" || a == "--colors" {
			t.Errorf("args = %v, did not expect %s with zero Options", args, a)
		}
	}
	if len(args) != 3 || args[2] != "-" {
		t.Errorf("args = %v, want [--format sixel -]", args)
	}
}

func TestConvert_MissingBinaryPath(t *testing.T) {
	r := New(Options{Binary: "/nonexistent/chafa"})
	_, err := r.Convert(context.Background(), []byte("img"))
	if !errors.Is(err, ErrNoChafa) {
		t.Errorf("err = %v, want ErrNoChafa", err)
	}
}

func TestConvert_MissingInPATH(t *testing.T) {
	empty := t.TempDir() // no chafa inside
	t.Setenv("PATH", empty)

	r := New(Options{}) // Binary="" → LookPath
	_, err := r.Convert(context.Background(), []byte("img"))
	if !errors.Is(err, ErrNoChafa) {
		t.Errorf("err = %v, want ErrNoChafa", err)
	}
}

func TestConvert_NonZeroExitStderr(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chafa")
	script := "#!/bin/sh\necho boom >&2\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake chafa: %v", err)
	}

	r := New(Options{Binary: path})
	_, err := r.Convert(context.Background(), []byte("img"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, want := range []string{"boom", "1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to contain %q", err, want)
		}
	}
}

func TestFormatString(t *testing.T) {
	for _, tc := range []struct {
		f    Format
		want string
	}{
		{FormatSixel, "sixel"},
		{FormatKitty, "kitty"},
		{FormatSymbols, "symbols"},
		{Format(99), "symbols"},
	} {
		if got := tc.f.String(); got != tc.want {
			t.Errorf("Format(%d).String() = %q, want %q", tc.f, got, tc.want)
		}
	}
}
