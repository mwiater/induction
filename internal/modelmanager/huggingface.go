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
}

// NewHFCLIClient locates hf and returns actionable installation guidance.
func NewHFCLIClient() (*HFCLIClient, error) {
	path, err := exec.LookPath("hf")
	if err != nil {
		return nil, errors.New("Hugging Face CLI 'hf' was not found; install it with 'pip install -U huggingface_hub' and ensure hf is on PATH")
	}
	return &HFCLIClient{Path: path, HTTPClient: http.DefaultClient, APIBaseURL: "https://huggingface.co"}, nil
}

type hubModel struct {
	ID           string    `json:"id"`
	ModelID      string    `json:"modelId"`
	SHA          string    `json:"sha"`
	Author       string    `json:"author"`
	Downloads    int64     `json:"downloads"`
	Likes        int64     `json:"likes"`
	LastModified time.Time `json:"lastModified"`
	Gated        any       `json:"gated"`
	Private      bool      `json:"private"`
	Siblings     []struct {
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

// ListFiles returns the immutable revision and downloadable files for modelID.
func (c *HFCLIClient) ListFiles(ctx context.Context, modelID string) (string, []ModelFile, error) {
	cmd := exec.CommandContext(ctx, c.Path, "models", "info", modelID, "--format", "json")
	cmd.Env = os.Environ()
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
	if token := os.Getenv("HF_TOKEN"); token != "" {
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
