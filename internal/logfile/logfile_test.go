package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func size(t *testing.T, path string) int64 {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Size()
}

func TestStaysUnderLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pricecheck.log")
	var opened []bool
	l, err := Open(path, 100, func(f *os.File) { opened = append(opened, f != nil) })
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("x", 19) + "\n"
	for i := 0; i < 20; i++ {
		if _, err := l.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	l.Write([]byte("last\n"))
	if s := size(t, path); s > 50 {
		t.Errorf("log is %d bytes, want ≤ 50", s)
	}
	if s := size(t, path+".1"); s > 50 || s == 0 {
		t.Errorf("log.1 is %d bytes, want 1..50", s)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasSuffix(string(b), "last\n") {
		t.Errorf("newest line missing: %q", b)
	}
	if len(opened) < 3 || !opened[0] || opened[1] || !opened[2] {
		t.Errorf("onOpen calls %v, want open, release, open, …", opened)
	}
}

func TestBigFileAtStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pricecheck.log")
	os.WriteFile(path, []byte(strings.Repeat("old\n", 100)), 0o644)
	l, err := Open(path, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	l.Write([]byte("new\n"))
	if s := size(t, path); s != 4 {
		t.Errorf("log is %d bytes, want 4", s)
	}
	if s := size(t, path+".1"); s != 400 {
		t.Errorf("log.1 is %d bytes, want the old 400", s)
	}
}
