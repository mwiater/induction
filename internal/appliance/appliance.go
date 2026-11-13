package appliance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
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
	applianceName    = "induction-image-meta"
	applianceVersion = "1.0.0"
	logName          = "induction-image-meta.log"
)

const (
	ExitOK        = 0
	ExitArguments = 10
	ExitEmbedded  = 11
	ExitSandbox   = 12
	ExitLog       = 13
	ExitPreflight = 14
	ExitPartial   = 15
	ExitIntegrity = 16
)

type Assets struct{ Pipeline, Config []byte }

type applianceConfig struct {
	Server           string             `yaml:"server"`
	Timeout          induction.Duration `yaml:"timeout"`
	PollInterval     induction.Duration `yaml:"pollInterval"`
	LoadWaitInterval induction.Duration `yaml:"loadWaitInterval"`
	MCPServers       []map[string]any   `yaml:"mcpServers"`
	Log              map[string]any     `yaml:"log"`
}

type manifest struct {
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	PipelineSHA256 string   `json:"pipeline_sha256"`
	ConfigSHA256   string   `json:"config_sha256"`
	SchemaSHA256   string   `json:"schema_sha256"`
	Models         []string `json:"models"`
}

type journal struct {
	RunID        string
	Workspace    string
	WorkspaceDir *os.File
	Log          *os.File
	locked       bool
	manifest     manifest
	mu           sync.Mutex
}

