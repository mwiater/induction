package pipelineoptimizer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	induction "github.com/mwiater/induction"
	"gopkg.in/yaml.v3"
)

var altSuffix = regexp.MustCompile(`^(.*)-alt-([1-9][0-9]*)$`)

func familyName(path string) (string, error) {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != ".yaml" && ext != ".yml" {
		return "", errors.New("pipeline must have .yaml or .yml extension")
	}
	stem := strings.TrimSuffix(base, ext)
	if match := altSuffix.FindStringSubmatch(stem); match != nil {
		stem = match[1]
	}
	if stem == "" || stem == "." || stem == ".." || strings.HasPrefix(stem, ".") {
		return "", fmt.Errorf("unsafe pipeline filename %q", base)
	}
	return stem, nil
}

func safeOutputDirectory(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect output directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output directory %q is not a real directory", dir)
	}
	// Refuse a symlink in any existing ancestor, including the output root.
	for p := dir; p != filepath.Dir(p); p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output path contains symlink %q", p)
		}
	}
	return nil
}

func validateCandidate(data []byte, original *induction.Pipeline, inputPath, outputDir string) (*induction.Pipeline, error) {
	if len(data) == 0 || len(data) > maxPipelineBytes {
		return nil, errors.New("candidate YAML is empty or exceeds size limit")
	}
	var decoded induction.Pipeline
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("strict YAML decode: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("candidate contains multiple YAML documents")
		}
		return nil, fmt.Errorf("decode trailing YAML: %w", err)
	}
	if decoded.Name != original.Name {
		return nil, errors.New("candidate must preserve pipeline name")
	}
	if decoded.Prompt != original.Prompt {
		return nil, errors.New("candidate must preserve top-level prompt")
	}
	if decoded.Config != original.Config {
		return nil, errors.New("candidate must preserve configuration path")
	}
	allowedModels := map[string]bool{}
	allowedAttachments := map[string]bool{}
	allowedSchemaFiles := map[string]bool{}
	for _, step := range original.Steps {
		if step.Model != "" {
			allowedModels[step.Model] = true
		}
		if step.Image != "" {
			allowedAttachments[step.Image] = true
		}
		if step.Document != "" {
			allowedAttachments[step.Document] = true
		}
		if step.Retry != nil && step.Retry.Validate != nil && step.Retry.Validate.SchemaFile != "" {
			allowedSchemaFiles[step.Retry.Validate.SchemaFile] = true
		}
	}
	for _, step := range decoded.Steps {
		if step.Model != "" && !allowedModels[step.Model] {
			return nil, fmt.Errorf("candidate introduces unverified model %q", step.Model)
		}
		if step.Retry != nil && step.Retry.Validate != nil && step.Retry.Validate.SchemaFile != "" && !allowedSchemaFiles[step.Retry.Validate.SchemaFile] {
			return nil, errors.New("candidate introduces a new retry schemaFile path")
		}
	}
	if original.Batch != nil {
		seenNoMCP, seenMCP := false, false
		for _, step := range decoded.Steps {
			if step.NoMCP {
				seenNoMCP = true
			} else if step.Transform == "" {
				seenMCP = true
			}
		}
		if seenNoMCP && seenMCP {
			return nil, errors.New("batch candidate mixes nomcp settings; batch CLI routing is pipeline-wide, so set nomcp: true on every inference step or omit it from every step")
		}
	}
	originalSteps := map[string]induction.PipelineStep{}
	for _, step := range original.Steps {
		originalSteps[step.Name] = step
	}
	for _, step := range decoded.Steps {
		if prior, ok := originalSteps[step.Name]; ok && prior.NoMCP && !step.NoMCP {
			return nil, fmt.Errorf("candidate enables MCP for step %q", step.Name)
		}
		if _, ok := originalSteps[step.Name]; !ok && step.Transform == "" && !step.NoMCP {
			return nil, fmt.Errorf("new step %q must set nomcp: true", step.Name)
		}
	}
	// ParsePipeline is the same strict loader used by normal file execution. It
	// performs static validation and read-only source path resolution.
	name := filepath.Join(outputDir, filepath.Base(inputPath))
	candidate, err := induction.ParsePipeline(data, name)
	if err != nil {
		return nil, err
	}
	originalInput, _ := yaml.Marshal(original.Inputs)
	candidateInput, _ := yaml.Marshal(candidate.Inputs)
	if string(originalInput) != string(candidateInput) {
		return nil, errors.New("candidate must preserve external inputs")
	}
	originalBatch, _ := yaml.Marshal(original.Batch)
	candidateBatch, _ := yaml.Marshal(candidate.Batch)
	if string(originalBatch) != string(candidateBatch) {
		return nil, errors.New("candidate must preserve batch configuration")
	}
	for _, step := range candidate.Steps {
		if step.Image != "" && !allowedAttachments[step.Image] {
			return nil, errors.New("candidate introduces a new image path")
		}
		if step.Document != "" && !allowedAttachments[step.Document] {
			return nil, errors.New("candidate introduces a new document path")
		}
	}
	lastName := original.Steps[len(original.Steps)-1].Name
	if candidate.Steps[len(candidate.Steps)-1].Name != lastName {
		return nil, fmt.Errorf("candidate must preserve final step %q", lastName)
	}
	neededArtifacts := map[string]bool{}
	for _, step := range original.Steps {
		if step.Output != nil && step.Output.Artifact != "" {
			neededArtifacts[step.Output.Artifact] = true
		}
	}
	for _, step := range candidate.Steps {
		if step.Output != nil {
			delete(neededArtifacts, step.Output.Artifact)
		}
	}
	for artifact := range neededArtifacts {
		return nil, fmt.Errorf("candidate removes required artifact %q", artifact)
	}
	return candidate, nil
}

