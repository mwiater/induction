package modelmanager

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// InteractionLogPath is the local audit-log filename used by the model manager.
const InteractionLogPath = "induction-model-manager.log"

var interactionLogMu sync.Mutex

// LogInteraction appends a sanitized, timestamped model-manager event.
func LogInteraction(event string, fields ...string) error {
	interactionLogMu.Lock()
	defer interactionLogMu.Unlock()
	file, err := os.OpenFile(InteractionLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open model-manager interaction log: %w", err)
	}
	defer file.Close()
	clean := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.ReplaceAll(field, "\n", " ")
		field = strings.ReplaceAll(field, "\r", " ")
		clean = append(clean, field)
	}
	line := time.Now().UTC().Format(time.RFC3339Nano) + " event=" + event
	if len(clean) > 0 {
		line += " " + strings.Join(clean, " ")
	}
	if _, err = file.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("write model-manager interaction log: %w", err)
	}
	return nil
}

func recordInteraction(event string, fields ...string) { _ = LogInteraction(event, fields...) }
