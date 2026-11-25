// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"log"
	"os"
	"sync"
)

// applicationLogPath identifies a fixed value used internally by this package.
const applicationLogPath = "induction.log"

// configuredApplicationLogger holds package-level state used internally by this package.
var configuredApplicationLogger struct {
	sync.Once
	logger Logger
}

// NewConfiguredLogger creates a logger described by the application config.
// All application diagnostics are written to induction.log. The first logger
// created in a process honors TruncateOnRun; later logger instances append to
// the same file without truncating it again.
// Additional context: Keep diagnostics visible if the required log cannot be opened.
func NewConfiguredLogger(config LogConfig) Logger {
	configuredApplicationLogger.Do(func() {
		flags := os.O_CREATE | os.O_WRONLY
		if config.TruncateOnRun {
			flags |= os.O_TRUNC
		} else {
			flags |= os.O_APPEND
		}
		file, err := os.OpenFile(applicationLogPath, flags, 0o600)
		if err != nil {

			configuredApplicationLogger.logger = log.New(os.Stderr, config.Prefix, logFlags(config))
			return
		}
		configuredApplicationLogger.logger = log.New(file, config.Prefix, logFlags(config))
	})
	return configuredApplicationLogger.logger
}

// configuredLogger performs the internal operation described by its signature.
func configuredLogger(config LogConfig) Logger { return NewConfiguredLogger(config) }

// configuredUILogger performs the internal operation described by its signature.
func configuredUILogger(config LogConfig) Logger {
	return NewConfiguredLogger(config)
}

// logFlags performs the internal operation described by its signature.
func logFlags(config LogConfig) int {
	flags := log.Ldate | log.Ltime
	if config.Microseconds {
		flags |= log.Lmicroseconds
	}
	return flags
}
