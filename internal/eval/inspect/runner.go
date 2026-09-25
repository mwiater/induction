package inspect

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Command struct {
	Task, Model string
	Start       int
	Limit       int
	Temperature float64
	MaxTokens   int
	TaskArgs    map[string]string
	LogDir      string
}
type Execution struct{ Version, LogPath, Stdout, Stderr string }
type Runner struct {
	LookPath   func(string) (string, error)
	RunCommand func(context.Context, string, []string, []string) ([]byte, []byte, error)
	Progress   func(string)
}

var progressPattern = regexp.MustCompile(`(?:^|\s)([0-9]+)\s*/\s*([0-9]+)(?:\s|$)`)

func Progress(line string) (int, int, bool) {
	match := progressPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 3 {
		return 0, 0, false
	}
	done, errDone := strconv.Atoi(match[1])
	total, errTotal := strconv.Atoi(match[2])
	return done, total, errDone == nil && errTotal == nil && total > 0 && done <= total
}

func (r Runner) CheckAvailable(ctx context.Context) (string, error) {
	lookup := r.LookPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	path, err := lookup("inspect")
	if err != nil {
		//nolint:staticcheck // preserve the established user-facing error text.
		return "", fmt.Errorf("Inspect AI is required to run evaluations\n\nInstall the evaluation dependencies:\n\n    pip install inspect-evals openai")
	}
	stdout, _, err := r.run(ctx, path, []string{"--version"}, nil)
	if err != nil {
		//nolint:staticcheck // preserve the established user-facing error text.
		return "", fmt.Errorf("Inspect AI is required to run evaluations: %w", err)
	}
	return string(bytes.TrimSpace(stdout)), nil
}

func (r Runner) BuildArgs(c Command) []string {
	a := []string{"eval", c.Task, "--model", "openai-api/induction/" + c.Model}
	if c.Limit > 0 {
		limit := strconv.Itoa(c.Limit)
		if c.Start > 0 && c.Start < c.Limit {
			limit = strconv.Itoa(c.Start) + "-" + strconv.Itoa(c.Limit)
		}
		a = append(a, "--limit", limit)
	}
	a = append(a, "--temperature", strconv.FormatFloat(c.Temperature, 'g', -1, 64))
	if c.MaxTokens > 0 {
		a = append(a, "--max-tokens", strconv.Itoa(c.MaxTokens))
	}
	keys := make([]string, 0, len(c.TaskArgs))
	for key := range c.TaskArgs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		a = append(a, "-T", key+"="+c.TaskArgs[key])
	}
	// The bundled HumanEval task's Docker sandbox image does not include the
	// `python` command used by its verifier. Run only this verifier locally so
	// the evaluatorEnvironment compatibility shim can provide python3 safely.
	if c.Task == "inspect_evals/humaneval" {
		a = append(a, "--sandbox", "local")
	}
	// Checkpoint every turn so an interrupted run can be resumed at sample
	// boundaries by eval-retry instead of starting the benchmark over.
	a = append(a, "--checkpoint=turn:1", "--log-format", "json", "--display", "plain", "--log-buffer", "1")
	if c.LogDir != "" {
		a = append(a, "--log-dir", c.LogDir)
	}
	return a
}

// FindLatestLog returns the newest Inspect log in dir. JSON logs are preferred
// because this adapter deliberately requests Inspect's JSON log format, but
// eval logs are also returned so eval-retry can recover them.
func FindLatestLog(dir string) (string, error) {
	type logFile struct {
		path string
		when int64
	}
	var files []logFile
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (filepath.Ext(path) == ".json" || filepath.Ext(path) == ".eval") {
			files = append(files, logFile{path: path, when: info.ModTime().UnixNano()})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", os.ErrNotExist
	}
	for i := 1; i < len(files); i++ {
		for j := i; j > 0 && files[j].when > files[j-1].when; j-- {
			files[j], files[j-1] = files[j-1], files[j]
		}
	}
	return files[0].path, nil
}

