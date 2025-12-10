package webhook_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWebhookHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webhook Handler Suite")
}
