// Package cli defines Induction's command-line interface.
package cli

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"hash"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	induction "github.com/mwiater/induction"
)

// runApplicationTools performs the internal operation described by its signature.
func runApplicationTools(ctx context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, endpoint string, options ...induction.ClientOption) error {
	return induction.InferApplicationToolsChat(ctx, withLocalTools(req), in, out, func(toolCtx context.Context, name, arguments string) (string, error) {
		return localToolResult(toolCtx, name, arguments, endpoint)
	}, append(options, induction.WithApplicationToolChain(chainLocalTools))...)
}

// applicationToolDefinition is an internal data type used by this package.
type applicationToolDefinition struct {
	// Name stores the value represented by this member.
	Name string
	// Description stores the value represented by this member.
	Description string
	// Parameters stores the value represented by this member.
	Parameters map[string]any
}

// applicationToolDefinitions performs the internal operation described by its signature.
func applicationToolDefinitions() []applicationToolDefinition {
	return []applicationToolDefinition{
		{Name: "get_current_datetime", Description: "Return the current system date and time.", Parameters: strictSchema()},
		{Name: "get_free_disk_space", Description: "Return free space on /.", Parameters: strictSchema()},
		{Name: "get_available_memory", Description: "Return available system memory.", Parameters: strictSchema()},
		{Name: "list_model_ids", Description: "Return the current model IDs from the llama.cpp /v1/models endpoint.", Parameters: strictSchema()},
		{Name: "list_model_profiles", Description: "Query the llama.cpp /v1/models endpoint and return enriched routing profiles with local paths, Hugging Face metadata, context capacity, vision support, parameter sizes, and architecture flags.", Parameters: strictSchema()},
		{Name: "read_text_file", Description: "Read a UTF-8 text file within the application workspace, subject to a byte limit.", Parameters: objectSchema(map[string]any{"path": map[string]any{"type": "string"}, "max_bytes": map[string]any{"type": "integer", "minimum": 1}})},
		{Name: "list_directory_contents", Description: "List files and directories at a workspace-relative path with basic metadata.", Parameters: objectSchema(map[string]any{"path": map[string]any{"type": "string"}})},
		{Name: "search_workspace", Description: "Search workspace text with ripgrep using a query, optional path and glob, and a result limit.", Parameters: objectSchema(map[string]any{"query": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}, "glob": map[string]any{"type": "string"}, "max_results": map[string]any{"type": "integer", "minimum": 1}})},
		{Name: "get_file_metadata", Description: "Return size, modification time, permissions, type, and workspace-relative path for a file.", Parameters: objectSchema(map[string]any{"path": map[string]any{"type": "string"}})},
		{Name: "read_json_file", Description: "Read a JSON file within the workspace, optionally returning a top-level property.", Parameters: objectSchema(map[string]any{"path": map[string]any{"type": "string"}, "property": map[string]any{"type": "string"}})},
		{Name: "validate_json", Description: "Validate JSON content or a JSON file and return parse status with an error offset when invalid.", Parameters: objectSchema(map[string]any{"content": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}})},
		{Name: "hash_file", Description: "Return a digest for a workspace file using sha256, sha512, sha1, or md5.", Parameters: objectSchema(map[string]any{"path": map[string]any{"type": "string"}, "algorithm": map[string]any{"type": "string", "enum": []string{"sha256", "sha512", "sha1", "md5"}}})},
		{Name: "parse_go_ast", Description: "Parse one or more workspace Go files and return their abstract syntax tree representations.", Parameters: objectSchema(map[string]any{"file": map[string]any{"type": "string"}, "files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}})},
	}
}

// withLocalTools performs the internal operation described by its signature.
func withLocalTools(req *induction.ChatRequest) *induction.ChatRequest {
	requestCopy := *req
	requestCopy.Tools = make([]induction.Tool, 0, len(applicationToolDefinitions()))
	for _, definition := range applicationToolDefinitions() {
		requestCopy.Tools = append(requestCopy.Tools, induction.Tool{Type: "function", Function: induction.ToolFunction{Name: definition.Name, Description: definition.Description, Parameters: definition.Parameters}})
	}
	requestCopy.ToolChoice = "auto"
	return &requestCopy
}

// strictSchema performs the internal operation described by its signature.
func strictSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
}

