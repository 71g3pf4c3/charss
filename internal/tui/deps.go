package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/71g3pf4c3/charss/internal/feed"
	"github.com/71g3pf4c3/charss/internal/store"
	"github.com/71g3pf4c3/charss/internal/urls"
)

// Browser displays article HTML in an external browser process (chawan)
// that owns the terminal while it runs. It is implemented by
// *render.Chawan; tests inject fakes. The TUI never renders HTML itself.
type Browser interface {
	ShowHTML(ctx context.Context, html string) error
}

// Fetcher fetches feeds over HTTP. It is implemented by *feed.Fetcher;
// tests inject fakes. The subset the TUI needs is conditional requests.
type Fetcher interface {
	FetchIfModified(ctx context.Context, f urls.Feed, etag, lastModified string) (feed.Fetched, error)
}

// Storer persists per-feed state. It is implemented by *store.Store;
// tests inject fakes.
type Storer interface {
	Load(feedURL string) (store.State, bool, error)
	Save(feedURL string, st store.State) error
}

// Terminal hands the terminal over to an external process and takes it
// back afterwards. It exists so the chawan handoff can be unit-tested
// without a real *tea.Program.
type Terminal interface {
	// Release suspends the TUI's input reader and renderer so another
	// process can draw on the terminal.
	Release() error
	// Restore reinitializes input and rendering and repaints the screen.
	Restore() error
}

// ProgramTerminal adapts *tea.Program to Terminal.
//
// The program is attached after the model is constructed (tea.NewProgram
// copies the model), so the adapter is a shared pointer: every copy of the
// Model sees the attached program.
type ProgramTerminal struct {
	prog *tea.Program
}

// Attach wires the running program. Must be called before Program.Run.
func (t *ProgramTerminal) Attach(p *tea.Program) { t.prog = p }

// Release implements Terminal.
func (t *ProgramTerminal) Release() error {
	if t.prog == nil {
		return errTerminalNotAttached
	}
	return t.prog.ReleaseTerminal()
}

// Restore implements Terminal.
func (t *ProgramTerminal) Restore() error {
	if t.prog == nil {
		return errTerminalNotAttached
	}
	return t.prog.RestoreTerminal()
}
