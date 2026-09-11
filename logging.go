package induction

import (
	"io"
	"log"
	"os"
)

// NewConfiguredLogger creates a logger described by the application config.
// File destinations are intentionally ignored: application diagnostics must
// not create application.log or any other persistent application log file.
func NewConfiguredLogger(config LogConfig) Logger {
	if !config.Console {
		return log.New(io.Discard, config.Prefix, logFlags(config))
	}
	return log.New(os.Stderr, config.Prefix, logFlags(config))
}

func configuredLogger(config LogConfig) Logger { return NewConfiguredLogger(config) }

func configuredUILogger(config LogConfig) Logger {
	config.Console = false
	return NewConfiguredLogger(config)
}

func logFlags(config LogConfig) int {
	flags := log.Ldate | log.Ltime
	if config.Microseconds {
		flags |= log.Lmicroseconds
	}
	return flags
}