// objectSchema performs the internal operation described by its signature.
func objectSchema(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
}

// chainLocalTools performs the internal operation described by its signature.
func chainLocalTools(c []induction.InferenceToolCall) []induction.InferenceToolCall {
	for _, x := range c {
		if x.Function.Name == "get_current_datetime" {
			return c
		}
	}
	if len(c) > 0 {
		return append(c, induction.InferenceToolCall{ID: "autotime1", Type: "function", Function: induction.InferenceFunctionCall{Name: "get_current_datetime", Arguments: "{}"}})
	}
	return c
}

// localToolResult performs the internal operation described by its signature.
func localToolResult(ctx context.Context, name, arguments, endpoint string) (string, error) {
	switch name {
	case "get_current_datetime":
		return jsonResult(map[string]any{"date_time": time.Now().Format(time.RFC3339), "timezone": time.Now().Location().String()})
	case "get_free_disk_space":
		return freeDiskSpace(ctx)
	case "get_available_memory":
		return availableMemory(ctx)
	case "list_model_ids":
		return fetchModelIDs(ctx, endpoint, nil)
	case "list_model_profiles":
		return inspectAndProfileRuntimeModels(ctx, endpoint, nil)
	case "read_text_file":
		return readTextFile(arguments)
	case "list_directory_contents":
		return listDirectory(arguments)
	case "search_workspace":
		return searchWorkspace(ctx, arguments)
	case "get_file_metadata":
		return fileMetadata(arguments)
	case "read_json_file":
		return readJSONFile(arguments)
	case "validate_json":
		return validateJSON(arguments)
	case "hash_file":
		return hashFile(arguments)
	case "parse_go_ast":
		return goAST(arguments)
	default:
		return "", fmt.Errorf("unknown local tool %q", name)
	}
}

// freeDiskSpace performs the internal operation described by its signature.
func freeDiskSpace(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "df", "-Pk", "/").Output()
	if err != nil {
		return "", fmt.Errorf("read free disk space: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return "", errors.New("unexpected df output")
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 6 {
		return "", errors.New("unexpected df row")
	}
	blocks, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse disk size: %w", err)
	}
	used, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse disk usage: %w", err)
	}
	available, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse available disk space: %w", err)
	}
	percent, err := strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
	if err != nil {
		return "", fmt.Errorf("parse disk usage percent: %w", err)
	}
	return jsonResult(map[string]any{
		"filesystem":      fields[0],
		"total_bytes":     blocks * 1024,
		"used_bytes":      used * 1024,
		"available_bytes": available * 1024,
		"used_percent":    percent,
		"mount":           strings.Join(fields[5:], " "),
	})
}

// availableMemory performs the internal operation described by its signature.
func availableMemory(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "free", "-b").Output()
	if err != nil {
		return "", fmt.Errorf("read available memory: %w", err)
	}
	result := map[string]any{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || (fields[0] != "Mem:" && fields[0] != "Swap:") {
			continue
		}
		values := make([]int64, len(fields)-1)
		for i, field := range fields[1:] {
			value, parseErr := strconv.ParseInt(field, 10, 64)
			if parseErr != nil {
				return "", fmt.Errorf("parse memory value %q: %w", field, parseErr)
			}
			values[i] = value
		}
		prefix := strings.ToLower(strings.TrimSuffix(fields[0], ":"))
		if prefix == "mem" && len(values) >= 6 {
			result["total_bytes"] = values[0]
			result["used_bytes"] = values[1]
			result["free_bytes"] = values[2]
			result["shared_bytes"] = values[3]
			result["cache_bytes"] = values[4]
			result["available_bytes"] = values[5]
		}
		if prefix == "swap" && len(values) >= 3 {
			result["swap_total_bytes"] = values[0]
			result["swap_used_bytes"] = values[1]
			result["swap_free_bytes"] = values[2]
		}
	}
	if _, ok := result["total_bytes"]; !ok {
		return "", errors.New("unexpected free output")
	}
	return jsonResult(result)
}

