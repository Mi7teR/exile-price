// Package logfile is the tool's log: the current file plus one older ".1"
// file, each at most half the limit, so both together stay under it.
package logfile

import (
	"os"
	"sync"
)

// File is an io.Writer for slog. It is safe for concurrent use.
type File struct {
	mu     sync.Mutex
	path   string
	max    int64 // per file
	f      *os.File
	size   int64
	onOpen func(*os.File) // told about each new file, and nil before one closes
}

// Open appends to path. onOpen may be nil; it is meant for
// debug.SetCrashOutput, which keeps its own handle and so has to let go of
// the file before it is renamed on Windows.
func Open(path string, limit int64, onOpen func(*os.File)) (*File, error) {
	l := &File{path: path, max: limit / 2, onOpen: onOpen}
	return l, l.open(os.O_APPEND)
}

func (l *File) open(flag int) error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|flag, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.f, l.size = f, st.Size()
	if l.onOpen != nil {
		l.onOpen(f)
	}
	return nil
}

// Write starts a new file first when p would take this one over its half.
func (l *File) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil || l.size > 0 && l.size+int64(len(p)) > l.max {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

// rotate moves the file to ".1", dropping the older one. If the move fails
// the file is emptied instead, so the limit holds either way.
func (l *File) rotate() error {
	if l.f != nil {
		if l.onOpen != nil {
			l.onOpen(nil)
		}
		l.f.Close()
		l.f = nil
	}
	_ = os.Remove(l.path + ".1")
	flag := os.O_APPEND
	if err := os.Rename(l.path, l.path+".1"); err != nil && !os.IsNotExist(err) {
		flag = os.O_TRUNC
	}
	return l.open(flag)
}