func nextSlot(dir, family string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	prefix := family + "-alt-"
	max := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		numText := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
		n, err := strconv.Atoi(numText)
		if err == nil && n > max {
			max = n
		}
	}
	return max + 1, nil
}

func publish(ctx context.Context, dir, family string, yamlData []byte, report func(string) []byte) (string, string, error) {
	if err := safeOutputDirectory(dir); err != nil {
		return "", "", err
	}
	lockPath := filepath.Join(dir, "."+family+"-optimizer.lock")
	deadline := time.Now().Add(5 * time.Second)
	var lock *os.File
	for {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		var err error
		lock, err = os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return "", "", fmt.Errorf("reserve optimizer family: %w", err)
		}
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("optimizer family %q is locked; retry after the other optimizer finishes", family)
		}
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	defer func() { _ = lock.Close(); _ = os.Remove(lockPath) }()
	slot, err := nextSlot(dir, family)
	if err != nil {
		return "", "", err
	}
	stem := fmt.Sprintf("%s-alt-%d", family, slot)
	yamlPath := filepath.Join(dir, stem+".yaml")
	reportPath := filepath.Join(dir, stem+".analysis.md")
	if _, err := os.Lstat(reportPath); err == nil {
		// A report published before an interrupted YAML publication is an
		// orphan. The family lock makes its cleanup safe.
		if _, yamlErr := os.Lstat(yamlPath); os.IsNotExist(yamlErr) {
			contents, readErr := os.ReadFile(reportPath)
			if readErr != nil || !strings.HasPrefix(string(contents), "<!-- induction-optimizer-v1 -->\n") {
				return "", "", fmt.Errorf("report %s already exists and is not a recognized interrupted optimizer report", reportPath)
			}
			if err := os.Remove(reportPath); err != nil {
				return "", "", fmt.Errorf("clean orphan report: %w", err)
			}
		} else {
			return "", "", fmt.Errorf("report %s already exists", reportPath)
		}
	} else if !os.IsNotExist(err) {
		return "", "", err
	}
	yamlTemp, err := stage(dir, ".optimizer-*.yaml.tmp", yamlData)
	if err != nil {
		return "", "", err
	}
	defer os.Remove(yamlTemp)
	reportTemp, err := stage(dir, ".optimizer-*.md.tmp", report(yamlPath))
	if err != nil {
		return "", "", err
	}
	defer os.Remove(reportTemp)
	// Hard links publish without replacement. Publish report first so an
	// interruption cannot leave a runnable-looking YAML without analysis.
	if err := os.Link(reportTemp, reportPath); err != nil {
		return "", "", fmt.Errorf("publish report: %w", err)
	}
	if err := os.Link(yamlTemp, yamlPath); err != nil {
		_ = os.Remove(reportPath)
		return "", "", fmt.Errorf("publish alternative: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(yamlPath)
		_ = os.Remove(reportPath)
		return "", "", err
	}
	return yamlPath, reportPath, nil
}

