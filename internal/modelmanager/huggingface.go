package modelmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SearchResult is a stable project-owned representation of a Hub repository.
type SearchResult struct {
	SchemaVersion int       `json:"schemaVersion"`
	ID            string    `json:"id"`
	Revision      string    `json:"revision,omitempty"`
	Provider      string    `json:"provider"`
	Downloads     int64     `json:"downloads"`
	Likes         int64     `json:"likes"`
	LastModified  time.Time `json:"lastModified,omitempty"`
	Gated         bool      `json:"gated"`
	Private       bool      `json:"private"`
}

// HubClient abstracts Hugging Face access for deterministic tests.
type HubClient interface {
	Search(ctx context.Context, query string, limit int, provider string) ([]SearchResult, error)
	ListFiles(ctx context.Context, modelID string) (revision string, files []ModelFile, err error)
}

// EvalHubClient is the Hub surface required by evaluation discovery. It is
// intentionally separate so existing model-search callers and test doubles
// do not need to implement evaluation-specific operations.
type EvalHubClient interface {
	Search(ctx context.Context, query string, limit int, provider string) ([]SearchResult, error)
	ListFiles(ctx context.Context, modelID string) (revision string, files []ModelFile, err error)
	ModelMetadata(ctx context.Context, modelID string) (ModelMetadata, error)
	DownloadFile(ctx context.Context, modelID, revision, remotePath, localDir string) (string, error)
}

// ModelMetadata contains the identity signals used for safe secondary eval
// inheritance.
type ModelMetadata struct {
	ID                string    `json:"id"`
	Revision          string    `json:"revision,omitempty"`
	BaseModels        []string  `json:"baseModels,omitempty"`
	BaseModelRelation string    `json:"baseModelRelation,omitempty"`
	Downloads         int64     `json:"downloads,omitempty"`
	Likes             int64     `json:"likes,omitempty"`
	LastModified      time.Time `json:"lastModified,omitempty"`
	Private           bool      `json:"private,omitempty"`
	Gated             bool      `json:"gated,omitempty"`
	Architecture      string    `json:"architecture,omitempty"`
	ModelFamily       string    `json:"modelFamily,omitempty"`
	Tags              []string  `json:"tags,omitempty"`
}

// ModelFile describes one exact repository artifact.
type ModelFile struct {
	Path         string `json:"path"`
	Size         int64  `json:"sizeBytes"`
	ETag         string `json:"etag,omitempty"`
	LFSOID       string `json:"lfsOid,omitempty"`
	Quantization string `json:"quantization,omitempty"`
}

// HFCLIClient invokes the modern Hugging Face hf executable.
type HFCLIClient struct {
	Path       string
	HTTPClient *http.Client
	APIBaseURL string
	Token      string
}

// NewHFCLIClient locates hf and returns actionable installation guidance.
func NewHFCLIClient(configuredToken ...string) (*HFCLIClient, error) {
	path, err := exec.LookPath("hf")
	if err != nil {
		return nil, errors.New("Hugging Face CLI 'hf' was not found; install it with 'pip install -U huggingface_hub' and ensure hf is on PATH")
	}
	token := ""
	if len(configuredToken) > 0 {
		token = strings.TrimSpace(configuredToken[0])
	}
	return &HFCLIClient{Path: path, HTTPClient: http.DefaultClient, APIBaseURL: "https://huggingface.co", Token: token}, nil
}

func (c *HFCLIClient) commandEnv() []string {
	return commandEnvWithToken(c.Token)
}

func commandEnvWithToken(token string) []string {
	env := os.Environ()
	if token == "" {
		return env
	}
	for i, value := range env {
		if strings.HasPrefix(value, "HF_TOKEN=") {
			env[i] = "HF_TOKEN=" + token
			return env
		}
	}
	return append(env, "HF_TOKEN="+token)
}

func (c *HFCLIClient) authToken() string {
	if c.Token != "" {
		return c.Token
	}
	return os.Getenv("HF_TOKEN")
}

type hubModel struct {
	ID            string         `json:"id"`
	ModelID       string         `json:"modelId"`
	SHA           string         `json:"sha"`
	Author        string         `json:"author"`
	Downloads     int64          `json:"downloads"`
	Likes         int64          `json:"likes"`
	LastModified  time.Time      `json:"lastModified"`
	Gated         any            `json:"gated"`
	Private       bool           `json:"private"`
	BaseModel     any            `json:"base_model"`
	BaseModels    any            `json:"base_models"`
	BaseModelAlt  any            `json:"baseModel"`
	BaseModelsAlt any            `json:"baseModels"`
	CardData      map[string]any `json:"cardData"`
	Architecture  string         `json:"architecture"`
	ModelFamily   string         `json:"model_family"`
	Tags          []string       `json:"tags"`
	Siblings      []struct {
		RFilename string `json:"rfilename"`
		Path      string `json:"path"`
		Size      int64  `json:"size"`
		ETag      string `json:"etag"`
		LFS       *struct {
			OID  string `json:"oid"`
			Size int64  `json:"size"`
		} `json:"lfs"`
	} `json:"siblings"`
}

