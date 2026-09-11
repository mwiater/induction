package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	induction "github.com/mwiater/induction"
	"github.com/spf13/cobra"
)

type inferenceFlags struct {
	config, model, userPrompt, systemPrompt, responseFormat, jsonSchema string
	image, document, pipeline                                           string
	parameterOverrides, autosubmit, autoexit, nomcp                     bool
	temperature, topP, repeatPenalty                                    float64
	topK, maxTokens, seed                                               int
	parameterSet                                                        map[string]bool
}

func addInferenceFlags(root *cobra.Command, f *inferenceFlags) {
	root.Flags().StringVar(&f.model, "model", "", "model ID to use for inference")
	root.Flags().StringVar(&f.userPrompt, "userPrompt", "", "initial user prompt")
	root.Flags().StringVar(&f.systemPrompt, "systemPrompt", "", "system prompt override")
	root.Flags().StringVar(&f.responseFormat, "responseFormat", "", "response format: text, json_object, or json_schema")
	root.Flags().StringVar(&f.jsonSchema, "jsonSchema", "", "inline JSON schema object")
	root.Flags().StringVar(&f.image, "image", "", "local image path")
	root.Flags().StringVar(&f.document, "document", "", "local PDF/document path")
	root.Flags().StringVar(&f.pipeline, "pipeline", "", "pipeline YAML configuration path")
	root.Flags().Float64Var(&f.temperature, "temperature", 0, "request temperature override")
	root.Flags().Float64Var(&f.topP, "top-p", 0, "request top-p override")
	root.Flags().IntVar(&f.topK, "top-k", 0, "request top-k override")
	root.Flags().IntVar(&f.maxTokens, "max-tokens", 0, "request maximum token override")
	root.Flags().Float64Var(&f.repeatPenalty, "repeat-penalty", 0, "request repeat-penalty override")
	root.Flags().IntVar(&f.seed, "seed", 0, "request seed override")
	root.Flags().BoolVar(&f.autosubmit, "autosubmit", false, "submit --userPrompt automatically")
	root.Flags().BoolVar(&f.autoexit, "autoexit", false, "exit after the automated response and session save")
	root.Flags().BoolVar(&f.nomcp, "nomcp", false, "disable configured MCP servers for this inference")
}

