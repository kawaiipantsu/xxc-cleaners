// Package cleaners defines the cleaner framework and the built-in cleaners that
// reclaim disk space from caches, logs, dumps and build artifacts.
package cleaners

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kawaiipantsu/xxc-cleaners/internal/config"
	"github.com/kawaiipantsu/xxc-cleaners/internal/logx"
	"github.com/kawaiipantsu/xxc-cleaners/internal/protect"
	"github.com/kawaiipantsu/xxc-cleaners/internal/scan"
)

// Command is an external command a cleaner runs to reclaim space (e.g.
// "journalctl --vacuum-size", "docker builder prune").
type Command struct {
	Label   string   // human description
	Argv    []string // program + args
	Reclaim int64    // estimated bytes freed, -1 if unknown
}

// Plan is the result of a cleaner inspecting the system. It lists files that can
// be deleted and/or external commands that reclaim space.
type Plan struct {
	Cleaner     string
	Title       string
	Icon        string
	Findings    []scan.Finding
	Commands    []Command
	Skipped     int    // entries skipped because they were protected
	Note        string // optional human note (e.g. "docker not installed")
	Unavailable bool   // cleaner does not apply on this host
}

// ReclaimBytes returns the total reclaimable size (files + known command estimates).
func (p *Plan) ReclaimBytes() int64 {
	t := scan.TotalSize(p.Findings)
	for _, c := range p.Commands {
		if c.Reclaim > 0 {
			t += c.Reclaim
		}
	}
	return t
}

// Empty reports whether the plan would do nothing.
func (p *Plan) Empty() bool { return len(p.Findings) == 0 && len(p.Commands) == 0 }

// Context carries shared state into a cleaner's Plan method.
type Context struct {
	Config  *config.Config
	Protect *protect.Set
	Log     *logx.Logger
	Now     time.Time
}

// OlderThan returns the cutoff time for a cleaner key based on its retention days.
func (c *Context) OlderThan(key string) time.Time {
	days := c.Config.RetentionFor(key)
	if days <= 0 {
		return c.Now.Add(24 * time.Hour) // everything qualifies (future cutoff)
	}
	return c.Now.AddDate(0, 0, -days)
}

// Cleaner inspects part of the system and proposes reclaimable items.
type Cleaner interface {
	Key() string       // stable identifier used in config and --only/--except
	Title() string     // human title
	Icon() string      // nerdfont glyph
	Category() string  // grouping (caches, logs, system, dev, containers)
	Available() bool   // whether this cleaner applies on this host
	Plan(*Context) (*Plan, error)
}

// registry holds all registered cleaners in registration order.
var registry []Cleaner

func register(c Cleaner) { registry = append(registry, c) }

// All returns every registered cleaner.
func All() []Cleaner {
	out := make([]Cleaner, len(registry))
	copy(out, registry)
	return out
}

// Get returns the cleaner with the given key, or nil.
func Get(key string) Cleaner {
	for _, c := range registry {
		if c.Key() == key {
			return c
		}
	}
	return nil
}

// Keys returns all cleaner keys (sorted).
func Keys() []string {
	k := make([]string, 0, len(registry))
	for _, c := range registry {
		k = append(k, c.Key())
	}
	sort.Strings(k)
	return k
}

// ----- shared helpers -----

// home describes a user home directory to scan.
type home struct {
	User string
	Dir  string
}

// userHomes returns real user home directories (uid 0 and >= 1000) plus /root,
// read from /etc/passwd. Non-existent directories are dropped.
func userHomes() []home {
	var out []home
	f, err := os.Open("/etc/passwd")
	if err != nil {
		// Fallback: at least try the invoking user's home.
		if h, _ := os.UserHomeDir(); h != "" {
			out = append(out, home{User: "self", Dir: h})
		}
		return out
	}
	defer f.Close()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ":")
		if len(parts) < 6 {
			continue
		}
		name, uidStr, dir := parts[0], parts[2], parts[5]
		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			continue
		}
		if uid != 0 && uid < 1000 {
			continue // system account
		}
		if dir == "" || dir == "/" || dir == "/nonexistent" || seen[dir] {
			continue
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		seen[dir] = true
		out = append(out, home{User: name, Dir: dir})
	}
	return out
}

// hasCmd reports whether an executable is on PATH.
func hasCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// dedupeFindings removes duplicate paths, keeping the first occurrence.
func dedupeFindings(in []scan.Finding) []scan.Finding {
	seen := map[string]bool{}
	out := in[:0]
	for _, f := range in {
		c := filepath.Clean(f.Path)
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, f)
	}
	return out
}
