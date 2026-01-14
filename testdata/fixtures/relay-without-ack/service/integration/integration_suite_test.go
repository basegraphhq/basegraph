package integration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIntegrationService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Service Suite")
}
