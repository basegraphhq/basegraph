package main

import (
	"fmt"

	"github.com/humanbeeng/lepo/prototypes/codegraph/extract/golang"
	"github.com/humanbeeng/lepo/prototypes/codegraph/process"
)

func main() {
	// TODO : Remove extract interface and pass cmd line args for dir and pkg
	e := golang.NewGoExtractor()
	process.Orchestrate(e)
	fmt.Println("Code graph extraction completed successfully.")
}
