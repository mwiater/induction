package modelmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type EvalAuthority string

const (
	EvalAuthorityPrimary   EvalAuthority = "primary"
	EvalAuthoritySecondary EvalAuthority = "secondary"
)

type EvalRelationship string

const (
	EvalRelationshipExactRepo       EvalRelationship = "exact_repo"
	EvalRelationshipSameBaseModel   EvalRelationship = "same_base_model"
	EvalRelationshipDerivedFromSame EvalRelationship = "derived_from_same_base_model"
	EvalRelationshipNormalizedModel EvalRelationship = "normalized_model_identity"
)

type EvalManifest struct {
	SchemaVersion      int              `json:"schemaVersion"`
	ModelRepositoryID  string           `json:"modelRepositoryId"`
	SourceRepositoryID string           `json:"sourceRepositoryId"`
	Authority          EvalAuthority    `json:"authority"`
	Relationship       EvalRelationship `json:"relationship"`
	SourceRevision     string           `json:"sourceRevision"`
	BaseModelID        string           `json:"baseModelId,omitempty"`
	RetrievedAt        time.Time        `json:"retrievedAt"`
	SourceFiles        []EvalSourceFile `json:"sourceFiles"`
}
type EvalSourceFile struct {
	RemotePath string `json:"remotePath"`
	LocalPath  string `json:"localPath"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"sizeBytes"`
}
type NormalizedEvalBundle struct {
	SchemaVersion int                    `json:"schemaVersion"`
	SourceRepo    string                 `json:"sourceRepositoryId"`
	SourceRev     string                 `json:"sourceRevision"`
	Authority     EvalAuthority          `json:"authority"`
	Results       []NormalizedEvalResult `json:"results"`
}
type NormalizedEvalResult struct {
	BenchmarkID string         `json:"benchmarkId,omitempty"`
	TaskID      string         `json:"taskId,omitempty"`
	Metric      string         `json:"metric,omitempty"`
	Value       *float64       `json:"value,omitempty"`
	Split       string         `json:"split,omitempty"`
	Verified    *bool          `json:"verified,omitempty"`
	Date        string         `json:"date,omitempty"`
	SourceFile  string         `json:"sourceFile"`
	Raw         map[string]any `json:"raw,omitempty"`
}
type EvalUpdateStatus string

const (
	EvalStatusCreatedPrimary     EvalUpdateStatus = "created_primary"
	EvalStatusCreatedSecondary   EvalUpdateStatus = "created_secondary"
	EvalStatusRefreshedPrimary   EvalUpdateStatus = "refreshed_primary"
	EvalStatusRefreshedSecondary EvalUpdateStatus = "refreshed_secondary"
	EvalStatusPromoted           EvalUpdateStatus = "promoted"
	EvalStatusReplacedSecondary  EvalUpdateStatus = "replaced_secondary"
	EvalStatusRemoved            EvalUpdateStatus = "removed"
	EvalStatusUnchanged          EvalUpdateStatus = "unchanged"
	EvalStatusNotFound           EvalUpdateStatus = "not_found"
)

type EvalUpdateResult struct {
	ModelRepositoryID  string           `json:"modelRepositoryId"`
	Status             EvalUpdateStatus `json:"status"`
	Authority          EvalAuthority    `json:"authority,omitempty"`
	SourceRepositoryID string           `json:"sourceRepositoryId,omitempty"`
	SourceRevision     string           `json:"sourceRevision,omitempty"`
	Relationship       EvalRelationship `json:"relationship,omitempty"`
	ResultCount        int              `json:"resultCount,omitempty"`
}
type EvalUpdateError struct {
	ModelRepositoryID string `json:"modelRepositoryId"`
	Error             string `json:"error"`
}
type EvalUpdateSummary struct {
	SchemaVersion int                `json:"schemaVersion"`
	Scanned       int                `json:"scanned"`
	Updated       int                `json:"updated"`
	Unchanged     int                `json:"unchanged"`
	NotFound      int                `json:"notFound"`
	Removed       int                `json:"removed"`
	Failed        int                `json:"failed"`
	Results       []EvalUpdateResult `json:"results"`
	Errors        []EvalUpdateError  `json:"errors,omitempty"`
}
type DiscoveredEval struct {
	SourceRepositoryID string
	SourceRevision     string
	Authority          EvalAuthority
	Relationship       EvalRelationship
	BaseModelID        string
	Files              []ModelFile
	Rank               int
}
type EvalDestination struct{ ModelDirectory, EvalDirectory, Manifest, Normalized, SourceDir string }