// fetchModelIDs performs the internal operation described by its signature.
func fetchModelIDs(ctx context.Context, endpoint string, httpClient *http.Client) (string, error) {
	var options []induction.ClientOption
	if httpClient != nil {
		options = append(options, induction.WithHTTPClient(httpClient))
	}
	models, err := induction.NewClient(ctx, endpoint, options...).ListModelCatalog(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch model catalog from %s: %w", endpoint, err)
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		if model.ID != "" {
			ids = append(ids, model.ID)
		}
	}
	return jsonResult(ids)
}

// runtimeModelProfile is an internal data type used by this package.
type runtimeModelProfile struct {
	// ID stores the value represented by this member.
	ID string `json:"id"`
	// FamilySlug stores the value represented by this member.
	FamilySlug string `json:"family_slug"`
	// LocalPath stores the value represented by this member.
	LocalPath string `json:"local_path"`
	// HuggingFaceURL stores the value represented by this member.
	HuggingFaceURL string `json:"huggingface_url"`
	// HuggingFaceAuthor stores the value represented by this member.
	HuggingFaceAuthor string `json:"huggingface_author"`
	// HuggingFaceRepo stores the value represented by this member.
	HuggingFaceRepo string `json:"huggingface_repo"`
	// QuantizationFile stores the value represented by this member.
	QuantizationFile string `json:"quantization_file"`
	// ContextCapacity stores the value represented by this member.
	ContextCapacity int `json:"context_capacity"`
	// VisionSupported stores the value represented by this member.
	VisionSupported bool `json:"vision_supported"`
	// InferredParameters stores the value represented by this member.
	InferredParameters string `json:"inferred_parameters"`
	// ArchitectureFlags stores the value represented by this member.
	ArchitectureFlags []string `json:"architecture_flags"`
}

