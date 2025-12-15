package main

import (
	"golang.org/x/tools/go/analysis"

	"basegraph.app/api-server/tools/linters/enumvalidator"
)

type AnalyzerPlugin struct{}

func (*AnalyzerPlugin) GetAnalyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		enumvalidator.Analyzer,
	}
}

func New(conf any) ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{enumvalidator.Analyzer}, nil
}
