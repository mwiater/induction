// Command induction provides the command-line interface for the Induction
// client and model manager.
package main

import (
	"os"

	"github.com/mwiater/induction/internal/cli"
)

func main() {
	// Ensure the terminal UI uses its truecolor palette in every environment,
	// including local launches outside the Docker image.
	if err := os.Setenv("COLORTERM", "truecolor"); err != nil {
		panic(err)
	}
	os.Exit(cli.Execute())
}
