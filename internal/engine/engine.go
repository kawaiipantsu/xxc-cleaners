// Package engine selects, plans and executes cleaners safely.
package engine

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kawaiipantsu/xxc-cleaners/internal/cleaners"
	"github.com/kawaiipantsu/xxc-cleaners/internal/config"
	"github.com/kawaiipantsu/xxc-cleaners/internal/humanize"
	"github.com/kawaiipantsu/xxc-cleaners/internal/logx"
	"github.com/kawaiipantsu/xxc-cleaners/internal/protect"
)

// Engine ties configuration, protection and logging together.
type Engine struct {
	ctx *cleaners.Context
}

// New builds an Engine.
func New(cfg *config.Config, p *protect.Set, log *logx.Logger) *Engine {
	return &Engine{ctx: &cleaners.Context{Config: cfg, Protect: p, Log: log, Now: time.Now()}}
}

// Context exposes the cleaning context (used by the TUI).
func (e *Engine) Context() *cleaners.Context { return e.ctx }

// Select returns the cleaners to run, honoring availability, config enable
// flags and the --only / --except filters. only wins when non-empty.
func (e *Engine) Select(only, except []string) []cleaners.Cleaner {
	onlySet := toSet(only)
	exceptSet := toSet(except)
	var out []cleaners.Cleaner
	for _, c := range cleaners.All() {
		if len(onlySet) > 0 {
			if !onlySet[c.Key()] {
				continue
			}
		} else {
			if exceptSet[c.Key()] {
				continue
			}
			if !e.ctx.Config.Enabled(c.Key()) {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// Plan runs a single cleaner's planning phase.
func (e *Engine) Plan(c cleaners.Cleaner) *cleaners.Plan {
	if !c.Available() {
		return &cleaners.Plan{Cleaner: c.Key(), Title: c.Title(), Icon: c.Icon(), Unavailable: true}
	}
	p, err := c.Plan(e.ctx)
	if err != nil {
		e.ctx.Log.Warn("%s: planning failed: %v", c.Key(), err)
		if p == nil {
			p = &cleaners.Plan{Cleaner: c.Key(), Title: c.Title(), Icon: c.Icon()}
		}
	}
	return p
}

// PlanAll plans every provided cleaner.
func (e *Engine) PlanAll(cs []cleaners.Cleaner) []*cleaners.Plan {
	out := make([]*cleaners.Plan, 0, len(cs))
	for _, c := range cs {
		e.ctx.Log.Debug("planning %s", c.Key())
		out = append(out, e.Plan(c))
	}
	return out
}

// Result summarizes an execution.
type Result struct {
	FilesDeleted int
	BytesFreed   int64
	CommandsRun  int
	Errors       int
	DryRun       bool
}

// Add merges another result in.
func (r *Result) Add(o Result) {
	r.FilesDeleted += o.FilesDeleted
	r.BytesFreed += o.BytesFreed
	r.CommandsRun += o.CommandsRun
	r.Errors += o.Errors
}

// Execute carries out a plan. In dry-run mode nothing is modified; actions are
// only logged. Every deletion passes the protection check again as a backstop.
func (e *Engine) Execute(p *cleaners.Plan) Result {
	dry := e.ctx.Config.DryRun
	res := Result{DryRun: dry}
	if p.Unavailable || p.Empty() {
		return res
	}
	prefix := ""
	if dry {
		prefix = "[dry-run] "
	}

	for _, f := range p.Findings {
		if blocked, which := e.ctx.Protect.Blocked(f.Path); blocked {
			e.ctx.Log.Warn("refusing to remove protected path %s (under %s)", f.Path, which)
			res.Errors++
			continue
		}
		e.ctx.Log.Debug("%sremove %s (%s)", prefix, f.Path, humanize.Bytes(f.Size))
		if dry {
			res.FilesDeleted++
			res.BytesFreed += f.Size
			continue
		}
		if err := os.RemoveAll(f.Path); err != nil {
			e.ctx.Log.Warn("remove %s: %v", f.Path, err)
			res.Errors++
			continue
		}
		res.FilesDeleted++
		res.BytesFreed += f.Size
	}

	for _, c := range p.Commands {
		e.ctx.Log.Info("%s%s  ($ %s)", prefix, c.Label, strings.Join(c.Argv, " "))
		if dry {
			res.CommandsRun++
			if c.Reclaim > 0 {
				res.BytesFreed += c.Reclaim
			}
			continue
		}
		if err := e.run(c); err != nil {
			e.ctx.Log.Warn("%s: %v", c.Label, err)
			res.Errors++
			continue
		}
		res.CommandsRun++
		if c.Reclaim > 0 {
			res.BytesFreed += c.Reclaim
		}
	}
	return res
}

// ExecuteAll runs every plan and returns the aggregate result.
func (e *Engine) ExecuteAll(plans []*cleaners.Plan) Result {
	var total Result
	total.DryRun = e.ctx.Config.DryRun
	for _, p := range plans {
		if p.Empty() || p.Unavailable {
			continue
		}
		e.ctx.Log.Info("» %s", p.Title)
		total.Add(e.Execute(p))
	}
	return total
}

func (e *Engine) run(c cleaners.Command) error {
	if len(c.Argv) == 0 {
		return fmt.Errorf("empty command")
	}
	cmd := exec.Command(c.Argv[0], c.Argv[1:]...) //nolint:gosec // argv is built from trusted cleaner definitions
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		e.ctx.Log.Debug("output: %s", strings.TrimSpace(string(out)))
	}
	return err
}

func toSet(ss []string) map[string]bool {
	if len(ss) == 0 {
		return nil
	}
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" {
			m[s] = true
		}
	}
	return m
}
