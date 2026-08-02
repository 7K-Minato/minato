package audit

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/7k-minato/minato/internal/controlplane/auth"
)

func TestLogAction(t *testing.T) {
	var buf bytes.Buffer
	old := output
	output = &buf
	t.Cleanup(func() { output = old })

	user := &auth.User{ID: "1", Username: "alice", Role: "admin"}
	LogAction("apikey.created", "ci-key", user)

	var event Event
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("invalid audit JSON: %v", err)
	}
	if event.Action != "apikey.created" {
		t.Fatalf("unexpected action: %q", event.Action)
	}
	if event.Resource != "ci-key" {
		t.Fatalf("unexpected resource: %q", event.Resource)
	}
	if event.User != "alice" || event.Role != "admin" {
		t.Fatalf("unexpected actor: %q/%q", event.User, event.Role)
	}
	if event.Result != "success" {
		t.Fatalf("unexpected result: %q", event.Result)
	}
}

func TestLogAction_Anonymous(t *testing.T) {
	var buf bytes.Buffer
	old := output
	output = &buf
	t.Cleanup(func() { output = old })

	LogAction("apikey.deleted", "old-key", nil)

	var event Event
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("invalid audit JSON: %v", err)
	}
	if event.User != "anonymous" {
		t.Fatalf("unexpected user: %q", event.User)
	}
}