const evalSearchLimit = 20
const maxEvalFileSize int64 = 16 << 20
const maxEvalBundleSize int64 = 64 << 20

// EvalProgressFunc receives human-readable progress events during a refresh.
// Events are deliberately plain text so callers can render them for a
// terminal without coupling the model manager to a UI framework.
type EvalProgressFunc func(string)

func EvalFiles(files []ModelFile) []ModelFile {
	out := make([]ModelFile, 0)
	for _, file := range files {
		if _, err := safeEvalRemotePath(file.Path); err == nil && strings.HasPrefix(filepath.ToSlash(file.Path), ".eval_results/") && (strings.EqualFold(filepath.Ext(file.Path), ".yaml") || strings.EqualFold(filepath.Ext(file.Path), ".yml")) {
			out = append(out, file)
		}
	}
	sort.Slice(out, func(i, j int) bool { return filepath.ToSlash(out[i].Path) < filepath.ToSlash(out[j].Path) })
	return out
}

func safeEvalRemotePath(path string) (string, error) {
	path = filepath.ToSlash(path)
	if path == ".eval_results" || !strings.HasPrefix(path, ".eval_results/") || filepath.IsAbs(path) {
		return "", fmt.Errorf("unsafe eval path %q", path)
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == ".eval_results" || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." || strings.Contains(path, "//") {
		return "", fmt.Errorf("unsafe eval path %q", path)
	}
	return clean, nil
}

func ResolveEvalDestination(modelsPath, repositoryID string) (EvalDestination, error) {
	parts := strings.Split(repositoryID, "/")
	if len(parts) != 2 || !safeComponent(parts[0]) || !safeComponent(parts[1]) {
		return EvalDestination{}, fmt.Errorf("repository ID must contain exactly two safe components")
	}
	base, err := filepath.Abs(modelsPath)
	if err != nil {
		return EvalDestination{}, err
	}
	dir := filepath.Join(base, parts[0], parts[1])
	if err := confined(base, dir); err != nil {
		return EvalDestination{}, err
	}
	eval := filepath.Join(dir, "evals")
	return EvalDestination{ModelDirectory: dir, EvalDirectory: eval, Manifest: filepath.Join(eval, "manifest.json"), Normalized: filepath.Join(eval, "normalized.json"), SourceDir: filepath.Join(eval, "source")}, nil
}

