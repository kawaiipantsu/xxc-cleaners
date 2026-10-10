// Package config loads and merges settings from /etc/xxc/cleaners.conf and the CLI.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kawaiipantsu/xxc-cleaners/internal/humanize"
)

// DefaultPath is the global configuration file location.
const DefaultPath = "/etc/xxc/cleaners.conf"

// Config is the merged runtime configuration.
type Config struct {
	// Behavior
	DryRun    bool
	Silent    bool
	Verbose   bool
	AssumeYes bool // when true, non-interactive runs will actually delete

	// Scanning / analysis
	RetentionDays int   // global default "older than" in days
	MinLargeFile  int64 // bytes; files >= this are flagged as "large"

	// Journal
	JournalMaxSize string // e.g. "500M" (passed to journalctl --vacuum-size)
	JournalMaxAge  string // e.g. "30d" (passed to journalctl --vacuum-time)

	// Docker
	DockerPrune    bool // prune builder cache + dangling
	DockerPruneAll bool // full system prune (dangerous: unused images too)

	// Per-cleaner overrides
	enabled   map[string]bool // explicit enable/disable; absent => enabled
	retention map[string]int  // per-cleaner retention day override

	// Protection
	Protected []string

	// Metadata
	SourcePath string // where the config was loaded from ("" if defaults only)
}

// Default returns a Config populated with safe defaults.
func Default() *Config {
	return &Config{
		RetentionDays:  14,
		MinLargeFile:   100 << 20, // 100 MiB
		JournalMaxSize: "500M",
		JournalMaxAge:  "30d",
		DockerPrune:    false,
		DockerPruneAll: false,
		enabled:        map[string]bool{},
		retention:      map[string]int{},
		Protected:      DefaultProtected(),
	}
}

// DefaultProtected lists paths that must never be touched, regardless of config.
func DefaultProtected() []string {
	return []string{
		"/", "/boot", "/etc", "/usr", "/bin", "/sbin", "/lib", "/lib64",
		"/proc", "/sys", "/dev", "/run",
		"/root/.ssh", "/root/.gnupg",
		"/var/lib/mysql", "/var/lib/mariadb", "/var/lib/postgresql",
		"/var/lib/mongodb", "/var/lib/redis", "/var/lib/influxdb",
		"/var/lib/docker/volumes", "/var/lib/libvirt", "/var/lib/dpkg",
		"/var/lib/rpm", "/var/spool/cron", "/etc/ssl", "/etc/letsencrypt",
	}
}

// Enabled reports whether a cleaner key is enabled (default true).
func (c *Config) Enabled(key string) bool {
	if v, ok := c.enabled[key]; ok {
		return v
	}
	return true
}

// SetEnabled forces a cleaner on or off.
func (c *Config) SetEnabled(key string, v bool) { c.enabled[key] = v }

// RetentionFor returns the retention days for a cleaner key, falling back to the global default.
func (c *Config) RetentionFor(key string) int {
	if v, ok := c.retention[key]; ok {
		return v
	}
	return c.RetentionDays
}

// SetRetention overrides retention for a specific cleaner.
func (c *Config) SetRetention(key string, days int) { c.retention[key] = days }

// Load reads a config file, layering it over defaults. A missing file at the
// default path is not an error (defaults are used). A missing explicit path is.
func Load(path string, explicit bool) (*Config, error) {
	c := Default()
	f, err := os.Open(path) //nolint:gosec // path is operator-controlled
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return c, nil
		}
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()
	c.SourcePath = path

	section := "global"
	sc := bufio.NewScanner(f)
	ln := 0
	for sc.Scan() {
		ln++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}

		// The [protect] section is a bare list of paths.
		if section == "protect" {
			c.Protected = append(c.Protected, filepath.Clean(line))
			continue
		}

		key, val, ok := splitKV(line)
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected key = value, got %q", path, ln, line)
		}
		if err := c.apply(section, key, val); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, ln, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return c, nil
}

func splitKV(line string) (key, val string, ok bool) {
	i := strings.IndexAny(line, "=")
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(strings.ToLower(line[:i])), strings.TrimSpace(line[i+1:]), true
}

func (c *Config) apply(section, key, val string) error {
	switch section {
	case "global", "":
		switch key {
		case "retention_days":
			n, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("retention_days: %w", err)
			}
			c.RetentionDays = n
		case "min_large_file":
			n, err := humanize.ParseBytes(val)
			if err != nil {
				return err
			}
			c.MinLargeFile = n
		case "dry_run":
			c.DryRun = parseBool(val)
		case "silent":
			c.Silent = parseBool(val)
		case "verbose":
			c.Verbose = parseBool(val)
		case "assume_yes":
			c.AssumeYes = parseBool(val)
		default:
			return fmt.Errorf("unknown global key %q", key)
		}
	case "journal":
		switch key {
		case "max_size":
			c.JournalMaxSize = val
		case "max_age":
			c.JournalMaxAge = val
		default:
			return fmt.Errorf("unknown journal key %q", key)
		}
	case "docker":
		switch key {
		case "prune":
			c.DockerPrune = parseBool(val)
		case "prune_all":
			c.DockerPruneAll = parseBool(val)
		default:
			return fmt.Errorf("unknown docker key %q", key)
		}
	case "cleaners":
		c.enabled[key] = parseBool(val)
	case "retention":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("retention[%s]: %w", key, err)
		}
		c.retention[key] = n
	default:
		return fmt.Errorf("unknown section [%s]", section)
	}
	return nil
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on", "y":
		return true
	default:
		return false
	}
}
