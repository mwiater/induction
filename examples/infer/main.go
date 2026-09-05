// Command infer demonstrates a streaming chat session.
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
var inferChat = induction.InferStreamChat
var runMain = run
var fatal = log.Fatal

func request(model string) *induction.ChatRequest {
	return &induction.ChatRequest{Model: model, Messages: []induction.Message{{Role: "system", Content: systemPrompt}}}
}

func run(ctx context.Context, model string, in io.Reader, out io.Writer) error {
	return runWithOptions(ctx, model, in, out, "", false, false)
}

func runWithOptions(ctx context.Context, model string, in io.Reader, out io.Writer, prompt string, autosubmit, autoexit bool) error {
	if _, err := fmt.Fprintln(out, "Ask me anything. Press Ctrl-C to end the chat."); err != nil {
		return fmt.Errorf("write chat prompt: %w", err)
	}
	options := []induction.ClientOption(nil)
	if prompt != "" {
		options = append(options, induction.WithInitialChatPrompt(prompt, autosubmit))
	}
	if autoexit {
		options = append(options, induction.WithAutoExitAfterInitialChat(true))
	}
	if err := inferChat(ctx, request(model), in, out, options...); err != nil {
		return fmt.Errorf("infer chat: %w", err)
	}
	return nil
}

func parseArgs(args []string) (string, string, bool, bool, error) {
	flags := flag.NewFlagSet("infer", flag.ContinueOnError)
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
