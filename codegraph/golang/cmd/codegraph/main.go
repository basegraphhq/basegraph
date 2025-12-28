package main

import (
	"github.com/joho/godotenv"

	"github.com/humanbeeng/lepo/prototypes/codegraph/extract/golang"
	"github.com/humanbeeng/lepo/prototypes/codegraph/process"
)

func main() {
	_ = godotenv.Load()
	process.Orchestrate(golang.NewGoExtractor())
}
