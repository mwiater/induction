package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	induction "github.com/mwiater/induction"
	"github.com/mwiater/induction/internal/eval/inspect"
)

func BaseURL(server string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(server), "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid server URL %q", server)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(u.Path, "/v1") {
		u.Path += "/v1"
	}
	return strings.TrimRight(u.String(), "/"), nil
}

type BenchmarkCompletion struct {
	Result   BenchmarkResult
	Duration time.Duration
}

type RunOptions struct {
	OnBenchmarkComplete func(BenchmarkCompletion)
}

func Run(ctx context.Context, cfg *induction.Config, suite *Config, model, root string, out io.Writer, ir inspect.Runner, options ...RunOptions) (*Result, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("--model is required")
	}
	if cfg == nil || suite == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	base, err := BaseURL(cfg.Server)
	if err != nil {
		return nil, err
	}
	hash, err := suite.Hash()
	if err != nil {
		return nil, err
	}
	result, err := LoadResult(root, model, suite.Name)
	if err != nil {
		return nil, err
	}
	// A result file is identified by suite name and model. The config hash is
	// useful provenance, but must not prevent resuming when a benchmark is
	// added or its sample limit changes.
	if result != nil && result.Model.Name == model {
		result.Benchmarks = configuredBenchmarks(result.Benchmarks, suite.Evals)
		allComplete := result.Status == "success"
		for _, definition := range suite.Evals {
			if !hasCompleteBenchmark(result.Benchmarks, definition) {
				allComplete = false
				break
			}
		}
		if allComplete {
			result.Reused = true
			// Keep the normalized result in sync with the current config even
			// when the only change was adding/removing a completed benchmark.
			result.Suite.ConfigHash = hash
			if _, saveErr := SaveResult(root, result); saveErr != nil {
				return result, saveErr
			}
			for i, definition := range suite.Evals {
				_, _ = fmt.Fprintf(out, "[%d/%d] %s (already complete)\n", i+1, len(suite.Evals), definition.Name)
			}
			return result, nil
		}
	}
	inspectVersion, err := ir.CheckAvailable(ctx)
	if err != nil {
		return nil, err
	}
	client := induction.NewClient(ctx, cfg.Server, induction.WithHTTPClient(&http.Client{Timeout: time.Duration(cfg.Timeout)}))
	if _, err = client.InspectServer(ctx); err != nil {
		return nil, fmt.Errorf("llama.cpp server unavailable: %w", err)
	}
	mi, err := client.InspectModel(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("model %q is not available on the configured llama.cpp server", model)
	}
	if mi.Failed {
		return nil, fmt.Errorf("model %q is not available on the configured llama.cpp server", model)
	}
	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	if result != nil && result.Model.Name == model {
		runID = result.RunID
	}
	raw := filepath.Join(root, "data", "evals", "raw", runID)
	if err = os.MkdirAll(raw, 0o755); err != nil {
		return nil, err
	}
	started := time.Now()
	if result == nil || result.Model.Name != model {
		result = &Result{SchemaVersion: 1, RunID: runID, Suite: SuiteResult{Name: suite.Name, ConfigHash: hash}, Model: ModelResult{Name: model}, Engine: EngineResult{Name: "inspect-ai", Version: inspectVersion}, Server: ServerResult{Type: "llama.cpp", URL: cfg.Server}, StartedAt: time.Now(), Status: "failed"}
	} else {
		result.Resumed = true
		result.Suite.ConfigHash = hash
		result.Benchmarks = configuredBenchmarks(result.Benchmarks, suite.Evals)
		result.Engine.Version = inspectVersion
		result.Server = ServerResult{Type: "llama.cpp", URL: cfg.Server}
		result.Error = nil
		result.Aggregate = nil
		result.Status = "failed"
		if !result.StartedAt.IsZero() {
			started = result.StartedAt
		}
	}
	for i, e := range suite.Evals {
		maxTokens := suite.Generation.MaxTokens
		if e.MaxTokens > 0 {
			maxTokens = e.MaxTokens
		}
		runtimeVersion := benchmarkRuntimeVersion(e.Task)
		alreadyComplete := hasCompleteBenchmark(result.Benchmarks, e)
		if alreadyComplete {
			_, _ = fmt.Fprintf(out, "[%d/%d] %s (already complete)\n", i+1, len(suite.Evals), e.Name)
			continue
		}
		if e.RequiresImageInput && !mi.Capabilities.ImageInput {
			skipped := BenchmarkResult{Name: e.Name, Tag: e.Tag, Task: e.Task, Limit: e.Limit, MaxTokens: maxTokens, Skipped: true, SkipReason: "model does not report image input capability"}
			result.Benchmarks = replaceBenchmark(result.Benchmarks, skipped)
			result.Status = "running"
			if _, saveErr := SaveResult(root, result); saveErr != nil {
				return result, saveErr
			}
			_, _ = fmt.Fprintf(out, "[%d/%d] %s (skipped: model has no image input)\n", i+1, len(suite.Evals), e.Name)
			continue
		}
		_, _ = fmt.Fprintf(out, "[%d/%d] %s\n", i+1, len(suite.Evals), e.Name)
		evalStarted := time.Now()
		logName := e.Name
		if previous, ok := findBenchmark(result.Benchmarks, e.Name); ok && (!compatibleBenchmark(previous, e, maxTokens, runtimeVersion) || previous.Limit > e.Limit) {
			// Keep incompatible historical logs intact and give the changed
			// task configuration a fresh, deterministic log directory.
			logName = e.Name + "-" + shortBenchmarkHash(e.TaskArgs, maxTokens, runtimeVersion)
		} else if previous, ok := findBenchmark(result.Benchmarks, e.Name); ok && previous.RawLog != "" {
			// A larger limit is a continuation of the same benchmark identity.
			// Keep the original directory so its completed samples remain visible.
			logName = previous.RawLog
		}
		logDir := filepath.Join(raw, logName)
		resumeLog, _ := inspect.FindLatestLog(logDir)
		resumeInfo, resumeErr := inspect.ParseLogInfo(logDir)
		previous, hasPrevious := findBenchmark(result.Benchmarks, e.Name)
		if !hasPrevious || !compatibleBenchmark(previous, e, maxTokens, runtimeVersion) || previous.Limit >= e.Limit {
			previous = BenchmarkResult{}
		}
		continuationStart := 0
		if previous.Samples > 0 && previous.Limit < e.Limit {
			continuationStart = previous.Samples
		}
		resumeSamples := continuationStart
		if resumeErr == nil && resumeInfo.Status != "success" {
			resumeSamples += resumeInfo.Parsed.Samples
		}
		if resumeSamples > e.Limit {
			resumeSamples = e.Limit
		}
		if resumeSamples > 0 {
			_, _ = fmt.Fprintf(out, "  resuming from %d/%d samples\n", resumeSamples, e.Limit)
		}
		_, _ = fmt.Fprintf(out, "  samples: %d/%d | elapsed: 0s\n", resumeSamples, e.Limit)
		lastProgress := resumeSamples
		ir.Progress = func(line string) {
			if done, total, ok := inspect.Progress(line); ok && done > lastProgress {
				lastProgress = done
				_, _ = fmt.Fprintf(out, "  samples: %d/%d | elapsed: %s\n", done, total, time.Since(evalStarted).Round(time.Second))
			}
		}
		// Persist the suite state before launching Inspect. This makes the
		// normalized result a durable marker even if the process is killed.
		result.Status = "running"
		if result.Error != nil && result.Error.Benchmark == e.Name {
			result.Error = nil
		}
		if _, saveErr := SaveResult(root, result); saveErr != nil {
			return result, saveErr
		}
		env := []string{"INDUCTION_BASE_URL=" + base, "INDUCTION_API_KEY=induction-local"}
		compatEnv, cleanup, envErr := evaluatorEnvironment(e.Task)
		if envErr != nil {
			result.Error = &ErrorResult{Benchmark: e.Name, Message: envErr.Error()}
			if suite.Execution.FailFast {
				break
			}
			continue
		}
		env = append(env, compatEnv...)
		var ex inspect.Execution
		var runErr error
		if resumeErr == nil && resumeInfo.Status == "success" && resumeInfo.Parsed.Samples >= e.Limit {
			benchmark := mergeBenchmark(previous, e, maxTokens, runtimeVersion, logName, resumeInfo.Parsed)
			result.Benchmarks = replaceBenchmark(result.Benchmarks, benchmark)
			_, _ = fmt.Fprintf(out, "  samples: %d/%d | recovered from existing continuation\n", benchmark.Samples, e.Limit)
			cleanup()
			if len(options) > 0 && options[0].OnBenchmarkComplete != nil {
				options[0].OnBenchmarkComplete(BenchmarkCompletion{Result: benchmark})
			}
			continue
		}
		if resumeLog != "" && (resumeErr != nil || resumeInfo.Status != "success") {
			ex, runErr = ir.Retry(ctx, resumeLog, env, e.Limit)
		} else {
			if continuationStart > 0 {
				if err := saveContinuationState(logDir, continuationState{Start: continuationStart, Target: e.Limit}); err != nil {
					cleanup()
					result.Error = &ErrorResult{Benchmark: e.Name, Message: err.Error()}
					if suite.Execution.FailFast {
						break
					}
					continue
				}
			}
			ex, runErr = ir.Run(ctx, inspect.Command{Task: e.Task, Model: model, Start: continuationStart, Limit: e.Limit, Temperature: suite.Generation.Temperature, MaxTokens: maxTokens, TaskArgs: e.TaskArgs, LogDir: logDir}, env)
		}
		cleanup()
		if runErr != nil {
			diagnostic := strings.TrimSpace(ex.Stderr)
			if diagnostic == "" {
				diagnostic = strings.TrimSpace(ex.Stdout)
			}
			if diagnostic == "" {
				diagnostic = "Inspect produced no diagnostic output; see " + ex.LogPath
			}
			result.Error = &ErrorResult{Benchmark: e.Name, Message: fmt.Sprintf("%v: %s", runErr, diagnostic)}
			if suite.Execution.FailFast {
				break
			}
			continue
		}
		p, parseErr := inspect.ParseLog(logDir)
		if parseErr != nil {
			result.Error = &ErrorResult{Benchmark: e.Name, Message: parseErr.Error()}
			if suite.Execution.FailFast {
				break
			}
			continue
		}
		benchmark := mergeBenchmark(previous, e, maxTokens, runtimeVersion, logName, p)
		result.Benchmarks = replaceBenchmark(result.Benchmarks, benchmark)
		_, _ = fmt.Fprintf(out, "  samples: %d/%d | completed in: %s\n", p.Samples, e.Limit, time.Since(evalStarted).Round(time.Millisecond))
		if len(options) > 0 && options[0].OnBenchmarkComplete != nil {
			options[0].OnBenchmarkComplete(BenchmarkCompletion{Result: benchmark, Duration: time.Since(evalStarted)})
		}
	}
	result.CompletedAt = time.Now()
	result.DurationMS = result.CompletedAt.Sub(started).Milliseconds()
	if result.Error == nil {
		result.Status = "success"
		var total float64
		completed := 0
		for _, b := range result.Benchmarks {
			if b.Skipped {
				continue
			}
			total += b.Score
			completed++
		}
		if completed > 0 {
			result.Aggregate = &AggregateResult{Score: total / float64(completed), Method: "mean"}
		}
	} else {
		result.Status = "failed"
	}
	path, saveErr := SaveResult(root, result)
	if saveErr != nil {
		return result, saveErr
	}
	if result.Error != nil {
		return result, fmt.Errorf("evaluation failed: %s (result: %s)", result.Error.Message, path)
	}
	return result, nil
}

