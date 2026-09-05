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
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/mwiater/induction"
)

var inferChat = induction.InferApplicationToolsChat
var fatal = log.Fatal

const (
	systemTimeTool = "current_system_date_time"
	diskSpaceTool  = "current_free_disk_space"
	freeRAMTool    = "current_free_ram"
)

var applicationTools = []induction.Tool{
	localTool(systemTimeTool, "Return the current system date and time.", nil),
	localTool(diskSpaceTool, "Return the free space remaining on the root filesystem at /. Include bytes and percentage of total.", nil),
	localTool(freeRAMTool, "Return the currently available system RAM. Include bytes and percentage of total.", nil),
}

func localTool(name, description string, properties any) induction.Tool {
	if properties == nil {
		properties = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	}
	return induction.Tool{Type: "function", Function: induction.ToolFunction{Name: name, Description: description, Parameters: properties}}
}

func request(model string, messages []induction.Message) *induction.ChatRequest {
	return &induction.ChatRequest{Model: model, Messages: messages, Tools: applicationTools, ToolChoice: "auto"}
}

func run(ctx context.Context, model string, out io.Writer) error {
	return runWithOptions(ctx, model, os.Stdin, out, "", false, false)
}

func runWithOptions(ctx context.Context, model string, in io.Reader, out io.Writer, prompt string, autosubmit, autoexit bool) error {
	options := []induction.ClientOption{induction.WithInitialChatPrompt(prompt, autosubmit), induction.WithAutoExitAfterInitialChat(autoexit)}
	options = append(options, induction.WithApplicationToolChain(chainApplicationToolCalls))
	if err := inferChat(ctx, request(model, nil), in, out, func(toolCtx context.Context, name, _ string) (string, error) {
		return runLocalTool(toolCtx, name)
	}, options...); err != nil {
		return fmt.Errorf("chat with local tools: %w", err)
	}
	return nil
}

func chainApplicationToolCalls(calls []induction.InferenceToolCall) []induction.InferenceToolCall {
	if len(calls) > 0 && !requestsTool(calls, systemTimeTool) {
		return append(calls, induction.InferenceToolCall{ID: "auto-" + systemTimeTool, Type: "function", Function: induction.InferenceFunctionCall{Name: systemTimeTool, Arguments: "{}"}})
	}
	return calls
}

func requestsTool(calls []induction.InferenceToolCall, name string) bool {
	for _, call := range calls {
		if call.Function.Name == name {
			return true
		}
	}
	return false
}

func runLocalTool(ctx context.Context, name string) (string, error) {
	switch name {
	case systemTimeTool:
		return jsonResult(map[string]any{"date_time": time.Now().Format(time.RFC3339), "timezone": time.Now().Location().String()})
	case diskSpaceTool:
		return diskSpace(ctx)
	case freeRAMTool:
		return freeRAM(ctx)
	default:
		return "", fmt.Errorf("unknown local tool %q", name)
	}
}

func jsonResult(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode local tool result: %w", err)
	}
	return string(encoded), nil
}

func diskSpace(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "df", "-Pk", "/").Output()
	if err != nil {
		return "", fmt.Errorf("run df for /: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return "", errors.New("df returned no filesystem data for /")
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 5 {
		return "", fmt.Errorf("unexpected df output: %q", lines[len(lines)-1])
	}
	total, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse df total: %w", err)
	}
	available, err := strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse df available: %w", err)
	}
	return jsonResult(map[string]any{"path": "/", "total_bytes": total * 1024, "available_bytes": available * 1024, "available_percent": percent(available, total), "total": humanBytes(total * 1024), "available": humanBytes(available * 1024)})
}

func freeRAM(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "free", "-b").Output()
	if err != nil {
		return "", fmt.Errorf("run free: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 7 && fields[0] == "Mem:" {
			total, err1 := strconv.ParseUint(fields[1], 10, 64)
			available, err2 := strconv.ParseUint(fields[6], 10, 64)
			if err1 != nil || err2 != nil {
				return "", fmt.Errorf("parse free output: %q", line)
			}
			return jsonResult(map[string]any{"total_bytes": total, "available_bytes": available, "available_percent": percent(available, total), "total": humanBytes(total), "available": humanBytes(available)})
		}
	}
	return "", fmt.Errorf("free returned no Mem data: %q", string(output))
}

func percent(value, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}

func humanBytes(value uint64) string {
	const unit = 1024.0
	amount := float64(value)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for amount >= unit && i < len(units)-1 {
		amount /= unit
		i++
	}
	return fmt.Sprintf("%.2f %s", amount, units[i])
}

func main() {
	flags := flag.NewFlagSet("infer_tools", flag.ExitOnError)
	model := flags.String("model", "", "model ID to use for inference (required)")
	prompt := flags.String("prompt", "", "initial user prompt")
	autosubmit := flags.Bool("autosubmit", false, "submit --prompt automatically after the model is ready")
	autoexit := flags.Bool("autoexit", false, "exit after the automated response and session save")
	flags.Parse(os.Args[1:])
	if *model == "" {
		fatal("missing required --model flag")
		return
	}
	if *autosubmit && strings.TrimSpace(*prompt) == "" {
		fatal("--autosubmit requires --prompt with non-empty text")
		return
	}
	if *autoexit && !*autosubmit {
		fatal("--autoexit requires --autosubmit")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runWithOptions(ctx, *model, os.Stdin, os.Stdout, *prompt, *autosubmit, *autoexit); err != nil {
		fatal(err)
	}
}
