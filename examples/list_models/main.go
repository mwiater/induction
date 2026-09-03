// Command list_models prints the models exposed by the configured server.
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/mwiater/induction"
)

var loadConfig = induction.LoadConfig
var listModels = induction.ListModels
var runMain = run
var fatal = log.Fatal

func run(out io.Writer) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := log.New(out, "", 0)
	if err := listModels(cfg.Server, induction.WithLogger(logger)); err != nil {
		return fmt.Errorf("list models: %w", err)
	}
	return nil
}

func main() {
	err := runMain(os.Stdout)
	induction.Cleanup(os.Stdout)
	if err != nil {
		fatal(err)
	}
}
