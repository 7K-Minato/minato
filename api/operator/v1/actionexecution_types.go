/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ActionConcurrency defines how concurrent executions of this action are handled.
type ActionConcurrency string

const (
	ActionConcurrencyAllow     ActionConcurrency = "allow"
	ActionConcurrencySerialize ActionConcurrency = "serialize"
	ActionConcurrencyExclusive ActionConcurrency = "exclusive"
)

// ParamSchema defines the schema for an action parameter.
type ActionParamSchema struct {
	// type is the parameter type (string, int, bool).
	// +optional
	Type string `json:"type,omitempty"`

	// required indicates whether this parameter must be provided.
	// +optional
	Required bool `json:"required,omitempty"`

	// description is a human-readable description of the parameter.
	// +optional
	Description string `json:"description,omitempty"`

	// default is the default value if not provided.
	// +optional
	Default string `json:"default,omitempty"`
}

// ActionDecl defines a declared action in a GameProfile catalog.
type ActionDecl struct {
	// name is the action identifier.
	// +required
	Name string `json:"name"`

	// description is a human-readable description.
	// +optional
	Description string `json:"description,omitempty"`

	// params defines the parameter schema.
	// +optional
	Params map[string]ActionParamSchema `json:"params,omitempty"`

	// returns describes the return value.
	// +optional
	Returns string `json:"returns,omitempty"`

	// concurrency controls how concurrent executions are handled.
	// +optional
	Concurrency ActionConcurrency `json:"concurrency,omitempty"`

	// timeout is the maximum duration for this action.
	// +optional
	Timeout string `json:"timeout,omitempty"`
}

// TargetRef defines the target of an ActionExecution.
type TargetRef struct {
	// apiVersion of the target resource.
	// +required
	APIVersion string `json:"apiVersion"`

	// kind of the target resource.
	// +required
	Kind string `json:"kind"`

	// name of the target resource.
	// +required
	Name string `json:"name"`

	// namespace of the target resource (empty for cluster-scoped resources).
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ActionExecutionSpec defines the desired state of ActionExecution.
type ActionExecutionSpec struct {
	// targetRef references the GameServer to execute the action on.
	// +required
	TargetRef TargetRef `json:"targetRef"`

	// actionName is the name of the action to execute.
	// +required
	ActionName string `json:"actionName"`

	// params are the action parameters.
	// +optional
	Params map[string]string `json:"params,omitempty"`

	// caller is the identity that initiated this execution.
	// +optional
	Caller string `json:"caller,omitempty"`
}

// ActionExecutionState represents the state of an action execution.
type ActionExecutionState string

const (
	ActionExecutionPending   ActionExecutionState = "Pending"
	ActionExecutionRunning   ActionExecutionState = "Running"
	ActionExecutionSucceeded ActionExecutionState = "Succeeded"
	ActionExecutionFailed    ActionExecutionState = "Failed"
	ActionExecutionTimedOut  ActionExecutionState = "TimedOut"
	ActionExecutionRejected  ActionExecutionState = "Rejected"
)

// ActionExecutionStatus defines the observed state of ActionExecution.
type ActionExecutionStatus struct {
	// state is the current execution state.
	// +optional
	State ActionExecutionState `json:"state,omitempty"`

	// startedAt is when execution began.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// endedAt is when execution completed.
	// +optional
	EndedAt *metav1.Time `json:"endedAt,omitempty"`

	// agentResponse is the JSON-encoded response from the agent.
	// +optional
	AgentResponse string `json:"agentResponse,omitempty"`

	// error is the error message if execution failed.
	// +optional
	Error string `json:"error,omitempty"`

	// conditions represent the current state of the ActionExecution.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ae

// ActionExecution is the Schema for the actionexecutions API.
type ActionExecution struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ActionExecution.
	// +required
	Spec ActionExecutionSpec `json:"spec"`

	// status defines the observed state of ActionExecution.
	// +optional
	Status ActionExecutionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ActionExecutionList contains a list of ActionExecution.
type ActionExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ActionExecution `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ActionExecution{}, &ActionExecutionList{})
}
