package main

import (
	"github.com/humanbeeng/lepo/prototypes/codegraph/extract/golang"
	"github.com/humanbeeng/lepo/prototypes/codegraph/process"
)

func main() {
	process.Orchestrate(golang.NewGoExtractor())
}
