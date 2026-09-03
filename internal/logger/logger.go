// Package logger provides a small leveled-logging helper used across the
// SWAGGYMUSIC GitHub Controller Bot.
//
// The logger is intentionally minimal: it writes structured key=value lines
// to stdout (or any io.Writer) and supports debug/info/warn/error levels. It
// deliberately avoids third-party dependencies and never prints secret
// material — callers are responsible for not passing secrets into log calls.
package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level is the severity of a log entry.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns a human-readable label for the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger is a small leveled logger safe for concurrent use.
type Logger struct {
	mu     sync.Mutex
	out    io.Writer
	level  Level
	prefix string
}

// New creates a logger writing to w with the given minimum level.
func New(out io.Writer, level Level, prefix string) *Logger {
	if out == nil {
		out = os.Stdout
	}
	return &Logger{out: out, level: level, prefix: prefix}
}

// Default returns a logger writing to stdout at the configured level.
func Default(level Level) *Logger {
	return New(os.Stdout, level, "swaggymusic")
}

// ParseLevel converts a string like "debug" or "INFO" into a Level.
// Unknown values default to LevelInfo.
func ParseLevel(s string) Level {
	switch s {
	case "debug", "DEBUG", "Debug":
		return LevelDebug
	case "info", "INFO", "Info":
		return LevelInfo
	case "warn", "WARN", "Warn", "warning", "WARNING":
		return LevelWarn
	case "error", "ERROR", "Error":
		return LevelError
	default:
		return LevelInfo
	}
}

func (l *Logger) log(level Level, format string, args ...any) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := time.Now().UTC().Format(time.RFC3339)
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(l.out, "%s [%s] %s %s\n", ts, level.String(), l.prefix, msg)
}

// Debugf logs at debug level.
func (l *Logger) Debugf(format string, args ...any) { l.log(LevelDebug, format, args...) }

// Infof logs at info level.
func (l *Logger) Infof(format string, args ...any) { l.log(LevelInfo, format, args...) }

// Warnf logs at warn level.
func (l *Logger) Warnf(format string, args ...any) { l.log(LevelWarn, format, args...) }

// Errorf logs at error level.
func (l *Logger) Errorf(format string, args ...any) { l.log(LevelError, format, args...) }

// Sync flushes any buffered output. Provided for API compatibility.
func (l *Logger) Sync() error { return nil }
