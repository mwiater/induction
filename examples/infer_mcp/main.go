// Command infer_mcp demonstrates the supported MCP inference output modes.
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
var userPrompt = "Use the available MCP tools to explain what information they provide."
var inferMCP = induction.InferMCP
var inferMCPStream = induction.InferMCPStream
var inferMCPChat = induction.InferMCPChat
var inferMCPSnapshot = induction.InferMCPSnapshot
var runMain = run
var fatal = log.Fatal

func request(model string) *induction.ChatRequest {
	return &induction.ChatRequest{
		Model:    model,
		Messages: []induction.Message{{Role: "user", Content: userPrompt}},
	}
}

func run(ctx context.Context, model, mode string, in io.Reader, out io.Writer) error {
	switch mode {
	case modeDefault:
		response, err := inferMCP(ctx, request(model))
		if err != nil {
			return fmt.Errorf("infer with MCP: %w", err)
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("encode MCP response: %w", err)
		}
		return nil
	case modeStream:
		if err := inferMCPStream(ctx, request(model), out); err != nil {
			return fmt.Errorf("stream with MCP: %w", err)
		}
		return nil
	case modeChat:
		if _, err := fmt.Fprintln(out, "Ask me anything. Press Ctrl-C to end the MCP chat."); err != nil {
			return fmt.Errorf("write MCP chat prompt: %w", err)
		}
		chatRequest := request(model)
		chatRequest.Messages = nil
		if err := inferMCPChat(ctx, chatRequest, in, out); err != nil {
			return fmt.Errorf("chat with MCP: %w", err)
		}
		return nil
	case modeSnapshot:
		snapshot, err := inferMCPSnapshot(ctx, request(model))
		if err != nil {
			return fmt.Errorf("snapshot with MCP: %w", err)
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(snapshot); err != nil {
			return fmt.Errorf("encode MCP snapshot: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported MCP mode %q (choose default, stream, chat, or snapshot)", mode)
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
