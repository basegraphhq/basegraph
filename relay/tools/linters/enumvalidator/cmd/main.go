package main

import (
	"basegraph.app/relay/tools/linters/enumvalidator"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(enumvalidator.Analyzer)
}
