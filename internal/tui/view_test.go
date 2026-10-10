package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/xxc-cleaners/internal/disk"
	"github.com/kawaiipantsu/xxc-cleaners/internal/survey"
)

func sampleModel(w, h int) model {
	ti := textinput.New()
	ti.Width = uiContentWidth(w) - 4
	m := model{
		selected:    map[int]bool{},
		pages:       map[viewMode][]survey.Item{},
		pageLoaded:  map[viewMode]bool{},
		pageLoading: map[viewMode]bool{},
		search:      ti,
		width:    w,
		height:   h,
		view:     viewFindings,
		rows: []row{
			{cleaner: "go-build-cache", icon: "G", label: "/root/.cache/go-build", size: 1 << 31, detail: "go"},
			{cleaner: "node-cache", icon: "N", label: "/home/dev/.npm/_cacache/this/is/a/really/long/path/that/should/truncate/cleanly", size: 1 << 28},
			{cleaner: "journal", icon: "J", label: "vacuum journal to 500M", size: 0, detail: "command", isCmd: true},
		},
		mounts: []disk.Mount{
			{Device: "/dev/sda1", MountPoint: "/", FSType: "ext4", TotalBytes: 100 << 30, UsedBytes: 97 << 30, FreeBytes: 3 << 30, TotalInode: 1000, UsedInode: 160, FreeInode: 840},
			{Device: "/dev/sdb1", MountPoint: "/var", FSType: "ext4", TotalBytes: 50 << 30, UsedBytes: 10 << 30, FreeBytes: 40 << 30, TotalInode: 1000, UsedInode: 40, FreeInode: 960},
		},
	}
	for i := range m.rows {
		m.filtered = append(m.filtered, i)
	}
	m.selected[0] = true

	// Populate a sample usage page for each survey view.
	usage := []survey.Item{
		{Label: "www-data", Path: "/var/www/html/site-with-a-long-path/public", Size: 9 << 30, Detail: "www"},
		{Label: "logs", Path: "/var/log/nginx", Size: 512 << 20, Detail: "logs"},
		{Label: "cache", Path: "/var/cache/nginx", Size: 0, Detail: "cache"},
	}
	for _, v := range []viewMode{viewHome, viewDatabases, viewMail, viewWeb} {
		m.pages[v] = usage
		m.pageLoaded[v] = true
	}
	_ = time.Now
	return m
}

// assertUniformWidth checks every line of s has the same display width.
func assertUniformWidth(t *testing.T, name, s string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatalf("%s: no lines", name)
	}
	want := lipgloss.Width(lines[0])
	for i, ln := range lines {
		if got := lipgloss.Width(ln); got != want {
			t.Errorf("%s: line %d width = %d, want %d (ragged border)\n  line: %q", name, i, got, want, ln)
		}
	}
}

func TestBoxWidthsAreUniform(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 40}, {200, 60}} {
		m := sampleModel(sz[0], sz[1])
		cw := uiContentWidth(sz[0])

		fv := m.findingsView(cw)
		assertUniformWidth(t, "findingsView", fv)
		// Outer width (content + padding + border) must equal the capped UI width.
		if got := lipgloss.Width(strings.Split(fv, "\n")[0]); got != uiWidth(sz[0]) {
			t.Errorf("findingsView outer width = %d, want %d (term %d)", got, uiWidth(sz[0]), sz[0])
		}

		m.view = viewDisk
		assertUniformWidth(t, "diskView", m.diskView(cw))

		for _, v := range []viewMode{viewHome, viewDatabases, viewMail, viewWeb} {
			m.view = v
			assertUniformWidth(t, "usageView", m.usageView(cw))
		}
	}
}

func TestViewDoesNotPanicAndGatesSmall(t *testing.T) {
	// Too small -> gate message, must not panic.
	small := sampleModel(40, 10)
	if out := small.View(); out == "" {
		t.Error("small View() returned empty")
	}
	// Normal render.
	ok := sampleModel(120, 40)
	if out := ok.View(); !strings.Contains(stripANSI(out), "XXC The Cleaners") {
		t.Error("View() missing title")
	}
}

// stripANSI removes ANSI escape sequences for substring checks.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
