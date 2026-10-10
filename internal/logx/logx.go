// Package logx is a tiny leveled logger that honors --silent and --verbose.
package logx

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Level controls how chatty the logger is.
type Level int

const (
	// LevelSilent suppresses everything except Error (which still goes to stderr).
	LevelSilent Level = iota
	// LevelNormal prints Info, Warn, Error.
	LevelNormal
	// LevelVerbose prints Debug and everything above.
	LevelVerbose
)

// Logger is a minimal leveled logger.
type Logger struct {
	level Level
	out   io.Writer
	err   io.Writer
	ts    bool
}

// New builds a Logger. When silent is set it wins over verbose.
func New(silent, verbose bool) *Logger {
	lvl := LevelNormal
	switch {
	case silent:
		lvl = LevelSilent
	case verbose:
		lvl = LevelVerbose
	}
	return &Logger{level: lvl, out: os.Stdout, err: os.Stderr, ts: verbose}
}

// SetWriters lets tests or the TUI redirect output.
func (l *Logger) SetWriters(out, err io.Writer) { l.out, l.err = out, err }

// Level returns the current level.
func (l *Logger) Level() Level { return l.level }

func (l *Logger) stamp() string {
	if !l.ts {
		return ""
	}
	return time.Now().Format("15:04:05.000 ")
}

// Debug prints only in verbose mode.
func (l *Logger) Debug(format string, a ...any) {
	if l.level >= LevelVerbose {
		fmt.Fprintf(l.out, l.stamp()+"[debug] "+format+"\n", a...)
	}
}

// Info prints in normal and verbose mode.
func (l *Logger) Info(format string, a ...any) {
	if l.level >= LevelNormal {
		fmt.Fprintf(l.out, l.stamp()+format+"\n", a...)
	}
}

// Warn prints in normal and verbose mode.
func (l *Logger) Warn(format string, a ...any) {
	if l.level >= LevelNormal {
		fmt.Fprintf(l.err, l.stamp()+"[warn] "+format+"\n", a...)
	}
}

// Error always prints (even when silent) to stderr.
func (l *Logger) Error(format string, a ...any) {
	fmt.Fprintf(l.err, l.stamp()+"[error] "+format+"\n", a...)
}