func ReadEvalManifest(modelsPath, repositoryID string) (*EvalManifest, error) {
	d, err := ResolveEvalDestination(modelsPath, repositoryID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(d.Manifest)
	if err != nil {
		return nil, err
	}
	var manifest EvalManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode eval manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.ModelRepositoryID != repositoryID {
		return nil, errors.New("invalid eval manifest")
	}
	return &manifest, nil
}

func NormalizeModelIdentity(repoName string) string {
	name := strings.ToLower(strings.TrimSpace(repoName))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.NewReplacer("_", "-", ".", "-", " ", "-").Replace(name)
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")
	name = strings.TrimSuffix(name, "-gguf")
	name = strings.TrimSuffix(name, "-ggml")
	if reQuant.MatchString(name) {
		name = reQuant.ReplaceAllString(name, "")
	}
	name = strings.TrimSuffix(name, "-gguf")
	name = strings.TrimSuffix(name, "-ggml")
	return strings.Trim(name, "-")
}

var reQuant = mustEvalRegexp(`(?i)-(q[0-9]+(?:-[a-z0-9]+)*|iq[0-9]+(?:-[a-z0-9]+)*)$`)

func mustEvalRegexp(pattern string) *regexp.Regexp { return regexp.MustCompile(pattern) }

func DiscoverPrimary(ctx context.Context, client EvalHubClient, repoID string) (*DiscoveredEval, error) {
	revision, files, err := client.ListFiles(ctx, repoID)
	if err != nil {
		return nil, err
	}
	evals := EvalFiles(files)
	if len(evals) == 0 {
		return nil, nil
	}
	return &DiscoveredEval{SourceRepositoryID: repoID, SourceRevision: revision, Authority: EvalAuthorityPrimary, Relationship: EvalRelationshipExactRepo, Files: evals}, nil
}

func DiscoverSecondary(ctx context.Context, client EvalHubClient, repoID string) (*DiscoveredEval, error) {
	installed, err := client.ModelMetadata(ctx, repoID)
	if err != nil {
		return nil, err
	}
	query := evalSearchQuery(repoName(repoID))
	if query == "" {
		return nil, nil
	}
	results, err := client.Search(ctx, query, evalSearchLimit, "")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	candidates := make([]SearchResult, 0, len(results)+len(installed.BaseModels))
	for _, r := range results {
		if !seen[strings.ToLower(r.ID)] {
			seen[strings.ToLower(r.ID)] = true
			candidates = append(candidates, r)
		}
	}
	for _, base := range installed.BaseModels {
		if !seen[strings.ToLower(base)] {
			seen[strings.ToLower(base)] = true
			candidates = append(candidates, SearchResult{ID: base})
		}
	}
	var best *DiscoveredEval
	for _, candidate := range candidates {
		if sameRepo(candidate.ID, repoID) || candidate.Private || candidate.Gated {
			continue
		}
		metadata, metaErr := client.ModelMetadata(ctx, candidate.ID)
		if metaErr != nil {
			continue
		}
		rel, baseID := relationshipWithLineage(ctx, client, installed, metadata)
		if rel == "" {
			continue
		}
		revision, files, listErr := client.ListFiles(ctx, candidate.ID)
		if listErr != nil {
			continue
		}
		files = EvalFiles(files)
		if len(files) == 0 {
			continue
		}
		item := &DiscoveredEval{SourceRepositoryID: candidate.ID, SourceRevision: revision, Authority: EvalAuthoritySecondary, Relationship: rel, BaseModelID: baseID, Files: files}
		item.Rank = relationshipRank(rel)
		if best == nil || betterEval(item, best, candidate, results) {
			best = item
		}
	}
	return best, nil
}

func repoName(id string) string {
	parts := strings.Split(id, "/")
	if len(parts) == 2 {
		return parts[1]
	}
	return id
}

// evalSearchQuery removes packaging decorations while preserving model
// punctuation such as the dot in GLM-4.7, which improves Hub search recall.
func evalSearchQuery(name string) string {
	name = strings.TrimSpace(name)
	for _, suffix := range []string{".gguf", ".ggml", "-gguf", "-ggml", "_gguf", "_ggml"} {
		if len(name) >= len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix) {
			name = name[:len(name)-len(suffix)]
			break
		}
	}
	name = reEvalQuantSuffix.ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, "_", "-")
	return strings.Trim(name, "-_. ")
}

var reEvalQuantSuffix = regexp.MustCompile(`(?i)(?:[-_.](?:q[0-9]+|iq[0-9]+)(?:[-_.][a-z0-9]+)*)$`)

func sameRepo(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }
func relationship(installed, candidate ModelMetadata) (EvalRelationship, string) {
	if len(installed.BaseModels) == 1 && len(candidate.BaseModels) == 1 && sameRepo(installed.BaseModels[0], candidate.BaseModels[0]) {
		return EvalRelationshipSameBaseModel, installed.BaseModels[0]
	}
	for _, base := range installed.BaseModels {
		if sameRepo(base, candidate.ID) {
			return EvalRelationshipDerivedFromSame, base
		}
	}
	if len(installed.BaseModels) == 0 && len(candidate.BaseModels) == 0 && NormalizeModelIdentity(repoName(installed.ID)) == NormalizeModelIdentity(repoName(candidate.ID)) {
		if (installed.Architecture != "" && installed.Architecture == candidate.Architecture) || (installed.ModelFamily != "" && installed.ModelFamily == candidate.ModelFamily) || sharedModelTag(installed.Tags, candidate.Tags) {
			return EvalRelationshipNormalizedModel, ""
		}
	}
	return "", ""
}

func sharedModelTag(left, right []string) bool {
	generic := map[string]bool{"transformers": true, "text-generation": true, "conversational": true, "license:mit": true, "safetensors": true, "gguf": true}
	seen := map[string]bool{}
	for _, tag := range left {
		seen[strings.ToLower(tag)] = true
	}
	for _, tag := range right {
		key := strings.ToLower(tag)
		if seen[key] && !generic[key] {
			return true
		}
	}
	return false
}

