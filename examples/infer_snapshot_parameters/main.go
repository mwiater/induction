// Command infer_snapshot_parameters sends a parameterized chat request and prints the resulting model snapshot.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/mwiater/induction"
)

var systemPrompt = "You are a precise technical assistant."
var userPrompt = "Explain the purpose of an atomic pointer in Go in two detailed paragraphs."

// Sampling and generation parameters can be changed for each request.
var temperature = 0.2
var topP = 0.9
var topK = 40
var maxTokens = 512
var repeatPenalty = 1.1
var seed = 42

var inferSnapshot = induction.InferSnapshot
var runMain = run
var fatal = log.Fatal

func run(ctx context.Context, model string, out io.Writer) error {
	req := &induction.ChatRequest{
		Model: model,
		Messages: []induction.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:   &temperature,
		TopP:          &topP,
		TopK:          &topK,
		MaxTokens:     &maxTokens,
		RepeatPenalty: &repeatPenalty,
		Seed:          &seed,
	}

	snapshot, err := inferSnapshot(ctx, req)
	if err != nil {
		return fmt.Errorf("infer snapshot: %w", err)
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	return nil
}

func main() {
	if flag.Lookup("model") == nil {
		flag.String("model", "", "model ID to use for inference (required)")
	}
	flag.Parse()
	model := flag.Lookup("model").Value.String()
	if model == "" {
		fatal("missing required --model flag")
		return
	}
	err := runMain(context.Background(), model, os.Stdout)
	induction.Cleanup(os.Stdout)
	if err != nil {
		fatal(err)
	}
}