func (r Runner) Run(ctx context.Context, c Command, env []string) (Execution, error) {
	lookup := r.LookPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	path, err := lookup("inspect")
	if err != nil {
		return Execution{}, err
	}
	if err := os.MkdirAll(filepath.Clean(c.LogDir), 0o755); err != nil {
		return Execution{}, err
	}
	stopProgress := make(chan struct{})
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		r.watchLogProgress(ctx, c.LogDir, c.Limit, stopProgress)
	}()
	out, stderr, err := r.run(ctx, path, r.BuildArgs(c), env)
	close(stopProgress)
	<-progressDone
	// Preserve process diagnostics even when Inspect fails before creating an
	// EvalLog (for example, during provider setup or task loading).
	_ = os.WriteFile(filepath.Join(c.LogDir, "inspect.stdout.log"), out, 0o600)
	_ = os.WriteFile(filepath.Join(c.LogDir, "inspect.stderr.log"), stderr, 0o600)
	return Execution{Stdout: string(out), Stderr: string(stderr), LogPath: c.LogDir}, err
}

// Retry resumes an interrupted Inspect evaluation from its checkpoint log.
// Inspect writes the recovered log alongside the original log; ParseLog then
// selects the newest result on the next pass.
func (r Runner) Retry(ctx context.Context, logPath string, env []string, limits ...int) (Execution, error) {
	lookup := r.LookPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	path, err := lookup("inspect")
	if err != nil {
		return Execution{}, err
	}
	logDir := filepath.Dir(logPath)
	stopProgress := make(chan struct{})
	progressDone := make(chan struct{})
	total := 0
	if len(limits) > 0 {
		total = limits[0]
	}
	go func() {
		defer close(progressDone)
		r.watchLogProgress(ctx, logDir, total, stopProgress)
	}()
	out, stderr, err := r.run(ctx, path, []string{"eval-retry", logPath, "--checkpoint=turn:1", "--display", "plain", "--log-buffer", "1", "--log-dir", logDir}, env)
	close(stopProgress)
	<-progressDone
	_ = os.WriteFile(filepath.Join(logDir, "inspect.stdout.log"), out, 0o600)
	_ = os.WriteFile(filepath.Join(logDir, "inspect.stderr.log"), stderr, 0o600)
	return Execution{Stdout: string(out), Stderr: string(stderr), LogPath: logDir}, err
}

// watchLogProgress handles Inspect configurations that do not emit display
// progress. JSON logs are written incrementally, so completed score events
// provide a reliable sample counter while the subprocess is still running.
func (r Runner) watchLogProgress(ctx context.Context, dir string, total int, stop <-chan struct{}) {
	if r.Progress == nil || dir == "" {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	last := 0
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			path, err := FindLatestLog(dir)
			if err != nil {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			completed := bytes.Count(data, []byte(`"event": "score"`))
			if completed > last {
				last = completed
				if total > 0 {
					r.Progress(fmt.Sprintf("%d/%d", completed, total))
				}
			}
		}
	}
}

func (r Runner) run(ctx context.Context, path string, args, env []string) ([]byte, []byte, error) {
	if r.RunCommand != nil {
		return r.RunCommand(ctx, path, args, env)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = mergeEnv(os.Environ(), env)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	var out bytes.Buffer
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		out.WriteString(line)
		out.WriteByte('\n')
		if r.Progress != nil {
			r.Progress(line)
		}
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return out.Bytes(), stderr.Bytes(), ctx.Err()
	}
	return out.Bytes(), stderr.Bytes(), err
}

func mergeEnv(base, overrides []string) []string {
	merged := append([]string(nil), base...)
	positions := make(map[string]int, len(merged))
	for i, entry := range merged {
		if key, _, ok := strings.Cut(entry, "="); ok {
			positions[key] = i
		}
	}
	for _, entry := range overrides {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		if i, exists := positions[key]; exists {
			merged[i] = entry
		} else {
			positions[key] = len(merged)
			merged = append(merged, entry)
		}
	}
	return merged
}
