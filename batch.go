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

type BatchStatus string

const (
	BatchPending             BatchStatus = "pending"
	BatchValidating          BatchStatus = "validating"
	BatchRunning             BatchStatus = "running"
	BatchCompleted           BatchStatus = "completed"
	BatchCompletedWithErrors BatchStatus = "completed_with_errors"
)

type BatchItemStatus string

const (
	BatchItemPending   BatchItemStatus = "pending"
	BatchItemInvalid   BatchItemStatus = "invalid"
	BatchItemRunning   BatchItemStatus = "running"
	BatchItemCompleted BatchItemStatus = "completed"
	BatchItemFailed    BatchItemStatus = "failed"
)

// Batch is durable orchestration state. Detailed inference remains in the
// child pipeline session/run identified by PipelineRunID.
type Batch struct {
	ID             string      `json:"id"`
	Pipeline       string      `json:"pipeline"`
	ConfigIdentity string      `json:"config_identity"`
	Status         BatchStatus `json:"status"`
	CreatedAt      time.Time   `json:"created_at"`
	StartedAt      time.Time   `json:"started_at,omitempty"`
	CompletedAt    time.Time   `json:"completed_at,omitempty"`
	Items          []BatchItem `json:"items"`
}

type BatchItem struct {
	ID            string          `json:"id"`
	BatchID       string          `json:"batch_id,omitempty"`
	Status        BatchItemStatus `json:"status"`
	InputSet      InputSet        `json:"inputs"`
	PipelineRunID string          `json:"pipeline_run_id,omitempty"`
	Error         string          `json:"error,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	StartedAt     time.Time       `json:"started_at,omitempty"`
	CompletedAt   time.Time       `json:"completed_at,omitempty"`
}

type BatchSummary struct {
	Total, Valid, Invalid, Completed, Failed, Pending int
}

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

type BatchExecutor func(context.Context, *Pipeline, InputSet, *BatchItem) error

// RunBatch validates and sequentially executes each independent input set.
// The executor is intentionally injected so orchestration can be tested
// without an inference server and future execution policies can be added.
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
	} else if batch.ConfigIdentity != identity || batch.Pipeline != pipeline.Name {
		return nil, fmt.Errorf("persisted batch %q does not match the current pipeline configuration", batch.ID)
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

func batchIdentity(p *Pipeline) string {
	data, _ := json.Marshal(struct {
		Name  string
		Steps []PipelineStep
		Batch *BatchConfig
	}{p.Name, p.Steps, p.Batch})
	sum := sha256.Sum256(data)
	return "batch-" + hex.EncodeToString(sum[:])[:16]
}

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
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(directory, batch.ID+".json"))
}

func (s InputSet) allPaths() []string {
	return append(append(append([]string{}, s.Files...), s.Images...), s.Documents...)
}
func (s InputSet) String() string { return strings.Join(s.allPaths(), ", ") }
