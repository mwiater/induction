// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BatchStatus is an exported type in this package's public data model.
type BatchStatus string

const (
	// BatchPending identifies a fixed value used internally by this package.
	BatchPending BatchStatus = "pending"
	// BatchValidating identifies a fixed value used internally by this package.
	BatchValidating BatchStatus = "validating"
	// BatchRunning identifies a fixed value used internally by this package.
	BatchRunning BatchStatus = "running"
	// BatchCompleted identifies a fixed value used internally by this package.
	BatchCompleted BatchStatus = "completed"
	// BatchCompletedWithErrors identifies a fixed value used internally by this package.
	BatchCompletedWithErrors BatchStatus = "completed_with_errors"
)

// BatchItemStatus is an exported type in this package's public data model.
type BatchItemStatus string

const (
	// BatchItemPending identifies a fixed value used internally by this package.
	BatchItemPending BatchItemStatus = "pending"
	// BatchItemInvalid identifies a fixed value used internally by this package.
	BatchItemInvalid BatchItemStatus = "invalid"
	// BatchItemRunning identifies a fixed value used internally by this package.
	BatchItemRunning BatchItemStatus = "running"
	// BatchItemCompleted identifies a fixed value used internally by this package.
	BatchItemCompleted BatchItemStatus = "completed"
	// BatchItemFailed identifies a fixed value used internally by this package.
	BatchItemFailed BatchItemStatus = "failed"
)

// Batch is durable orchestration state. Detailed inference remains in the
// child pipeline session/run identified by PipelineRunID.
// Batch is an exported type in this package's public data model.
type Batch struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// Pipeline stores the value represented by this member.
	Pipeline string `json:"pipeline"`
	// ConfigIdentity stores the value represented by this member.
	ConfigIdentity string `json:"config_identity"`
	// Status stores the value represented by this member.
	Status BatchStatus `json:"status"`
	// CreatedAt stores the value represented by this member.
	CreatedAt time.Time `json:"created_at"`
	// StartedAt stores the value represented by this member.
	StartedAt time.Time `json:"started_at,omitempty"`
	// CompletedAt stores the value represented by this member.
	CompletedAt time.Time `json:"completed_at,omitempty"`
	// Items stores the value represented by this member.
	Items []BatchItem `json:"items"`
}

// BatchItem is an exported type in this package's public data model.
type BatchItem struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// BatchID stores the value represented by this member.
	BatchID string `json:"batch_id,omitempty"`
	// Status stores the value represented by this member.
	Status BatchItemStatus `json:"status"`
	// InputSet stores the value represented by this member.
	InputSet InputSet `json:"inputs"`
	// PipelineRunID stores the value represented by this member.
	PipelineRunID string `json:"pipeline_run_id,omitempty"`
	// Error stores the value represented by this member.
	Error string `json:"error,omitempty"`
	// CreatedAt stores the value represented by this member.
	CreatedAt time.Time `json:"created_at"`
	// StartedAt stores the value represented by this member.
	StartedAt time.Time `json:"started_at,omitempty"`
	// CompletedAt stores the value represented by this member.
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// BatchSummary is an exported type in this package's public data model.
type BatchSummary struct {
	// Pending stores the value represented by this member.
	// Failed stores the value represented by this member.
	// Completed stores the value represented by this member.
	// Invalid stores the value represented by this member.
	// Valid stores the value represented by this member.
	// Total stores the value represented by this member.
	Total, Valid, Invalid, Completed, Failed, Pending int
}

// Summary is an exported operation provided by this package.
func (b *Batch) Summary() BatchSummary {
	var s BatchSummary
	s.Total = len(b.Items)
	for _, item := range b.Items {
		switch item.Status {
		case BatchItemInvalid:
			s.Invalid++
		case BatchItemCompleted:
			s.Completed++
		case BatchItemFailed:
			s.Failed++
		default:
			s.Pending++
		}
	}
	s.Valid = s.Total - s.Invalid
	return s
}

// BatchExecutor is an exported type in this package's public data model.
type BatchExecutor func(context.Context, *Pipeline, InputSet, *BatchItem) error

