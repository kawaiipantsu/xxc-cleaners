package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/xxc-cleaners/internal/humanize"
)

var (
	colAccent = lipgloss.Color("170") // magenta/pink — THUGS(red) vibe
	colRed    = lipgloss.Color("196")
	colDim    = lipgloss.Color("244")
	colGreen  = lipgloss.Color("78")
	colYellow = lipgloss.Color("214")

	styTitle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styDim   = lipgloss.NewStyle().Foreground(colDim)
	stySel   = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(colAccent)
	styMark  = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	styWarn  = lipgloss.NewStyle().Foreground(colYellow)
	styErr   = lipgloss.NewStyle().Foreground(colRed).Bold(true)

	box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(0, 1)
)

func (m model) View() string {
	if m.width == 0 {
		return "initializing…"
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")
	if m.loading {
		b.WriteString(box.Width(m.width - 2).Render("\n    " + m.status + "\n"))
		b.WriteString("\n" + m.footer())
		return b.String()
	}
	switch m.view {
	case viewDisk:
		b.WriteString(m.diskView())
	default:
		b.WriteString(m.findingsView())
	}
	b.WriteString("\n")
	if m.confirm {
		b.WriteString(m.confirmBar())
	} else {
		b.WriteString(m.footer())
	}
	return b.String()
}

func (m model) header() string {
	mode := styGreenOrRed(m.dryRun)
	left := styTitle.Render(" 󱆜 XXC The Cleaners ") // nf-md-broom
	tabs := m.tabBar()
	right := mode + "  " + styDim.Render("THUGS(red)")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(tabs) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	line := left + tabs + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().Width(m.width).Render(line)
}

func styGreenOrRed(dry bool) string {
	if dry {
		return lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render(" DRY-RUN")
	}
	return styErr.Render(" LIVE")
}

func (m model) tabBar() string {
	tab := func(name string, on bool) string {
		s := lipgloss.NewStyle().Padding(0, 1)
		if on {
			return s.Foreground(lipgloss.Color("231")).Background(colAccent).Render(name)
		}
		return s.Foreground(colDim).Render(name)
	}
	return "  " + tab(" Findings", m.view == viewFindings) + tab(" Disk", m.view == viewDisk)
}

func (m model) findingsView() string {
	inner := m.width - 4
	if inner < 20 {
		inner = 20
	}
	var b strings.Builder

	// Search box
	searchStyle := styDim
	if m.searching {
		searchStyle = lipgloss.NewStyle().Foreground(colAccent)
	}
	b.WriteString(searchStyle.Render(m.search.View()))
	b.WriteString("\n")

	// Column layout
	sizeW, cleanerW := 9, 16
	pathW := inner - sizeW - cleanerW - 6
	if pathW < 10 {
		pathW = 10
	}

	head := fmt.Sprintf(" %-3s %-*s %-*s %s", "", sizeW, "SIZE", cleanerW, "CLEANER", "PATH / ACTION")
	b.WriteString(styDim.Render(head) + "\n")

	rows := m.visibleRows(pathW, sizeW, cleanerW)
	b.WriteString(rows)

	content := b.String()
	return box.Width(m.width - 2).Render(content)
}

func (m model) visibleRows(pathW, sizeW, cleanerW int) string {
	if len(m.filtered) == 0 {
		return styDim.Render("\n  (nothing matches — clean machine, or adjust your filter)\n")
	}
	// window
	rowsVisible := m.height - 11
	if rowsVisible < 3 {
		rowsVisible = 3
	}
	start := 0
	if m.cursor >= rowsVisible {
		start = m.cursor - rowsVisible + 1
	}
	end := min(len(m.filtered), start+rowsVisible)

	var b strings.Builder
	for i := start; i < end; i++ {
		idx := m.filtered[i]
		r := m.rows[idx]
		cursor := " "
		if i == m.cursor {
			cursor = styTitle.Render("") // >
		}
		mark := " "
		if m.selected[idx] {
			mark = styMark.Render("") // check
		}
		size := humanize.Bytes(r.size)
		if r.isCmd && r.size == 0 {
			size = "cmd"
		}
		label := truncate(r.label, pathW)
		cleaner := truncate(r.icon+" "+r.cleaner, cleanerW)
		line := fmt.Sprintf("%s%s %-*s %-*s %s", cursor, mark, sizeW, size, cleanerW, cleaner, label)
		if i == m.cursor {
			line = stySel.Render(fmt.Sprintf("%-*s", pathW+sizeW+cleanerW+6, strings.TrimRight(line, " ")))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(styDim.Render(fmt.Sprintf("\n  %d/%d shown · %d selected · %s reclaimable",
		len(m.filtered), len(m.rows), m.selectedCount(), humanize.Bytes(m.selectedBytes()))))
	return b.String()
}

func (m model) diskView() string {
	var b strings.Builder
	b.WriteString(styTitle.Render("  Filesystems") + "\n\n")
	if len(m.mounts) == 0 {
		b.WriteString(styDim.Render("  no filesystems detected"))
		return box.Width(m.width - 2).Render(b.String())
	}
	barW := m.width - 40
	if barW < 10 {
		barW = 10
	}
	for _, mt := range m.mounts {
		name := truncate(mt.MountPoint, 18)
		usage := fmt.Sprintf("%6s/%-6s", humanize.Bytes(mt.UsedBytes), humanize.Bytes(mt.TotalBytes))
		b.WriteString(fmt.Sprintf(" %-18s %s %s %s\n",
			name, colorBar(mt.UsedPercent(), barW), usage, pctLabel(mt.UsedPercent())))
		b.WriteString(styDim.Render(fmt.Sprintf(" %-18s %s  inodes %s\n",
			"", truncate(mt.FSType+" on "+mt.Device, barW+1), pctLabel(mt.InodePercent()))))
	}
	return box.Width(m.width - 2).Render(b.String())
}

func colorBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct/100*float64(width) + 0.5)
	full := strings.Repeat("█", filled)
	empty := strings.Repeat("░", width-filled)
	c := colGreen
	switch {
	case pct >= 90:
		c = colRed
	case pct >= 75:
		c = colYellow
	}
	return lipgloss.NewStyle().Foreground(c).Render(full) + styDim.Render(empty)
}

func pctLabel(pct float64) string {
	s := fmt.Sprintf("%3.0f%%", pct)
	switch {
	case pct >= 90:
		return styErr.Render(s)
	case pct >= 75:
		return styWarn.Render(s)
	default:
		return styMark.Render(s)
	}
}

func (m model) footer() string {
	keys := []string{
		" tab:view", "/ search", "↑↓ move", "space:select",
		"a:all", "A:none", "c:clean", "d:dry-run", "r:rescan", "q:quit",
	}
	return styDim.Render(" " + strings.Join(keys, "  ·  "))
}

func (m model) confirmBar() string {
	n := m.countActionable()
	mode := "DELETE"
	if m.dryRun {
		mode = "preview (dry-run)"
	}
	msg := fmt.Sprintf("  %s %d item(s)? ", mode, n)
	return styWarn.Bold(true).Render(msg) + styDim.Render(" [y] confirm   [n/esc] cancel")
}

func (m model) selectedCount() int {
	n := 0
	for _, v := range m.selected {
		if v {
			n++
		}
	}
	return n
}

func (m model) selectedBytes() int64 {
	var t int64
	for i, v := range m.selected {
		if v && i < len(m.rows) {
			t += m.rows[i].size
		}
	}
	return t
}

func truncate(s string, w int) string {
	if w <= 1 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	// Truncate from the left for paths so the filename stays visible.
	r := []rune(s)
	if len(r) > w {
		return "…" + string(r[len(r)-w+1:])
	}
	return s
}
