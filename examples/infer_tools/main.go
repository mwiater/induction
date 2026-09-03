// Command infer_tools demonstrates an application-managed function-calling loop.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/mwiater/induction"
)

var infer = induction.Infer

type weatherArguments struct {
	City string `json:"city"`
}

var weatherTool = induction.Tool{Type: "function", Function: induction.ToolFunction{
	Name: "get_weather", Description: "Get the current weather for a city.",
	Parameters: map[string]any{"type": "object", "properties": map[string]any{
		"city": map[string]any{"type": "string", "description": "City name"},
	}, "required": []string{"city"}, "additionalProperties": false},
}}

func request(model string, messages []induction.Message) *induction.ChatRequest {
	return &induction.ChatRequest{Model: model, Messages: messages, Tools: []induction.Tool{weatherTool}, ToolChoice: "auto"}
}

func run(ctx context.Context, model string, out io.Writer) error {
	messages := []induction.Message{{Role: "user", Content: "What is the weather in Paris?"}}
	first, err := infer(ctx, request(model, messages))
	if err != nil {
		return fmt.Errorf("initial inference: %w", err)
	}
	if len(first.Choices) == 0 || first.Choices[0].Message == nil {
		return errors.New("model returned no assistant message")
	}
	assistant := first.Choices[0].Message
	if len(assistant.ToolCalls) == 0 {
		return errors.New("model did not request get_weather")
	}
	messages = append(messages, induction.Message{Role: "assistant", Content: assistant.Content, ToolCalls: assistant.ToolCalls})
	for _, call := range assistant.ToolCalls {
		if call.Function.Name != weatherTool.Function.Name {
			return fmt.Errorf("unknown function %q", call.Function.Name)
		}
		var args weatherArguments
		decoder := json.NewDecoder(strings.NewReader(call.Function.Arguments))
		if err := decoder.Decode(&args); err != nil || strings.TrimSpace(args.City) == "" {
			return fmt.Errorf("invalid get_weather arguments: %q", call.Function.Arguments)
		}
		result, err := getWeather(args)
		if err != nil {
			return err
		}
		messages = append(messages, induction.Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: result})
	}
	final, err := infer(ctx, request(model, messages))
	if err != nil {
		return fmt.Errorf("final inference: %w", err)
	}
	if len(final.Choices) == 0 || final.Choices[0].Message == nil || strings.TrimSpace(final.Choices[0].Message.Content) == "" {
		return errors.New("final response has no textual content")
	}
	_, err = fmt.Fprintln(out, final.Choices[0].Message.Content)
	return err
}

func getWeather(args weatherArguments) (string, error) {
	if strings.EqualFold(strings.TrimSpace(args.City), "paris") {
		return "Paris: sunny, 18°C.", nil
	}
	return "Weather unavailable for " + args.City + ".", nil
}

func main() {
	model := flag.String("model", "", "model ID to use for inference (required)")
	flag.Parse()
	if *model == "" {
		log.Fatal("missing required --model flag")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, *model, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
