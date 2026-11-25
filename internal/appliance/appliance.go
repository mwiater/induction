// Package appliance provides the implementation for this subsystem.
package appliance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mwiater/induction"
	"gopkg.in/yaml.v3"
)

const (
	// applianceName identifies a fixed value used internally by this package.
	applianceName = "induction-image-meta"
	// applianceVersion identifies a fixed value used internally by this package.
	applianceVersion = "1.1.0"
	// logName identifies a fixed value used internally by this package.
	logName = "induction-image-meta.log"
)

const (
	// ExitOK identifies a fixed value used internally by this package.
	ExitOK = 0
	// ExitArguments identifies a fixed value used internally by this package.
	ExitArguments = 10
	// ExitEmbedded identifies a fixed value used internally by this package.
	ExitEmbedded = 11
	// ExitSandbox identifies a fixed value used internally by this package.
	ExitSandbox = 12
	// ExitLog identifies a fixed value used internally by this package.
	ExitLog = 13
	// ExitPreflight identifies a fixed value used internally by this package.
	ExitPreflight = 14
	// ExitPartial identifies a fixed value used internally by this package.
	ExitPartial = 15
	// ExitIntegrity identifies a fixed value used internally by this package.
	ExitIntegrity = 16
)

// Assets is an exported type in this package's public data model.
type Assets struct { // Config contains the embedded appliance configuration.
	// Pipeline stores the value represented by this member.
	Pipeline, Config []byte
}

// applianceConfig is an internal data type used by this package.
type applianceConfig struct {
	// Server stores the value represented by this member.
	Server string `yaml:"server"`
	// Timeout stores the value represented by this member.
	Timeout induction.Duration `yaml:"timeout"`
	// PollInterval stores the value represented by this member.
	PollInterval induction.Duration `yaml:"pollInterval"`
	// LoadWaitInterval stores the value represented by this member.
	LoadWaitInterval induction.Duration `yaml:"loadWaitInterval"`
	// SidebarWidth stores the value represented by this member.
	SidebarWidth int `yaml:"sidebarWidth"`
	// MCPServers stores the value represented by this member.
	MCPServers []map[string]any `yaml:"mcpServers"`
	// Log stores the value represented by this member.
	Log map[string]any `yaml:"log"`
}

// manifest is an internal data type used by this package.
type manifest struct {
	// Name stores the value represented by this member.
	Name string `json:"name"`
	// Version stores the value represented by this member.
	Version string `json:"version"`
	// PipelineSHA256 stores the value represented by this member.
	PipelineSHA256 string `json:"pipeline_sha256"`
	// ConfigSHA256 stores the value represented by this member.
	ConfigSHA256 string `json:"config_sha256"`
	// SchemaSHA256 stores the value represented by this member.
	SchemaSHA256 string `json:"schema_sha256"`
	// Models stores the value represented by this member.
	Models []string `json:"models"`
}

// journal is an internal data type used by this package.
type journal struct {
	// RunID stores the value represented by this member.
	RunID string
	// Workspace stores the value represented by this member.
	Workspace string
	// WorkspaceDir stores the value represented by this member.
	WorkspaceDir *os.File
	// Log stores the value represented by this member.
	Log *os.File
	// locked stores the value represented by this member.
	locked bool
	// manifest stores the value represented by this member.
	manifest manifest
	// mu stores the value represented by this member.
	mu sync.Mutex
}