func replaceBenchmark(all []BenchmarkResult, replacement BenchmarkResult) []BenchmarkResult {
	for i := range all {
		if all[i].Name == replacement.Name {
			all[i] = replacement
			return all
		}
	}
	return append(all, replacement)
}

// configuredBenchmarks drops results for benchmarks no longer in the config
// and drops incompatible task/name pairs. Results with an old limit are kept
// so their raw Inspect log can be resumed when the limit is increased or
// otherwise changed.
func configuredBenchmarks(existing []BenchmarkResult, definitions []Definition) []BenchmarkResult {
	allowed := make(map[string]string, len(definitions))
	for _, definition := range definitions {
		allowed[definition.Name] = definition.Task
	}
	filtered := make([]BenchmarkResult, 0, len(existing))
	for _, benchmark := range existing {
		if task, ok := allowed[benchmark.Name]; ok && task == benchmark.Task {
			filtered = append(filtered, benchmark)
		}
	}
	return filtered
}

func hasCompleteBenchmark(benchmarks []BenchmarkResult, definition Definition) bool {
	for _, benchmark := range benchmarks {
		if benchmark.Name == definition.Name && benchmark.Task == definition.Task && benchmark.Limit >= definition.Limit && (definition.MaxTokens <= 0 || benchmark.MaxTokens == definition.MaxTokens) && sameTaskArgs(benchmark.TaskArgs, definition.TaskArgs) && benchmark.RuntimeVersion == benchmarkRuntimeVersion(definition.Task) && (len(definition.TaskArgs) == 0 || benchmark.RawLog != "") && (definition.Limit <= 0 || benchmark.Samples >= definition.Limit) {
			return true
		}
	}
	return false
}

