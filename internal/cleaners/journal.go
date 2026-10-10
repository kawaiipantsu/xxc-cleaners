package cleaners

import (
	"fmt"

	"github.com/kawaiipantsu/xxc-cleaners/internal/humanize"
	"github.com/kawaiipantsu/xxc-cleaners/internal/scan"
)

// journalCleaner vacuums the systemd journal by size and age.
type journalCleaner struct{}

func init() { register(&journalCleaner{}) }

func (j *journalCleaner) Key() string      { return "journal" }
func (j *journalCleaner) Title() string    { return "systemd journal (vacuum/purge)" }
func (j *journalCleaner) Icon() string     { return "" } // nf-fa-book
func (j *journalCleaner) Category() string { return catLogs }

func (j *journalCleaner) Available() bool {
	return hasCmd("journalctl") && dirExists("/var/log/journal")
}

func (j *journalCleaner) Plan(ctx *Context) (*Plan, error) {
	p := &Plan{Cleaner: j.Key(), Title: j.Title(), Icon: j.Icon()}
	if !j.Available() {
		p.Unavailable = true
		p.Note = "journalctl/persistent journal not present"
		return p, nil
	}
	cur, _ := scan.DirSize("/var/log/journal")
	size := ctx.Config.JournalMaxSize
	age := ctx.Config.JournalMaxAge
	if size != "" {
		p.Commands = append(p.Commands, Command{
			Label:   fmt.Sprintf("vacuum journal to %s", size),
			Argv:    []string{"journalctl", "--vacuum-size=" + size},
			Reclaim: -1,
		})
	}
	if age != "" {
		p.Commands = append(p.Commands, Command{
			Label:   fmt.Sprintf("purge journal older than %s", age),
			Argv:    []string{"journalctl", "--vacuum-time=" + age},
			Reclaim: -1,
		})
	}
	p.Note = fmt.Sprintf("current journal size: %s", humanize.Bytes(cur))
	return p, nil
}
