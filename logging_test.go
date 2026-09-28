package induction

import (
	"context"
	"log"
	"testing"
)

func TestNewConfiguredLoggerWritesApplicationLog(t *testing.T) {
	logger := NewConfiguredLogger(LogConfig{Prefix: "app: ", TruncateOnRun: true})
	if logger == nil {
		t.Fatal("expected logger")
	}
	logger.Printf("diagnostic")
}

func TestNewConfiguredLoggerIgnoresConsoleSetting(t *testing.T) {
	logger := NewConfiguredLogger(LogConfig{Console: true})
	if logger == nil {
		t.Fatal("expected logger")
	}
}

func TestNewClientDefaultLoggerIsDiscard(t *testing.T) {
	client := NewClient(context.TODO(), "")
	if client.opts.logger == nil {
		t.Fatal("expected default logger")
	}
	if _, ok := client.opts.logger.(*log.Logger); !ok {
		t.Fatalf("unexpected logger type %T", client.opts.logger)
	}
}
