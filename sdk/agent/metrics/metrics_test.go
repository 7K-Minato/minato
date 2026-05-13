package metrics

import "testing"

func TestPlayerCount(t *testing.T) {
	value := PlayerCount("minecraft", "server-1")
	if value == "" {
		t.Fatalf("expected metric name")
	}
}
