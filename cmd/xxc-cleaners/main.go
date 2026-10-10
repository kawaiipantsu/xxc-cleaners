// Command xxc-cleaners — "XXC The Cleaners": a retention & cleanup tool for Linux.
//
// When things get messy, you call in The Cleaners.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kawaiipantsu/xxc-cleaners/internal/config"
	"github.com/kawaiipantsu/xxc-cleaners/internal/engine"
	"github.com/kawaiipantsu/xxc-cleaners/internal/humanize"
	"github.com/kawaiipantsu/xxc-cleaners/internal/logx"
	"github.com/kawaiipantsu/xxc-cleaners/internal/protect"
	"github.com/kawaiipantsu/xxc-cleaners/internal/tui"
	"github.com/kawaiipantsu/xxc-cleaners/internal/version"
)

type options struct {
	configPath  string
	dryRun      bool
	silent      bool
	verbose     bool
	interactive bool
	assumeYes   bool
	only        string
	except      string
	retention   int
	minSize     string
	showList    bool
	showDisk    bool
	analyze     string
	showVersion bool
	showAbout   bool
}

func main() {
	os.Exit(run())
}

func run() int {
	opt, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	switch {
	case opt.showVersion:
		fmt.Println(version.Long())
		return 0
	case opt.showAbout:
		fmt.Println(version.About())
		return 0
	}

	// Load configuration (defaults when the file is absent at the default path).
	explicit := opt.configPath != config.DefaultPath
	cfg, err := config.Load(opt.configPath, explicit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}

	// Layer CLI flags over config values.
	applyOverrides(cfg, opt)

	log := logx.New(cfg.Silent, cfg.Verbose)
	prot := protect.New(cfg.Protected)
	eng := engine.New(cfg, prot, log)

	switch {
	case opt.showList:
		printCleaners(cfg)
		return 0
	case opt.showDisk:
		return printDiskOverview(log)
	case opt.analyze != "":
		return printAnalysis(cfg, opt.analyze, log)
	case opt.interactive:
		if err := tui.Run(eng); err != nil {
			log.Error("tui: %v", err)
			return 1
		}
		return 0
	}

	return runClean(eng, cfg, log, splitList(opt.only), splitList(opt.except))
}

func parseFlags(args []string) (options, error) {
	var opt options
	fs := flag.NewFlagSet(version.Name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.StringVar(&opt.configPath, "config", config.DefaultPath, "path to configuration file")
	fs.BoolVar(&opt.dryRun, "dry-run", false, "show what would be done without changing anything")
	fs.BoolVar(&opt.dryRun, "n", false, "alias for --dry-run")
	fs.BoolVar(&opt.silent, "silent", false, "suppress all non-error output (for crontab)")
	fs.BoolVar(&opt.silent, "s", false, "alias for --silent")
	fs.BoolVar(&opt.verbose, "verbose", false, "verbose, timestamped output")
	fs.BoolVar(&opt.verbose, "v", false, "alias for --verbose")
	fs.BoolVar(&opt.interactive, "interactive", false, "launch the visual TUI")
	fs.BoolVar(&opt.interactive, "i", false, "alias for --interactive")
	fs.BoolVar(&opt.assumeYes, "yes", false, "actually perform deletions (required for non-interactive cleaning)")
	fs.BoolVar(&opt.assumeYes, "y", false, "alias for --yes")
	fs.StringVar(&opt.only, "only", "", "comma-separated cleaner keys to run (exclusively)")
	fs.StringVar(&opt.except, "except", "", "comma-separated cleaner keys to skip")
	fs.IntVar(&opt.retention, "retention", -1, "override global retention in days (older than N days)")
	fs.StringVar(&opt.minSize, "min-size", "", "large-file threshold for analysis (e.g. 100M)")
	fs.BoolVar(&opt.showList, "list", false, "list available cleaners and exit")
	fs.BoolVar(&opt.showDisk, "disk", false, "print disk/partition/inode overview and exit")
	fs.StringVar(&opt.analyze, "analyze", "", "storage analysis: show largest files under PATH and exit")
	fs.BoolVar(&opt.showVersion, "version", false, "print version and exit")
	fs.BoolVar(&opt.showAbout, "about", false, "print about information and exit")

	fs.Usage = func() { usage(fs) }
	if err := fs.Parse(args); err != nil {
		return opt, err
	}
	return opt, nil
}

func applyOverrides(cfg *config.Config, opt options) {
	if opt.dryRun {
		cfg.DryRun = true
	}
	if opt.silent {
		cfg.Silent = true
	}
	if opt.verbose {
		cfg.Verbose = true
	}
	if opt.assumeYes {
		cfg.AssumeYes = true
	}
	if opt.retention >= 0 {
		cfg.RetentionDays = opt.retention
	}
	if opt.minSize != "" {
		if n, err := humanize.ParseBytes(opt.minSize); err == nil {
			cfg.MinLargeFile = n
		}
	}
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func usage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, "%s — %s\n\n", version.Pretty, "retention & cleanup for Linux")
	fmt.Fprintf(os.Stderr, "Usage:\n  %s [flags]\n\nFlags:\n", version.Name)
	fs.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Examples:
  %[1]s --list                      # see what can be cleaned
  %[1]s --dry-run                   # preview a full clean (default when --yes is absent)
  %[1]s --interactive               # visual TUI with search, filters & disk map
  %[1]s --only tmp,apt-cache --yes  # clean just these, for real
  %[1]s --disk                      # disk / partition / inode overview
  %[1]s --analyze /var --min-size 50M

Crontab (autonomous):
  0 4 * * *  root  %[1]s --silent --yes --only tmp,var-log,apt-cache
  30 4 * * 0 root  %[1]s --silent --yes --only journal,docker --retention 30

Config: %[2]s   (see --about)
`, version.Name, config.DefaultPath)
}