// event is an internal data type used by this package.
type event struct {
	// Timestamp stores the value represented by this member.
	Timestamp string `json:"timestamp"`
	// Event stores the value represented by this member.
	Event string `json:"event"`
	// RunID stores the value represented by this member.
	RunID string `json:"run_id"`
	// Appliance stores the value represented by this member.
	Appliance string `json:"appliance"`
	// Version stores the value represented by this member.
	Version string `json:"version"`
	// PipelineSHA256 stores the value represented by this member.
	PipelineSHA256 string `json:"pipeline_sha256"`
	// ConfigSHA256 stores the value represented by this member.
	ConfigSHA256 string `json:"config_sha256,omitempty"`
	// SchemaSHA256 stores the value represented by this member.
	SchemaSHA256 string `json:"schema_sha256,omitempty"`
	// Image stores the value represented by this member.
	Image string `json:"image,omitempty"`
	// Step stores the value represented by this member.
	Step string `json:"step,omitempty"`
	// StepNumber stores the value represented by this member.
	StepNumber int `json:"step_number,omitempty"`
	// StepTotal stores the value represented by this member.
	StepTotal int `json:"step_total,omitempty"`
	// Model stores the value represented by this member.
	Model string `json:"model,omitempty"`
	// Outcome stores the value represented by this member.
	Outcome string `json:"outcome,omitempty"`
	// Message stores the value represented by this member.
	Message string `json:"message,omitempty"`
	// ErrorCategory stores the value represented by this member.
	ErrorCategory string `json:"error_category,omitempty"`
	// Error stores the value represented by this member.
	Error string `json:"error,omitempty"`
	// DurationMS stores the value represented by this member.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// Stage stores the value represented by this member.
	Stage string `json:"stage,omitempty"`
	// Staging stores the value represented by this member.
	Staging string `json:"staging,omitempty"`
	// Device stores the value represented by this member.
	Device uint64 `json:"device,omitempty"`
	// Inode stores the value represented by this member.
	Inode uint64 `json:"inode,omitempty"`
	// Size stores the value represented by this member.
	Size int64 `json:"size,omitempty"`
}