type event struct {
	Timestamp      string `json:"timestamp"`
	Event          string `json:"event"`
	RunID          string `json:"run_id"`
	Appliance      string `json:"appliance"`
	Version        string `json:"version"`
	PipelineSHA256 string `json:"pipeline_sha256"`
	ConfigSHA256   string `json:"config_sha256,omitempty"`
	SchemaSHA256   string `json:"schema_sha256,omitempty"`
	Image          string `json:"image,omitempty"`
	Step           string `json:"step,omitempty"`
	StepNumber     int    `json:"step_number,omitempty"`
	StepTotal      int    `json:"step_total,omitempty"`
	Model          string `json:"model,omitempty"`
	Outcome        string `json:"outcome,omitempty"`
	Message        string `json:"message,omitempty"`
	ErrorCategory  string `json:"error_category,omitempty"`
	Error          string `json:"error,omitempty"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
	Stage          string `json:"stage,omitempty"`
	Staging        string `json:"staging,omitempty"`
}

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
	recoverAmbiguousStages(workspace, j)
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
	if err := preflight(context.Background(), pipeline, cfg); err != nil {
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
	client := induction.NewClient(context.Background(), cfg.Server, induction.WithHTTPClient(applianceHTTPClient(20*time.Minute)), induction.WithPollInterval(time.Duration(cfg.PollInterval)), induction.WithLoadWaitInterval(time.Duration(cfg.LoadWaitInterval)), induction.WithLogger(discardLogger{}))
	failures := discoveryFailures
	for _, image := range images {
		if image.conflict {
			continue
		}
		if image.skip {
			_ = j.write(event{Event: "image_skipped_valid", Image: image.name, Outcome: "skipped"})
			continue
		}
		started := time.Now()
		_ = j.write(event{Event: "image_started", Image: image.name, Outcome: "processing"})
		input := induction.InputSet{Images: []string{image.path}}
		result, runErr := induction.RunPipelineItemWithObserver(context.Background(), client, pipeline, input, func(progress induction.PipelineStepEvent) {
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
	outcome := "completed"
	code := ExitOK
	if failures > 0 {
		outcome = "completed_with_errors"
		code = ExitPartial
	}
	_ = j.write(event{Event: "run_end", Outcome: outcome, DurationMS: time.Since(start).Milliseconds(), Message: fmt.Sprintf("run finished: %d eligible image(s), %d failure(s)", len(images), failures)})
	return code
}

func recoverAmbiguousStages(workspace string, j *journal) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".induction-image-meta.") || !strings.HasSuffix(entry.Name(), ".stage") {
			continue
		}
		info, err := os.Lstat(filepath.Join(workspace, entry.Name()))
		if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			_ = j.write(event{Event: "recovery_ambiguous", Outcome: "left_in_place", Staging: entry.Name()})
		}
	}
}

func preflightFailure(j *journal, started time.Time, category string, err error) {
	_ = j.write(event{Event: "preflight_failed", Outcome: "failed", ErrorCategory: category, Error: detailedError(err), Message: "no image inference was started"})
	_ = j.write(event{Event: "run_end", Outcome: "failed", ErrorCategory: category, Error: detailedError(err), DurationMS: time.Since(started).Milliseconds(), Message: "run stopped during preflight; zero images were processed"})
}

type discovered struct {
	name, path, output string
	skip, conflict     bool
}

func discover(workspace string, j *journal) ([]discovered, int, error) {
	entries, err := os.ReadDir(workspace)
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
		info, err := os.Lstat(filepath.Join(workspace, name))
		if err != nil {
			return nil, 0, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !supportedImage(name) {
			continue
		}
		path := filepath.Join(workspace, name)
		if err := validateImage(path); err != nil {
			_ = j.write(event{Event: "image_discovered", Image: name, Outcome: "invalid", ErrorCategory: "image", Error: safeError(err)})
			conflicts++
			continue
		}
		output := path + ".json"
		state, err := existingOutput(output)
		if err != nil {
			return nil, 0, err
		}
		d := discovered{name: name, path: path, output: output, skip: state == outputValid, conflict: state == outputConflict}
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

type outputState int

const (
	outputAbsent outputState = iota
	outputValid
	outputConflict
)

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

func validateFinal(raw []byte) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return validateJSONSchema(value, finalOutputSchema(), "$")
}

// finalOutputSchema is the appliance's complete output contract. It mirrors
// the schema embedded in appliances/image-meta/pipeline.yaml and is kept
// here so publication validation does not depend on the model honoring its
// response_format request.
func finalOutputSchema() map[string]any {
	stringField := func() map[string]any { return map[string]any{"type": "string"} }
	stringArray := func() map[string]any {
		return map[string]any{"type": "array", "items": stringField()}
	}
	estimate := func(extra map[string]any, required []any) map[string]any {
		properties := map[string]any{
			"value":      map[string]any{"type": []any{"string", "null"}},
			"confidence": map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0},
			"evidence":   stringArray(),
			"caveats":    stringField(),
		}
		for name, value := range extra {
			properties[name] = value
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subjects":                      stringField(),
			"composition":                   stringField(),
			"environmental_context":         stringField(),
			"title":                         stringField(),
			"caption":                       stringField(),
			"image_category":                stringField(),
			"activities":                    stringArray(),
			"keywords":                      stringArray(),
			"visible_text":                  stringArray(),
			"distinctive_details":           stringArray(),
			"lighting":                      stringField(),
			"color_palette":                 stringArray(),
			"photographic_technique":        stringField(),
			"image_quality":                 stringField(),
			"editing_or_reproduction_clues": stringField(),
			"location_estimate":             estimate(nil, []any{"value", "confidence", "evidence", "caveats"}),
			"camera_estimate":               estimate(map[string]any{"camera_category": map[string]any{"type": []any{"string", "null"}}}, []any{"value", "confidence", "evidence", "caveats", "camera_category"}),
			"film_estimate": estimate(map[string]any{
				"medium":     stringField(),
				"film_type":  map[string]any{"type": []any{"string", "null"}},
				"film_stock": map[string]any{"type": []any{"string", "null"}},
			}, []any{"value", "confidence", "evidence", "caveats", "medium", "film_type", "film_stock"}),
			"capture_date_estimate": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value":         map[string]any{"type": []any{"integer", "null"}},
					"confidence":    map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0},
					"evidence":      stringArray(),
					"caveats":       stringField(),
					"earliest_year": map[string]any{"type": []any{"integer", "null"}},
					"latest_year":   map[string]any{"type": []any{"integer", "null"}},
					"scene_era":     map[string]any{"type": []any{"string", "null"}},
				},
				"required":             []any{"value", "confidence", "evidence", "caveats", "earliest_year", "latest_year", "scene_era"},
				"additionalProperties": false,
			},
		},
		"required":             []any{"subjects", "composition", "environmental_context", "title", "caption", "image_category", "activities", "keywords", "visible_text", "distinctive_details", "lighting", "color_palette", "photographic_technique", "image_quality", "editing_or_reproduction_clues", "location_estimate", "camera_estimate", "film_estimate", "capture_date_estimate"},
		"additionalProperties": false,
	}
}

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

func matchesJSONType(value any, want any) bool {
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
func normalizeJSON(raw []byte) []byte {
	var v map[string]any
	_ = json.Unmarshal(raw, &v)
	b, _ := json.Marshal(v)
	return append(b, '\n')
}
func validateFinalResult(raw json.RawMessage) error { return validateFinal(raw) }

func publish(j *journal, target string, data []byte, image string) error {
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
		path := filepath.Join(j.Workspace, stage)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
		if err != nil {
			continue
		}
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
			return err
		}
		_ = j.write(event{Event: "stage_created", Image: image, Stage: "created", Staging: stage})
		err = renameNoReplace(j.workspaceFD(), stage, base)
		if err == nil {
			_ = syncDir(j.workspaceFD())
			return nil
		}
		_ = os.Remove(path)
		if errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("output appeared during publication")
		}
		return err
	}
	return fmt.Errorf("could not create unique staging file")
}

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

func errorCategory(err error) string {
	if err == nil {
		return ""
	}
	return "inference"
}

type discardLogger struct{}

func (discardLogger) Printf(string, ...interface{}) {}

func openJournal(workspace string, assets Assets) (*journal, error) {
	path := filepath.Join(workspace, logName)
	info, err := os.Lstat(path)
	if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || linkCount(info) != 1) {
		return nil, fmt.Errorf("unsafe log")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	dir, err := os.Open(workspace)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	idBytes := make([]byte, 16)
	_, _ = rand.Read(idBytes)
	return &journal{RunID: hex.EncodeToString(idBytes), Workspace: workspace, WorkspaceDir: dir, Log: f, locked: true}, nil
}
func (j *journal) workspaceFD() int {
	if j == nil || j.WorkspaceDir == nil {
		return -1
	}
	return int(j.WorkspaceDir.Fd())
}
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
	if p.Batch != nil && len(p.Batch.Items) == 0 {
		// The appliance replaces the normal fixture source with a runtime
		// generated item list. Validate the surrounding batch contract without
		// embedding a runnable fixture path.
		p.Batch.Items = []induction.BatchItemConfig{{ID: "runtime-image", InputSet: induction.InputSet{Images: []string{"runtime-image.jpg"}}}}
	}
	if err := p.Validate(); err != nil {
		return nil, nil, manifest{}, err
	}
	var c applianceConfig
	if err := yaml.Unmarshal(a.Config, &c); err != nil {
		return nil, nil, manifest{}, err
	}
	u, err := url.Parse(c.Server)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return nil, nil, manifest{}, fmt.Errorf("invalid endpoint")
	}
	if len(c.MCPServers) > 0 {
		return nil, nil, manifest{}, fmt.Errorf("MCP is forbidden")
	}
	if len(p.Steps) < 2 || p.Steps[len(p.Steps)-1].Name != "return-json" {
		return nil, nil, manifest{}, fmt.Errorf("appliance requires an analysis pipeline ending in return-json")
	}
	if p.Batch != nil {
		p.Batch.Items = nil
	}
	models := make([]string, 0, len(p.Steps))
	seenModels := make(map[string]struct{}, len(p.Steps))
	for _, s := range p.Steps {
		if strings.TrimSpace(s.Model) == "" || !s.NoMCP {
			return nil, nil, manifest{}, fmt.Errorf("step policy mismatch")
		}
		if _, ok := seenModels[s.Model]; !ok {
			seenModels[s.Model] = struct{}{}
			models = append(models, s.Model)
		}
	}
	if p.Steps[len(p.Steps)-1].JSONSchema == nil || p.Steps[len(p.Steps)-1].Output == nil || p.Steps[len(p.Steps)-1].Output.Artifact != "{{source.basename}}.{{source.extension}}.json" {
		return nil, nil, manifest{}, fmt.Errorf("final schema or output declaration mismatch")
	}
	sum := sha256.Sum256(a.Pipeline)
	cfg := sha256.Sum256(a.Config)
	sch, _ := json.Marshal(p.Steps[len(p.Steps)-1].JSONSchema)
	sh := sha256.Sum256(sch)
	return &p, &c, manifest{Name: applianceName, Version: applianceVersion, PipelineSHA256: hex.EncodeToString(sum[:]), ConfigSHA256: hex.EncodeToString(cfg[:]), SchemaSHA256: hex.EncodeToString(sh[:]), Models: models}, nil
}

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
		for _, m := range status.Models {
			if m.ID == requiredModel {
				if m.Failed {
					return fmt.Errorf("required model %q failed", requiredModel)
				}
				if m.State != induction.ModelRuntimeLoaded {
					_, err = client.LoadModel(ctx, requiredModel)
					return err
				}
			}
		}
	}
	return nil
}

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

func applianceHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}}
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}
