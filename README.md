# charss

A terminal RSS reader — a newsboat alternative built on Go and the charmbracelet stack. Articles are rendered as HTML by the [chawan](https://chawan.net) browser; images are converted for the terminal by [chafa](https://hpjansson.org/chafa/). Sixel is the primary graphics path, with kitty graphics and symbol-based output as fallbacks.

The TUI itself never renders article HTML — it is a navigation and selection layer that spawns chawan for reading, the same way newsboat spawns an external browser.

## Status

Early development. Do not expect a usable reader yet.

What exists and is tested:

- Newsboat-compatible urls-file parser: feed URL, quoted title, tags, per-feed `"key: value"` pairs.
- Feed engine: gofeed-based RSS/Atom fetching with conditional requests (`ETag` / `Last-Modified`), typed HTTP/parse errors, and a JSON per-feed cache with atomic writes under `$XDG_CACHE_HOME/charss/feeds/` (articles, read flags, validators).
- Chawan driver: spawns chawan on a temp file or URL, inherits the terminal, tears down the whole process group on cancel.
- Image pipeline: HTTP image fetch plus chafa conversion with sixel as the default format; format detection from `$TERM`; kitty and symbols are fallbacks only.
- OPML import/export (`import` merges by default, `--replace` rewrites; `export` refuses to clobber the urls file itself).
- Feed-list TUI: j/k navigation, q to quit. Opening articles and reloading feeds are stubs.

Not yet done: the pieces above are not wired together. The TUI does not fetch feeds, show article lists, or launch chawan/chafa yet.

## Install

```sh
go install github.com/71g3pf4c3/charss@latest
```

Or grab a tarball from GitHub Releases: linux and darwin, amd64 and arm64. Windows is out of scope by design.

## Runtime dependencies

Both must be on `$PATH`:

- [chawan](https://chawan.net) — renders article HTML.
- [chafa](https://hpjansson.org/chafa/) — converts images to terminal output.

For sixel output you need a sixel-capable terminal: foot, xterm (`xterm-sixel`), wezterm, contour, rio, st, or mintty. Elsewhere charss falls back to symbol-based rendering; kitty graphics must be requested explicitly.

## Configuration

Config file: `$XDG_CONFIG_HOME/charss/config.toml` (defaults apply if missing; malformed TOML is an error):

```toml
browser = "chawan"  # binary used to render articles
chafa = "chafa"     # binary used to convert images
```

Feed list: `$XDG_CONFIG_HOME/charss/urls` — a separate plain-text file in the newsboat format, one feed per line:

```
https://example.com/feed.xml "Example" tech news
```

First field is the URL, then an optional quoted title, then tags. `#` starts a comment line. A missing urls file yields an empty feed list, not an error.

## Usage

```sh
charss                    # start the TUI
charss import <file.opml> # merge OPML subscriptions into the urls file
charss import --replace <file.opml>  # rewrite the urls file from the OPML
charss export <file.opml> # export the urls file as OPML
charss version            # print build metadata
```

Global flags, accepted by every subcommand: `--config <file>` and `--urls <file>` to override the default paths.

TUI keys: `j`/`k` or arrows to move, `Enter` to open, `r` to reload, `Ctrl+R` to redraw, `q` to quit.

## Roadmap

- Wire the feed engine and article store into the TUI: reload, article list, read state.
- Article viewing via chawan; image display via chafa with sixel first.
- Newsboat-parity UX: tag filtering, query selection, keybinding configuration.
- Terminal capability probing (DA1) instead of `$TERM` heuristics for sixel detection.

## Development

```sh
go build ./...
go test ./...
go vet ./...
gofmt -l .
```

CI runs gofmt, vet, `go test -race`, and a build. Releases are GoReleaser drafts triggered by `v*` tags.
