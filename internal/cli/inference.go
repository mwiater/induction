// Package cli defines Induction's command-line interface.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	induction "github.com/mwiater/induction"
	"github.com/spf13/cobra"
)

// inferenceFlags is an internal data type used by this package.
type inferenceFlags struct {
	// jsonSchema stores the value represented by this member.
	// responseFormat stores the value represented by this member.
	// systemPrompt stores the value represented by this member.
	// userPrompt stores the value represented by this member.
	// pipelineModel stores the value represented by this member.
	// pipelineDecisionModel stores the value represented by this member.
	// model stores the value represented by this member.
	// config stores the value represented by this member.
	config, model, pipelineModel, pipelineDecisionModel, userPrompt, systemPrompt, responseFormat, jsonSchema string
	// pipeline stores the value represented by this member.
	// documents stores the value represented by this member.
	// document stores the value represented by this member.
	// image stores the value represented by this member.
	image, document, documents, pipeline string
	// usemcp stores the value represented by this member.
	// autoexit stores the value represented by this member.
	// autosubmit stores the value represented by this member.
	// parameterOverrides stores the value represented by this member.
	parameterOverrides, autosubmit, autoexit, usemcp bool
	// xtcThreshold stores the value represented by this member.
	// xtcProbability stores the value represented by this member.
	// minP stores the value represented by this member.
	// repeatPenalty stores the value represented by this member.
	// topP stores the value represented by this member.
	// temperature stores the value represented by this member.
	temperature, topP, repeatPenalty, minP, xtcProbability, xtcThreshold float64
	// frequencyPenalty stores the value represented by this member.
	// presencePenalty stores the value represented by this member.
	// typicalP stores the value represented by this member.
	typicalP, presencePenalty, frequencyPenalty float64
	// repeatLastN stores the value represented by this member.
	// seed stores the value represented by this member.
	// maxTokens stores the value represented by this member.
	// topK stores the value represented by this member.
	topK, maxTokens, seed, repeatLastN int
	// parameterSet stores the value represented by this member.
	parameterSet map[string]bool
}

// addInferenceFlags performs the internal operation described by its signature.
func addInferenceFlags(root *cobra.Command, f *inferenceFlags) {
	root.Flags().StringVar(&f.model, "model", "", "model ID to use for inference")
	root.Flags().StringVar(&f.userPrompt, "userPrompt", "", "initial user prompt")
	root.Flags().StringVar(&f.systemPrompt, "systemPrompt", "", "system prompt override")
	root.Flags().StringVar(&f.responseFormat, "responseFormat", "", "response format: text, json_object, or json_schema")
	root.Flags().StringVar(&f.jsonSchema, "jsonSchema", "", "inline JSON schema object")
	root.Flags().StringVar(&f.image, "image", "", "local image path")
	root.Flags().StringVar(&f.document, "document", "", "local PDF/document path")
	root.Flags().StringVar(&f.documents, "documents", "", "directory containing local PDF files")
	root.Flags().StringVar(&f.pipeline, "pipeline", "", "pipeline YAML configuration path")
	root.Flags().StringVar(&f.pipelineModel, "pipeline-model", "", "override non-decision pipeline steps with this model ID")
	root.Flags().StringVar(&f.pipelineDecisionModel, "pipeline-decision-model", "", "override decision and classification pipeline steps with this model ID")
	root.Flags().Float64Var(&f.temperature, "temperature", 0, "request temperature override")
	root.Flags().Float64Var(&f.topP, "top-p", 0, "request top-p override")
	root.Flags().IntVar(&f.topK, "top-k", 0, "request top-k override")
	root.Flags().IntVar(&f.maxTokens, "max-tokens", 0, "request maximum token override")
	root.Flags().Float64Var(&f.repeatPenalty, "repeat-penalty", 0, "request repeat-penalty override")
	root.Flags().IntVar(&f.seed, "seed", 0, "request seed override")
	root.Flags().Float64Var(&f.minP, "min-p", 0, "request min-p override")
	root.Flags().Float64Var(&f.xtcProbability, "xtc-probability", 0, "request XTC probability override")
	root.Flags().Float64Var(&f.xtcThreshold, "xtc-threshold", 0, "request XTC threshold override")
	root.Flags().Float64Var(&f.typicalP, "typical-p", 0, "request typical-p override")
	root.Flags().Float64Var(&f.presencePenalty, "presence-penalty", 0, "request presence-penalty override")
	root.Flags().Float64Var(&f.frequencyPenalty, "frequency-penalty", 0, "request frequency-penalty override")
	root.Flags().IntVar(&f.repeatLastN, "repeat-last-n", 0, "request repeat-last-n override")
	root.Flags().BoolVar(&f.autosubmit, "autosubmit", false, "submit --userPrompt automatically")
	root.Flags().BoolVar(&f.autoexit, "autoexit", false, "exit after the automated response and session save")
	root.Flags().BoolVar(&f.usemcp, "usemcp", false, "enable configured MCP servers for this inference")
}

