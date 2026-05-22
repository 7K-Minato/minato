package actions

import (
	"os"
	"strings"
)

// CatalogFromEnv builds a catalog from environment variables.
// Format:
// minato_ACTION_<ACTION>_STEP_<STEP>=<type>
// minato_ACTION_<ACTION>_INPUT_<STEP>_<INPUT>=<value>
func CatalogFromEnv() Catalog {
	actionsMap := map[string]*ActionDefinition{}
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := parts[1]
		if !strings.HasPrefix(key, "minato_ACTION_") || strings.Contains(key, "_INPUT_") {
			continue
		}
		remainder := strings.TrimPrefix(key, "minato_ACTION_")
		segments := strings.SplitN(remainder, "_STEP_", 2)
		if len(segments) != 2 {
			continue
		}
		actionName := strings.ToLower(segments[0])
		stepName := strings.ToLower(segments[1])
		if actionName == "" || stepName == "" {
			continue
		}
		action, ok := actionsMap[actionName]
		if !ok {
			action = &ActionDefinition{Name: actionName, Steps: []Step{}}
			actionsMap[actionName] = action
		}
		stepType := value
		inputs := map[string]string{}
		inputKeyPrefix := "minato_ACTION_" + strings.ToUpper(actionName) + "_INPUT_" + strings.ToUpper(stepName) + "_"
		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) != 2 {
				continue
			}
			if !strings.HasPrefix(parts[0], inputKeyPrefix) {
				continue
			}
			inputName := strings.TrimPrefix(parts[0], inputKeyPrefix)
			inputs[strings.ToLower(inputName)] = parts[1]
		}
		action.Steps = append(action.Steps, Step{Name: stepName, Type: strings.ToLower(stepType), Inputs: inputs})
	}

	list := make([]ActionDefinition, 0, len(actionsMap))
	for _, action := range actionsMap {
		list = append(list, *action)
	}
	return Catalog{Actions: list}
}
