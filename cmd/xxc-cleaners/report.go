package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/kawaiipantsu/xxc-cleaners/internal/cleaners"
	"github.com/kawaiipantsu/xxc-cleaners/internal/config"
	"github.com/kawaiipantsu/xxc-cleaners/internal/disk"
	"github.com/kawaiipantsu/xxc-cleaners/internal/engine"
	"github.com/kawaiipantsu/xxc-cleaners/internal/humanize"
	"github.com/kawaiipantsu/xxc-cleaners/internal/logx"
	"github.com/kawaiipantsu/xxc-cleaners/internal/scan"
)

// printCleaners lists all cleaners, their category, state and retention.
func printCleaners(cfg *config.Config) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tCATEGORY\tSTATE\tRETENTION\tDESCRIPTION")
	cs := cleaners.All()
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].Category() != cs[j].Category() {
			return cs[i].Category() < cs[j].Category()
		}
		return cs[i].Key() < cs[j].Key()
	})
	for _, c := range cs {
		state := "enabled"
		if !cfg.Enabled(c.Key()) {
			state = "disabled"
		}
		if !c.Available() {
			state = "n/a"
		}
		ret := fmt.Sprintf("%dd", cfg.RetentionFor(c.Key()))
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", c.Key(), c.Category(), state, ret, c.Title())
	}
	_ = w.Flush()
}

// printDiskOverview prints mounted filesystems, inode usage and block devices.
func printDiskOverview(log *logx.Logger) int {
	mounts, err := disk.Mounts(false)
	if err != nil {
		log.Error("disk: %v", err)
		return 1
	}
	fmt.Println("Filesystems")
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "MOUNT\tTYPE\tSIZE\tUSED\tAVAIL\tUSE%\tINODES%\tBAR")
	for _, m := range mounts {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			m.MountPoint, m.FSType,
			humanize.Bytes(m.TotalBytes), humanize.Bytes(m.UsedBytes), humanize.Bytes(m.FreeBytes),
			fmt.Sprintf("%.0f%%", m.UsedPercent()), fmt.Sprintf("%.0f%%", m.InodePercent()),
			bar(m.UsedPercent(), 20))
	}
	_ = w.Flush()

	devs, err := disk.Partitions()
	if err == nil && len(devs) > 0 {
		fmt.Println("\nBlock devices")
		dw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(dw, "NAME\tSIZE")
		for _, d := range devs {
			fmt.Fprintf(dw, "%s\t%s\n", d.Name, humanize.Bytes(d.SizeBytes()))
		}
		_ = dw.Flush()
	}
	return 0
}

// printAnalysis shows the largest files under a path.
func printAnalysis(cfg *config.Config, path string, log *logx.Logger) int {
	fmt.Printf("Storage analysis of %s (files >= %s)\n\n", path, humanize.Bytes(cfg.MinLargeFile))
	files, err := scan.LargestFiles([]string{path}, cfg.MinLargeFile, 40)
	if err != nil {
		log.Error("analyze: %v", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Println("No files above the threshold.")
		return 0
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "SIZE\tMODIFIED\tPATH")
	for _, f := range files {
		fmt.Fprintf(w, "%s\t%s\t%s\n", humanize.Bytes(f.Size), f.ModTime.Format("2006-01-02"), f.Path)
	}
	_ = w.Flush()
	fmt.Printf("\nTotal of listed: %s\n", humanize.Bytes(scan.TotalSize(files)))
	return 0
}

// runClean plans and executes the selected cleaners (non-interactive).
func runClean(eng *engine.Engine, cfg *config.Config, log *logx.Logger, only, except []string) int {
	// Safety gate: without --yes (and not a dry-run), force a dry-run so a bare
	// invocation never deletes anything by surprise.
	forcedDry := false
	if !cfg.AssumeYes && !cfg.DryRun {
		cfg.DryRun = true
		forcedDry = true
	}

	selected := eng.Select(only, except)
	if len(selected) == 0 {
		log.Warn("no cleaners selected")
		return 0
	}
	plans := eng.PlanAll(selected)

	// Summary header
	var totalReclaim int64
	for _, p := range plans {
		totalReclaim += p.ReclaimBytes()
	}
	mode := "CLEAN"
	if cfg.DryRun {
		mode = "DRY-RUN"
	}
	log.Info("XXC The Cleaners — %s  (reclaimable: %s)", mode, humanize.Bytes(totalReclaim))
	if forcedDry {
		log.Info("(no --yes given — showing a preview only; add --yes to actually clean)")
	}
	log.Info("")
	printPlanSummary(plans, log)
	log.Info("")

	res := eng.ExecuteAll(plans)

	verb := "freed"
	if res.DryRun {
		verb = "would free"
	}
	log.Info("")
	log.Info("Done. %s %s across %d files and %d commands (%d errors).",
		verb, humanize.Bytes(res.BytesFreed), res.FilesDeleted, res.CommandsRun, res.Errors)
	if res.Errors > 0 {
		return 1
	}
	return 0
}

func printPlanSummary(plans []*cleaners.Plan, log *logx.Logger) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "  CLEANER\tITEMS\tRECLAIM\tNOTE")
	for _, p := range plans {
		if p.Unavailable {
			fmt.Fprintf(w, "  %s %s\t-\t-\t%s\n", p.Icon, p.Cleaner, orNA(p.Note, "not available"))
			continue
		}
		items := len(p.Findings) + len(p.Commands)
		if items == 0 && p.Note == "" {
			continue
		}
		fmt.Fprintf(w, "  %s %s\t%d\t%s\t%s\n", p.Icon, p.Cleaner, items, humanize.Bytes(p.ReclaimBytes()), p.Note)
	}
	_ = w.Flush()
	// tabwriter writes straight to stdout; honor silent via logger gate.
	_ = log
}

func orNA(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// bar renders a simple ASCII usage bar.
func bar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct/100*float64(width) + 0.5)
	out := make([]rune, width)
	for i := 0; i < width; i++ {
		if i < filled {
			out[i] = '#'
		} else {
			out[i] = '.'
		}
	}
	return "[" + string(out) + "]"
}
