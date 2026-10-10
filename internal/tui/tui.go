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
	"github.com/kawaiipantsu/xxc-cleaners/internal/survey"
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
	viewHome
	viewDatabases
	viewMail
	viewWeb
)

// viewOrder is the left-to-right tab order (and 1..N number-key order).
var viewOrder = []viewMode{viewFindings, viewDisk, viewHome, viewDatabases, viewMail, viewWeb}

// tabLabel is the short label shown in the tab bar (with a nerdfont icon).
func tabLabel(v viewMode) string {
	switch v {
	case viewFindings:
		return " Findings"
	case viewDisk:
		return " Disk"
	case viewHome:
		return " Homes"
	case viewDatabases:
		return " DBs"
	case viewMail:
		return " Mail"
	case viewWeb:
		return " Web"
	}
	return "?"
}

// pageTitle is the heading shown on a usage page.
func pageTitle(v viewMode) string {
	switch v {
	case viewHome:
		return " Disk usage per user home"
	case viewDatabases:
		return " Databases, caches & queues"
	case viewMail:
		return " Mail storage"
	case viewWeb:
		return " Web storage"
	}
	return ""
}

// isUsagePage reports whether v is one of the lazily-loaded survey pages.
func isUsagePage(v viewMode) bool {
	return v == viewHome || v == viewDatabases || v == viewMail || v == viewWeb
}

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

	// Lazily-loaded usage pages (Homes / Databases / Mail / Web).
	pages       map[viewMode][]survey.Item
	pageLoaded  map[viewMode]bool
	pageLoading map[viewMode]bool

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
		eng:         eng,
		dryRun:      eng.Context().Config.DryRun,
		selected:    map[int]bool{},
		pages:       map[viewMode][]survey.Item{},
		pageLoaded:  map[viewMode]bool{},
		pageLoading: map[viewMode]bool{},
		search:      ti,
		loading:     true,
		view:        viewFindings,
		status:      "scanning…",
	}
}

// pageMsg carries the result of loading a usage page in the background.
type pageMsg struct {
	view  viewMode
	items []survey.Item
}

// loadPageCmd computes a usage page off the UI thread.
func loadPageCmd(v viewMode) tea.Cmd {
	return func() tea.Msg {
		var items []survey.Item
		switch v {
		case viewHome:
			items = survey.Homes()
		case viewDatabases:
			items = survey.Databases()
		case viewMail:
			items = survey.Mail()
		case viewWeb:
			items = survey.Web()
		}
		return pageMsg{view: v, items: items}
	}
}

// ensurePage loads the current usage page if it hasn't been loaded yet.
func (m *model) ensurePage() tea.Cmd {
	v := m.view
	if !isUsagePage(v) || m.pageLoaded[v] || m.pageLoading[v] {
		return nil
	}
	m.pageLoading[v] = true
	return loadPageCmd(v)
}

// pageLen returns the number of navigable rows in the current view.
func (m model) pageLen() int {
	switch m.view {
	case viewFindings:
		return len(m.filtered)
	case viewHome, viewDatabases, viewMail, viewWeb:
		return len(m.pages[m.view])
	default:
		return 0
	}
}

// cycleView moves the active tab by delta (wrapping).
func cycleView(v viewMode, delta int) viewMode {
	idx := 0
	for i, vv := range viewOrder {
		if vv == v {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(viewOrder)) % len(viewOrder)
	return viewOrder[idx]
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
		m.search.Width = uiContentWidth(m.width) - 4
		return m, nil

	case scannedMsg:
		m.plans = msg.plans
		m.mounts = msg.mounts
		m.loading = false
		m.rebuildRows()
		m.status = fmt.Sprintf("%d items found", len(m.rows))
		return m, nil

	case pageMsg:
		m.pages[msg.view] = msg.items
		m.pageLoaded[msg.view] = true
		m.pageLoading[msg.view] = false
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
	case "tab", "right", "l":
		m.view = cycleView(m.view, +1)
		m.cursor = 0
		return m, m.ensurePage()
	case "shift+tab", "left", "h":
		m.view = cycleView(m.view, -1)
		m.cursor = 0
		return m, m.ensurePage()
	case "1", "2", "3", "4", "5", "6":
		if n := int(msg.String()[0] - '1'); n >= 0 && n < len(viewOrder) {
			m.view = viewOrder[n]
			m.cursor = 0
			return m, m.ensurePage()
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
		if m.cursor < m.pageLen()-1 {
			m.cursor++
		}
	case "pgup":
		m.cursor = max(0, m.cursor-10)
	case "pgdown":
		m.cursor = min(m.pageLen()-1, m.cursor+10)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(0, m.pageLen()-1)
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
		// Invalidate cached usage pages so they reload on next visit.
		m.pageLoaded = map[viewMode]bool{}
		m.pageLoading = map[viewMode]bool{}
		m.pages = map[viewMode][]survey.Item{}
		if isUsagePage(m.view) {
			return m, m.ensurePage()
		}
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
