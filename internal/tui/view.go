package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/xxc-cleaners/internal/humanize"
)

// Layout limits. The UI never renders below the minimum and is centered in the
// terminal; on larger terminals it grows up to the cap and stays centered.
const (
	minWidth  = 80
	minHeight = 24
	maxWidth  = 120
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

// uiWidth returns the outer UI width (border included) for a terminal width.
func uiWidth(termW int) int {
	w := termW
	if w > maxWidth {
		w = maxWidth
	}
	if w < minWidth {
		w = minWidth
	}
	return w
}

// uiContentWidth is the usable width inside the box (minus border + padding).
func uiContentWidth(termW int) int { return uiWidth(termW) - 4 }

func (m model) View() string {
	if m.width == 0 {
		return "initializing…"
	}
	// Minimum-size gate.
	if m.width < minWidth || m.height < minHeight {
		msg := styWarn.Render(fmt.Sprintf(
			"  Terminal too small\n\n  need at least %d×%d\n  current  %d×%d\n\n  resize, or press q to quit",
			minWidth, minHeight, m.width, m.height))
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			box.Render(msg))
	}

	outerW := uiWidth(m.width)
	contentW := outerW - 4

	var body string
	if m.loading {
		body = box.Width(contentW + 2).Render("\n    " + m.status + "\n")
	} else if m.view == viewDisk {
		body = m.diskView(contentW)
	} else if isUsagePage(m.view) {
		body = m.usageView(contentW)
	} else {
		body = m.findingsView(contentW)
	}

	var bottom string
	if m.confirm {
		bottom = m.confirmBar(outerW)
	} else {
		bottom = m.footer(outerW)
	}

	block := lipgloss.JoinVertical(lipgloss.Left,
		m.header(outerW),
		body,
		bottom,
	)
	// Center the whole block in the terminal, horizontally and vertically.
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

func (m model) header(outerW int) string {
	left := styTitle.Render("󱆜 XXC The Cleaners") // nf-md-broom
	mode := styGreenOrRed(m.dryRun)
	right := mode + "  " + styDim.Render("THUGS(red)")

	gap := outerW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	titleLine := lipgloss.NewStyle().Width(outerW).MaxWidth(outerW).
		Render(left + strings.Repeat(" ", gap) + right)

	// Tab bar on its own full-width line so all tabs fit at 80 columns.
	tabLine := lipgloss.NewStyle().Width(outerW).MaxWidth(outerW).Render(m.tabBar())

	return lipgloss.JoinVertical(lipgloss.Left, titleLine, tabLine)
}

func styGreenOrRed(dry bool) string {
	if dry {
		return lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render(" DRY-RUN")
	}
	return styErr.Render(" LIVE")
}

func (m model) tabBar() string {
	tab := func(name string, on bool) string {
		st := lipgloss.NewStyle().Padding(0, 1)
		if on {
			return st.Foreground(lipgloss.Color("231")).Background(colAccent).Bold(true).Render(name)
		}
		return st.Foreground(colDim).Render(name)
	}
	var b strings.Builder
	b.WriteString(" ")
	for i, v := range viewOrder {
		b.WriteString(tab(fmt.Sprintf("%d %s", i+1, tabLabel(v)), m.view == v))
	}
	return b.String()
}

func (m model) findingsView(contentW int) string {
	var b strings.Builder

	// Search box (its own line; width was set on resize).
	searchStyle := styDim
	if m.searching {
		searchStyle = lipgloss.NewStyle().Foreground(colAccent)
	}
	b.WriteString(searchStyle.Render(m.search.View()))
	b.WriteString("\n")

	// Column widths (fixed columns; path takes the rest).
	const (
		wCursor  = 1
		wMark    = 1
		wSize    = 9
		wCleaner = 18
		gaps     = 4 // spaces between the 5 fields
	)
	wPath := contentW - wCursor - wMark - wSize - wCleaner - gaps
	if wPath < 10 {
		wPath = 10
	}

	// Header row.
	head := fmt.Sprintf("%s %s %s %s",
		fitPlain("", wCursor+wMark),
		fitPlain("SIZE", wSize),
		fitPlain("CLEANER", wCleaner),
		fitPlain("PATH / ACTION", wPath))
	b.WriteString(styDim.Render(head) + "\n")

	b.WriteString(m.rowsBlock(wSize, wCleaner, wPath))

	return box.Width(contentW + 2).Render(b.String())
}

func (m model) rowsBlock(wSize, wCleaner, wPath int) string {
	if len(m.filtered) == 0 {
		return styDim.Render("\n  (nothing matches — clean machine, or adjust your filter)")
	}

	rowsVisible := m.height - 12
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
		sizeStr := humanize.Bytes(r.size)
		if r.isCmd && r.size == 0 {
			sizeStr = "cmd"
		}
		cur := " "
		if i == m.cursor {
			cur = "" // chevron
		}
		mark := " "
		if m.selected[idx] {
			mark = "" // check
		}
		size := fitPlain(sizeStr, wSize)
		cleaner := fitPlain(r.icon+" "+r.cleaner, wCleaner)
		path := fitPlain(r.label, wPath)

		if i == m.cursor {
			// Selected: one background style over the whole plain line.
			plain := fmt.Sprintf("%s%s %s %s %s", cur, mark, size, cleaner, path)
			b.WriteString(stySel.Render(plain))
		} else {
			markR := mark
			if m.selected[idx] {
				markR = styMark.Render(mark)
			}
			b.WriteString(fmt.Sprintf("%s%s %s %s %s",
				styDim.Render(cur), markR, size, styDim.Render(cleaner), path))
		}
		b.WriteString("\n")
	}
	b.WriteString(styDim.Render(fmt.Sprintf("\n  %d/%d shown · %d selected · %s reclaimable",
		len(m.filtered), len(m.rows), m.selectedCount(), humanize.Bytes(m.selectedBytes()))))
	return b.String()
}