// inspectAndProfileRuntimeModels performs the internal operation described by its signature.
func inspectAndProfileRuntimeModels(ctx context.Context, endpoint string, httpClient *http.Client) (string, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/v1/models", http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create runtime model request: %w", err)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("query runtime models: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("query runtime models: unexpected status code: %d", response.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
			Architecture  struct {
				InputModalities []string `json:"input_modalities"`
			} `json:"architecture"`
			Status struct {
				Args   []string `json:"args"`
				Preset string   `json:"preset"`
			} `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode runtime models: %w", err)
	}

	profiles := make([]runtimeModelProfile, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID == "" {
			continue
		}
		family := canonicalModelFamily(model.ID)
		localPath := runtimeModelPath(model.Status.Args, model.Status.Preset)
		author, repo := huggingFaceRepository(localPath)
		flags := modelArchitectureFlags(model.ID)
		quantizationFile := ""
		if localPath != "" {
			quantizationFile = filepath.Base(localPath)
		}
		profiles = append(profiles, runtimeModelProfile{
			ID:                 model.ID,
			FamilySlug:         family,
			LocalPath:          localPath,
			HuggingFaceURL:     huggingFaceURL(author, repo),
			HuggingFaceAuthor:  author,
			HuggingFaceRepo:    repo,
			QuantizationFile:   quantizationFile,
			ContextCapacity:    model.ContextLength,
			VisionSupported:    containsString(model.Architecture.InputModalities, "image"),
			InferredParameters: inferredParameterSize(model.ID),
			ArchitectureFlags:  flags,
		})
	}
	return jsonResult(profiles)
}

// runtimeModelPath performs the internal operation described by its signature.
func runtimeModelPath(args []string, preset string) string {
	for i, arg := range args {
		if arg == "--model" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
	}
	for _, line := range strings.Split(preset, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "model") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != "model" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		return value
	}
	return ""
}

// huggingFaceRepository performs the internal operation described by its signature.
func huggingFaceRepository(localPath string) (author, repo string) {
	parts := strings.Split(filepath.ToSlash(localPath), "/")
	for i, part := range parts {
		if part == "models" && i+2 < len(parts) {
			return parts[i+1], parts[i+2]
		}
	}
	return "", ""
}

// huggingFaceURL performs the internal operation described by its signature.
func huggingFaceURL(author, repo string) string {
	if author == "" || repo == "" {
		return ""
	}
	return "https://huggingface.co/" + author + "/" + repo
}

// inferredParameterPattern holds package-level state used internally by this package.
var inferredParameterPattern = regexp.MustCompile(`(?i)(?:^|[-])([0-9]+(?:\.[0-9]+)?b)(?:-|$)`)

// inferredParameterSize performs the internal operation described by its signature.
func inferredParameterSize(id string) string {
	matches := inferredParameterPattern.FindAllStringSubmatch(id, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.ToUpper(matches[len(matches)-1][1])
}

// modelArchitectureFlags performs the internal operation described by its signature.
func modelArchitectureFlags(id string) []string {
	flags := make([]string, 0, 2)
	if match := modelMoEMarker.FindStringSubmatch(id); len(match) > 1 {
		flags = append(flags, "MoE:A"+strings.ToUpper(match[1]))
	}
	if modelMTPMarker.MatchString(id) {
		flags = append(flags, "MTP")
	}
	return flags
}

// containsString performs the internal operation described by its signature.
func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

// defaultToolMaxBytes identifies a fixed value used internally by this package.
const defaultToolMaxBytes = 1 << 20

// workspacePath performs the internal operation described by its signature.
func workspacePath(raw string, allowEmpty bool) (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get application workspace: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		if allowEmpty {
			return root, nil
		}
		return "", errors.New("path is required")
	}
	if filepath.IsAbs(raw) {
		return "", errors.New("path must be relative to the application workspace")
	}
	candidate := filepath.Clean(filepath.Join(root, raw))
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the application workspace")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve application workspace: %w", err)
	}
	if resolvedCandidate, resolveErr := filepath.EvalSymlinks(candidate); resolveErr == nil {
		resolvedRelative, relativeErr := filepath.Rel(resolvedRoot, resolvedCandidate)
		if relativeErr != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
			return "", errors.New("path escapes the application workspace through a symlink")
		}
	} else if !os.IsNotExist(resolveErr) {
		return "", fmt.Errorf("resolve workspace path: %w", resolveErr)
	}
	return candidate, nil
}

// readTextFile performs the internal operation described by its signature.
func readTextFile(arguments string) (string, error) {
	var args struct {
		Path     string `json:"path"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("parse read_text_file arguments: %w", err)
	}
	path, err := workspacePath(args.Path, false)
	if err != nil {
		return "", err
	}
	maxBytes := args.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultToolMaxBytes
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > int64(maxBytes) {
		return "", fmt.Errorf("file is %d bytes, exceeding max_bytes %d", info.Size(), maxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", errors.New("file is not valid UTF-8")
	}
	return jsonResult(map[string]any{"path": filepath.ToSlash(args.Path), "content": string(data), "bytes": len(data)})
}

// listDirectory performs the internal operation described by its signature.
func listDirectory(arguments string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("parse list_directory_contents arguments: %w", err)
	}
	path, err := workspacePath(args.Path, true)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	type directoryEntry struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		Type     string `json:"type"`
		Size     int64  `json:"size"`
		Modified string `json:"modified"`
		Mode     string `json:"mode"`
	}
	result := make([]directoryEntry, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return "", infoErr
		}
		typeName := "file"
		if info.IsDir() {
			typeName = "directory"
		} else if info.Mode()&os.ModeSymlink != 0 {
			typeName = "symlink"
		}
		result = append(result, directoryEntry{Name: entry.Name(), Path: filepath.ToSlash(filepath.Join(args.Path, entry.Name())), Type: typeName, Size: info.Size(), Modified: info.ModTime().Format(time.RFC3339), Mode: info.Mode().String()})
	}
	return jsonResult(result)
}