func (c *HFCLIClient) ModelMetadata(ctx context.Context, modelID string) (ModelMetadata, error) {
	cmd := exec.CommandContext(ctx, c.Path, "models", "info", modelID, "--format", "json")
	cmd.Env = c.commandEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	var raw hubModel
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ModelMetadata{}, ctx.Err()
		}
		if !unsupportedModelsCommand(stderr.String()) {
			return ModelMetadata{}, fmt.Errorf("hf model info failed: %s", sanitizeDiagnostic(stderr.String()))
		}
		parts := strings.Split(modelID, "/")
		if len(parts) != 2 {
			return ModelMetadata{}, fmt.Errorf("invalid model ID %q", modelID)
		}
		if err := c.getHubJSON(ctx, "/api/models/"+url.PathEscape(parts[0])+"/"+url.PathEscape(parts[1]), &raw); err != nil {
			return ModelMetadata{}, fmt.Errorf("Hugging Face model metadata fallback failed: %w", err)
		}
	} else if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return ModelMetadata{}, fmt.Errorf("decode hf model metadata JSON: %w", err)
	}
	id := raw.ID
	if id == "" {
		id = raw.ModelID
	}
	if id == "" {
		id = modelID
	}
	return ModelMetadata{ID: id, Revision: raw.SHA, BaseModels: extractBaseModels(raw.BaseModel, raw.BaseModels, raw.BaseModelAlt, raw.BaseModelsAlt, raw.CardData["base_model"], raw.CardData["baseModel"]), Downloads: raw.Downloads, Likes: raw.Likes, LastModified: raw.LastModified, Private: raw.Private, Gated: gatedValue(raw.Gated), Architecture: raw.Architecture, ModelFamily: raw.ModelFamily, Tags: raw.Tags}, nil
}

func (c *HFCLIClient) DownloadFile(ctx context.Context, modelID, revision, remotePath, localDir string) (string, error) {
	if revision == "" {
		return "", errors.New("immutable revision is required")
	}
	if _, err := safeEvalRemotePath(remotePath); err != nil {
		return "", err
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, c.Path, "download", modelID, remotePath, "--revision", revision, "--local-dir", localDir)
	cmd.Env = c.commandEnv()
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("hf eval download failed: %s", sanitizeDiagnostic(output.String()))
	}
	path := filepath.Join(localDir, filepath.FromSlash(remotePath))
	if err := confined(localDir, path); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("eval download completed but file is missing: %w", err)
	}
	return path, nil
}

// ListFiles returns the immutable revision and downloadable files for modelID.
func (c *HFCLIClient) ListFiles(ctx context.Context, modelID string) (string, []ModelFile, error) {
	cmd := exec.CommandContext(ctx, c.Path, "models", "info", modelID, "--format", "json")
	cmd.Env = c.commandEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		if unsupportedModelsCommand(stderr.String()) {
			return c.listFilesAPI(ctx, modelID)
		}
		return "", nil, fmt.Errorf("hf model info failed: %s", sanitizeDiagnostic(stderr.String()))
	}
	var raw hubModel
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return "", nil, fmt.Errorf("decode hf model info JSON: %w", err)
	}
	if raw.SHA == "" {
		return "", nil, errors.New("decode hf model info JSON: missing immutable revision")
	}
	files := make([]ModelFile, 0, len(raw.Siblings))
	for _, entry := range raw.Siblings {
		path := entry.RFilename
		if path == "" {
			path = entry.Path
		}
		if path == "" {
			continue
		}
		size := entry.Size
		oid := ""
		if entry.LFS != nil {
			oid = entry.LFS.OID
			if size == 0 {
				size = entry.LFS.Size
			}
		}
		files = append(files, ModelFile{Path: path, Size: size, ETag: entry.ETag, LFSOID: oid, Quantization: DetectQuantization(path)})
	}
	return raw.SHA, files, nil
}

// Search returns repositories matching query, optionally scoped to provider.
func (c *HFCLIClient) Search(ctx context.Context, query string, limit int, provider string) ([]SearchResult, error) {
	args := []string{"models", "ls", "--search", query, "--limit", fmt.Sprint(limit), "--format", "json"}
	if provider != "" {
		args = append(args, "--author", provider)
	}
	cmd := exec.CommandContext(ctx, c.Path, args...)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if unsupportedModelsCommand(stderr.String()) {
			return c.searchAPI(ctx, query, limit, provider)
		}
		return nil, fmt.Errorf("hf models search failed: %s", sanitizeDiagnostic(stderr.String()))
	}
	var raw []hubModel
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("decode hf model search JSON: %w", err)
	}
	results := make([]SearchResult, 0, len(raw))
	for _, item := range raw {
		id := item.ID
		if id == "" {
			id = item.ModelID
		}
		if id == "" {
			return nil, errors.New("decode hf model search JSON: result is missing id")
		}
		author := item.Author
		if author == "" {
			author, _, _ = strings.Cut(id, "/")
		}
		gated := false
		switch value := item.Gated.(type) {
		case bool:
			gated = value
		case string:
			gated = value != "" && value != "false"
		}
		results = append(results, SearchResult{SchemaVersion: 1, ID: id, Revision: item.SHA, Provider: author, Downloads: item.Downloads, Likes: item.Likes, LastModified: item.LastModified, Gated: gated, Private: item.Private})
	}
	return results, nil
}