// RunBatch validates and sequentially executes each independent input set.
// The executor is intentionally injected so orchestration can be tested
// without an inference server and future execution policies can be added.
// Additional context: A failed batch is an explicit retry boundary. Recreate every item so a rerun regenerates all pipeline artifacts, including artifacts from items that happened to succeed during the previous attempt.
func RunBatch(ctx context.Context, pipeline *Pipeline, directory string, executor BatchExecutor, progress func(*Batch)) (*Batch, error) {
	if pipeline == nil || pipeline.Batch == nil {
		return nil, fmt.Errorf("pipeline batch configuration is required")
	}
	if executor == nil {
		return nil, fmt.Errorf("batch executor is required")
	}
	if err := pipeline.Validate(); err != nil {
		return nil, err
	}
	if directory == "" {
		directory = ".batches"
	}
	identity := batchIdentity(pipeline)
	path := filepath.Join(directory, identity+".json")
	batch, err := loadBatch(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if batch == nil {
		now := time.Now().UTC()
		batch = &Batch{ID: identity, Pipeline: pipeline.Name, ConfigIdentity: identity, Status: BatchPending, CreatedAt: now}
		for _, config := range pipeline.Batch.Items {
			batch.Items = append(batch.Items, BatchItem{ID: config.ID, BatchID: identity, Status: BatchItemPending, InputSet: config.InputSet, CreatedAt: now})
		}
	} else {
		if batch.ConfigIdentity != identity || batch.Pipeline != pipeline.Name {
			return nil, fmt.Errorf("persisted batch %q does not match the current pipeline configuration", batch.ID)
		}
		if batch.Status == BatchCompletedWithErrors {
			now := time.Now().UTC()
			batch.Status = BatchPending
			batch.StartedAt = time.Time{}
			batch.CompletedAt = time.Time{}
			batch.Items = batch.Items[:0]
			for _, config := range pipeline.Batch.Items {
				batch.Items = append(batch.Items, BatchItem{ID: config.ID, BatchID: identity, Status: BatchItemPending, InputSet: config.InputSet, CreatedAt: now})
			}
		}
	}

	batch.Status = BatchValidating
	if err := saveBatch(directory, batch); err != nil {
		return nil, err
	}
	if progress != nil {
		progress(batch)
	}
	for i := range batch.Items {
		item := &batch.Items[i]
		if item.Status == BatchItemCompleted {
			continue
		}
		if item.Status != BatchItemRunning {
			if err := item.InputSet.Validate("batch item " + item.ID); err != nil {
				item.Status, item.Error = BatchItemInvalid, err.Error()
				_ = saveBatch(directory, batch)
				continue
			}
		}
	}
	batch.Status = BatchRunning
	if batch.StartedAt.IsZero() {
		batch.StartedAt = time.Now().UTC()
	}
	if err := saveBatch(directory, batch); err != nil {
		return nil, err
	}
	if progress != nil {
		progress(batch)
	}
	for i := range batch.Items {
		item := &batch.Items[i]
		if item.Status == BatchItemCompleted || item.Status == BatchItemInvalid {
			continue
		}
		item.Status, item.Error, item.StartedAt = BatchItemRunning, "", time.Now().UTC()
		if err := saveBatch(directory, batch); err != nil {
			return nil, err
		}
		if progress != nil {
			progress(batch)
		}
		if err := executor(ctx, pipeline, item.InputSet, item); err != nil {
			item.Status, item.Error = BatchItemFailed, err.Error()
		} else {
			item.Status, item.CompletedAt = BatchItemCompleted, time.Now().UTC()
		}
		if err := saveBatch(directory, batch); err != nil {
			return nil, err
		}
		if progress != nil {
			progress(batch)
		}
		if pipeline.Batch.StopOnError && item.Status == BatchItemFailed {
			break
		}
	}
	summary := batch.Summary()
	if summary.Failed > 0 || summary.Invalid > 0 {
		batch.Status = BatchCompletedWithErrors
	} else {
		batch.Status = BatchCompleted
	}
	batch.CompletedAt = time.Now().UTC()
	if err := saveBatch(directory, batch); err != nil {
		return nil, err
	}
	if progress != nil {
		progress(batch)
	}
	return batch, nil
}

// batchIdentity performs the internal operation described by its signature.
func batchIdentity(p *Pipeline) string {
	data, _ := json.Marshal(struct {
		Name  string
		Steps []PipelineStep
		Batch *BatchConfig
	}{p.Name, p.Steps, p.Batch})
	sum := sha256.Sum256(data)
	return "batch-" + hex.EncodeToString(sum[:])[:16]
}

// loadBatch performs the internal operation described by its signature.
func loadBatch(path string) (*Batch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b Batch
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("decode batch: %w", err)
	}
	return &b, nil
}

// saveBatch performs the internal operation described by its signature.
func saveBatch(directory string, batch *Batch) error {
	if batch == nil {
		return fmt.Errorf("batch is nil")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create batch directory: %w", err)
	}
	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(directory, ".batch-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(directory, batch.ID+".json"))
}

// allPaths performs the internal operation described by its signature.
func (s InputSet) allPaths() []string {
	return append(append(append([]string{}, s.Files...), s.Images...), s.Documents...)
}

// String is an exported operation provided by this package.
func (s InputSet) String() string { return strings.Join(s.allPaths(), ", ") }
