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
go test ./...         # unit tests (most internal packages have table tests)
go vet ./...
gofmt -l .            # CI fails on unformatted files — run gofmt -w before committing
go run . version      # prints injected build metadata (dev/none/unknown locally)

# GoReleaser (no local binary needed; config lives in .goreleaser.yaml)
go run github.com/goreleaser/goreleaser/v2@latest check
go run github.com/goreleaser/goreleaser/v2@latest build --snapshot --clean --single-target

# Nix
nix flake check                    # flake + HM module checks
nix build .#charss                 # build via nix (version 0.1.0 from flake)
nix develop                         # shell with go, gopls, delve, chawan, chafa
nix fmt                             # format .nix files (nixfmt)
```

CI (GitHub Actions) runs: gofmt check → vet → `go test -race` → build (`ci.yml`), `nix flake check` (`nix.yml`). Releases are GoReleaser drafts triggered by `v*` tags (`release.yml`); artifacts are linux/darwin amd64/arm64 tarballs — windows is intentionally excluded.

## Layout

- `cmd/` — cobra commands. `root.go` (runs TUI by default, like bare `newsboat`), `tui.go` (urls loading, refresh wiring, Program startup), `import.go` (OPML import/export), `preview.go` (standalone chafa/sixel image preview), `version.go`.
- `internal/version/` — build metadata vars, injected via ldflags in `.goreleaser.yaml` AND `nix/package.nix`. Change the import path there if the module path changes.
- `internal/urls/` — newsboat-compatible urls-file parser + `Format` (roundtrips with `Parse`). Has table tests — extend them when adding fields.
- `internal/config/` — viper loading. Precedence: CLI flags → `$XDG_CONFIG_HOME/charss/config.toml` → defaults. Keys so far: `browser` (chawan), `chafa`. A missing config file is not an error; a malformed one is.
- `internal/feed/` — HTTP fetching via gofeed, conditional requests (ETag/304 → `ErrNotModified`), typed `HTTPError`/`ParseError`.
- `internal/store/` — JSON per-feed cache in `$XDG_CACHE_HOME/charss/feeds/`, atomic writes, read-state per article.
- `internal/render/` — chawan process driver: temp HTML files, process group, ctx kill, `ExitError`. Unit tests use fake browser scripts.
- `internal/image/` — chafa conversion (sixel default, kitty/symbols fallback only), TERM-based `Detect` with override, HTTP fetcher.
- `internal/tui/` — Bubble Tea screens: feed list → article list → open in chawan (terminal handoff via Release/Restore). Key bindings live in `keys.go` (`KeyMap`), not inline in `Update`. Refresh pipeline in `refresh.go`, pure merge logic is table-tested (`merge_test.go`).
- `nix/` — `package.nix` (buildGoModule, shared by flake and HM module default), `hm-module.nix` (Home Manager module: `programs.charss.{enable,package,settings,urls}`).
- `flake.nix` — package, devShell (with real chawan + chafa), homeManagerModules, checks, formatter.

## Nix notes

- `flake.lock` is committed — always commit it together with `flake.nix` changes.
- Bumping the Go version in `go.mod` requires updating `vendorHash` in `nix/package.nix` (run `nix build .#charss`, take the "got:" hash).
- The HM module uses only standard option types — no home-manager lib imports in the module body itself, so it stays flake-checkable.

## Product invariants

- Articles are rendered **externally by chawan**; the TUI never renders article HTML itself. The TUI is a navigation/selection layer only.
- The urls file is intentionally **not** part of the viper config — it stays a separate newsboat-style plain-text file, same as newsboat's split of `config` + `urls`.
- A missing urls file yields an empty feed list (newsboat behavior), not a crash.

## Definition of done for rendering features

A rendering feature (HTML, images, sixel) is done only when it is verified in a real terminal that supports the target protocol (e.g. sixel-capable terminal for sixel output). "Compiles and draws something" is not done.