// Run is an exported operation provided by this package.
func Run(args []string, assets Assets) int {
	workspace, err := os.Getwd()
	if err != nil {
		return ExitIntegrity
	}
	j, err := openJournal(workspace, assets)
	if err != nil {
		return ExitLog
	}
	defer j.close()
	if len(args) != 0 {
		_ = j.write(event{Event: "arguments_rejected", Outcome: "rejected", ErrorCategory: "arguments"})
		_ = j.write(event{Event: "run_end", Outcome: "rejected", Message: "arguments were rejected; the appliance accepts zero arguments only"})
		return ExitArguments
	}
	if err := j.write(event{Event: "run_start", Outcome: "started"}); err != nil {
		return ExitLog
	}
	recoverAmbiguousStages(j)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()
	pipeline, cfg, man, err := parseAssets(assets)
	if err != nil {
		preflightFailure(j, start, "embedded", err)
		return ExitEmbedded
	}
	j.manifest = man
	_ = j.write(event{Event: "preflight_check", Outcome: "passed", Message: fmt.Sprintf("embedded assets valid; pipeline has %d required steps; models=%s", len(pipeline.Steps), strings.Join(man.Models, ","))})
	if err := installConfinement(j.workspaceFD()); err != nil {
		preflightFailure(j, start, "sandbox", err)
		return ExitSandbox
	}
	_ = j.write(event{Event: "preflight_check", Outcome: "passed", Message: "Landlock filesystem confinement installed for the startup workspace"})
	if err := preflight(ctx, pipeline, cfg); err != nil {
		preflightFailure(j, start, "server_or_model", err)
		return ExitPreflight
	}
	_ = j.write(event{Event: "preflight_check", Outcome: "passed", Message: "endpoint reachable; all pipeline models available with verified image input capability"})
	images, discoveryFailures, err := discover(workspace, j)
	if err != nil {
		preflightFailure(j, start, "filesystem", err)
		return ExitIntegrity
	}
	_ = j.write(event{Event: "discovery_summary", Outcome: "completed", Message: fmt.Sprintf("discovered %d eligible image(s); %d skipped or rejected", len(images), discoveryFailures)})
	client := induction.NewClient(ctx, cfg.Server, induction.WithHTTPClient(applianceHTTPClient(20*time.Minute)), induction.WithPollInterval(time.Duration(cfg.PollInterval)), induction.WithLoadWaitInterval(time.Duration(cfg.LoadWaitInterval)), induction.WithLogger(discardLogger{}))
	failures := discoveryFailures
	systemicFailure := false
	for _, image := range images {
		if ctx.Err() != nil {
			systemicFailure = true
			break
		}
		if image.conflict {
			continue
		}
		if image.skip {
			_ = j.write(event{Event: "image_skipped_valid", Image: image.name, Outcome: "skipped"})
			continue
		}
		started := time.Now()
		if !inputStillUnchanged(j, image) {
			failures++
			_ = j.write(event{Event: "image_failed", Image: image.name, Outcome: "failed", ErrorCategory: "input_changed", Message: "input changed after discovery; no output was published"})
			continue
		}
		_ = j.write(event{Event: "image_started", Image: image.name, Outcome: "processing"})
		input := induction.InputSet{Images: []string{image.path}}
		result, runErr := induction.RunPipelineItemWithObserver(ctx, client, pipeline, input, func(progress induction.PipelineStepEvent) {
			if progress.Started {
				_ = j.write(event{Event: "pipeline_step_started", Image: image.name, Step: progress.Name, StepNumber: progress.Index + 1, StepTotal: progress.Total, Model: progress.Model, Outcome: "started", Message: fmt.Sprintf("starting step %d/%d", progress.Index+1, progress.Total)})
				return
			}
			outcome := "completed"
			if progress.Err != nil {
				outcome = "failed"
			}
			_ = j.write(event{Event: "pipeline_step_completed", Image: image.name, Step: progress.Name, StepNumber: progress.Index + 1, StepTotal: progress.Total, Model: progress.Model, Outcome: outcome, DurationMS: progress.Duration.Milliseconds(), ErrorCategory: errorCategory(progress.Err), Error: detailedError(progress.Err), Message: fmt.Sprintf("step %d/%d %s in %s", progress.Index+1, progress.Total, outcome, progress.Duration.Round(time.Millisecond))})
		})
		if runErr == nil {
			runErr = validateFinal(result.Outputs["return-json"])
		}
		if runErr != nil {
			failures++
			_ = j.write(event{Event: "image_failed", Image: image.name, Outcome: "failed", ErrorCategory: errorCategory(runErr), Error: detailedError(runErr), DurationMS: time.Since(started).Milliseconds(), Message: "image processing failed; no output was published"})
			if isSystemicFailure(runErr) {
				systemicFailure = true
				break
			}
			continue
		}
		if !inputStillUnchanged(j, image) {
			failures++
			_ = j.write(event{Event: "image_failed", Image: image.name, Outcome: "failed", ErrorCategory: "input_changed", Error: "source changed during inference; no output was published", DurationMS: time.Since(started).Milliseconds()})
			continue
		}
		data := normalizeJSON(result.Outputs["return-json"])
		if err := publish(j, image.output, data, image.name); err != nil {
			failures++
			_ = j.write(event{Event: "image_failed", Image: image.name, Outcome: "failed", ErrorCategory: "publisher", Error: detailedError(err), DurationMS: time.Since(started).Milliseconds(), Message: "output publication failed; existing files were preserved"})
			continue
		}
		_ = j.write(event{Event: "output_committed", Image: image.name, Outcome: "committed", DurationMS: time.Since(started).Milliseconds()})
	}
	if systemicFailure && failures == 0 {
		failures++
	}
	outcome := "completed"
	code := ExitOK
	if failures > 0 {
		outcome = "completed_with_errors"
		code = ExitPartial
	}
	_ = j.write(event{Event: "run_end", Outcome: outcome, DurationMS: time.Since(start).Milliseconds(), Message: fmt.Sprintf("run finished: %d eligible image(s), %d failure(s)", len(images), failures)})
	return code
}

// recoverAmbiguousStages performs the internal operation described by its signature.
func recoverAmbiguousStages(j *journal) {
	_ = rewindWorkspaceDir(j.WorkspaceDir)
	owned := make(map[string]event)
	if log, err := openWorkspaceFile(j.workspaceFD(), logName, os.O_RDONLY, 0); err == nil {
		data, _ := io.ReadAll(log)
		_ = log.Close()
		lines := strings.Split(string(data), "\n")
		for index, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var previous event
			if err := json.Unmarshal([]byte(line), &previous); err != nil {
				if index == len(lines)-1 {
					_ = j.write(event{Event: "recovery_ambiguous", Outcome: "truncated_log_line", Message: "ignored a truncated trailing log line"})
				}
				continue
			}
			switch previous.Event {
			case "stage_created":
				owned[previous.Staging] = previous
			case "output_committed", "recovery_retired":
				delete(owned, previous.Staging)
			}
		}
	}
	entries, err := j.WorkspaceDir.ReadDir(0)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".induction-image-meta.") || !strings.HasSuffix(entry.Name(), ".stage") {
			continue
		}
		info, err := statWorkspaceFile(j.workspaceFD(), entry.Name())
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || linkCount(info) != 1 {
			_ = j.write(event{Event: "recovery_ambiguous", Outcome: "left_in_place", Staging: entry.Name(), Message: "staging file could not be verified as a regular appliance-owned file"})
			continue
		}
		previous, ok := owned[entry.Name()]
		if ok && previous.Device == deviceInode(info) && previous.Inode == inodeNumber(info) && previous.Size == info.Size() {
			if err := removeWorkspaceFile(j.workspaceFD(), entry.Name()); err == nil {
				_ = j.write(event{Event: "recovery_retired", Outcome: "retired", Staging: entry.Name(), Device: previous.Device, Inode: previous.Inode, Size: previous.Size})
				continue
			}
		}
		_ = j.write(event{Event: "recovery_ambiguous", Outcome: "left_in_place", Staging: entry.Name(), Device: deviceInode(info), Inode: inodeNumber(info), Size: info.Size()})
	}
}

