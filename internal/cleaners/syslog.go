package cleaners

import (
	"os"
	"path/filepath"
)

// syslogCleaner truncates the live rsyslog/syslog files and restarts the
// service so it reopens fresh handles. Unlike var-log it touches the *active*
// logs, so it is retention-exempt but conservative (truncate, not delete).
type syslogCleaner struct{}

func init() { register(&syslogCleaner{}) }

func (s *syslogCleaner) Key() string      { return "syslog" }
func (s *syslogCleaner) Title() string    { return "Live syslog (truncate + restart rsyslog)" }
func (s *syslogCleaner) Icon() string     { return "" } // nf-fa-cogs
func (s *syslogCleaner) Category() string { return catLogs }

// liveLogs are the active log files this cleaner may truncate.
func (s *syslogCleaner) liveLogs() []string {
	return []string{
		"/var/log/syslog", "/var/log/messages", "/var/log/kern.log",
		"/var/log/auth.log", "/var/log/debug", "/var/log/daemon.log",
	}
}

func (s *syslogCleaner) Available() bool {
	for _, l := range s.liveLogs() {
		if fileExists(l) {
			return true
		}
	}
	return false
}

func (s *syslogCleaner) Plan(ctx *Context) (*Plan, error) {
	p := &Plan{Cleaner: s.Key(), Title: s.Title(), Icon: s.Icon()}
	if !s.Available() {
		p.Unavailable = true
		return p, nil
	}
	var reclaim int64
	for _, l := range s.liveLogs() {
		fi, err := os.Stat(l)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
			continue
		}
		if blocked, _ := ctx.Protect.Blocked(l); blocked {
			p.Skipped++
			continue
		}
		reclaim += fi.Size()
		// Represent truncation as a "finding" the engine will truncate (dir=false,
		// detail marks it) — but truncation differs from deletion, so we use a
		// command instead to keep deletion semantics clean.
		p.Commands = append(p.Commands, Command{
			Label:   "truncate " + filepath.Base(l),
			Argv:    []string{"truncate", "-s", "0", l},
			Reclaim: fi.Size(),
		})
	}
	if reclaim > 0 && hasCmd("systemctl") {
		p.Commands = append(p.Commands, Command{
			Label:   "restart rsyslog service",
			Argv:    []string{"systemctl", "restart", "rsyslog"},
			Reclaim: 0,
		})
	}
	return p, nil
}