func unsupportedModelsCommand(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "invalid choice: 'models'") || strings.Contains(lower, "no such command 'models'") || strings.Contains(lower, "invalid choice: 'info'")
}

func (c *HFCLIClient) searchAPI(ctx context.Context, query string, limit int, provider string) ([]SearchResult, error) {
	values := url.Values{"search": {query}, "limit": {fmt.Sprint(limit)}, "full": {"true"}}
	if provider != "" {
		values.Set("author", provider)
	}
	var raw []hubModel
	if err := c.getHubJSON(ctx, "/api/models?"+values.Encode(), &raw); err != nil {
		return nil, fmt.Errorf("Hugging Face model search fallback failed: %w", err)
	}
	return decodeSearchResults(raw)
}

func (c *HFCLIClient) listFilesAPI(ctx context.Context, modelID string) (string, []ModelFile, error) {
	parts := strings.Split(modelID, "/")
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("invalid model ID %q", modelID)
	}
	var raw hubModel
	if err := c.getHubJSON(ctx, "/api/models/"+url.PathEscape(parts[0])+"/"+url.PathEscape(parts[1])+"?blobs=true", &raw); err != nil {
		return "", nil, fmt.Errorf("Hugging Face model info fallback failed: %w", err)
	}
	if raw.SHA == "" {
		return "", nil, errors.New("Hugging Face model info fallback returned no immutable revision")
	}
	return raw.SHA, decodeModelFiles(raw.Siblings), nil
}

func (c *HFCLIClient) getHubJSON(ctx context.Context, path string, target any) error {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(c.APIBaseURL, "/")
	if base == "" {
		base = "https://huggingface.co"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	if token := c.authToken(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Hub API returned %s", response.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 32<<20))
	if err = decoder.Decode(target); err != nil {
		return fmt.Errorf("decode Hub API JSON: %w", err)
	}
	return nil
}

func decodeModelFiles(entries []struct {
	RFilename string `json:"rfilename"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	ETag      string `json:"etag"`
	LFS       *struct {
		OID  string `json:"oid"`
		Size int64  `json:"size"`
	} `json:"lfs"`
}) []ModelFile {
	files := make([]ModelFile, 0, len(entries))
	for _, entry := range entries {
		path := entry.RFilename
		if path == "" {
			path = entry.Path
		}
		if path == "" {
			continue
		}
		size := entry.Size
		oid := ""
		if entry.LFS != nil {
			oid = entry.LFS.OID
			if size == 0 {
				size = entry.LFS.Size
			}
		}
		files = append(files, ModelFile{Path: path, Size: size, ETag: entry.ETag, LFSOID: oid, Quantization: DetectQuantization(path)})
	}
	return files
}

func decodeSearchResults(raw []hubModel) ([]SearchResult, error) {
	results := make([]SearchResult, 0, len(raw))
	for _, item := range raw {
		id := item.ID
		if id == "" {
			id = item.ModelID
		}
		if id == "" {
			return nil, errors.New("decode hf model search JSON: result is missing id")
		}
		author := item.Author
		if author == "" {
			author, _, _ = strings.Cut(id, "/")
		}
		gated := false
		switch value := item.Gated.(type) {
		case bool:
			gated = value
		case string:
			gated = value != "" && value != "false"
		}
		results = append(results, SearchResult{SchemaVersion: 1, ID: id, Revision: item.SHA, Provider: author, Downloads: item.Downloads, Likes: item.Likes, LastModified: item.LastModified, Gated: gated, Private: item.Private})
	}
	return results, nil
}

func sanitizeDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	for _, marker := range []string{"HF_TOKEN=", "token=", "Bearer "} {
		if at := strings.Index(strings.ToLower(value), strings.ToLower(marker)); at >= 0 {
			end := strings.IndexAny(value[at+len(marker):], " \t\r\n&")
			if end < 0 {
				end = len(value) - at - len(marker)
			}
			value = value[:at+len(marker)] + "[REDACTED]" + value[at+len(marker)+end:]
		}
	}
	if len(value) > 500 {
		value = value[:500] + "…"
	}
	if value == "" {
		return "no diagnostic output"
	}
	return value
}

func gatedValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != "" && v != "false"
	default:
		return false
	}
}

func extractBaseModels(values ...any) []string {
	var result []string
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				result = append(result, strings.TrimSpace(v))
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		case map[string]any:
			for _, key := range []string{"id", "model_id", "modelId", "name"} {
				if item, ok := v[key]; ok {
					walk(item)
					return
				}
			}
		}
	}
	for _, value := range values {
		walk(value)
	}
	seen := map[string]bool{}
	out := result[:0]
	for _, item := range result {
		key := strings.ToLower(item)
		if !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}