// preflightFailure performs the internal operation described by its signature.
func preflightFailure(j *journal, started time.Time, category string, err error) {
	_ = j.write(event{Event: "preflight_failed", Outcome: "failed", ErrorCategory: category, Error: detailedError(err), Message: "no image inference was started"})
	_ = j.write(event{Event: "run_end", Outcome: "failed", ErrorCategory: category, Error: detailedError(err), DurationMS: time.Since(started).Milliseconds(), Message: "run stopped during preflight; zero images were processed"})
}

// discovered is an internal data type used by this package.
type discovered struct {
	// output stores the value represented by this member.
	// path stores the value represented by this member.
	// name stores the value represented by this member.
	name, path, output string
	// conflict stores the value represented by this member.
	// skip stores the value represented by this member.
	skip, conflict bool
	// inode stores the value represented by this member.
	// device stores the value represented by this member.
	device, inode uint64
	// size stores the value represented by this member.
	size int64
}

// discover performs the internal operation described by its signature.
func discover(workspace string, j *journal) ([]discovered, int, error) {
	if err := rewindWorkspaceDir(j.WorkspaceDir); err != nil {
		return nil, 0, err
	}
	entries, err := j.WorkspaceDir.ReadDir(0)
	if err != nil {
		return nil, 0, err
	}
	var result []discovered
	conflicts := 0
	for _, entry := range entries {
		name := entry.Name()
		if name == logName {
			continue
		}
		info, err := statWorkspaceFile(j.workspaceFD(), name)
		if err != nil {
			if errors.Is(err, syscall.ELOOP) {
				continue
			}
			return nil, 0, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !supportedImage(name) {
			continue
		}
		path := filepath.Join(workspace, name)
		if err := validateImageAt(j.workspaceFD(), name); err != nil {
			_ = j.write(event{Event: "image_discovered", Image: name, Outcome: "invalid", ErrorCategory: "image", Error: safeError(err)})
			conflicts++
			continue
		}
		output := path + ".json"
		state, err := existingOutputAt(j.workspaceFD(), filepath.Base(output))
		if err != nil {
			return nil, 0, err
		}
		d := discovered{name: name, path: path, output: output, skip: state == outputValid, conflict: state == outputConflict, device: deviceInode(info), inode: inodeNumber(info), size: info.Size()}
		_ = j.write(event{Event: "image_discovered", Image: name, Outcome: map[bool]string{true: "conflict", false: "eligible"}[d.conflict]})
		if state == outputConflict {
			conflicts++
			_ = j.write(event{Event: "existing_output_conflict", Image: name, Outcome: "conflict"})
		}
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result, conflicts, nil
}

// inputStillUnchanged performs the internal operation described by its signature.
func inputStillUnchanged(j *journal, item discovered) bool {
	info, err := statWorkspaceFile(j.workspaceFD(), item.name)
	return err == nil && info.Mode().IsRegular() && deviceInode(info) == item.device && inodeNumber(info) == item.inode && info.Size() == item.size
}

// outputState is an internal data type used by this package.
type outputState int

const (
	// outputAbsent identifies a fixed value used internally by this package.
	outputAbsent outputState = iota
	// outputValid identifies a fixed value used internally by this package.
	outputValid
	// outputConflict identifies a fixed value used internally by this package.
	outputConflict
)

// existingOutput performs the internal operation described by its signature.
func existingOutput(path string) (outputState, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return outputAbsent, nil
	}
	if err != nil {
		return outputConflict, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || linkCount(info) != 1 {
		return outputConflict, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return outputConflict, nil
	}
	if validateFinal(data) == nil {
		return outputValid, nil
	}
	return outputConflict, nil
}

// existingOutputAt performs the internal operation described by its signature.
func existingOutputAt(dirfd int, name string) (outputState, error) {
	info, err := statWorkspaceFile(dirfd, name)
	if os.IsNotExist(err) {
		return outputAbsent, nil
	}
	if err != nil {
		return outputConflict, nil
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || linkCount(info) != 1 {
		return outputConflict, nil
	}
	data, _, err := readWorkspaceFile(dirfd, name, maxImageBytes)
	if err != nil {
		return outputConflict, nil
	}
	if validateFinal(data) == nil {
		return outputValid, nil
	}
	return outputConflict, nil
}

// validateFinal performs the internal operation described by its signature.
func validateFinal(raw []byte) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return validateJSONSchema(value, finalOutputSchema(), "$")
}

// finalOutputSchema is the appliance's simplified output contract. It mirrors
// the schema embedded in appliances/image-meta/pipeline.yaml and is kept
// here so publication validation does not depend on the model honoring its
// response_format request.
func finalOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subjects":              map[string]any{"type": "string"},
			"composition":           map[string]any{"type": "string"},
			"environmental_context": map[string]any{"type": "string"},
		},
		"required":             []any{"subjects", "composition", "environmental_context"},
		"additionalProperties": false,
	}
}

