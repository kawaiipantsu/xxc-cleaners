package cleaners

import (
	"github.com/kawaiipantsu/xxc-cleaners/internal/scan"
)

// scanMode selects how a target directory is walked.
type scanMode int

const (
	// modeFiles walks recursively and reports individual files.
	modeFiles scanMode = iota
	// modeEntries reports immediate children (files and whole dirs).
	modeEntries
)

// pathCleaner is a data-driven cleaner that reclaims files under a set of
// directories/globs, optionally filtered by age and size. Most built-in
// cleaners are instances of this type.
type pathCleaner struct {
	key, title, icon, category string

	// roots returns the directories/globs to scan for this host.
	roots func(ctx *Context) []string

	mode   scanMode
	detail string

	// useRetention applies the per-cleaner "older than" cutoff when true.
	useRetention bool
	// minSize, when > 0, only reports files at least this large.
	minSize int64

	// available overrides availability detection (nil => always available).
	available func() bool
}

func (c *pathCleaner) Key() string      { return c.key }
func (c *pathCleaner) Title() string    { return c.title }
func (c *pathCleaner) Icon() string     { return c.icon }
func (c *pathCleaner) Category() string { return c.category }

func (c *pathCleaner) Available() bool {
	if c.available != nil {
		return c.available()
	}
	return true
}

func (c *pathCleaner) Plan(ctx *Context) (*Plan, error) {
	p := &Plan{Cleaner: c.key, Title: c.title, Icon: c.icon}
	opts := scan.Options{MinSize: c.minSize}
	if c.useRetention {
		opts.OlderThan = ctx.OlderThan(c.key)
	}

	for _, root := range c.roots(ctx) {
		for _, path := range expandRoot(root) {
			// Never descend into or remove a protected tree.
			if blocked, _ := ctx.Protect.Blocked(path); blocked {
				p.Skipped++
				continue
			}
			var (
				found []scan.Finding
				err   error
			)
			switch c.mode {
			case modeEntries:
				found, err = scan.Entries(path, opts, c.detail)
			default:
				found, err = scan.Files(path, opts, c.detail)
			}
			if err != nil {
				ctx.Log.Debug("%s: scan %s: %v", c.key, path, err)
			}
			for _, f := range found {
				if blocked, which := ctx.Protect.Blocked(f.Path); blocked {
					ctx.Log.Debug("%s: skip protected %s (under %s)", c.key, f.Path, which)
					p.Skipped++
					continue
				}
				p.Findings = append(p.Findings, f)
			}
		}
	}
	p.Findings = dedupeFindings(p.Findings)
	scan.SortBySizeDesc(p.Findings)
	return p, nil
}

// expandRoot returns matching paths for a root that may contain glob wildcards.
func expandRoot(root string) []string {
	if hasGlob(root) {
		return scan.Expand(root)
	}
	return []string{root}
}

func hasGlob(s string) bool {
	for _, r := range s {
		if r == '*' || r == '?' || r == '[' {
			return true
		}
	}
	return false
}