func compatibleBenchmark(previous BenchmarkResult, definition Definition, maxTokens int, runtimeVersion string) bool {
	return previous.Name == definition.Name &&
		previous.Task == definition.Task &&
		(definition.MaxTokens <= 0 || previous.MaxTokens == maxTokens) &&
		sameTaskArgs(previous.TaskArgs, definition.TaskArgs) &&
		previous.RuntimeVersion == runtimeVersion
}

func mergeBenchmark(previous BenchmarkResult, definition Definition, maxTokens int, runtimeVersion, rawLog string, current inspect.Parsed) BenchmarkResult {
	if previous.Samples == 0 {
		return BenchmarkResult{Name: definition.Name, Tag: definition.Tag, Task: definition.Task, Limit: definition.Limit, MaxTokens: maxTokens, TaskArgs: definition.TaskArgs, RuntimeVersion: runtimeVersion, RawLog: rawLog, Samples: current.Samples, Score: current.Score, Metrics: current.Metrics}
	}
	total := previous.Samples + current.Samples
	metrics := make(map[string]float64, len(previous.Metrics)+len(current.Metrics))
	for name, value := range previous.Metrics {
		metrics[name] = value * float64(previous.Samples)
	}
	for name, value := range current.Metrics {
		metrics[name] += value * float64(current.Samples)
	}
	for name, value := range metrics {
		metrics[name] = value / float64(total)
	}
	return BenchmarkResult{Name: definition.Name, Tag: definition.Tag, Task: definition.Task, Limit: definition.Limit, MaxTokens: maxTokens, TaskArgs: definition.TaskArgs, RuntimeVersion: runtimeVersion, RawLog: rawLog, Samples: total, Score: (previous.Score*float64(previous.Samples) + current.Score*float64(current.Samples)) / float64(total), Metrics: metrics}
}

