// Command infer_param_overrides demonstrates per-request parameter overrides
// in a streaming chat session.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/mwiater/induction"
)

var systemPrompt = "You are a precise technical assistant."
var userPrompt = "Explain the purpose of an atomic pointer in Go in two detailed paragraphs."

// Sampling and generation parameters can be changed for each request.
var temperature = 0.85
var topP = 0.95
var topK = 20
var maxTokens = 1024
var repeatPenalty = 1.0
var seed = 42

var inferChat = induction.InferStreamChat
var runMain = run
var fatal = log.Fatal

func request(model string) *induction.ChatRequest {
	return &induction.ChatRequest{
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
}

func run(ctx context.Context, model string, in io.Reader, out io.Writer) error {
	return runWithOptions(ctx, model, in, out, "", false, false)
}

func runWithOptions(ctx context.Context, model string, in io.Reader, out io.Writer, prompt string, autosubmit, autoexit bool) error {
	if _, err := fmt.Fprintln(out, "Ask me anything. Press Ctrl-C to end the chat."); err != nil {
		return fmt.Errorf("write chat prompt: %w", err)
	}
	chatRequest := request(model)
	options := []induction.ClientOption(nil)
	if prompt != "" {
		// The default request prompt is for the interactive example; a CLI
		// prompt replaces it as the first user turn.
		chatRequest.Messages = chatRequest.Messages[:1]
		options = append(options, induction.WithInitialChatPrompt(prompt, autosubmit))
	}
	if autoexit {
		options = append(options, induction.WithAutoExitAfterInitialChat(true))
	}
	if err := inferChat(ctx, chatRequest, in, out, options...); err != nil {
		return fmt.Errorf("infer chat with parameter overrides: %w", err)
	}
	return nil
}

func main() {
	model, prompt, autosubmit, autoexit, err := parseArgs(os.Args[1:])
	if err != nil {
		fatal(err)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runWithOptions(ctx, model, os.Stdin, os.Stdout, prompt, autosubmit, autoexit); err != nil {
		fatal(err)
	}
}

func parseArgs(args []string) (string, string, bool, bool, error) {
	flags := flag.NewFlagSet("infer_param_overrides", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	model := flags.String("model", "", "model ID to use for inference (required)")
	prompt := flags.String("prompt", "", "initial user prompt")
	autosubmit := flags.Bool("autosubmit", false, "submit --prompt automatically after the model is ready")
	autoexit := flags.Bool("autoexit", false, "exit after the automated response and session save")
	if err := flags.Parse(args); err != nil {
		return "", "", false, false, err
	}
	if *model == "" {
		return "", "", false, false, fmt.Errorf("missing required --model flag")
	}
	if *autosubmit && strings.TrimSpace(*prompt) == "" {
		return "", "", false, false, fmt.Errorf("--autosubmit requires --prompt with non-empty text")
	}
	if *autoexit && !*autosubmit {
		return "", "", false, false, fmt.Errorf("--autoexit requires --autosubmit")
	}
	return *model, *prompt, *autosubmit, *autoexit, nil
}