func runInference(ctx context.Context, f inferenceFlags, in io.Reader, out io.Writer) error {
	if f.image != "" && f.document != "" {
		return errors.New("--image and --document cannot be combined")
	}
	if f.pipeline != "" {
		if f.model != "" || f.userPrompt != "" || f.systemPrompt != "" || f.responseFormat != "" || f.jsonSchema != "" || f.image != "" || f.document != "" || f.nomcp || f.autosubmit || f.autoexit || f.parameterOverrides {
			return errors.New("--pipeline cannot be combined with direct inference flags")
		}
		p, err := induction.LoadPipeline(f.pipeline)
		if err != nil {
			return err
		}
		if len(p.Steps) == 0 {
			return errors.New("pipeline has no steps")
		}
		configPath := f.config
		if strings.TrimSpace(p.Config) != "" {
			configPath = p.Config
		}
		options := []induction.ClientOption{induction.WithConfigPath(configPath), induction.WithPipeline(p), induction.WithAutoExitAfterInitialChat(true)}
		pipelineReq := &induction.ChatRequest{Model: p.Steps[0].Model}
		applyParameters(pipelineReq, f)
		cfg, err := induction.LoadConfig(configPath)
		if err != nil {
			return err
		}
		if pipelineUsesMCP(p) && len(cfg.MCPServers) > 0 {
			return runMCP(ctx, pipelineReq, in, out, options...)
		}
		return runConfiguredChat(ctx, pipelineReq, in, out, options...)
	}
	if f.model == "" {
		return errors.New("missing required --model flag (unless --pipeline is used)")
	}
	if f.autosubmit && strings.TrimSpace(f.userPrompt) == "" {
		return errors.New("--autosubmit requires --userPrompt with non-empty text")
	}
	if f.autoexit && !f.autosubmit {
		return errors.New("--autoexit requires --autosubmit")
	}

	var req *induction.ChatRequest
	switch {
	case f.image != "":
		data, e := induction.ImageDataURL(f.image, induction.DefaultAttachmentMaxBytes)
		if e != nil {
			return fmt.Errorf("prepare image: %w", e)
		}
		prompt := f.userPrompt
		if prompt == "" {
			prompt = "Analyze the provided image in detail."
		}
		req = &induction.ChatRequest{Model: f.model, ImageFilename: f.image, Messages: []induction.Message{{Role: "user", Content: []induction.ContentPart{{Type: "text", Text: prompt}, {Type: "image_url", ImageURL: &induction.ImageURLPart{URL: data, Detail: "low"}}}}}}
		f.userPrompt = prompt
	case f.document != "":
		_, filename, e := induction.FileDataURL(f.document, induction.DefaultAttachmentMaxBytes)
		if e != nil {
			return fmt.Errorf("prepare document: %w", e)
		}
		text, e := induction.ExtractPDFText(f.document, induction.DefaultAttachmentMaxBytes)
		if e != nil {
			return fmt.Errorf("extract document text: %w", e)
		}
		prompt := f.userPrompt
		if prompt == "" {
			prompt = "Read the provided document and summarize its primary argument."
		}
		req = &induction.ChatRequest{Model: f.model, DocumentFilename: filename, Messages: []induction.Message{{Role: "user", Content: []induction.ContentPart{{Type: "text", Text: prompt}, {Type: "text", Text: "Document filename: " + filename + "\n\nExtracted document text:\n" + text}}}}}
		f.userPrompt = prompt
	default:
		systemPrompt := f.systemPrompt
		if systemPrompt == "" {
			systemPrompt = "You are a precise technical assistant."
		}
		req = &induction.ChatRequest{Model: f.model, Messages: []induction.Message{{Role: "system", Content: systemPrompt}}}
	}
	if f.systemPrompt != "" && len(req.Messages) > 0 && req.Messages[0].Role != "system" {
		req.Messages = append([]induction.Message{{Role: "system", Content: f.systemPrompt}}, req.Messages...)
	} else if f.systemPrompt != "" && len(req.Messages) > 0 && req.Messages[0].Role == "system" {
		req.Messages[0].Content = f.systemPrompt
	}
	if err := applyOutputFormat(req, f); err != nil {
		return err
	}
	applyParameters(req, f)
	options := []induction.ClientOption{induction.WithConfigPath(f.config)}
	if f.userPrompt != "" {
		options = append(options, induction.WithInitialChatPrompt(f.userPrompt, f.autosubmit))
	}
	if f.autoexit {
		options = append(options, induction.WithAutoExitAfterInitialChat(true))
	}
	cfg, e := induction.LoadConfig(f.config)
	if e != nil {
		return e
	}
	if !f.nomcp && len(cfg.MCPServers) > 0 {
		return runMCP(ctx, req, in, out, options...)
	}
	return runApplicationTools(ctx, req, in, out, options...)
}

func applyOutputFormat(req *induction.ChatRequest, f inferenceFlags) error {
	typeName := strings.ToLower(strings.TrimSpace(f.responseFormat))
	if typeName != "" {
		switch typeName {
		case "text", "json", "json_object", "json_schema":
		default:
			return fmt.Errorf("--responseFormat has unsupported type %q", f.responseFormat)
		}
		req.ResponseFormat = &induction.ResponseFormat{Type: typeName}
	}
	if strings.TrimSpace(f.jsonSchema) == "" {
		return nil
	}
	if req.ResponseFormat == nil || (req.ResponseFormat.Type != "json" && req.ResponseFormat.Type != "json_object" && req.ResponseFormat.Type != "json_schema") {
		return errors.New("--jsonSchema requires --responseFormat json_object or json_schema")
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(f.jsonSchema), &schema); err != nil || schema == nil {
		return fmt.Errorf("--jsonSchema must be a valid JSON object: %w", err)
	}
	req.JSONSchema = schema
	return nil
}

