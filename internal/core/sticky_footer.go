// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

const (
	// ansiSaveCursor identifies a fixed value used internally by this package.
	ansiSaveCursor = "\x1b[s"
	// ansiRestoreCursor identifies a fixed value used internally by this package.
	ansiRestoreCursor = "\x1b[u"
	// ansiClearLine identifies a fixed value used internally by this package.
	ansiClearLine = "\x1b[2K"
	// ansiResetScrollRegion identifies a fixed value used internally by this package.
	ansiResetScrollRegion = "\x1b[r"
	// ansiHideCursor identifies a fixed value used internally by this package.
	ansiHideCursor = "\x1b[?25l"
	// ansiShowCursor identifies a fixed value used internally by this package.
	ansiShowCursor = "\x1b[?25h"
)

// stickyFooter reserves the terminal's final row and keeps ordinary output in
// the scrolling region above it. Calls write complete escape sequences in one
// operation to minimize interference with application log writes.
// stickyFooter is an internal data type used by this package.
type stickyFooter struct {
	// mu stores the value represented by this member.
	mu sync.Mutex
	// writer stores the value represented by this member.
	writer io.Writer
	// size stores the value represented by this member.
	size func() (width, height int, err error)
	// width stores the value represented by this member.
	width int
	// height stores the value represented by this member.
	height int
	// active stores the value represented by this member.
	active bool
	// rows stores the value represented by this member.
	rows int
}

// newStickyFooter performs the internal operation described by its signature.
func newStickyFooter(output *os.File) (*stickyFooter, bool) {
	return newStickyFooterRows(output, 1)
}

// newStickyFooterRows performs the internal operation described by its signature.
func newStickyFooterRows(output *os.File, rows int) (*stickyFooter, bool) {
	if output == nil || !term.IsTerminal(int(output.Fd())) {
		return nil, false
	}
	if rows < 1 {
		rows = 1
	}
	footer := &stickyFooter{
		writer: output,
		rows:   rows,
		size: func() (int, int, error) {
			return term.GetSize(int(output.Fd()))
		},
	}
	width, height, err := footer.size()
	if err != nil || width < 2 || height <= rows {
		return nil, false
	}
	footer.width, footer.height, footer.active = width, height, true
	footer.reserve(width, height)
	return footer, true
}

// Update is an exported operation provided by this package.
func (f *stickyFooter) Update(content string) {
	f.UpdateRows(content)
}

// UpdateRows is an exported operation provided by this package.
func (f *stickyFooter) UpdateRows(contents ...string) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.active {
		return
	}

	width, height, err := f.size()
	if err == nil && width >= 2 && height >= 2 && (width != f.width || height != f.height) {
		f.reconfigure(width, height)
	}

	rows := f.rowCount()
	start := f.height - rows + 1
	sequence := ansiSaveCursor
	for row := 0; row < rows; row++ {
		content := ""
		if row < len(contents) {
			content = contents[row]
		}
		sequence += fmt.Sprintf("\x1b[%d;1H", start+row) + ansiClearLine + content
	}
	sequence += ansiRestoreCursor
	_, _ = io.WriteString(f.writer, sequence)
}

// Stop is an exported operation provided by this package.
// Additional context: Clear the reserved row, restore normal full-terminal scrolling, and leave the cursor on the last row so subsequent application output is natural.
func (f *stickyFooter) Stop() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.active {
		return
	}

	sequence := ""
	for row := f.height - f.rowCount() + 1; row <= f.height; row++ {
		sequence += fmt.Sprintf("\x1b[%d;1H", row) + ansiClearLine
	}
	sequence += ansiResetScrollRegion +
		fmt.Sprintf("\x1b[%d;1H", f.height) +
		ansiShowCursor
	_, _ = io.WriteString(f.writer, sequence)
	f.active = false
}

// reserve performs the internal operation described by its signature.
func (f *stickyFooter) reserve(_ int, height int) {
	rows := f.rowCount()
	sequence := ansiHideCursor +
		fmt.Sprintf("\x1b[1;%dr", height-rows) +
		fmt.Sprintf("\x1b[%d;1H", height-rows)
	_, _ = io.WriteString(f.writer, sequence)
}

// reconfigure performs the internal operation described by its signature.
// Additional context: Remove the old footer before establishing the new scrolling region.
func (f *stickyFooter) reconfigure(width, height int) {

	sequence := ""
	for row := f.height - f.rowCount() + 1; row <= f.height; row++ {
		sequence += fmt.Sprintf("\x1b[%d;1H", row) + ansiClearLine
	}
	sequence += ansiResetScrollRegion
	_, _ = io.WriteString(f.writer, sequence)
	f.width, f.height = width, height
	f.reserve(width, height)
}

// rowCount performs the internal operation described by its signature.
func (f *stickyFooter) rowCount() int {
	if f.rows < 1 {
		return 1
	}
	return f.rows
}