func findBenchmark(benchmarks []BenchmarkResult, name string) (BenchmarkResult, bool) {
	for _, benchmark := range benchmarks {
		if benchmark.Name == name {
			return benchmark, true
		}
	}
	return BenchmarkResult{}, false
}

func sameTaskArgs(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func shortBenchmarkHash(args map[string]string, maxTokens int, runtimeVersion string) string {
	b, _ := json.Marshal(struct {
		TaskArgs       map[string]string `json:"task_args,omitempty"`
		MaxTokens      int               `json:"max_tokens,omitempty"`
		RuntimeVersion string            `json:"runtime_version,omitempty"`
	}{TaskArgs: args, MaxTokens: maxTokens, RuntimeVersion: runtimeVersion})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])[:12]
}

// HumanEval's verifier executes generated code with the command "python".
// Minimal Linux installations commonly provide only python3, so expose the
// installed interpreter under the command name expected by Inspect.
func evaluatorEnvironment(task string) ([]string, func(), error) {
	if task != "inspect_evals/humaneval" {
		return nil, func() {}, nil
	}
	python3, err := exec.LookPath("python3")
	if err != nil {
		return nil, func() {}, fmt.Errorf("HumanEval requires python3 for verification: %w", err)
	}
	shimDir, err := os.MkdirTemp("", "induction-eval-python-")
	if err != nil {
		return nil, func() {}, fmt.Errorf("create HumanEval Python compatibility shim: %w", err)
	}
	if err := os.Symlink(python3, filepath.Join(shimDir, "python")); err != nil {
		_ = os.RemoveAll(shimDir)
		return nil, func() {}, fmt.Errorf("create HumanEval Python compatibility shim: %w", err)
	}
	pathValue := shimDir
	if existing := os.Getenv("PATH"); existing != "" {
		pathValue += string(os.PathListSeparator) + existing
	}
	return []string{"PATH=" + pathValue}, func() { _ = os.RemoveAll(shimDir) }, nil
}

func benchmarkRuntimeVersion(task string) string {
	if task == "inspect_evals/humaneval" {
		return "python3-local-shim-v2"
	}
	return ""
}

type continuationState struct {
	Start  int `json:"start"`
	Target int `json:"target"`
}

func continuationStatePath(dir string) string {
	return filepath.Join(dir, "continuation.state")
}

func saveContinuationState(dir string, state continuationState) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(continuationStatePath(dir), b, 0o600)
}
