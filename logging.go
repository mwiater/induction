package induction

import (
	"log"
	"os"
	"sync"
)

const applicationLogPath = "induction.log"

var configuredApplicationLogger struct {
	sync.Once
	logger Logger
}

// NewConfiguredLogger creates a logger described by the application config.
// All application diagnostics are written to induction.log. The first logger
// created in a process honors TruncateOnRun; later logger instances append to
// the same file without truncating it again.
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
			// Keep diagnostics visible if the required log cannot be opened.
			configuredApplicationLogger.logger = log.New(os.Stderr, config.Prefix, logFlags(config))
			return
		}
		configuredApplicationLogger.logger = log.New(file, config.Prefix, logFlags(config))
	})
	return configuredApplicationLogger.logger
}

func configuredLogger(config LogConfig) Logger { return NewConfiguredLogger(config) }

func configuredUILogger(config LogConfig) Logger {
	return NewConfiguredLogger(config)
}

func logFlags(config LogConfig) int {
	flags := log.Ldate | log.Ltime
	if config.Microseconds {
		flags |= log.Lmicroseconds
	}
	return flags
}
