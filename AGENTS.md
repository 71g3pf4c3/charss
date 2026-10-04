# AGENTS.md

Guidance for AI coding agents (OpenCode, Claude Code, etc.) working in this repo.

## Project intent

**charss** — a terminal RSS reader, a newsboat alternative, with HTML rendering and image support in the terminal.

Hard product constraints (do not substitute or drop these):

- **Stack: Go + charmbracelet (charm.land).** TUI is Bubble Tea / Lip Gloss / related charm libs. Do not propose Ratatui, tview, or "wrap newsboat in C".
- **HTML rendering: delegated to chawan (https://chawan.net).** Chawan is the browser used to display article HTML. Do not implement a built-in HTML rendering engine; integration means driving chawan as an external browser process.
- **Images: chafa.** Image-to-terminal conversion goes through chafa.
- **Full sixel support is a requirement**, not a nice-to-have. Do not treat kitty graphics protocol or half-block/unicode fallbacks as sufficient — those may exist as fallbacks, but sixel must work.

## Reference behavior

Newsboat is the UX/feature reference ("1:1 alternative"). When behavior is ambiguous, check how newsboat does it (keybindings, feed list / article list / pager semantics, config and urls-file concepts) before inventing new UX.

## Repo state and conventions

- Stack (fixed, per owner): **cobra** for the CLI, **viper** for config, **charmbracelet** (Bubble Tea / Lip Gloss / bubbles) for the TUI, **GoReleaser** for releases.
- `go.work` is gitignored — this is a single-module project, not a Go workspace.
- `.env` is gitignored — never commit env files or hardcode credentials/tokens.
- `dist/` is gitignored — GoReleaser output, always disposable.

## Commands

```sh
go build ./...        # build everything
go test ./...         # unit tests (internal/urls has table tests)
go vet ./...
gofmt -l .            # CI fails on unformatted files — run gofmt -w before committing
go run . version      # prints injected build metadata (dev/none/unknown locally)

# GoReleaser (no local binary needed; config lives in .goreleaser.yaml)
go run github.com/goreleaser/goreleaser/v2@latest check
go run github.com/goreleaser/goreleaser/v2@latest build --snapshot --clean --single-target
```

CI (GitHub Actions) runs: gofmt check → vet → `go test -race` → build. Releases are GoReleaser drafts triggered by `v*` tags (`.github/workflows/release.yml`); artifacts are linux/darwin amd64/arm64 tarballs — windows is intentionally excluded.

## Layout

- `cmd/` — cobra commands. `root.go` (runs TUI by default, like bare `newsboat`), `tui.go` (urls-file loading + Program startup), `version.go`.
- `internal/version/` — build metadata vars, injected via ldflags in `.goreleaser.yaml`. Change the import path there if the module path changes.
- `internal/urls/` — newsboat-compatible urls-file parser (URL, quoted title, tags, per-feed `"key: value"` pairs). Has table tests — extend them when adding fields.
- `internal/config/` — viper loading. Precedence: CLI flags → `$XDG_CONFIG_HOME/charss/config.toml` → defaults. Keys so far: `browser` (chawan), `chafa`. A missing config file is not an error; a malformed one is.
- `internal/tui/` — Bubble Tea feed list (newsboat-style keys: j/k, q, r, Enter). Key bindings live in `KeyMap`, not inline in `Update`.

## Product invariants

- Articles are rendered **externally by chawan**; the TUI never renders article HTML itself. The TUI is a navigation/selection layer only.
- The urls file is intentionally **not** part of the viper config — it stays a separate newsboat-style plain-text file, same as newsboat's split of `config` + `urls`.
- A missing urls file yields an empty feed list (newsboat behavior), not a crash.

## Definition of done for rendering features

A rendering feature (HTML, images, sixel) is done only when it is verified in a real terminal that supports the target protocol (e.g. sixel-capable terminal for sixel output). "Compiles and draws something" is not done.