func stage(dir, pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func renderReport(inputPath, inputHash, outputPath, outputHash string, source sourceEvidence, sessions []sessionEvidence, findings []Finding, manifest, hypotheses, warnings []string, focus string) string {
	var b strings.Builder
	b.WriteString("<!-- induction-optimizer-v1 -->\n")
	family, _ := familyName(inputPath)
	fmt.Fprintf(&b, "# Pipeline alternative analysis\n\nInput: `%s`\n\nInput SHA-256: `%s`\n\nAlternative: `%s`\n\nAlternative SHA-256: `%s`\n\nSource revision: `%s`\n\nFocus: `%s`\n\n", inputPath, inputHash, outputPath, outputHash, source.Revision, focus)
	fmt.Fprintf(&b, "Family root: `%s`. The input above is the immediate predecessor of this alternative.\n\n", family)
	fmt.Fprintf(&b, "## Source inspection\n\nInspected: %s. The checked-out parser and validator are the static authority.\n\nSupported keys: %s\n\n", strings.Join(source.Files, ", "), source.Keys)
	fmt.Fprintf(&b, "## Historical completed sessions (%d)\n\n", len(sessions))
	if len(sessions) == 0 {
		b.WriteString("No completed relevant historical sessions were available.\n\n")
	}
	for _, e := range sessions {
		fmt.Fprintf(&b, "- `%s` (`%s`): %s; %s; %d snapshots; steps %s; models %s; %s.\n", e.ID, e.Path, e.Match, e.Outcome, e.Snapshots, strings.Join(e.Steps, ", "), strings.Join(e.Models, ", "), e.Metrics)
	}
	b.WriteString("\n## Findings\n\n")
	for _, f := range findings {
		fmt.Fprintf(&b, "- **%s** (%s; confidence %s): %s Evidence: %s. Hypothesis: %s. Remedy: %s.\n", f.ID, f.Category, f.Confidence, redact(f.Observed), strings.Join(f.Evidence, ", "), redact(f.Hypothesis), redact(f.Remedy))
	}
	b.WriteString("\n## Actual change manifest\n\n")
	if len(manifest) == 0 {
		b.WriteString("- No structural or field changes detected.\n")
	}
	for _, change := range manifest {
		fmt.Fprintf(&b, "- %s\n", change)
	}
	b.WriteString("\n## Expected effects\n\n")
	for _, effect := range hypotheses {
		fmt.Fprintf(&b, "- %s — hypothesis, not measured improvement.\n", redact(effect))
	}
	if strings.HasPrefix(outputPath, "(dry-run;") {
		b.WriteString("\n## Static validation\n\n- The input pipeline passed strict YAML decoding, existing pipeline validation, references, and objective checks.\n- Candidate generation was skipped; there is no alternative YAML to execute.\n\n")
	} else {
		b.WriteString("\n## Static validation\n\n- Strict YAML decoding, existing pipeline validation, references, and original objective fields passed.\n- Candidate was not executed or benchmarked. Semantic equivalence requires manual review.\n\n")
	}
	for _, warning := range warnings {
		fmt.Fprintf(&b, "- Warning: %s\n", warning)
	}
	if strings.HasPrefix(outputPath, "(dry-run;") {
		b.WriteString("\n## Next step\n\nRun the optimizer again without `--dry-run` when the local optimizer model is available. That run will generate and statically validate one alternative YAML and its report.\n")
	} else {
		b.WriteString("\n## Manual test\n\nRun the alternative with `induction --pipeline <alternative path>`. Compare final output quality, failures, step snapshots, token counts, and elapsed time against completed sessions for the same inputs. Treat any quality regression or new failure as a failed experiment. Then optimize the alternative using its own completed sessions.\n\n**Not executed or performance-tested; run manually to evaluate.**\n")
	}
	return b.String()
}

func describeChanges(original, candidate *induction.Pipeline) []string {
	prior := map[string]induction.PipelineStep{}
	for _, step := range original.Steps {
		prior[step.Name] = step
	}
	remaining := map[string]bool{}
	for name := range prior {
		remaining[name] = true
	}
	var changes []string
	for _, step := range candidate.Steps {
		before, exists := prior[step.Name]
		if !exists {
			changes = append(changes, fmt.Sprintf("Added step `%s`.", step.Name))
			continue
		}
		delete(remaining, step.Name)
		var fields []string
		if before.UserPrompt != step.UserPrompt || before.SystemPrompt != step.SystemPrompt {
			fields = append(fields, "prompts")
		}
		if before.Model != step.Model {
			fields = append(fields, "model")
		}
		if !reflect.DeepEqual(before.Parameters, step.Parameters) || !reflect.DeepEqual(before.Temperature, step.Temperature) {
			fields = append(fields, "parameters")
		}
		if !reflect.DeepEqual(before.Retry, step.Retry) || !reflect.DeepEqual(before.Quality, step.Quality) || !reflect.DeepEqual(before.Contract, step.Contract) {
			fields = append(fields, "validation/retry")
		}
		if !reflect.DeepEqual(before.Output, step.Output) || !reflect.DeepEqual(before.ResponseFormat, step.ResponseFormat) || !reflect.DeepEqual(before.JSONSchema, step.JSONSchema) {
			fields = append(fields, "output")
		}
		if !reflect.DeepEqual(before.When, step.When) || !reflect.DeepEqual(before.Decision, step.Decision) || before.ForEach != step.ForEach || before.As != step.As {
			fields = append(fields, "routing/references")
		}
		if len(fields) > 0 {
			changes = append(changes, fmt.Sprintf("Modified step `%s`: %s.", step.Name, strings.Join(fields, ", ")))
		}
	}
	for _, step := range original.Steps {
		if remaining[step.Name] {
			changes = append(changes, fmt.Sprintf("Removed step `%s`.", step.Name))
		}
	}
	return changes
}