func relationshipWithLineage(ctx context.Context, client EvalHubClient, installed, candidate ModelMetadata) (EvalRelationship, string) {
	if rel, base := relationship(installed, candidate); rel != "" {
		return rel, base
	}
	installedRoots := lineageRoots(ctx, client, installed, 3, map[string]bool{})
	candidateRoots := lineageRoots(ctx, client, candidate, 3, map[string]bool{})
	for _, left := range installedRoots {
		for _, right := range candidateRoots {
			if sameRepo(left, right) {
				return EvalRelationshipDerivedFromSame, left
			}
		}
	}
	return "", ""
}

func lineageRoots(ctx context.Context, client EvalHubClient, metadata ModelMetadata, depth int, visited map[string]bool) []string {
	if depth <= 0 || len(metadata.BaseModels) == 0 {
		if metadata.ID != "" {
			return []string{metadata.ID}
		}
		return nil
	}
	var roots []string
	for _, base := range metadata.BaseModels {
		if visited[strings.ToLower(base)] {
			continue
		}
		visited[strings.ToLower(base)] = true
		next, err := client.ModelMetadata(ctx, base)
		if err != nil {
			roots = append(roots, base)
			continue
		}
		roots = append(roots, lineageRoots(ctx, client, next, depth-1, visited)...)
	}
	return roots
}
func relationshipRank(r EvalRelationship) int {
	switch r {
	case EvalRelationshipSameBaseModel:
		return 3
	case EvalRelationshipDerivedFromSame:
		return 2
	case EvalRelationshipNormalizedModel:
		return 1
	}
	return 0
}
func betterEval(item, best *DiscoveredEval, candidate SearchResult, all []SearchResult) bool {
	if item.Rank != best.Rank {
		return item.Rank > best.Rank
	}
	var old SearchResult
	for _, r := range all {
		if sameRepo(r.ID, best.SourceRepositoryID) {
			old = r
			break
		}
	}
	if candidate.Downloads != old.Downloads {
		return candidate.Downloads > old.Downloads
	}
	if candidate.Likes != old.Likes {
		return candidate.Likes > old.Likes
	}
	if !candidate.LastModified.Equal(old.LastModified) {
		return candidate.LastModified.After(old.LastModified)
	}
	return candidate.ID < best.SourceRepositoryID
}

func ReconcileEvals(ctx context.Context, client EvalHubClient, modelsPath, repoID string) (EvalUpdateResult, error) {
	return ReconcileEvalsWithProgress(ctx, client, modelsPath, repoID, nil)
}

