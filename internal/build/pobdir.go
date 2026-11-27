package build

import (
	"encoding/xml"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry is one saved PoB build file.
type Entry struct {
	Name string // path relative to the builds folder, '/' separated, without ".xml"
	Path string
	Mod  time.Time
}

// DefaultPoBDir returns PoB's builds folder, or "" if it does not exist.
func DefaultPoBDir() string { return defaultDirFrom(pobUserPath()) }

func defaultDirFrom(userPath string) string {
	if userPath == "" {
		return ""
	}
	for _, dir := range []string{buildsDir(userPath), filepath.Join(userPath, "Builds")} {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return ""
}

// buildsDir returns <Misc buildPath> from userPath/Settings.xml, or userPath/Builds.
func buildsDir(userPath string) string {
	def := filepath.Join(userPath, "Builds")
	data, err := os.ReadFile(filepath.Join(userPath, "Settings.xml"))
	if err != nil {
		return def
	}
	var s struct {
		Misc struct {
			BuildPath string `xml:"buildPath,attr"`
		} `xml:"Misc"`
	}
	if err := xml.Unmarshal(data, &s); err != nil || s.Misc.BuildPath == "" {
		return def
	}
	return s.Misc.BuildPath
}

// ListBuilds returns all *.xml files under dir recursively, newest first.
func ListBuilds(dir string) []Entry {
	var out []Entry
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".xml") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		out = append(out, Entry{Name: rel[:len(rel)-len(".xml")], Path: p, Mod: info.ModTime()})
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Mod.After(out[j].Mod) })
	return out
}
