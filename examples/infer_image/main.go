// Command infer_image demonstrates ordered text and local data-URL image parts.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/mwiater/induction"
)

var infer = induction.Infer
var imagePath = filepath.Join("examples", "assets", "images", "fixture.jpg")

func request(model, path string) (*induction.ChatRequest, error) {
	dataURL, err := induction.ImageDataURL(path, induction.DefaultAttachmentMaxBytes)
	if err != nil {
		return nil, err
	}
	return &induction.ChatRequest{Model: model, Messages: []induction.Message{{Role: "user", Content: []induction.ContentPart{
		{Type: "text", Text: `Analyze the provided image and give a highly detailed breakdown of the following elements. Format your response with clear, bolded headings for each section:

Subject Matter: Describe the primary and secondary subjects, the spatial composition, and the environmental context.

Color Palette: Identify the dominant colors, contrast levels, and the overall atmospheric mood conveyed by the hues.

Lighting: Analyze the primary light source, shadow direction, and time of day, noting any specific optical or atmospheric effects.

Action: Describe any dynamic movement, kinetic energy, or implied motion occurring in the frame.

Location Guess: Provide your top 2 best guesses for the real-world location where this was photographed. Format each guess with a specific confidence percentage (e.g., "1. Location Name - 85%").

Film Stock Guess: Provide your top 2 best guesses for the analog film stock (or digital emulation) this aesthetic most closely resembles. Format each guess with a specific confidence percentage.`},
		{Type: "image_url", ImageURL: &induction.ImageURLPart{URL: dataURL, Detail: "low"}},
	}}}}, nil
}

func run(ctx context.Context, model string, out io.Writer) error {
	req, err := request(model, imagePath)
	if err != nil {
		return fmt.Errorf("prepare image: %w", err)
	}
	response, err := infer(ctx, req)
	if err != nil {
		return fmt.Errorf("infer image: %w", err)
	}
	if len(response.Choices) == 0 || response.Choices[0].Message == nil {
		return fmt.Errorf("image inference returned no answer")
	}
	_, err = fmt.Fprintln(out, response.Choices[0].Message.Content)
	return err
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
