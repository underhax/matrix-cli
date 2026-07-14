// Package logger provides logging utilities.
package logger

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/rs/zerolog"
)

// Logger is an alias for zerolog.Logger to avoid importing zerolog everywhere.
type Logger = zerolog.Logger

// Nop returns a disabled logger.
func Nop() Logger {
	return zerolog.Nop()
}

// LevelFlag implements flag.Value and flag.boolFlag to allow a flag to act
// as both a boolean and a valued flag (e.g., --debug or --debug=2).
type LevelFlag struct {
	Level *int
}

// Set parses the flag value.
func (f *LevelFlag) Set(s string) error {
	if s == "true" {
		*f.Level = 1
		return nil
	}
	if s == "false" {
		*f.Level = 0
		return nil
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("parse level: %w", err)
	}
	*f.Level = val
	return nil
}

// String returns the string representation of the current level.
func (f *LevelFlag) String() string {
	if f.Level == nil {
		return "0"
	}
	return strconv.Itoa(*f.Level)
}

// IsBoolFlag tells the flag package that this flag can be used without an argument.
func (f *LevelFlag) IsBoolFlag() bool {
	return true
}

// Setup initializes a Logger based on the provided level.
// If level is 0, it returns a Nop logger.
func Setup(level int, out io.Writer) Logger {
	if level == 0 {
		return Nop()
	}

	consoleWriter := zerolog.ConsoleWriter{
		Out:        out,
		TimeFormat: time.RFC3339,
	}

	logger := zerolog.New(consoleWriter).With().Timestamp().Logger()

	if level >= 2 {
		logger = logger.Level(zerolog.TraceLevel)
	} else {
		logger = logger.Level(zerolog.DebugLevel)
	}

	return logger
}
