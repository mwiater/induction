// Command infer demonstrates the default, streaming, chat, and snapshot
// inference output modes.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"

	"github.com/mwiater/induction"
)

const (
	modeDefault  = "default"
	modeStream   = "stream"
	modeChat     = "chat"
	modeSnapshot = "snapshot"
)

var systemPrompt = "You are a precise technical assistant."
var userPrompt = "Explain the purpose of an atomic pointer in Go in two detailed paragraphs."

var infer = induction.Infer
var inferStream = induction.InferStream
var inferStreamChat = induction.InferStreamChat
var inferSnapshot = induction.InferSnapshot
var runMain = run
var fatal = log.Fatal

func request(model string, includeUserPrompt bool) *induction.ChatRequest {
	messages := []induction.Message{{Role: "system", Content: systemPrompt}}
	if includeUserPrompt {
		messages = append(messages, induction.Message{Role: "user", Content: userPrompt})
	}
	return &induction.ChatRequest{Model: model, Messages: messages}
}

func encode(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func run(ctx context.Context, model, mode string, in io.Reader, out io.Writer) error {
	switch mode {
	case modeDefault:
		response, err := infer(ctx, request(model, true))
		if err != nil {
			return fmt.Errorf("infer: %w", err)
		}
		if err := encode(out, response); err != nil {
			return fmt.Errorf("encode response: %w", err)
		}
		return nil
	case modeStream:
		if err := inferStream(ctx, request(model, true), out); err != nil {
			return fmt.Errorf("infer stream: %w", err)
		}
		return nil
	case modeChat:
		if _, err := fmt.Fprintln(out, "Ask me anything. Press Ctrl-C to end the chat."); err != nil {
			return fmt.Errorf("write chat prompt: %w", err)
		}
		if err := inferStreamChat(ctx, request(model, false), in, out); err != nil {
			return fmt.Errorf("infer chat: %w", err)
		}
		return nil
	case modeSnapshot:
		snapshot, err := inferSnapshot(ctx, request(model, true))
		if err != nil {
			return fmt.Errorf("infer snapshot: %w", err)
		}
		if err := encode(out, snapshot); err != nil {
			return fmt.Errorf("encode snapshot: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported mode %q (choose default, stream, chat, or snapshot)", mode)
	}
}

func main() {
	mode := flag.String("mode", modeDefault, "output mode: default, stream, chat, or snapshot")
	model := flag.String("model", "", "model ID to use for inference (required)")
	flag.Parse()
	if *model == "" {
		fatal("missing required --model flag")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Println()
	fmt.Println("System Prompt:", systemPrompt)
	fmt.Println("User Prompt:  ", userPrompt)
	fmt.Println("Model:        ", *model)
	fmt.Println()

	err := runMain(ctx, *model, *mode, os.Stdin, os.Stdout)
	induction.Cleanup(os.Stdout)
	if err != nil {
		fatal(err)
	}
}