func pipelineUsesMCP(p *induction.Pipeline) bool {
	if p == nil {
		return false
	}
	for _, step := range p.Steps {
		if !step.NoMCP {
			return true
		}
	}
	return false
}

func runConfiguredChat(ctx context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, options ...induction.ClientOption) error {
	return induction.InferStreamChat(ctx, req, in, out, options...)
}

func runMCP(ctx context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, options ...induction.ClientOption) error {
	return induction.InferMCPStreamChat(ctx, req, in, out, options...)
}

func runApplicationTools(ctx context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, options ...induction.ClientOption) error {
	return induction.InferApplicationToolsChat(ctx, withLocalTools(req), in, out, func(toolCtx context.Context, name, _ string) (string, error) { return localToolResult(toolCtx, name) }, append(options, induction.WithApplicationToolChain(chainLocalTools))...)
}

func withLocalTools(req *induction.ChatRequest) *induction.ChatRequest {
	copy := *req
	copy.Tools = []induction.Tool{{Type: "function", Function: induction.ToolFunction{Name: "current_system_date_time", Description: "Return the current system date and time.", Parameters: strictSchema()}}, {Type: "function", Function: induction.ToolFunction{Name: "current_free_disk_space", Description: "Return free space on /.", Parameters: strictSchema()}}, {Type: "function", Function: induction.ToolFunction{Name: "current_free_ram", Description: "Return available system RAM.", Parameters: strictSchema()}}}
	copy.ToolChoice = "auto"
	return &copy
}
func strictSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
}
func chainLocalTools(c []induction.InferenceToolCall) []induction.InferenceToolCall {
	for _, x := range c {
		if x.Function.Name == "current_system_date_time" {
			return c
		}
	}
	if len(c) > 0 {
		return append(c, induction.InferenceToolCall{ID: "autotime1", Type: "function", Function: induction.InferenceFunctionCall{Name: "current_system_date_time", Arguments: "{}"}})
	}
	return c
}
func localToolResult(ctx context.Context, name string) (string, error) {
	switch name {
	case "current_system_date_time":
		return jsonResult(map[string]any{"date_time": time.Now().Format(time.RFC3339), "timezone": time.Now().Location().String()})
	case "current_free_disk_space":
		return commandJSON(ctx, "df", "-Pk", "/")
	case "current_free_ram":
		return commandJSON(ctx, "free", "-b")
	default:
		return "", fmt.Errorf("unknown local tool %q", name)
	}
}
func commandJSON(ctx context.Context, command string, args ...string) (string, error) {
	b, err := exec.CommandContext(ctx, command, args...).Output()
	if err != nil {
		return "", err
	}
	return jsonResult(map[string]string{"output": strings.TrimSpace(string(b))})
}
func jsonResult(v any) (string, error) { b, e := json.Marshal(v); return string(b), e }
func floatPtr(v float64) *float64      { return &v }
func intPtr(v int) *int                { return &v }

func applyParameters(req *induction.ChatRequest, f inferenceFlags) {
	if !f.parameterOverrides {
		return
	}
	if f.parameterSet["temperature"] {
		req.Temperature = floatPtr(f.temperature)
	}
	if f.parameterSet["top-p"] {
		req.TopP = floatPtr(f.topP)
	}
	if f.parameterSet["top-k"] {
		req.TopK = intPtr(f.topK)
	}
	if f.parameterSet["max-tokens"] {
		req.MaxTokens = intPtr(f.maxTokens)
	}
	if f.parameterSet["repeat-penalty"] {
		req.RepeatPenalty = floatPtr(f.repeatPenalty)
	}
	if f.parameterSet["seed"] {
		req.Seed = intPtr(f.seed)
	}
}
