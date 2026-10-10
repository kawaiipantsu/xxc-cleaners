package cleaners

import "path/filepath"

// Categories used for grouping in the TUI and reports.
const (
	catCaches     = "caches"
	catLogs       = "logs"
	catSystem     = "system"
	catDev        = "dev"
	catContainers = "containers"
)

func init() {
	// --- User & root caches -------------------------------------------------
	register(&pathCleaner{
		key: "user-cache", title: "User cache directories", icon: "", // nf-fa-user
		category: catCaches, mode: modeEntries, detail: "user cache",
		useRetention: true,
		roots: func(_ *Context) []string {
			var r []string
			for _, h := range userHomes() {
				if h.Dir == "/root" {
					continue // handled by root-cache
				}
				r = append(r, filepath.Join(h.Dir, ".cache"))
			}
			return r
		},
	})
	register(&pathCleaner{
		key: "root-cache", title: "Root cache directories", icon: "", // nf-fa-user_secret
		category: catCaches, mode: modeEntries, detail: "root cache",
		useRetention: true,
		roots:        func(_ *Context) []string { return []string{"/root/.cache"} },
	})

	// --- /tmp ---------------------------------------------------------------
	register(&pathCleaner{
		key: "tmp", title: "Temporary files (/tmp, /var/tmp)", icon: "", // nf-fa-trash
		category: catSystem, mode: modeEntries, detail: "temp file",
		useRetention: true,
		roots:        func(_ *Context) []string { return []string{"/tmp", "/var/tmp"} },
	})

	// --- /var/log rotated logs ---------------------------------------------
	register(&pathCleaner{
		key: "var-log", title: "Rotated system logs (/var/log)", icon: "", // nf-fa-book
		category: catLogs, mode: modeFiles, detail: "rotated log",
		useRetention: true,
		roots: func(_ *Context) []string {
			// Rotated/compressed logs only — never the live *.log files.
			return []string{
				"/var/log/*.[0-9]", "/var/log/*.[0-9].gz", "/var/log/*.[0-9].xz",
				"/var/log/*.gz", "/var/log/*.old",
				"/var/log/**/*.[0-9]", "/var/log/**/*.gz",
			}
		},
	})

	// --- Apache logs --------------------------------------------------------
	register(&pathCleaner{
		key: "apache-logs", title: "Apache rotated logs", icon: "", // nf-fa-wrench
		category: catLogs, mode: modeFiles, detail: "apache log",
		useRetention: true,
		available:    func() bool { return existsAny("/var/log/apache2", "/var/log/httpd") },
		roots: func(_ *Context) []string {
			return []string{
				"/var/log/apache2/*.[0-9]", "/var/log/apache2/*.gz", "/var/log/apache2/*.[0-9].gz",
				"/var/log/httpd/*.[0-9]", "/var/log/httpd/*.gz", "/var/log/httpd/*.[0-9].gz",
			}
		},
	})

	// --- Core dumps ---------------------------------------------------------
	register(&pathCleaner{
		key: "core-dumps", title: "Core dumps & crash reports", icon: "", // nf-fa-bomb
		category: catSystem, mode: modeFiles, detail: "core dump",
		useRetention: true,
		roots: func(_ *Context) []string {
			return []string{
				"/var/lib/systemd/coredump/*",
				"/var/crash/*",
				"/core", "/core.*",
				"/tmp/core.*",
			}
		},
	})

	// --- PIP cache ----------------------------------------------------------
	register(&pathCleaner{
		key: "pip-cache", title: "Python pip cache", icon: "", // nf-dev-python
		category: catDev, mode: modeEntries, detail: "pip cache",
		useRetention: true,
		roots: func(_ *Context) []string {
			var r []string
			for _, h := range allHomesRoot() {
				r = append(r,
					filepath.Join(h, ".cache", "pip"),
					filepath.Join(h, ".cache", "pypoetry"),
				)
			}
			return r
		},
	})

	// --- Node / npm / yarn / pnpm cache ------------------------------------
	register(&pathCleaner{
		key: "node-cache", title: "Node / npm / yarn / pnpm cache", icon: "", // nf-dev-nodejs
		category: catDev, mode: modeEntries, detail: "node cache",
		useRetention: true,
		roots: func(_ *Context) []string {
			var r []string
			for _, h := range allHomesRoot() {
				r = append(r,
					filepath.Join(h, ".npm", "_cacache"),
					filepath.Join(h, ".cache", "yarn"),
					filepath.Join(h, ".cache", "node-gyp"),
					filepath.Join(h, ".local", "share", "pnpm", "store"),
				)
			}
			return r
		},
	})

	// --- Go build cache -----------------------------------------------------
	register(&pathCleaner{
		key: "go-build-cache", title: "Go build cache", icon: "", // nf-seti-go
		category: catDev, mode: modeEntries, detail: "go build cache",
		useRetention: true,
		roots: func(_ *Context) []string {
			var r []string
			for _, h := range allHomesRoot() {
				r = append(r, filepath.Join(h, ".cache", "go-build"))
			}
			return r
		},
	})

	// --- Browser caches -----------------------------------------------------
	register(&pathCleaner{
		key: "browser-cache", title: "Browser caches", icon: "", // nf-fa-chrome
		category: catCaches, mode: modeEntries, detail: "browser cache",
		useRetention: true,
		roots: func(_ *Context) []string {
			var r []string
			for _, h := range allHomesRoot() {
				r = append(r,
					filepath.Join(h, ".cache", "mozilla", "firefox"),
					filepath.Join(h, ".cache", "google-chrome"),
					filepath.Join(h, ".cache", "chromium"),
					filepath.Join(h, ".cache", "microsoft-edge"),
					filepath.Join(h, ".cache", "BraveSoftware"),
					filepath.Join(h, ".mozilla", "firefox", "*", "cache2"),
					filepath.Join(h, ".config", "google-chrome", "*", "Cache"),
					filepath.Join(h, ".config", "chromium", "*", "Cache"),
				)
			}
			return r
		},
	})

	// --- AI assistant caches / old conversations ---------------------------
	register(&pathCleaner{
		key: "ai-caches", title: "AI assistant caches (Claude/Codex/Gemini…)", icon: "󰕨", // nf-md-robot
		category: catCaches, mode: modeEntries, detail: "AI cache",
		useRetention: true,
		roots: func(_ *Context) []string {
			var r []string
			for _, h := range allHomesRoot() {
				r = append(r,
					// Cache-only locations (never config/credentials).
					filepath.Join(h, ".cache", "claude-cli-nodejs"),
					filepath.Join(h, ".claude", "cache"),
					filepath.Join(h, ".claude", ".cache"),
					filepath.Join(h, ".config", "claude", "Cache"),
					filepath.Join(h, ".codex", "cache"),
					filepath.Join(h, ".cache", "codex"),
					filepath.Join(h, ".gemini", "cache"),
					filepath.Join(h, ".config", "gemini", "cache"),
					filepath.Join(h, ".cache", "gemini"),
					filepath.Join(h, ".ollama", "models", "blobs"),
				)
			}
			return r
		},
	})

	// Command-based cleaners register themselves in their own files:
	//   apt.go, journal.go, syslog.go, docker.go
}

// allHomesRoot returns every user home plus /root.
func allHomesRoot() []string {
	var out []string
	seen := map[string]bool{}
	for _, h := range userHomes() {
		if !seen[h.Dir] {
			seen[h.Dir] = true
			out = append(out, h.Dir)
		}
	}
	if !seen["/root"] {
		out = append(out, "/root")
	}
	return out
}

func existsAny(paths ...string) bool {
	for _, p := range paths {
		if dirExists(p) {
			return true
		}
	}
	return false
}
