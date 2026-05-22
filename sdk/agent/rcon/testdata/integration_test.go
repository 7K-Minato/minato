package testdata_test

import (
	"os"
	"testing"
)

func TestIntegrationFixtures(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if os.Getenv("minato_INTEGRATION_TESTS") != "1" {
		t.Skip("minato_INTEGRATION_TESTS not set")
	}
	// Placeholder: integration tests are run against docker-compose services.
}
