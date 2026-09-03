// Command induction provides the command-line interface for the Induction
// client and model manager.
package main

import (
	"os"

	"github.com/mwiater/induction/internal/cli"
)

func main() { os.Exit(cli.Execute()) }
