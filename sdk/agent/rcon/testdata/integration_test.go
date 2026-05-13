package testdata_test

import (
	"os"
	"testing"
)

func TestIntegrationFixtures(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if os.Getenv("MINAMI_INTEGRATION_TESTS") != "1" {
		t.Skip("MINAMI_INTEGRATION_TESTS not set")
	}
	// Placeholder: integration tests are run against docker-compose services.
}
