package induction

import (
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfiguredLoggerDoesNotCreateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.log")
	logger := NewConfiguredLogger(LogConfig{Path: path, Prefix: "app: ", TruncateOnRun: true})
	logger.Printf("diagnostic")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("configured logger created %q: %v", path, err)
	}
}

func TestNewConfiguredLoggerConsoleUsesStderrWithoutFile(t *testing.T) {
	logger := NewConfiguredLogger(LogConfig{Path: filepath.Join(t.TempDir(), "application.log"), Console: true})
	if logger == nil {
		t.Fatal("expected logger")
	}
}

func TestNewClientDefaultLoggerIsDiscard(t *testing.T) {
	client := NewClient(nil, "")
	if client.opts.logger == nil {
		t.Fatal("expected default logger")
	}
	if _, ok := client.opts.logger.(*log.Logger); !ok {
		t.Fatalf("unexpected logger type %T", client.opts.logger)
	}
}
