// Package tui implements the --interactive visual interface built on Bubble Tea.
package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kawaiipantsu/xxc-cleaners/internal/cleaners"
	"github.com/kawaiipantsu/xxc-cleaners/internal/disk"
	"github.com/kawaiipantsu/xxc-cleaners/internal/engine"
	"github.com/kawaiipantsu/xxc-cleaners/internal/scan"
)

// Run starts the interactive TUI.
func Run(eng *engine.Engine) error {
	m := newModel(eng)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type viewMode int

const (
	viewFindings viewMode = iota
	viewDisk
)

// row is one actionable line in the findings view.
type row struct {
	cleaner  string
	icon     string
	label    string // path or command label
	size     int64
	detail   string
	isCmd    bool
	argv     []string
}

type model struct {
	eng    *engine.Engine
	dryRun bool

	plans    []*cleaners.Plan
	rows     []row
	filtered []int
	selected map[int]bool

	mounts []disk.Mount

	search    textinput.Model
	searching bool

	view    viewMode
	cursor  int
	offset  int
	width   int
	height  int
	loading bool
	confirm bool
	status  string
}

// scannedMsg carries results of a background scan.
type scannedMsg struct {
	plans  []*cleaners.Plan
	mounts []disk.Mount
}

func newModel(eng *engine.Engine) model {
	ti := textinput.New()
	ti.Placeholder = "type to filter findings…"
	ti.Prompt = "  " // nf-fa-search
	ti.CharLimit = 128

	return model{
		eng:      eng,
		dryRun:   eng.Context().Config.DryRun,
		selected: map[int]bool{},
		search:   ti,
		loading:  true,
		view:     viewFindings,
		status:   "scanning…",
	}
}

func (m model) Init() tea.Cmd {
	return m.scanCmd()
}

func (m model) scanCmd() tea.Cmd {
	eng := m.eng
	return func() tea.Msg {
		cs := eng.Select(nil, nil)
		plans := eng.PlanAll(cs)
		mounts, _ := disk.Mounts(false)
		return scannedMsg{plans: plans, mounts: mounts}
	}
}

func (m *model) rebuildRows() {
	m.rows = m.rows[:0]
	for _, p := range m.plans {
		if p.Unavailable {
			continue
		}
		for _, f := range p.Findings {
			m.rows = append(m.rows, row{cleaner: p.Cleaner, icon: p.Icon, label: f.Path, size: f.Size, detail: f.Detail})
		}
		for _, c := range p.Commands {
			sz := c.Reclaim
			if sz < 0 {
				sz = 0
			}
			m.rows = append(m.rows, row{cleaner: p.Cleaner, icon: p.Icon, label: c.Label, size: sz, detail: "command", isCmd: true, argv: c.Argv})
		}
	}
	sort.SliceStable(m.rows, func(i, j int) bool { return m.rows[i].size > m.rows[j].size })
	m.applyFilter()
}

func (m *model) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	m.filtered = m.filtered[:0]
	for i, r := range m.rows {
		if q == "" || strings.Contains(strings.ToLower(r.label), q) || strings.Contains(strings.ToLower(r.cleaner), q) || strings.Contains(strings.ToLower(r.detail), q) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case scannedMsg:
		m.plans = msg.plans
		m.mounts = msg.mounts
		m.loading = false
		m.rebuildRows()
		m.status = fmt.Sprintf("%d items found", len(m.rows))
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.searching {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.applyFilter()
		return m, cmd
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Search box has focus: route keys to it, except Esc/Enter.
	if m.searching {
		switch msg.String() {
		case "esc":
			m.searching = false
			m.search.Blur()
			return m, nil
		case "enter":
			m.searching = false
			m.search.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.applyFilter()
		return m, cmd
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		if m.view == viewFindings {
			m.view = viewDisk
		} else {
			m.view = viewFindings
		}
	case "/":
		if m.view == viewFindings {
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink
		}
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case "pgup":
		m.cursor = max(0, m.cursor-10)
	case "pgdown":
		m.cursor = min(len(m.filtered)-1, m.cursor+10)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(0, len(m.filtered)-1)
	case " ":
		if m.view == viewFindings && m.cursor < len(m.filtered) {
			idx := m.filtered[m.cursor]
			m.selected[idx] = !m.selected[idx]
		}
	case "a":
		for _, idx := range m.filtered {
			m.selected[idx] = true
		}
	case "A":
		m.selected = map[int]bool{}
	case "d":
		m.dryRun = !m.dryRun
		m.eng.Context().Config.DryRun = m.dryRun
	case "r":
		m.loading = true
		m.status = "rescanning…"
		m.selected = map[int]bool{}
		return m, m.scanCmd()
	case "c":
		if m.view == viewFindings && m.countActionable() > 0 {
			m.confirm = true
		}
	case "y":
		if m.confirm {
			m.confirm = false
			return m, m.cleanCmd()
		}
	case "n", "esc":
		m.confirm = false
	}
	return m, nil
}

func (m *model) countActionable() int {
	if len(m.selected) > 0 {
		n := 0
		for _, v := range m.selected {
			if v {
				n++
			}
		}
		if n > 0 {
			return n
		}
	}
	return len(m.filtered)
}

// cleanCmd executes the selected (or all filtered) rows.
func (m model) cleanCmd() tea.Cmd {
	// Build a plan from selected rows, or all filtered rows if none selected.
	indices := []int{}
	anySel := false
	for _, v := range m.selected {
		if v {
			anySel = true
			break
		}
	}
	if anySel {
		for i, v := range m.selected {
			if v {
				indices = append(indices, i)
			}
		}
	} else {
		indices = append(indices, m.filtered...)
	}

	plan := &cleaners.Plan{Cleaner: "selection"}
	for _, i := range indices {
		if i < 0 || i >= len(m.rows) {
			continue
		}
		r := m.rows[i]
		if r.isCmd {
			plan.Commands = append(plan.Commands, cleaners.Command{Label: r.label, Argv: r.argv, Reclaim: r.size})
		} else {
			plan.Findings = append(plan.Findings, scan.Finding{Path: r.label, Size: r.size, Detail: r.detail})
		}
	}
	eng := m.eng
	return func() tea.Msg {
		res := eng.Execute(plan)
		_ = res
		cs := eng.Select(nil, nil)
		plans := eng.PlanAll(cs)
		mounts, _ := disk.Mounts(false)
		return scannedMsg{plans: plans, mounts: mounts}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
