package actions

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRuntime struct {
	lastCommand  string
	lastMethod   string
	lastURL      string
	lastBody     string
	lastTarget   string
	lastSignal   string
	lastDuration time.Duration
	returnValue  string
	returnErr    error
}

func (f *fakeRuntime) RCON(ctx context.Context, command string) (string, error) {
	f.lastCommand = command
	return f.returnValue, f.returnErr
}

func (f *fakeRuntime) Exec(ctx context.Context, command string, args []string) (string, error) {
	f.lastCommand = command
	return f.returnValue, f.returnErr
}

func (f *fakeRuntime) HTTP(ctx context.Context, method string, url string, body string) (string, error) {
	f.lastMethod = method
	f.lastURL = url
	f.lastBody = body
	return f.returnValue, f.returnErr
}

func (f *fakeRuntime) Signal(ctx context.Context, target string, signal string) error {
	f.lastTarget = target
	f.lastSignal = signal
	return f.returnErr
}

func (f *fakeRuntime) Sleep(ctx context.Context, duration time.Duration) error {
	f.lastDuration = duration
	return f.returnErr
}

func TestExecuteRCONStep(t *testing.T) {
	action := ActionDefinition{
		Name: "save",
		Params: map[string]ParamSchema{
			"world": {Type: "string", Required: true},
		},
		Steps: []Step{
			{Type: "rcon", Inputs: map[string]string{"command": "save {{.world}}"}},
		},
	}

	runtime := &fakeRuntime{returnValue: "ok"}
	result, err := Execute(context.Background(), action, map[string]string{"world": "main"}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastCommand != "save main" {
		t.Fatalf("expected rendered command, got %q", runtime.lastCommand)
	}
	if len(result.Outputs) != 0 {
		t.Fatalf("expected no outputs")
	}
}

func TestExecuteNilRuntime(t *testing.T) {
	action := ActionDefinition{Name: "noop"}
	_, err := Execute(context.Background(), action, map[string]string{}, nil)
	if err == nil {
		t.Fatalf("expected error for nil runtime")
	}
	if !errors.Is(err, ErrNoRuntime) {
		t.Fatalf("expected ErrNoRuntime, got %v", err)
	}
}

func TestExecuteAllStepTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("exec", func(t *testing.T) {
		action := ActionDefinition{
			Name: "exec",
			Steps: []Step{
				{Type: "exec", Inputs: map[string]string{"command": "echo", "args": "hello world"}},
			},
		}
		runtime := &fakeRuntime{}
		_, err := Execute(ctx, action, nil, runtime)
		if err != nil {
			t.Fatalf("exec step: %v", err)
		}
		if runtime.lastCommand != "echo" {
			t.Fatalf("expected command echo, got %q", runtime.lastCommand)
		}
	})

	t.Run("http", func(t *testing.T) {
		action := ActionDefinition{
			Name: "http",
			Steps: []Step{
				{Type: "http", Inputs: map[string]string{"method": "POST", "url": "http://localhost/api", "body": "{}"}},
			},
		}
		runtime := &fakeRuntime{}
		_, err := Execute(ctx, action, nil, runtime)
		if err != nil {
			t.Fatalf("http step: %v", err)
		}
		if runtime.lastMethod != "POST" || runtime.lastURL != "http://localhost/api" {
			t.Fatalf("expected POST http://localhost/api, got %s %s", runtime.lastMethod, runtime.lastURL)
		}
	})

	t.Run("signal", func(t *testing.T) {
		action := ActionDefinition{
			Name: "signal",
			Steps: []Step{
				{Type: "signal", Inputs: map[string]string{"target": "game", "signal": "SIGTERM"}},
			},
		}
		runtime := &fakeRuntime{}
		_, err := Execute(ctx, action, nil, runtime)
		if err != nil {
			t.Fatalf("signal step: %v", err)
		}
		if runtime.lastTarget != "game" || runtime.lastSignal != "SIGTERM" {
			t.Fatalf("expected game:SIGTERM, got %s:%s", runtime.lastTarget, runtime.lastSignal)
		}
	})

	t.Run("sleep", func(t *testing.T) {
		action := ActionDefinition{
			Name: "sleep",
			Steps: []Step{
				{Type: "sleep", Inputs: map[string]string{"duration": "1ms"}},
			},
		}
		runtime := &fakeRuntime{}
		_, err := Execute(ctx, action, nil, runtime)
		if err != nil {
			t.Fatalf("sleep step: %v", err)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		action := ActionDefinition{
			Name: "bad",
			Steps: []Step{
				{Type: "unknown"},
			},
		}
		runtime := &fakeRuntime{}
		_, err := Execute(ctx, action, nil, runtime)
		if err == nil {
			t.Fatalf("expected error for unsupported step type")
		}
	})

	t.Run("missing required param", func(t *testing.T) {
		action := ActionDefinition{
			Name: "save",
			Params: map[string]ParamSchema{
				"world": {Type: "string", Required: true},
			},
			Steps: []Step{
				{Type: "rcon", Inputs: map[string]string{"command": "save {{.world}}"}},
			},
		}
		runtime := &fakeRuntime{}
		_, err := Execute(ctx, action, map[string]string{}, runtime)
		if err == nil {
			t.Fatalf("expected error for missing required param")
		}
	})

	t.Run("default param", func(t *testing.T) {
		action := ActionDefinition{
			Name: "save",
			Params: map[string]ParamSchema{
				"world": {Type: "string", Default: "default-world"},
			},
			Steps: []Step{
				{Type: "rcon", Inputs: map[string]string{"command": "save {{.world}}"}},
			},
		}
		runtime := &fakeRuntime{}
		_, err := Execute(ctx, action, map[string]string{}, runtime)
		if err != nil {
			t.Fatalf("execute with default: %v", err)
		}
		if runtime.lastCommand != "save default-world" {
			t.Fatalf("expected save default-world, got %q", runtime.lastCommand)
		}
	})

	t.Run("step output", func(t *testing.T) {
		action := ActionDefinition{
			Name: "query",
			Steps: []Step{
				{Name: "result", Type: "rcon", Inputs: map[string]string{"command": "list"}},
			},
		}
		runtime := &fakeRuntime{returnValue: "ok"}
		result, err := Execute(ctx, action, nil, runtime)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if result.Outputs["result"] != "ok" {
			t.Fatalf("expected output ok, got %q", result.Outputs["result"])
		}
	})

	t.Run("invalid sleep duration", func(t *testing.T) {
		action := ActionDefinition{
			Name: "sleep",
			Steps: []Step{
				{Type: "sleep", Inputs: map[string]string{"duration": "invalid"}},
			},
		}
		runtime := &fakeRuntime{}
		_, err := Execute(ctx, action, nil, runtime)
		if err == nil {
			t.Fatalf("expected error for invalid duration")
		}
	})
}

func TestRenderTemplate(t *testing.T) {
	if got := renderTemplate("", nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := renderTemplate("hello {{.name}}", map[string]string{"name": "world"}); got != "hello world" {
		t.Fatalf("expected hello world, got %q", got)
	}
	if got := renderTemplate("{{.missing}}", nil); got != "" {
		t.Fatalf("expected empty for missing key, got %q", got)
	}
}

func TestSplitArgs(t *testing.T) {
	if got := splitArgs(""); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := splitArgs("  "); got != nil {
		t.Fatalf("expected nil for whitespace, got %v", got)
	}
	got := splitArgs("a b c")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("expected [a b c], got %v", got)
	}
}

func TestExecuteMissingRequiredParam(t *testing.T) {
	action := ActionDefinition{
		Name: "save",
		Params: map[string]ParamSchema{
			"world": {Type: "string", Required: true},
		},
		Steps: []Step{},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err == nil {
		t.Fatalf("expected error for missing required param")
	}
}

func TestExecuteOptionalParamWithDefault(t *testing.T) {
	action := ActionDefinition{
		Name: "save",
		Params: map[string]ParamSchema{
			"world": {Type: "string", Required: false, Default: "default-world"},
		},
		Steps: []Step{
			{Type: "rcon", Inputs: map[string]string{"command": "save {{.world}}"}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastCommand != "save default-world" {
		t.Fatalf("expected default value, got %q", runtime.lastCommand)
	}
}

func TestExecuteOptionalParamNoDefault(t *testing.T) {
	action := ActionDefinition{
		Name: "save",
		Params: map[string]ParamSchema{
			"world": {Type: "string", Required: false},
		},
		Steps: []Step{
			{Type: "rcon", Inputs: map[string]string{"command": "save {{.world}}"}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastCommand != "save " {
		t.Fatalf("expected empty value, got %q", runtime.lastCommand)
	}
}

func TestExecuteStepOutputCapture(t *testing.T) {
	action := ActionDefinition{
		Name: "query",
		Steps: []Step{
			{Name: "result", Type: "rcon", Inputs: map[string]string{"command": "list"}},
		},
	}
	runtime := &fakeRuntime{returnValue: "3 players online"}
	result, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Outputs["result"] != "3 players online" {
		t.Fatalf("expected output captured, got %q", result.Outputs["result"])
	}
}

func TestExecuteStepError(t *testing.T) {
	action := ActionDefinition{
		Name: "fail",
		Steps: []Step{
			{Type: "rcon", Inputs: map[string]string{"command": "fail"}},
		},
	}
	runtime := &fakeRuntime{returnErr: errors.New("rcon failed")}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestExecuteExecStep(t *testing.T) {
	action := ActionDefinition{
		Name: "exec-test",
		Steps: []Step{
			{Type: "exec", Inputs: map[string]string{"command": "echo", "args": "hello world"}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastCommand != "echo" {
		t.Fatalf("expected command echo, got %q", runtime.lastCommand)
	}
}

func TestExecuteSleepStep(t *testing.T) {
	action := ActionDefinition{
		Name: "sleep-test",
		Steps: []Step{
			{Type: "sleep", Inputs: map[string]string{"duration": "1ms"}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastDuration != 1*time.Millisecond {
		t.Fatalf("expected 1ms duration, got %v", runtime.lastDuration)
	}
}

func TestExecuteSleepStepInvalidDuration(t *testing.T) {
	action := ActionDefinition{
		Name: "sleep-test",
		Steps: []Step{
			{Type: "sleep", Inputs: map[string]string{"duration": "invalid"}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err == nil {
		t.Fatalf("expected error for invalid duration")
	}
}

func TestExecuteHTTPStep(t *testing.T) {
	action := ActionDefinition{
		Name: "http-test",
		Steps: []Step{
			{Type: "http", Inputs: map[string]string{"method": "POST", "url": "http://localhost/api", "body": "data"}},
		},
	}
	runtime := &fakeRuntime{returnValue: "response"}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastMethod != "POST" {
		t.Fatalf("expected method POST, got %q", runtime.lastMethod)
	}
	if runtime.lastURL != "http://localhost/api" {
		t.Fatalf("expected url http://localhost/api, got %q", runtime.lastURL)
	}
	if runtime.lastBody != "data" {
		t.Fatalf("expected body data, got %q", runtime.lastBody)
	}
}

func TestExecuteSignalStep(t *testing.T) {
	action := ActionDefinition{
		Name: "signal-test",
		Steps: []Step{
			{Type: "signal", Inputs: map[string]string{"target": "game", "signal": "TERM"}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastTarget != "game" {
		t.Fatalf("expected target game, got %q", runtime.lastTarget)
	}
	if runtime.lastSignal != "TERM" {
		t.Fatalf("expected signal TERM, got %q", runtime.lastSignal)
	}
}

func TestExecuteUnsupportedStep(t *testing.T) {
	action := ActionDefinition{
		Name: "bad",
		Steps: []Step{
			{Type: "unknown", Inputs: map[string]string{}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err == nil {
		t.Fatalf("expected error for unsupported step type")
	}
}

func TestExecuteEmptySteps(t *testing.T) {
	action := ActionDefinition{
		Name:  "noop",
		Steps: []Step{},
	}
	runtime := &fakeRuntime{}
	result, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Outputs) != 0 {
		t.Fatalf("expected no outputs")
	}
}

func TestRenderTemplateEmpty(t *testing.T) {
	result := renderTemplate("", map[string]string{"key": "value"})
	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

func TestRenderTemplateValid(t *testing.T) {
	result := renderTemplate("hello {{.name}}", map[string]string{"name": "world"})
	if result != "hello world" {
		t.Fatalf("expected 'hello world', got %q", result)
	}
}

func TestRenderTemplateMissingKey(t *testing.T) {
	result := renderTemplate("hello {{.name}}", map[string]string{})
	if result != "hello " {
		t.Fatalf("expected 'hello ', got %q", result)
	}
}

func TestRenderTemplateInvalidSyntax(t *testing.T) {
	result := renderTemplate("hello {{.name", map[string]string{"name": "world"})
	if result != "hello {{.name" {
		t.Fatalf("expected fallback to input, got %q", result)
	}
}

func TestSplitArgsEmpty(t *testing.T) {
	result := splitArgs("")
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestSplitArgsWhitespace(t *testing.T) {
	result := splitArgs("   ")
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestSplitArgsMultiple(t *testing.T) {
	result := splitArgs("a b c")
	if len(result) != 3 || result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Fatalf("expected [a b c], got %v", result)
	}
}

func TestExecuteStepCaseInsensitive(t *testing.T) {
	action := ActionDefinition{
		Name: "case-test",
		Steps: []Step{
			{Type: "RCON", Inputs: map[string]string{"command": "list"}},
		},
	}
	runtime := &fakeRuntime{returnValue: "ok"}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastCommand != "list" {
		t.Fatalf("expected command list, got %q", runtime.lastCommand)
	}
}

func TestExecuteMultipleSteps(t *testing.T) {
	action := ActionDefinition{
		Name: "multi",
		Steps: []Step{
			{Name: "step1", Type: "rcon", Inputs: map[string]string{"command": "cmd1"}},
			{Name: "step2", Type: "rcon", Inputs: map[string]string{"command": "cmd2"}},
		},
	}
	runtime := &fakeRuntime{returnValue: "result"}
	result, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(result.Outputs))
	}
	if result.Outputs["step1"] != "result" {
		t.Fatalf("expected step1 output, got %q", result.Outputs["step1"])
	}
	if result.Outputs["step2"] != "result" {
		t.Fatalf("expected step2 output, got %q", result.Outputs["step2"])
	}
}

func TestExecuteExecStepWithArgsTemplate(t *testing.T) {
	action := ActionDefinition{
		Name: "exec-template",
		Params: map[string]ParamSchema{
			"target": {Type: "string", Required: true},
		},
		Steps: []Step{
			{Type: "exec", Inputs: map[string]string{"command": "echo", "args": "{{.target}}"}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{"target": "world"}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastCommand != "echo" {
		t.Fatalf("expected command echo, got %q", runtime.lastCommand)
	}
}

func TestExecuteHTTPStepWithTemplates(t *testing.T) {
	action := ActionDefinition{
		Name: "http-template",
		Params: map[string]ParamSchema{
			"id": {Type: "string", Required: true},
		},
		Steps: []Step{
			{Type: "http", Inputs: map[string]string{
				"method": "GET",
				"url":    "http://localhost/{{.id}}",
				"body":   "id={{.id}}",
			}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{"id": "123"}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastURL != "http://localhost/123" {
		t.Fatalf("expected url http://localhost/123, got %q", runtime.lastURL)
	}
	if runtime.lastBody != "id=123" {
		t.Fatalf("expected body id=123, got %q", runtime.lastBody)
	}
}

func TestExecuteSignalStepWithTemplates(t *testing.T) {
	action := ActionDefinition{
		Name: "signal-template",
		Params: map[string]ParamSchema{
			"sig": {Type: "string", Required: true},
		},
		Steps: []Step{
			{Type: "signal", Inputs: map[string]string{"target": "game", "signal": "{{.sig}}"}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{"sig": "HUP"}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastSignal != "HUP" {
		t.Fatalf("expected signal HUP, got %q", runtime.lastSignal)
	}
}

func TestExecuteSleepStepWithTemplate(t *testing.T) {
	action := ActionDefinition{
		Name: "sleep-template",
		Params: map[string]ParamSchema{
			"dur": {Type: "string", Default: "1ms"},
		},
		Steps: []Step{
			{Type: "sleep", Inputs: map[string]string{"duration": "{{.dur}}"}},
		},
	}
	runtime := &fakeRuntime{}
	_, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastDuration != 1*time.Millisecond {
		t.Fatalf("expected 1ms, got %v", runtime.lastDuration)
	}
}

func TestExecuteStepOutputEmptyValue(t *testing.T) {
	action := ActionDefinition{
		Name: "empty-output",
		Steps: []Step{
			{Name: "empty", Type: "signal", Inputs: map[string]string{"target": "game", "signal": "TERM"}},
		},
	}
	runtime := &fakeRuntime{}
	result, err := Execute(context.Background(), action, map[string]string{}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := result.Outputs["empty"]; ok {
		t.Fatalf("expected no output for empty value step")
	}
}