// applyPipelineModelOverrides replaces the model used by ordinary and
// decision pipeline steps independently. Decision overrides intentionally also
// replace templated step models: autorun callers use this to rotate every
// discovered decision-capable model through a pipeline.
func applyPipelineModelOverrides(p *induction.Pipeline, pipelineModel, pipelineDecisionModel string) {
	if p == nil {
		return
	}
	for i := range p.Steps {
		step := &p.Steps[i]
		if strings.TrimSpace(pipelineDecisionModel) != "" && (step.Decision != nil || step.Classification != nil) && step.Model != "" {
			step.Model = pipelineDecisionModel
			continue
		}
		if strings.TrimSpace(pipelineModel) != "" && step.Model != "" && !strings.Contains(step.Model, "{{") && step.Decision == nil && step.Classification == nil {
			step.Model = pipelineModel
		}
	}
}

// runInference performs the internal operation described by its signature.
func runInference(ctx context.Context, f inferenceFlags, in io.Reader, out io.Writer) error {
	if f.image != "" && (f.document != "" || f.documents != "") {
		return errors.New("--image cannot be combined with --document or --documents")
	}
	if f.document != "" && f.documents != "" {
		return errors.New("--document and --documents cannot be combined")
	}
	if f.pipeline != "" {
		if f.model != "" || f.systemPrompt != "" || f.responseFormat != "" || f.jsonSchema != "" || f.image != "" || f.document != "" || f.documents != "" || f.usemcp || f.autosubmit || f.autoexit || f.parameterOverrides {
			return errors.New("--pipeline cannot be combined with these direct inference flags")
		}
		p, err := induction.LoadPipeline(f.pipeline)
		if err != nil {
			return err
		}
		if len(p.Steps) == 0 {
			return errors.New("pipeline has no steps")
		}
		applyPipelineModelOverrides(p, f.pipelineModel, f.pipelineDecisionModel)
		prompt := pipelineInitialPrompt(p, f.userPrompt)
		if prompt == "" {
			return errors.New("pipeline requires a top-level prompt, --userPrompt, or a first-step userPrompt")
		}
		configPath := f.config
		if strings.TrimSpace(p.Config) != "" {
			configPath = p.Config
		}
		options := []induction.ClientOption{induction.WithConfigPath(configPath), induction.WithPipeline(p), induction.WithAutoExitAfterInitialChat(true), induction.WithInitialChatPrompt(prompt, true)}
		pipelineReq := &induction.ChatRequest{Model: p.Steps[0].Model}
		applyParameters(pipelineReq, f)
		cfg, err := induction.LoadConfig(configPath)
		if err != nil {
			return err
		}
		if p.Batch != nil {
			batch, runErr := induction.RunBatch(ctx, p, ".batches", func(runCtx context.Context, pipeline *induction.Pipeline, input induction.InputSet, item *induction.BatchItem) error {
				child := *pipeline
				child.Batch = nil
				child.Inputs = &input
				childOptions := []induction.ClientOption{
					induction.WithConfigPath(configPath),
					induction.WithPipeline(&child),
					induction.WithAutoExitAfterInitialChat(true),
					induction.WithBatchMetadata(item.BatchID, item.ID),
					induction.WithSessionSaved(func(path string) {
						item.PipelineRunID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
					}),
				}
				batchIndex := 0
				for i := range pipeline.Batch.Items {
					if pipeline.Batch.Items[i].ID == item.ID {
						batchIndex = i
						break
					}
				}
				childOptions = append(childOptions, induction.WithArtifactPathContext(induction.ArtifactPathContext{InputSet: input, BatchID: item.ID, BatchIndex: batchIndex, IsBatch: true}))
				pipelineReq := &induction.ChatRequest{Model: child.Steps[0].Model}
				if pipelineUsesMCP(&child) && len(cfg.MCPServers) > 0 {
					return runMCP(runCtx, pipelineReq, in, out, childOptions...)
				}
				return runApplicationTools(runCtx, pipelineReq, in, out, cfg.Server, childOptions...)
			}, func(batch *induction.Batch) {
				summary := batch.Summary()
				_, _ = fmt.Fprintf(out, "Batch %s: %s (%d/%d completed, %d failed, %d invalid)\n", batch.ID, batch.Status, summary.Completed, summary.Total, summary.Failed, summary.Invalid)
			})
			if runErr != nil {
				return runErr
			}
			_, _ = fmt.Fprintf(out, "Batch complete: %s\n", batch.ID)
			return nil
		}
		if pipelineUsesMCP(p) && len(cfg.MCPServers) > 0 {
			return runMCP(ctx, pipelineReq, in, out, options...)
		}
		return runApplicationTools(ctx, pipelineReq, in, out, cfg.Server, options...)
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
		filename, text, e := prepareDocument(f.document)
		if e != nil {
			return fmt.Errorf("prepare document: %w", e)
		}
		prompt := f.userPrompt
		if prompt == "" {
			prompt = "Read the provided document and summarize its primary argument."
		}
		req = &induction.ChatRequest{Model: f.model, DocumentFilename: filename, Messages: []induction.Message{{Role: "user", Content: []induction.ContentPart{{Type: "text", Text: prompt}, {Type: "text", Text: text}}}}}
		f.userPrompt = prompt
	case f.documents != "":
		filename, text, e := prepareDocuments(f.documents)
		if e != nil {
			return fmt.Errorf("prepare documents: %w", e)
		}
		prompt := f.userPrompt
		if prompt == "" {
			prompt = "Read the provided documents and summarize their primary arguments."
		}
		req = &induction.ChatRequest{Model: f.model, DocumentFilename: filename, Messages: []induction.Message{{Role: "user", Content: []induction.ContentPart{{Type: "text", Text: prompt}, {Type: "text", Text: text}}}}}
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
	options = append(options, induction.WithMCPFooter(f.usemcp))
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
	if f.usemcp && len(cfg.MCPServers) > 0 {
		return runMCP(ctx, req, in, out, options...)
	}
	return runApplicationTools(ctx, req, in, out, cfg.Server, options...)
}

// pipelineInitialPrompt performs the internal operation described by its signature.
func pipelineInitialPrompt(p *induction.Pipeline, userPrompt string) string {
	if p == nil {
		return strings.TrimSpace(userPrompt)
	}
	if prompt := strings.TrimSpace(p.Prompt); prompt != "" {
		return prompt
	}
	if prompt := strings.TrimSpace(userPrompt); prompt != "" {
		return prompt
	}
	if len(p.Steps) > 0 {
		return strings.TrimSpace(p.Steps[0].UserPrompt)
	}
	return ""
}

// prepareDocument performs the internal operation described by its signature.
func prepareDocument(path string) (filename, text string, err error) {
	_, filename, err = induction.FileDataURL(path, induction.DefaultAttachmentMaxBytes)
	if err != nil {
		return "", "", err
	}
	text, err = induction.ExtractPDFText(path, induction.DefaultAttachmentMaxBytes)
	if err != nil {
		return "", "", fmt.Errorf("extract document text: %w", err)
	}
	return filename, "Document filename: " + filename + "\n\nExtracted document text:\n" + text, nil
}

// prepareDocuments performs the internal operation described by its signature.
func prepareDocuments(path string) (filename, text string, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return "", "", errors.New("path is not a directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", "", fmt.Errorf("read directory: %w", err)
	}

	var parts []string
	var filenames []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".pdf") {
			continue
		}
		_, part, err := prepareDocument(filepath.Join(path, entry.Name()))
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", entry.Name(), err)
		}
		filenames = append(filenames, entry.Name())
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "", "", errors.New("directory contains no PDF files")
	}
	return strings.Join(filenames, ", "), strings.Join(parts, "\n\n"), nil
}