func (m model) diskView(contentW int) string {
	var b strings.Builder
	b.WriteString(styTitle.Render(" Filesystems") + "\n\n")
	if len(m.mounts) == 0 {
		b.WriteString(styDim.Render("  no filesystems detected"))
		return box.Width(contentW + 2).Render(b.String())
	}
	nameW := 20
	// bar + "  used/total  pct" trailer; size the bar to the remaining width.
	trailerW := 24
	barW := contentW - nameW - trailerW - 2
	if barW < 10 {
		barW = 10
	}
	for _, mt := range m.mounts {
		name := fitPlain(mt.MountPoint, nameW)
		usage := fmt.Sprintf("%7s/%-7s", humanize.Bytes(mt.UsedBytes), humanize.Bytes(mt.TotalBytes))
		b.WriteString(fmt.Sprintf("%s %s %s %s\n",
			name, colorBar(mt.UsedPercent(), barW), usage, pctLabel(mt.UsedPercent())))
		sub := fmt.Sprintf("%s on %s  ·  inodes %.0f%%", mt.FSType, mt.Device, mt.InodePercent())
		b.WriteString(styDim.Render(fitPlain("  "+sub, contentW)) + "\n")
	}
	return box.Width(contentW + 2).Render(strings.TrimRight(b.String(), "\n"))
}

// usageView renders one of the lazily-loaded survey pages (Homes/DBs/Mail/Web).
func (m model) usageView(contentW int) string {
	var b strings.Builder
	b.WriteString(styTitle.Render(pageTitle(m.view)) + "\n\n")

	if m.pageLoading[m.view] || !m.pageLoaded[m.view] {
		b.WriteString(styDim.Render("    measuring… (this can take a moment on large trees)"))
		return box.Width(contentW + 2).Render(b.String())
	}

	items := m.pages[m.view]
	if len(items) == 0 {
		b.WriteString(styDim.Render("  nothing detected on this host"))
		return box.Width(contentW + 2).Render(b.String())
	}

	var maxSize, total int64
	for _, it := range items {
		if it.Size > maxSize {
			maxSize = it.Size
		}
		total += it.Size
	}

	const (
		wCursor = 2
		wLabel  = 22
		wSize   = 9
		wDetail = 13
	)
	wBar := contentW - wCursor - wLabel - wSize - wDetail - 3
	if wBar < 6 {
		wBar = 6
	}

	for i, it := range items {
		cursor := "  "
		if i == m.cursor {
			cursor = styTitle.Render(" ") // chevron
		}
		pct := 0.0
		if maxSize > 0 {
			pct = float64(it.Size) / float64(maxSize) * 100
		}
		label := fitPlain(it.Label, wLabel)
		size := fitPlain(humanize.Bytes(it.Size), wSize)
		detail := styDim.Render(fitPlain(it.Detail, wDetail))
		b.WriteString(fmt.Sprintf("%s%s %s %s %s\n",
			cursor, label, styMark.Render(size), colorBar(pct, wBar), detail))
		b.WriteString(styDim.Render(fitPlain("  "+it.Path, contentW)) + "\n")
	}

	b.WriteString(styDim.Render(fmt.Sprintf("\n  %d locations · %s total measured",
		len(items), humanize.Bytes(total))))
	return box.Width(contentW + 2).Render(strings.TrimRight(b.String(), "\n"))
}

func colorBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct/100*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
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

func (m model) footer(outerW int) string {
	keys := []string{
		"tab/1-6:pages", "/ search", "↑↓ move", "space:sel",
		"a:all", "c:clean", "d:dry-run", "r:rescan", "q:quit",
	}
	line := " " + strings.Join(keys, " · ")
	return styDim.Render(fitPlain(line, outerW))
}

func (m model) confirmBar(outerW int) string {
	n := m.countActionable()
	mode := "DELETE"
	if m.dryRun {
		mode = "preview (dry-run)"
	}
	msg := fmt.Sprintf("  %s %d item(s)?  [y] confirm   [n/esc] cancel", mode, n)
	return styWarn.Bold(true).Render(fitPlain(msg, outerW))
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

// fitPlain pads or truncates PLAIN (unstyled) text to exactly w display cells.
// Truncation adds an ellipsis; padding appends spaces. Operating on plain text
// keeps the box's right border perfectly aligned.
func fitPlain(s string, w int) string {
	if w <= 0 {
		return ""
	}
	vw := lipgloss.Width(s)
	if vw == w {
		return s
	}
	if vw < w {
		return s + strings.Repeat(" ", w-vw)
	}
	// Too wide: drop trailing runes until it fits with an ellipsis.
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	out := string(r) + "…"
	if pad := w - lipgloss.Width(out); pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out
}
