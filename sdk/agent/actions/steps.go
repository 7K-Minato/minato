package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"
)

type Step struct {
	Name   string            `json:"name" yaml:"name"`
	Type   string            `json:"type" yaml:"type"`
	Inputs map[string]string `json:"inputs" yaml:"inputs"`
	Parse  *ParseConfig      `json:"parse" yaml:"parse"`
}

type ParseConfig struct {
	Type   string            `json:"type" yaml:"type"`
	Regex  string            `json:"regex" yaml:"regex"`
	Fields []string          `json:"fields" yaml:"fields"`
	Map    map[string]string `json:"map" yaml:"map"`
}

type Result struct {
	Outputs map[string]string
}

func Execute(ctx context.Context, action ActionDefinition, params map[string]string, runtime Runtime) (Result, error) {
	if runtime == nil {
		return Result{}, ErrNoRuntime
	}
	vars := map[string]string{}
	for key, schema := range action.Params {
		if value, ok := params[key]; ok {
			vars[key] = value
			continue
		}
		if schema.Required {
			return Result{}, fmt.Errorf("missing required param %s", key)
		}
		if schema.Default != "" {
			vars[key] = schema.Default
		}
	}

	outputs := map[string]string{}
	for _, step := range action.Steps {
		value, err := executeStep(ctx, step, vars, runtime)
		if err != nil {
			return Result{}, err
		}
		if step.Name != "" && value != "" {
			outputs[step.Name] = value
		}
	}

	return Result{Outputs: outputs}, nil
}

func executeStep(ctx context.Context, step Step, vars map[string]string, runtime Runtime) (string, error) {
	switch strings.ToLower(step.Type) {
	case "rcon":
		command := renderTemplate(step.Inputs["command"], vars)
		return runtime.RCON(ctx, command)
	case "exec":
		command := renderTemplate(step.Inputs["command"], vars)
		args := splitArgs(renderTemplate(step.Inputs["args"], vars))
		return runtime.Exec(ctx, command, args)
	case "sleep":
		duration, err := time.ParseDuration(renderTemplate(step.Inputs["duration"], vars))
		if err != nil {
			return "", err
		}
		return "", runtime.Sleep(ctx, duration)
	case "http":
		method := renderTemplate(step.Inputs["method"], vars)
		url := renderTemplate(step.Inputs["url"], vars)
		body := renderTemplate(step.Inputs["body"], vars)
		return runtime.HTTP(ctx, method, url, body)
	case "signal":
		target := renderTemplate(step.Inputs["target"], vars)
		signal := renderTemplate(step.Inputs["signal"], vars)
		return "", runtime.Signal(ctx, target, signal)
	default:
		return "", fmt.Errorf("unsupported step type %s", step.Type)
	}
}

func renderTemplate(input string, vars map[string]string) string {
	if input == "" {
		return ""
	}

	tmpl, err := template.New("step").Option("missingkey=zero").Parse(input)
	if err != nil {
		return input
	}
	var builder strings.Builder
	if err := tmpl.Execute(&builder, vars); err != nil {
		return input
	}
	return builder.String()
}

func splitArgs(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	return strings.Fields(input)
}

var ErrNoRuntime = errors.New("runtime is nil")
