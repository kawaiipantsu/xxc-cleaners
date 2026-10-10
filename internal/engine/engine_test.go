package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kawaiipantsu/xxc-cleaners/internal/cleaners"
	"github.com/kawaiipantsu/xxc-cleaners/internal/config"
	"github.com/kawaiipantsu/xxc-cleaners/internal/logx"
	"github.com/kawaiipantsu/xxc-cleaners/internal/protect"
	"github.com/kawaiipantsu/xxc-cleaners/internal/scan"
)

func TestExecuteDeletesAndRespectsProtection(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "junk.log")
	keep := filepath.Join(dir, "keep", "important.db")
	if err := os.WriteFile(victim, []byte("delete me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.DryRun = false
	log := logx.New(true, false)
	prot := protect.New([]string{filepath.Dir(keep)}) // protect the keep/ dir
	eng := New(cfg, prot, log)

	plan := &cleaners.Plan{
		Cleaner: "test",
		Findings: []scan.Finding{
			{Path: victim, Size: 9},
			{Path: keep, Size: 8}, // protected — must be refused
		},
	}
	res := eng.Execute(plan)

	if _, err := os.Stat(victim); !os.IsNotExist(err) {
		t.Errorf("victim should have been deleted, stat err=%v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("protected file must survive, stat err=%v", err)
	}
	if res.FilesDeleted != 1 {
		t.Errorf("FilesDeleted = %d, want 1", res.FilesDeleted)
	}
	if res.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (the refused protected delete)", res.Errors)
	}
}

func TestDryRunDeletesNothing(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x")
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.DryRun = true
	eng := New(cfg, protect.New(nil), logx.New(true, false))
	res := eng.Execute(&cleaners.Plan{Findings: []scan.Finding{{Path: f, Size: 2}}})
	if _, err := os.Stat(f); err != nil {
		t.Errorf("dry-run must not delete; stat err=%v", err)
	}
	if res.FilesDeleted != 1 || res.BytesFreed != 2 {
		t.Errorf("dry-run should count would-free: got files=%d bytes=%d", res.FilesDeleted, res.BytesFreed)
	}
}

func TestSelectOnlyExcept(t *testing.T) {
	cfg := config.Default()
	eng := New(cfg, protect.New(nil), logx.New(true, false))

	only := eng.Select([]string{"tmp"}, nil)
	if len(only) != 1 || only[0].Key() != "tmp" {
		t.Errorf("Select only=tmp returned %d cleaners", len(only))
	}

	all := eng.Select(nil, nil)
	except := eng.Select(nil, []string{"tmp"})
	if len(except) != len(all)-1 {
		t.Errorf("Select except=tmp returned %d, want %d", len(except), len(all)-1)
	}
}
