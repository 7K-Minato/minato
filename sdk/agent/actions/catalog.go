package actions

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Catalog struct {
	Actions []ActionDefinition `json:"actions" yaml:"actions"`
}

type ActionDefinition struct {
	Name        string                 `json:"name" yaml:"name"`
	Description string                 `json:"description" yaml:"description"`
	Params      map[string]ParamSchema `json:"params" yaml:"params"`
	Steps       []Step                 `json:"steps" yaml:"steps"`
}

type ParamSchema struct {
	Type        string `json:"type" yaml:"type"`
	Required    bool   `json:"required" yaml:"required"`
	Description string `json:"description" yaml:"description"`
	Default     string `json:"default" yaml:"default"`
}

func LoadCatalogFromFile(path string) (Catalog, error) {
	if path == "" {
		return Catalog{}, errors.New("path is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, err
	}
	return LoadCatalogFromBytes(content)
}

func LoadCatalogFromBytes(content []byte) (Catalog, error) {
	reader := strings.TrimSpace(string(content))
	if reader == "" {
		return Catalog{}, errors.New("empty content")
	}
	if reader[0] == '{' || reader[0] == '[' {
		return decodeJSON(content)
	}
	return decodeYAML(content)
}

func decodeJSON(content []byte) (Catalog, error) {
	var catalog Catalog
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil && !errors.Is(err, io.EOF) {
		return Catalog{}, err
	}
	return catalog, nil
}

func decodeYAML(content []byte) (Catalog, error) {
	var catalog Catalog
	decoder := yaml.NewDecoder(strings.NewReader(string(content)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&catalog); err != nil && !errors.Is(err, io.EOF) {
		return Catalog{}, err
	}
	return catalog, nil
}

func (c Catalog) FindAction(name string) (ActionDefinition, bool) {
	for _, action := range c.Actions {
		if action.Name == name {
			return action, true
		}
	}
	return ActionDefinition{}, false
}
