// Command infer_document demonstrates attaching a local PDF as inline data.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mwiater/induction"
)

var documentPath = filepath.Join("examples", "assets", "documents", "fixture.pdf")
var infer = induction.Infer

func run(ctx context.Context, model string, out io.Writer) error {
	_, filename, err := induction.FileDataURL(documentPath, induction.DefaultAttachmentMaxBytes)
	if err != nil {
		return fmt.Errorf("prepare document: %w", err)
	}
	documentText, err := induction.ExtractPDFText(documentPath, induction.DefaultAttachmentMaxBytes)
	if err != nil {
		return fmt.Errorf("extract document text: %w", err)
	}
	req := request(model, documentText, filename)
	response, err := infer(ctx, req)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported content[].type") {
			return fmt.Errorf("configured inference server does not support inline document content (content type %q); use a document-capable server or extract the PDF text before inference", "file")
		}
		return fmt.Errorf("infer document: %w", err)
	}
	if len(response.Choices) == 0 || response.Choices[0].Message == nil {
		return fmt.Errorf("document inference returned no answer")
	}
	_, err = fmt.Fprintln(out, response.Choices[0].Message.Content)
	return err
}

func request(model, documentText, filename string) *induction.ChatRequest {
	return &induction.ChatRequest{Model: model, Messages: []induction.Message{{Role: "user", Content: []induction.ContentPart{
		{Type: "text", Text: `Read the provided document. As an expert researcher, provide a comprehensive, highly technical analysis of the text focusing on the following areas. Format your response with clear, bolded headings for each section:

Core Thesis: Summarize the primary argument, objective, or overarching research question presented in the document.

Methodology & Approach: Detail the specific methods, frameworks, or analytical processes the authors use to explore their thesis or build their system.

Key Metrics & Data: Extract and explain the core variables, datasets, or evaluation metrics used to measure success or validate the claims.

Summary of Results: Explain the primary findings, conclusions, and any reported limitations or implications for future work.`},
		{Type: "text", Text: "Document filename: " + filename + "\n\nExtracted document text:\n" + documentText},
	}}}}
}

func main() {
	model := flag.String("model", "", "model ID to use for inference (required)")
	flag.Parse()
	if *model == "" {
		log.Fatal("missing required --model flag")
	}
	if err := run(context.Background(), *model, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
