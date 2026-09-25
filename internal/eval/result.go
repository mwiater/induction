package eval

import "time"

type BenchmarkStatus struct {
	Definition Definition
	Samples    int
	Complete   bool
}

// Status reports completion for every benchmark in the current suite config.
// A benchmark is complete only when its saved result is compatible with the
// current definition and has enough samples to satisfy its limit.
func Status(suite *Config, result *Result) []BenchmarkStatus {
	if suite == nil {
		return nil
	}
	benchmarks := []BenchmarkResult(nil)
	if result != nil {
		benchmarks = result.Benchmarks
	}
	status := make([]BenchmarkStatus, 0, len(suite.Evals))
	for _, definition := range suite.Evals {
		benchmark, found := findBenchmark(benchmarks, definition.Name)
		if !found || benchmark.Task != definition.Task {
			status = append(status, BenchmarkStatus{Definition: definition})
			continue
		}
		status = append(status, BenchmarkStatus{
			Definition: definition,
			Samples:    benchmark.Samples,
			Complete:   hasCompleteBenchmark(benchmarks, definition),
		})
	}
	return status
}

type Result struct {
	SchemaVersion int               `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Suite         SuiteResult       `json:"suite"`
	Model         ModelResult       `json:"model"`
	Engine        EngineResult      `json:"engine"`
	Server        ServerResult      `json:"server"`
	StartedAt     time.Time         `json:"started_at"`
	CompletedAt   time.Time         `json:"completed_at"`
	DurationMS    int64             `json:"duration_ms"`
	Status        string            `json:"status"`
	Benchmarks    []BenchmarkResult `json:"benchmarks,omitempty"`
	Aggregate     *AggregateResult  `json:"aggregate,omitempty"`
	Error         *ErrorResult      `json:"error,omitempty"`
	Reused        bool              `json:"-"`
	Resumed       bool              `json:"-"`
}
type SuiteResult struct {
	Name       string `json:"name"`
	ConfigHash string `json:"config_hash"`
}

type ModelResult struct {
	Name string `json:"name"`
}
type EngineResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type ServerResult struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}
type BenchmarkResult struct {
	Name           string             `json:"name"`
	Tag            string             `json:"tag,omitempty"`
	Skipped        bool               `json:"skipped,omitempty"`
	SkipReason     string             `json:"skip_reason,omitempty"`
	Task           string             `json:"task"`
	Limit          int                `json:"limit"`
	MaxTokens      int                `json:"max_tokens,omitempty"`
	TaskArgs       map[string]string  `json:"task_args,omitempty"`
	RuntimeVersion string             `json:"runtime_version,omitempty"`
	RawLog         string             `json:"raw_log,omitempty"`
	Samples        int                `json:"samples"`
	Score          float64            `json:"score"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
}
type AggregateResult struct {
	Score  float64 `json:"score"`
	Method string  `json:"method"`
}
type ErrorResult struct {
	Benchmark string `json:"benchmark,omitempty"`
	Message   string `json:"message"`
}
