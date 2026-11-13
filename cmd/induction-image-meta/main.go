package main

import (
	"os"

	"github.com/mwiater/induction/appliances/image-meta"
	"github.com/mwiater/induction/internal/appliance"
)

func main() {
	os.Exit(appliance.Run(os.Args[1:], appliance.Assets{Pipeline: mustRead("pipeline.yaml"), Config: mustRead("config.yaml")}))
}

func mustRead(name string) []byte {
	data, err := imagemeta.Files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return data
}
