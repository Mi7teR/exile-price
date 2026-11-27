package main

import (
	"errors"
	"io/fs"
	"sync"
)

// buildIssue is a build problem for the overlay: its note is shown once,
// each error is logged once, and "build file not found" is not repeated
// while the file stays missing. Safe for concurrent use.
type buildIssue struct {
	mu        sync.Mutex
	note      string // a problem not shown yet; "" = none
	gone      bool   // the note says the build file is missing
	goneShown bool   // "build file not found" was shown and the file is still missing
	logged    string // the error last logged
}

// set makes text the note to show next; gone says it is about a missing file.
func (b *buildIssue) set(text string, gone bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.note, b.gone = text, gone
}

// ok records that the build loads: nothing to show, nothing logged.
func (b *buildIssue) ok() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.note, b.gone, b.goneShown, b.logged = "", false, false, ""
}

// failed records an error reading the build. A missing file becomes the
// note unless that was shown already. It reports whether err is new, i.e.
// should be logged.
func (b *buildIssue) failed(err error) (newErr bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if errors.Is(err, fs.ErrNotExist) && !b.goneShown {
		b.note, b.gone = buildErrText(err), true
	}
	msg := err.Error()
	newErr = msg != b.logged
	b.logged = msg
	return newErr
}

// take returns the note to show and clears it; "" when there is none.
func (b *buildIssue) take() (text string, gone bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	text, gone = b.note, b.gone
	b.note, b.gone = "", false
	return text, gone
}

// shown records that a note reached the screen.
func (b *buildIssue) shown(gone bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.goneShown = gone
}

// putBack keeps a taken note that did not reach the screen, unless a newer
// one came meanwhile.
func (b *buildIssue) putBack(text string, gone bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.note == "" {
		b.note, b.gone = text, gone
	}
}