// ReconcileEvalsWithProgress applies the same reconciliation policy as
// ReconcileEvals while reporting its current phase to the supplied callback.
func ReconcileEvalsWithProgress(ctx context.Context, client EvalHubClient, modelsPath, repoID string, progress EvalProgressFunc) (EvalUpdateResult, error) {
	notify := func(message string) {
		if progress != nil {
			progress(message)
		}
	}
	notify("reading current eval state")
	current, currentErr := ReadEvalManifest(modelsPath, repoID)
	if currentErr != nil && !errors.Is(currentErr, os.ErrNotExist) {
		return EvalUpdateResult{ModelRepositoryID: repoID}, currentErr
	}
	notify("checking primary repository")
	primary, err := DiscoverPrimary(ctx, client, repoID)
	if err != nil {
		return EvalUpdateResult{ModelRepositoryID: repoID}, err
	}
	if primary != nil {
		notify(fmt.Sprintf("primary found (%d eval file%s)", len(primary.Files), plural(len(primary.Files))))
		return activateEval(ctx, client, modelsPath, repoID, current, *primary, progress)
	}
	notify("primary not found; discovering secondary candidates")
	secondary, err := DiscoverSecondary(ctx, client, repoID)
	if err != nil {
		return EvalUpdateResult{ModelRepositoryID: repoID}, err
	}
	if secondary != nil {
		notify(fmt.Sprintf("secondary selected: %s (%d eval file%s)", secondary.SourceRepositoryID, len(secondary.Files), plural(len(secondary.Files))))
		return activateEval(ctx, client, modelsPath, repoID, current, *secondary, progress)
	}
	notify("no eligible evaluation source found")
	if current != nil {
		notify("removing stale eval bundle")
		if err := RemoveEvalBundleAtomic(modelsPath, repoID); err != nil {
			return EvalUpdateResult{ModelRepositoryID: repoID}, err
		}
		return EvalUpdateResult{ModelRepositoryID: repoID, Status: EvalStatusRemoved}, nil
	}
	return EvalUpdateResult{ModelRepositoryID: repoID, Status: EvalStatusNotFound}, nil
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func activateEval(ctx context.Context, client EvalHubClient, modelsPath, repoID string, current *EvalManifest, found DiscoveredEval, progress EvalProgressFunc) (EvalUpdateResult, error) {
	result := EvalUpdateResult{ModelRepositoryID: repoID, Authority: found.Authority, SourceRepositoryID: found.SourceRepositoryID, SourceRevision: found.SourceRevision, Relationship: found.Relationship}
	if current != nil && current.SourceRepositoryID == found.SourceRepositoryID && current.SourceRevision == found.SourceRevision && current.Authority == found.Authority && current.Relationship == found.Relationship && sameFiles(current.SourceFiles, found.Files) {
		result.Status = EvalStatusUnchanged
		result.ResultCount = normalizedCount(modelsPath, repoID)
		return result, nil
	}
	count, err := stageEval(ctx, client, modelsPath, repoID, found, progress)
	if err != nil {
		return EvalUpdateResult{ModelRepositoryID: repoID}, err
	}
	result.ResultCount = count
	if current == nil {
		if found.Authority == EvalAuthorityPrimary {
			result.Status = EvalStatusCreatedPrimary
		} else {
			result.Status = EvalStatusCreatedSecondary
		}
	} else if current.Authority == EvalAuthoritySecondary && found.Authority == EvalAuthorityPrimary {
		result.Status = EvalStatusPromoted
	} else if current.Authority == EvalAuthoritySecondary {
		result.Status = EvalStatusReplacedSecondary
	} else if found.Authority == EvalAuthorityPrimary {
		result.Status = EvalStatusRefreshedPrimary
	} else {
		result.Status = EvalStatusRefreshedSecondary
	}
	return result, nil
}

func sameFiles(current []EvalSourceFile, remote []ModelFile) bool {
	if len(current) != len(remote) {
		return false
	}
	for i, f := range remote {
		if filepath.ToSlash(strings.TrimPrefix(current[i].RemotePath, ".eval_results/")) != filepath.ToSlash(strings.TrimPrefix(f.Path, ".eval_results/")) {
			return false
		}
	}
	return true
}

func stageEval(ctx context.Context, client EvalHubClient, modelsPath, repoID string, found DiscoveredEval, progress EvalProgressFunc) (int, error) {
	notify := func(message string) {
		if progress != nil {
			progress(message)
		}
	}
	notify("creating staging directory")
	d, err := ResolveEvalDestination(modelsPath, repoID)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(d.ModelDirectory, 0o755); err != nil {
		return 0, err
	}
	stage, err := os.MkdirTemp(d.ModelDirectory, ".evals-staging-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(stage)
	source := filepath.Join(stage, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		return 0, err
	}
	manifest := EvalManifest{SchemaVersion: 1, ModelRepositoryID: repoID, SourceRepositoryID: found.SourceRepositoryID, Authority: found.Authority, Relationship: found.Relationship, SourceRevision: found.SourceRevision, BaseModelID: found.BaseModelID, RetrievedAt: time.Now().UTC()}
	var total int64
	for index, file := range found.Files {
		notify(fmt.Sprintf("downloading eval file %d/%d: %s", index+1, len(found.Files), file.Path))
		rel, _ := safeEvalRemotePath(file.Path)
		rel = strings.TrimPrefix(filepath.ToSlash(rel), ".eval_results/")
		local, err := client.DownloadFile(ctx, found.SourceRepositoryID, found.SourceRevision, file.Path, source)
		if err != nil {
			return 0, err
		}
		desired := filepath.Join(source, filepath.FromSlash(rel))
		relLocal, relErr := filepath.Rel(source, local)
		if relErr != nil || relLocal == ".." || strings.HasPrefix(relLocal, ".."+string(filepath.Separator)) {
			return 0, errors.New("downloaded eval escaped staging directory")
		}
		if local != desired {
			if err := os.MkdirAll(filepath.Dir(desired), 0o755); err != nil {
				return 0, err
			}
			if err := os.Rename(local, desired); err != nil {
				return 0, err
			}
		}
		local = desired
		info, err := os.Stat(local)
		if err != nil {
			return 0, err
		}
		if info.Size() > maxEvalFileSize {
			return 0, errors.New("evaluation file exceeds size limit")
		}
		total += info.Size()
		if total > maxEvalBundleSize {
			return 0, errors.New("evaluation bundle exceeds size limit")
		}
		hash, err := HashFile(ctx, local)
		if err != nil {
			return 0, err
		}
		manifest.SourceFiles = append(manifest.SourceFiles, EvalSourceFile{RemotePath: file.Path, LocalPath: filepath.ToSlash(filepath.Join("source", rel)), SHA256: hash, SizeBytes: info.Size()})
	}
	notify("normalizing evaluation results")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), append(data, '\n'), 0o644); err != nil {
		return 0, err
	}
	normalized, err := normalizeEvalFiles(source, manifest.SourceFiles, found)
	if err != nil {
		return 0, err
	}
	normData, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(stage, "normalized.json"), append(normData, '\n'), 0o644); err != nil {
		return 0, err
	}
	notify("activating eval bundle atomically")
	if err := activateEvalDir(d.EvalDirectory, stage); err != nil {
		return 0, err
	}
	return len(normalized.Results), nil
}

