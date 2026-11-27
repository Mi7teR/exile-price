package build

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Mi7teR/exile-calc/engine"
	"github.com/Mi7teR/exile-price/internal/config"
)

// stamp identifies a state of the PoB file; PoB rewrites it on every save.
type stamp struct {
	mod  time.Time
	size int64
}

// Store holds the chosen build and keeps it fresh: a PoB file is read again
// when it changes, a pasted code lives in appDir/build.txt. Safe for
// concurrent use.
type Store struct {
	appDir string
	calc   *engine.Calculator

	mu     sync.Mutex
	source string
	path   string // the PoB file
	cur    *Build
	stamp  stamp // of the file cur was loaded from
	bad    stamp // of the last file that failed to load
	badErr error
}

func NewStore(appDir string, calc *engine.Calculator) *Store {
	return &Store{appDir: appDir, calc: calc}
}

func (s *Store) buildTxt() string { return filepath.Join(s.appDir, "build.txt") }

// Use loads the build of the given source now. An empty source means no
// build: (nil, nil). A PoB file that fails to load is remembered anyway, so
// Current retries once the file changes.
func (s *Store) Use(source, pobBuild string) (*Build, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch source {
	case "":
		s.source, s.path, s.cur, s.stamp, s.bad, s.badErr = "", "", nil, stamp{}, stamp{}, nil
		return nil, nil
	case config.SourceClipboard:
		data, err := os.ReadFile(s.buildTxt())
		s.source, s.path, s.cur, s.stamp, s.bad, s.badErr = source, "", nil, stamp{}, stamp{}, nil
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		b, err := Load(s.calc, data)
		if err != nil {
			return nil, err
		}
		s.cur = b
		return b, nil
	case config.SourcePoB:
		s.source, s.path, s.cur, s.stamp, s.bad, s.badErr = source, pobBuild, nil, stamp{}, stamp{}, nil
		return s.reload()
	}
	return nil, fmt.Errorf("build: unknown source %q", source)
}

// Current returns the chosen build. For a PoB file it reads the file again
// if its modtime or size changed; if that fails the previous build comes
// back together with the error, and the same broken file is not read twice.
func (s *Store) Current() (*Build, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.source != config.SourcePoB {
		return s.cur, nil
	}
	return s.reload()
}

func (s *Store) reload() (*Build, error) {
	fi, err := os.Stat(s.path)
	if err != nil {
		return nil, err
	}
	st := stamp{fi.ModTime(), fi.Size()}
	if s.cur != nil && st == s.stamp {
		return s.cur, nil
	}
	if s.badErr != nil && st == s.bad {
		return s.cur, s.badErr
	}
	data, err := os.ReadFile(s.path)
	if err == nil {
		var b *Build
		if b, err = Load(s.calc, data); err == nil {
			s.cur, s.stamp, s.bad, s.badErr = b, st, stamp{}, nil
			return b, nil
		}
	}
	s.bad, s.badErr = st, err
	return s.cur, err
}

// SaveClipboard loads text as a build and, only if that works, stores it in
// appDir/build.txt (atomically, owner-only) and makes the clipboard the
// source. The text is never put into an error.
func (s *Store) SaveClipboard(text string) (*Build, error) {
	b, err := Load(s.calc, []byte(text))
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tmp := s.buildTxt() + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, s.buildTxt()); err != nil {
		return nil, err
	}
	s.source, s.path, s.cur, s.stamp, s.bad, s.badErr = config.SourceClipboard, "", b, stamp{}, stamp{}, nil
	return b, nil
}
