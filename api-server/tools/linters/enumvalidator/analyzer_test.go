package enumvalidator_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"basegraph.app/api-server/tools/linters/enumvalidator"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, enumvalidator.Analyzer, "example")
}