// applyOutputFormat performs the internal operation described by its signature.
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

// pipelineUsesMCP performs the internal operation described by its signature.
func pipelineUsesMCP(p *induction.Pipeline) bool {
	if p == nil {
		return false
	}
	for _, step := range p.Steps {
		if step.UseMCP {
			return true
		}
	}
	return false
}

// runConfiguredChat performs the internal operation described by its signature.
func runConfiguredChat(ctx context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, options ...induction.ClientOption) error {
	return induction.InferStreamChat(ctx, req, in, out, options...)
}

// runMCP performs the internal operation described by its signature.
func runMCP(ctx context.Context, req *induction.ChatRequest, in io.Reader, out io.Writer, options ...induction.ClientOption) error {
	return induction.InferMCPStreamChat(ctx, req, in, out, options...)
}

// floatPtr performs the internal operation described by its signature.
func floatPtr(v float64) *float64 { return &v }

// intPtr performs the internal operation described by its signature.
func intPtr(v int) *int { return &v }

// applyParameters performs the internal operation described by its signature.
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
	if f.parameterSet["min-p"] {
		req.MinP = floatPtr(f.minP)
	}
	if f.parameterSet["xtc-probability"] {
		req.XTCProbability = floatPtr(f.xtcProbability)
	}
	if f.parameterSet["xtc-threshold"] {
		req.XTCThreshold = floatPtr(f.xtcThreshold)
	}
	if f.parameterSet["typical-p"] {
		req.TypicalP = floatPtr(f.typicalP)
	}
	if f.parameterSet["presence-penalty"] {
		req.PresencePenalty = floatPtr(f.presencePenalty)
	}
	if f.parameterSet["frequency-penalty"] {
		req.FrequencyPenalty = floatPtr(f.frequencyPenalty)
	}
	if f.parameterSet["repeat-last-n"] {
		req.RepeatLastN = intPtr(f.repeatLastN)
	}
}