// searchWorkspace performs the internal operation described by its signature.
func searchWorkspace(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Query      string `json:"query"`
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("parse search_workspace arguments: %w", err)
	}
	if args.Query == "" {
		return "", errors.New("query is required")
	}
	path, err := workspacePath(args.Path, true)
	if err != nil {
		return "", err
	}
	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}
	commandArgs := []string{"--line-number", "--no-heading", "--color", "never"}
	if args.Glob != "" {
		commandArgs = append(commandArgs, "--glob", args.Glob)
	}
	commandArgs = append(commandArgs, "--", args.Query, path)
	output, err := exec.CommandContext(ctx, "rg", commandArgs...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return jsonResult(map[string]any{"query": args.Query, "matches": []string{}, "count": 0})
		}
		return "", fmt.Errorf("search workspace: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	truncated := len(lines) > maxResults
	if truncated {
		lines = lines[:maxResults]
	}
	return jsonResult(map[string]any{"query": args.Query, "matches": lines, "count": len(lines), "truncated": truncated})
}

// fileMetadata performs the internal operation described by its signature.
func fileMetadata(arguments string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("parse get_file_metadata arguments: %w", err)
	}
	path, err := workspacePath(args.Path, false)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	typeName := "file"
	if info.IsDir() {
		typeName = "directory"
	}
	return jsonResult(map[string]any{"path": filepath.ToSlash(args.Path), "size": info.Size(), "modified": info.ModTime().Format(time.RFC3339), "permissions": info.Mode().Perm().String(), "type": typeName})
}

// readJSONFile performs the internal operation described by its signature.
func readJSONFile(arguments string) (string, error) {
	var args struct {
		Path     string `json:"path"`
		Property string `json:"property"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("parse read_json_file arguments: %w", err)
	}
	path, err := workspacePath(args.Path, false)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("parse JSON file: %w", err)
	}
	if args.Property != "" {
		object, ok := value.(map[string]any)
		if !ok {
			return "", errors.New("property requires a top-level JSON object")
		}
		value, ok = object[args.Property]
		if !ok {
			return "", fmt.Errorf("top-level property %q not found", args.Property)
		}
	}
	return jsonResult(value)
}

// validateJSON performs the internal operation described by its signature.
func validateJSON(arguments string) (string, error) {
	var args struct {
		Content string `json:"content"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("parse validate_json arguments: %w", err)
	}
	content := args.Content
	if args.Path != "" {
		path, err := workspacePath(args.Path, false)
		if err != nil {
			return "", err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		content = string(raw)
	}
	if args.Content == "" && args.Path == "" {
		return "", errors.New("content or path is required")
	}
	var value any
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		result := map[string]any{"valid": false, "error": err.Error()}
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			result["offset"] = syntaxErr.Offset
		}
		return jsonResult(result)
	}
	return jsonResult(map[string]any{"valid": true})
}

