// Package scan provides reusable filesystem walkers used by cleaners and the
// storage-analysis views.
package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Finding is a single reclaimable item discovered on disk.
type Finding struct {
	Path    string
	Size    int64
	ModTime time.Time
	IsDir   bool
	Detail  string // short human note (e.g. "rotated log", "pip wheel cache")
}

// Options tune a walk.
type Options struct {
	OlderThan time.Time // only include entries modified strictly before this (zero = no filter)
	MinSize   int64     // only include files >= this many bytes (0 = any)
	MaxDepth  int       // 0 = unlimited
	FollowSym bool      // follow symlinks (default false)
}

// Expand resolves a glob pattern into existing paths. Non-matching patterns
// return an empty slice rather than an error.
func Expand(pattern string) []string {
	m, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return m
}

// Files walks root and returns individual files matching opts. It never
// descends into symlinked directories.
func Files(root string, opts Options, detail string) ([]Finding, error) {
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if f, ok := consider(root, info, opts, detail); ok {
			return []Finding{f}, nil
		}
		return nil, nil
	}

	var out []Finding
	baseDepth := depth(root)
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Unreadable entry — skip it, keep going.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if opts.MaxDepth > 0 && d.IsDir() && depth(path)-baseDepth > opts.MaxDepth {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if f, ok := consider(path, fi, opts, detail); ok {
			out = append(out, f)
		}
		return nil
	}
	if err := filepath.WalkDir(root, walkFn); err != nil {
		return out, err
	}
	return out, nil
}

// Entries returns the immediate children of dir (not recursive) that match
// opts. Directories are reported as a single Finding carrying their full size.
func Entries(dir string, opts Options, detail string) ([]Finding, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Finding
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if e.IsDir() {
			if !opts.OlderThan.IsZero() && !fi.ModTime().Before(opts.OlderThan) {
				continue
			}
			sz, _ := DirSize(p)
			if opts.MinSize > 0 && sz < opts.MinSize {
				continue
			}
			out = append(out, Finding{Path: p, Size: sz, ModTime: fi.ModTime(), IsDir: true, Detail: detail})
			continue
		}
		if f, ok := consider(p, fi, opts, detail); ok {
			out = append(out, f)
		}
	}
	return out, nil
}

func consider(path string, fi os.FileInfo, opts Options, detail string) (Finding, bool) {
	if fi.IsDir() {
		return Finding{}, false
	}
	if fi.Mode()&os.ModeSymlink != 0 && !opts.FollowSym {
		return Finding{}, false
	}
	if !fi.Mode().IsRegular() {
		return Finding{}, false
	}
	if opts.MinSize > 0 && fi.Size() < opts.MinSize {
		return Finding{}, false
	}
	if !opts.OlderThan.IsZero() && !fi.ModTime().Before(opts.OlderThan) {
		return Finding{}, false
	}
	return Finding{Path: path, Size: fi.Size(), ModTime: fi.ModTime(), Detail: detail}, true
}

// DirSize returns the total size of regular files under root.
func DirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if fi, err := d.Info(); err == nil && fi.Mode().IsRegular() {
			total += fi.Size()
		}
		return nil
	})
	return total, err
}

// LargestFiles scans roots and returns the topN largest files >= min bytes.
func LargestFiles(roots []string, min int64, topN int) ([]Finding, error) {
	var all []Finding
	for _, r := range roots {
		fs, err := Files(r, Options{MinSize: min}, "large file")
		if err != nil {
			continue
		}
		all = append(all, fs...)
	}
	SortBySizeDesc(all)
	if topN > 0 && len(all) > topN {
		all = all[:topN]
	}
	return all, nil
}

// SortBySizeDesc sorts findings largest-first, in place.
func SortBySizeDesc(f []Finding) {
	sort.Slice(f, func(i, j int) bool {
		if f[i].Size != f[j].Size {
			return f[i].Size > f[j].Size
		}
		return f[i].Path < f[j].Path
	})
}

// TotalSize sums the sizes of a finding slice.
func TotalSize(f []Finding) int64 {
	var t int64
	for _, x := range f {
		t += x.Size
	}
	return t
}

func depth(p string) int {
	return len(filepath.SplitList(p)) + len(splitSlashes(p))
}

func splitSlashes(p string) []string {
	var out []string
	cur := ""
	for _, r := range filepath.Clean(p) {
		if r == filepath.Separator {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