func activateEvalDir(live, stage string) error {
	backup := live + ".backup"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(live); err == nil {
		if err := os.Rename(live, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, live); err != nil {
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, live)
		}
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}
func RemoveEvalBundleAtomic(modelsPath, repoID string) error {
	d, err := ResolveEvalDestination(modelsPath, repoID)
	if err != nil {
		return err
	}
	if err := confined(d.ModelDirectory, d.EvalDirectory); err != nil {
		return err
	}
	return os.RemoveAll(d.EvalDirectory)
}
func normalizedCount(modelsPath, repoID string) int {
	d, err := ResolveEvalDestination(modelsPath, repoID)
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(d.Normalized)
	if err != nil {
		return 0
	}
	var n NormalizedEvalBundle
	if json.Unmarshal(data, &n) != nil {
		return 0
	}
	return len(n.Results)
}

func normalizeEvalFiles(source string, files []EvalSourceFile, found DiscoveredEval) (NormalizedEvalBundle, error) {
	out := NormalizedEvalBundle{SchemaVersion: 1, SourceRepo: found.SourceRepositoryID, SourceRev: found.SourceRevision, Authority: found.Authority}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(source, strings.TrimPrefix(file.LocalPath, "source/")))
		if err != nil {
			return out, err
		}
		var raw any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return out, fmt.Errorf("parse %s: %w", file.RemotePath, err)
		}
		normalizeAny(raw, file.RemotePath, &out.Results)
	}
	sort.Slice(out.Results, func(i, j int) bool {
		a, b := out.Results[i], out.Results[j]
		return strings.Join([]string{a.BenchmarkID, a.TaskID, a.Metric, a.SourceFile}, "\x00") < strings.Join([]string{b.BenchmarkID, b.TaskID, b.Metric, b.SourceFile}, "\x00")
	})
	return out, nil
}
func normalizeAny(value any, source string, results *[]NormalizedEvalResult) {
	switch v := value.(type) {
	case map[string]any:
		if val, ok := number(v["value"]); ok {
			r := NormalizedEvalResult{SourceFile: source, Raw: v, Value: &val}
			r.BenchmarkID = stringValue(v, "benchmark", "benchmarkId", "name")
			r.TaskID = stringValue(v, "task", "taskId")
			r.Metric = stringValue(v, "metric", "metricName")
			r.Split = stringValue(v, "split")
			r.Date = stringValue(v, "date")
			if b, ok := v["verified"].(bool); ok {
				r.Verified = &b
			}
			*results = append(*results, r)
		} else {
			if raw, ok := v["results"]; ok {
				normalizeAny(raw, source, results)
			} else if raw, ok := v["result"]; ok {
				normalizeAny(raw, source, results)
			} else {
				for _, item := range v {
					normalizeAny(item, source, results)
				}
			}
		}
	case []any:
		for _, item := range v {
			normalizeAny(item, source, results)
		}
	}
}
func stringValue(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok {
			return s
		}
	}
	return ""
}
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
func HashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
