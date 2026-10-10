// Package protect enforces whitelist paths that must never be modified.
package protect

import (
	"path/filepath"
	"strings"
)

// Set is an immutable collection of protected path prefixes.
type Set struct {
	paths []string
}

// New builds a protection Set from a list of paths. Entries are cleaned and
// de-duplicated. The root "/" is always treated as protected.
func New(paths []string) *Set {
	seen := map[string]bool{"/": true}
	out := []string{"/"}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		c := filepath.Clean(p)
		if !filepath.IsAbs(c) || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return &Set{paths: out}
}

// Paths returns the protected paths (sorted, for display).
func (s *Set) Paths() []string {
	cp := make([]string, len(s.paths))
	copy(cp, s.paths)
	return cp
}

// Blocked reports whether operating on path would endanger a protected path.
// A path is blocked when it equals a protected entry, lies inside one, or is an
// ancestor of one (deleting it would take the protected data with it). The
// returned string is the protected entry that triggered the block.
func (s *Set) Blocked(path string) (bool, string) {
	if path == "" {
		return true, "(empty)"
	}
	c := filepath.Clean(path)
	if !filepath.IsAbs(c) {
		// Refuse to act on relative paths; they are ambiguous.
		return true, "(relative path)"
	}
	// A bare root or very shallow path is always refused.
	if c == "/" {
		return true, "/"
	}
	for _, p := range s.paths {
		switch {
		case c == p:
			return true, p
		case p != "/" && strings.HasPrefix(c, ensureSlash(p)): // c is inside p
			return true, p
		case p != "/" && strings.HasPrefix(p, ensureSlash(c)): // c is an ancestor of p
			return true, p
		}
	}
	return false, ""
}

func ensureSlash(p string) string {
	if strings.HasSuffix(p, "/") {
		return p
	}
	return p + "/"
}