// validateJSONSchema performs the internal operation described by its signature.
func validateJSONSchema(value any, schema map[string]any, path string) error {
	if !matchesJSONType(value, schema["type"]) {
		return fmt.Errorf("%s has invalid type", path)
	}
	if number, ok := value.(float64); ok {
		if minimum, ok := schema["minimum"].(float64); ok && number < minimum {
			return fmt.Errorf("%s is below minimum", path)
		}
		if maximum, ok := schema["maximum"].(float64); ok && number > maximum {
			return fmt.Errorf("%s is above maximum", path)
		}
	}
	if integer, ok := value.(float64); ok && schema["type"] == "integer" && math.Trunc(integer) != integer {
		return fmt.Errorf("%s must be an integer", path)
	}
	if object, ok := value.(map[string]any); ok {
		properties, _ := schema["properties"].(map[string]any)
		for _, rawName := range schema["required"].([]any) {
			name := rawName.(string)
			if _, exists := object[name]; !exists {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
			for name := range object {
				if _, exists := properties[name]; !exists {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
			}
		}
		for name, child := range properties {
			if current, exists := object[name]; exists {
				if err := validateJSONSchema(current, child.(map[string]any), path+"."+name); err != nil {
					return err
				}
			}
		}
	}
	if array, ok := value.([]any); ok {
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for index, item := range array {
				if err := validateJSONSchema(item, itemSchema, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// matchesJSONType performs the internal operation described by its signature.
func matchesJSONType(value, want any) bool {
	types, ok := want.([]any)
	if !ok {
		types = []any{want}
	}
	for _, rawType := range types {
		switch rawType {
		case "object":
			if _, ok := value.(map[string]any); ok {
				return true
			}
		case "array":
			if _, ok := value.([]any); ok {
				return true
			}
		case "string":
			if _, ok := value.(string); ok {
				return true
			}
		case "number":
			if number, ok := value.(float64); ok && !math.IsNaN(number) && !math.IsInf(number, 0) {
				return true
			}
		case "integer":
			if number, ok := value.(float64); ok && !math.IsNaN(number) && !math.IsInf(number, 0) && math.Trunc(number) == number {
				return true
			}
		case "null":
			if value == nil {
				return true
			}
		}
	}
	return false
}

// normalizeJSON performs the internal operation described by its signature.
func normalizeJSON(raw []byte) []byte {
	var v map[string]any
	_ = json.Unmarshal(raw, &v)
	b, _ := json.Marshal(v)
	return append(b, '\n')
}

// validateFinalResult performs the internal operation described by its signature.
func validateFinalResult(raw json.RawMessage) error { return validateFinal(raw) }

// publish performs the internal operation described by its signature.
func publish(j *journal, target string, data []byte, image string) error {
	if err := validateFinal(data); err != nil {
		return fmt.Errorf("output schema validation failed: %w", err)
	}
	base := filepath.Base(target)
	if base != target && filepath.Dir(target) != j.Workspace {
		return fmt.Errorf("target escapes workspace")
	}
	for attempt := 0; attempt < 10; attempt++ {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return err
		}
		stage := ".induction-image-meta." + hex.EncodeToString(random[:]) + ".stage"
		f, err := openWorkspaceFile(j.workspaceFD(), stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
		if err != nil {
			continue
		}
		var written int
		written, err = f.Write(data)
		if err == nil && written != len(data) {
			err = io.ErrShortWrite
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = removeWorkspaceFile(j.workspaceFD(), stage)
			return err
		}
		info, _ := statWorkspaceFile(j.workspaceFD(), stage)
		message := ""
		if info != nil {
			message = fmt.Sprintf("device=%v inode=%v size=%d", deviceInode(info), inodeNumber(info), info.Size())
		}
		var stageSize int64
		if info != nil {
			stageSize = info.Size()
		}
		if err := j.write(event{Event: "stage_created", Image: image, Stage: "created", Staging: stage, Device: deviceInode(info), Inode: inodeNumber(info), Size: stageSize, Message: message}); err != nil {
			_ = removeWorkspaceFile(j.workspaceFD(), stage)
			return fmt.Errorf("journal stage creation: %w", err)
		}
		err = renameNoReplace(j.workspaceFD(), stage, base)
		if err == nil {
			if err := syncDir(j.workspaceFD()); err != nil {
				return err
			}
			return nil
		}
		_ = removeWorkspaceFile(j.workspaceFD(), stage)
		if errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("output appeared during publication")
		}
		return err
	}
	return fmt.Errorf("could not create unique staging file")
}

// safeError performs the internal operation described by its signature.
func safeError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(s) > 512 {
		s = s[:512]
	}
	return s
}

// detailedError performs the internal operation described by its signature.
func detailedError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(s) > 4096 {
		s = s[:4096] + "…"
	}
	return s
}

// errorCategory performs the internal operation described by its signature.
func errorCategory(err error) string {
	if err == nil {
		return ""
	}
	return "inference"
}

// discardLogger is an internal data type used by this package.
type discardLogger struct{}

// Printf is an exported operation provided by this package.
func (discardLogger) Printf(string, ...interface{}) {}

// openJournal performs the internal operation described by its signature.
func openJournal(workspace string, _ Assets) (*journal, error) {
	dir, err := os.Open(workspace)
	if err != nil {
		return nil, err
	}
	f, err := openWorkspaceFile(int(dir.Fd()), logName, os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		_ = dir.Close()
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || linkCount(info) != 1 {
		_ = f.Close()
		_ = dir.Close()
		return nil, fmt.Errorf("unsafe log")
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		_ = dir.Close()
		return nil, err
	}
	idBytes := make([]byte, 16)
	_, _ = rand.Read(idBytes)
	return &journal{RunID: hex.EncodeToString(idBytes), Workspace: workspace, WorkspaceDir: dir, Log: f, locked: true}, nil
}

// workspaceFD performs the internal operation described by its signature.
func (j *journal) workspaceFD() int {
	if j == nil || j.WorkspaceDir == nil {
		return -1
	}
	return int(j.WorkspaceDir.Fd())
}

// write performs the internal operation described by its signature.
func (j *journal) write(e event) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	e.RunID = j.RunID
	e.Appliance = applianceName
	e.Version = applianceVersion
	e.PipelineSHA256 = j.manifest.PipelineSHA256
	e.ConfigSHA256 = j.manifest.ConfigSHA256
	e.SchemaSHA256 = j.manifest.SchemaSHA256
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := j.Log.Write(data); err != nil {
		return err
	}
	if err := j.Log.Sync(); err != nil {
		return err
	}
	_, _ = os.Stdout.Write(data)
	return nil
}

// close performs the internal operation described by its signature.
func (j *journal) close() {
	if j == nil {
		return
	}
	if j.Log != nil && j.locked {
		_ = syscall.Flock(int(j.Log.Fd()), syscall.LOCK_UN)
	}
	if j.WorkspaceDir != nil {
		_ = j.WorkspaceDir.Close()
	}
	if j.Log != nil {
		_ = j.Log.Close()
	}
}

// parseAssets performs the internal operation described by its signature.
func parseAssets(a Assets) (*induction.Pipeline, *applianceConfig, manifest, error) {
	if len(a.Pipeline) == 0 || len(a.Config) == 0 {
		return nil, nil, manifest{}, fmt.Errorf("embedded assets missing")
	}
	var p induction.Pipeline
	dec := yaml.NewDecoder(strings.NewReader(string(a.Pipeline)))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, nil, manifest{}, err
	}
	if err := p.Validate(); err != nil {
		return nil, nil, manifest{}, err
	}
	var c applianceConfig
	configDecoder := yaml.NewDecoder(strings.NewReader(string(a.Config)))
	configDecoder.KnownFields(true)
	if err := configDecoder.Decode(&c); err != nil {
		return nil, nil, manifest{}, err
	}
	if strings.ContainsAny(c.Server, "${}") || c.Timeout <= 0 || c.PollInterval <= 0 || c.LoadWaitInterval <= 0 {
		return nil, nil, manifest{}, fmt.Errorf("config contains unresolved values or invalid durations")
	}
	u, err := url.Parse(c.Server)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, nil, manifest{}, fmt.Errorf("invalid endpoint")
	}
	if len(c.MCPServers) > 0 {
		return nil, nil, manifest{}, fmt.Errorf("MCP is forbidden")
	}
	expectedSteps := []struct {
		name, model string
	}{
		{name: "analyze-image", model: "Gemma-4-E4B-UNC-Q8_K_P"},
		{name: "analyze-context-and-estimates", model: "Granite-4.1-4B-Q4_K_M"},
		{name: "return-json", model: "MiniCPM-4.7-35B-A3B-Q4_K_M"},
	}
	if len(p.Steps) != len(expectedSteps) {
		return nil, nil, manifest{}, fmt.Errorf("appliance requires exactly %d image-analysis steps", len(expectedSteps))
	}
	for i, expected := range expectedSteps {
		if p.Steps[i].Name != expected.name || p.Steps[i].Model != expected.model {
			return nil, nil, manifest{}, fmt.Errorf("step %d must be %q using model %q", i+1, expected.name, expected.model)
		}
		step := p.Steps[i]
		if step.Image != "" || step.Document != "" || step.Input != nil || step.ForEach != "" || step.Transform != "" || step.When != nil || step.Decision != nil || step.Classification != nil || step.Quality != nil || step.Retry != nil || len(step.Contract) != 0 {
			return nil, nil, manifest{}, fmt.Errorf("step %q uses an unsupported appliance feature", step.Name)
		}
		if i < len(expectedSteps)-1 && (step.Output != nil || step.ResponseFormat != nil || step.JSONSchema != nil) {
			return nil, nil, manifest{}, fmt.Errorf("non-final step %q has structured-output configuration", step.Name)
		}
	}
	if p.Config != "" || p.Inputs != nil || p.Batch != nil {
		return nil, nil, manifest{}, fmt.Errorf("pipeline contains an external input or config reference")
	}
	models := make([]string, 0, len(p.Steps))
	seenModels := make(map[string]struct{}, len(p.Steps))
	for _, s := range p.Steps {
		if strings.TrimSpace(s.Model) == "" || s.UseMCP {
			return nil, nil, manifest{}, fmt.Errorf("step policy mismatch")
		}
		if _, ok := seenModels[s.Model]; !ok {
			seenModels[s.Model] = struct{}{}
			models = append(models, s.Model)
		}
	}
	final := p.Steps[len(p.Steps)-1]
	if final.JSONSchema == nil || !schemaMatchesFinalOutput(final.JSONSchema) || final.ResponseFormat == nil || !strings.EqualFold(final.ResponseFormat.Type, "json_object") || final.Output == nil || final.Output.Artifact != "{{source.basename}}.{{source.extension}}.json" || !strings.EqualFold(final.Output.Type, "json") {
		return nil, nil, manifest{}, fmt.Errorf("final schema or output declaration mismatch")
	}
	sum := sha256.Sum256(a.Pipeline)
	cfg := sha256.Sum256(a.Config)
	sch, _ := json.Marshal(p.Steps[len(p.Steps)-1].JSONSchema)
	sh := sha256.Sum256(sch)
	return &p, &c, manifest{Name: applianceName, Version: applianceVersion, PipelineSHA256: hex.EncodeToString(sum[:]), ConfigSHA256: hex.EncodeToString(cfg[:]), SchemaSHA256: hex.EncodeToString(sh[:]), Models: models}, nil
}

// schemaMatchesFinalOutput performs the internal operation described by its signature.
func schemaMatchesFinalOutput(raw any) bool {
	value, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	properties, ok := value["properties"].(map[string]any)
	if !ok || len(properties) != 3 {
		return false
	}
	for _, name := range []string{"subjects", "composition", "environmental_context"} {
		field, ok := properties[name].(map[string]any)
		if !ok || field["type"] != "string" {
			return false
		}
	}
	required, ok := value["required"].([]any)
	if !ok || len(required) != 3 {
		return false
	}
	seen := make(map[string]bool, 3)
	for _, item := range required {
		name, ok := item.(string)
		if !ok || seen[name] {
			return false
		}
		seen[name] = true
	}
	return seen["subjects"] && seen["composition"] && seen["environmental_context"] && value["additionalProperties"] == false
}

// preflight performs the internal operation described by its signature.
func preflight(ctx context.Context, p *induction.Pipeline, c *applianceConfig) error {
	client := induction.NewClient(ctx, c.Server, induction.WithHTTPClient(applianceHTTPClient(15*time.Second)), induction.WithPollInterval(time.Duration(c.PollInterval)), induction.WithLoadWaitInterval(time.Duration(c.LoadWaitInterval)), induction.WithLogger(discardLogger{}))
	if err := induction.CheckHealth(c.Server, induction.WithHTTPClient(applianceHTTPClient(15*time.Second))); err != nil {
		return err
	}
	catalog, err := client.ListModelCatalog(ctx)
	if err != nil {
		return err
	}
	for _, requiredModel := range requiredModels(p) {
		found := false
		vision := false
		for _, m := range catalog {
			if m.ID == requiredModel {
				found = true
				vision = m.Capabilities.ImageInput || contains(m.InputModes, "image") || contains(m.InputModes, "vision")
				break
			}
		}
		if !found {
			return fmt.Errorf("required model %q unavailable", requiredModel)
		}
		if !vision {
			return fmt.Errorf("required model %q has no verified image capability", requiredModel)
		}
	}
	status, err := client.GetRuntimeStatus(ctx)
	if err != nil {
		return err
	}
	for _, requiredModel := range requiredModels(p) {
		found := false
		for _, m := range status.Models {
			if m.ID == requiredModel {
				found = true
				if m.Failed {
					return fmt.Errorf("required model %q failed", requiredModel)
				}
				if m.State != induction.ModelRuntimeLoaded {
					if _, err := client.LoadModel(ctx, requiredModel); err != nil {
						return err
					}
					loadedStatus, err := client.GetRuntimeStatus(ctx)
					if err != nil {
						return err
					}
					ready := false
					for _, loadedModel := range loadedStatus.Models {
						if loadedModel.ID == requiredModel && loadedModel.State == induction.ModelRuntimeLoaded && !loadedModel.Failed {
							ready = true
						}
					}
					if !ready {
						return fmt.Errorf("required model %q did not become ready", requiredModel)
					}
				}
			}
		}
		if !found {
			return fmt.Errorf("required model %q has no runtime status", requiredModel)
		}
	}
	return nil
}

// requiredModels performs the internal operation described by its signature.
func requiredModels(p *induction.Pipeline) []string {
	seen := make(map[string]struct{}, len(p.Steps))
	models := make([]string, 0, len(p.Steps))
	for _, step := range p.Steps {
		if _, ok := seen[step.Model]; ok {
			continue
		}
		seen[step.Model] = struct{}{}
		models = append(models, step.Model)
	}
	return models
}

// applianceHTTPClient performs the internal operation described by its signature.
func applianceHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}}
}

// contains performs the internal operation described by its signature.
func contains(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// isSystemicFailure performs the internal operation described by its signature.
func isSystemicFailure(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"connection refused", "connection reset", "no such host", "context deadline exceeded", "server disconnected", "unexpected status code: 5"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