// hashFile performs the internal operation described by its signature.
func hashFile(arguments string) (string, error) {
	var args struct {
		Path      string `json:"path"`
		Algorithm string `json:"algorithm"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("parse hash_file arguments: %w", err)
	}
	path, err := workspacePath(args.Path, false)
	if err != nil {
		return "", err
	}
	algorithm := strings.ToLower(args.Algorithm)
	if algorithm == "" {
		algorithm = "sha256"
	}
	var newHash func() hash.Hash
	switch algorithm {
	case "sha256":
		newHash = sha256.New
	case "sha512":
		newHash = sha512.New
	case "sha1":
		newHash = sha1.New
	case "md5":
		newHash = md5.New
	default:
		return "", fmt.Errorf("unsupported hash algorithm %q", algorithm)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := newHash()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return jsonResult(map[string]any{"path": filepath.ToSlash(args.Path), "algorithm": algorithm, "digest": fmt.Sprintf("%x", digest.Sum(nil))})
}

// goAST performs the internal operation described by its signature.
func goAST(arguments string) (string, error) {
	var args struct {
		File  string   `json:"file"`
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("parse parse_go_ast arguments: %w", err)
	}
	paths := append([]string(nil), args.Files...)
	if args.File != "" {
		paths = append(paths, args.File)
	}
	if len(paths) == 0 {
		return "", errors.New("file or files is required")
	}
	type astFile struct {
		Path string `json:"path"`
		AST  string `json:"ast"`
	}
	result := make([]astFile, 0, len(paths))
	for _, rawPath := range paths {
		path, err := workspacePath(rawPath, false)
		if err != nil {
			return "", err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return "", fmt.Errorf("parse Go file %q: %w", rawPath, err)
		}
		var output bytes.Buffer
		if err := ast.Fprint(&output, fset, file, ast.NotNilFilter); err != nil {
			return "", fmt.Errorf("print Go AST %q: %w", rawPath, err)
		}
		result = append(result, astFile{Path: filepath.ToSlash(rawPath), AST: output.String()})
	}
	return jsonResult(result)
}

var (
	// modelShardSuffix holds package-level state used internally by this package.
	modelShardSuffix = regexp.MustCompile(`-[0-9]{5}-of-[0-9]{5}$`)
	// modelQuantSuffix holds package-level state used internally by this package.
	modelQuantSuffix = regexp.MustCompile(`-q[0-9]_[a-z0-9_]+$`)
	// modelNoiseSuffix holds package-level state used internally by this package.
	modelNoiseSuffix = regexp.MustCompile(`-(gguf|ud|i1|awq|gptq|exl2)$`)
	// modelFamilyNoise holds package-level state used internally by this package.
	modelFamilyNoise = regexp.MustCompile(`-(mtp|coding|general|reasoning|non-reasoning)$`)
	// modelActiveMarker holds package-level state used internally by this package.
	modelActiveMarker = regexp.MustCompile(`-a[0-9]+b$`)
	// modelActiveMarkerAny holds package-level state used internally by this package.
	modelActiveMarkerAny = regexp.MustCompile(`(?i)-a[0-9]+b`)
	// modelFamilyNoiseAny holds package-level state used internally by this package.
	modelFamilyNoiseAny = regexp.MustCompile(`(?i)-(mtp|coding|general|reasoning|non-reasoning|i-quality)`)
	// modelMoEMarker holds package-level state used internally by this package.
	modelMoEMarker = regexp.MustCompile(`(?i)(?:^|-)a([0-9]+b)(?:-|$)`)
	// modelMTPMarker holds package-level state used internally by this package.
	modelMTPMarker = regexp.MustCompile(`(?i)(?:^|-)mtp(?:-|$)`)
)

// canonicalModelFamily performs the internal operation described by its signature.
func canonicalModelFamily(id string) string {
	family := strings.ToLower(strings.TrimSpace(id))
	for {
		previous := family
		family = modelShardSuffix.ReplaceAllString(family, "")
		family = modelQuantSuffix.ReplaceAllString(family, "")
		family = modelNoiseSuffix.ReplaceAllString(family, "")
		family = modelActiveMarker.ReplaceAllString(family, "")
		family = modelFamilyNoise.ReplaceAllString(family, "")
		family = modelActiveMarkerAny.ReplaceAllString(family, "")
		family = modelFamilyNoiseAny.ReplaceAllString(family, "")
		if family == previous {
			return family
		}
	}
}

// jsonResult performs the internal operation described by its signature.
func jsonResult(v any) (string, error) { b, e := json.Marshal(v); return string(b), e }
