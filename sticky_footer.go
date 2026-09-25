package induction

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

const (
	ansiSaveCursor        = "\x1b[s"
	ansiRestoreCursor     = "\x1b[u"
	ansiClearLine         = "\x1b[2K"
	ansiResetScrollRegion = "\x1b[r"
	ansiHideCursor        = "\x1b[?25l"
	ansiShowCursor        = "\x1b[?25h"
)

// stickyFooter reserves the terminal's final row and keeps ordinary output in
// the scrolling region above it. Calls write complete escape sequences in one
// operation to minimize interference with application log writes.
type stickyFooter struct {
	mu     sync.Mutex
	writer io.Writer
	size   func() (width, height int, err error)
	width  int
	height int
	active bool
	rows   int
}

func newStickyFooter(output *os.File) (*stickyFooter, bool) {
	return newStickyFooterRows(output, 1)
}

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

func (f *stickyFooter) Update(content string) {
	f.UpdateRows(content)
}

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

func (f *stickyFooter) Stop() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.active {
		return
	}

	// Clear the reserved row, restore normal full-terminal scrolling, and leave
	// the cursor on the last row so subsequent application output is natural.
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

func (f *stickyFooter) reserve(width, height int) {
	rows := f.rowCount()
	sequence := ansiHideCursor +
		fmt.Sprintf("\x1b[1;%dr", height-rows) +
		fmt.Sprintf("\x1b[%d;1H", height-rows)
	_, _ = io.WriteString(f.writer, sequence)
}

func (f *stickyFooter) reconfigure(width, height int) {
	// Remove the old footer before establishing the new scrolling region.
	sequence := ""
	for row := f.height - f.rowCount() + 1; row <= f.height; row++ {
		sequence += fmt.Sprintf("\x1b[%d;1H", row) + ansiClearLine
	}
	sequence += ansiResetScrollRegion
	_, _ = io.WriteString(f.writer, sequence)
	f.width, f.height = width, height
	f.reserve(width, height)
}

func (f *stickyFooter) rowCount() int {
	if f.rows < 1 {
		return 1
	}
	return f.rows
}
