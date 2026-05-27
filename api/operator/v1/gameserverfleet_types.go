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

// RollingUpdateSpec defines the rolling update strategy.
type RollingUpdateSpec struct {
	// maxUnavailable is the maximum number of GameServers that can be unavailable during update.
	// +optional
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`

	// maxSurge is the maximum number of GameServers that can be created above desired replicas.
	// +optional
	MaxSurge *int32 `json:"maxSurge,omitempty"`

	// drainTimeoutSeconds is how long to wait for graceful shutdown before force-deleting.
	// +optional
	DrainTimeoutSeconds int32 `json:"drainTimeoutSeconds,omitempty"`
}

// FleetUpdateStrategy defines the update strategy for a GameServerFleet.
type FleetUpdateStrategy struct {
	// type is the update strategy type: RollingUpdate or OnDelete.
	// +optional
	Type string `json:"type,omitempty"`

	// rollingUpdate is the rolling update configuration.
	// +optional
	RollingUpdate *RollingUpdateSpec `json:"rollingUpdate,omitempty"`
}

// GameServerTemplateMetadata defines metadata for GameServer templates.
type GameServerTemplateMetadata struct {
	// labels to add to the GameServer.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations to add to the GameServer.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// FleetGameServerSpec defines the GameServer spec subset used in fleet templates.
// Profile is inherited from the fleet and should not be specified here.
type FleetGameServerSpec struct {
	// env provides environment overrides.
	// +optional
	Env map[string]string `json:"env,omitempty"`
}

// GameServerTemplateSpec defines the template for GameServers in a fleet.
type GameServerTemplateSpec struct {
	// metadata for the GameServer.
	// +optional
	Metadata GameServerTemplateMetadata `json:"metadata,omitempty"`

	// spec defines the GameServer spec (excluding profile which is inherited from fleet).
	// +optional
	Spec FleetGameServerSpec `json:"spec,omitempty"`
}

// GameServerFleetSpec defines the desired state of GameServerFleet.
type GameServerFleetSpec struct {
	// profile references the GameProfile name.
	// +required
	Profile string `json:"profile"`

	// replicas is the desired number of GameServers.
	// +required
	Replicas int32 `json:"replicas"`

	// template is the template for creating GameServers.
	// +optional
	Template GameServerTemplateSpec `json:"template,omitempty"`

	// updateStrategy controls how updates are rolled out.
	// +optional
	UpdateStrategy FleetUpdateStrategy `json:"updateStrategy,omitempty"`
}

// GameServerFleetStatus defines the observed state of GameServerFleet.
type GameServerFleetStatus struct {
	// replicas is the total number of GameServers.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// readyReplicas is the number of ready GameServers.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// updatedReplicas is the number of GameServers updated to latest spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// conditions represent the current state of the GameServerFleet.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=gsf

// GameServerFleet is the Schema for the gameserverfleets API.
type GameServerFleet struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of GameServerFleet.
	// +required
	Spec GameServerFleetSpec `json:"spec"`

	// status defines the observed state of GameServerFleet.
	// +optional
	Status GameServerFleetStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// GameServerFleetList contains a list of GameServerFleet.
type GameServerFleetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []GameServerFleet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GameServerFleet{}, &GameServerFleetList{})
}
