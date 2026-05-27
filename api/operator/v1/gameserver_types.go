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

// LifecycleSpec defines lifecycle settings for a GameServer.
type LifecycleSpec struct {
	// idleTimeoutSeconds is the time before auto-shutdown when no players are online.
	// 0 = never auto-shutdown.
	// +optional
	IdleTimeoutSeconds int32 `json:"idleTimeoutSeconds,omitempty"`

	// autoStart controls whether the server starts automatically.
	// +optional
	AutoStart bool `json:"autoStart,omitempty"`
}

// GameServerSpec defines the desired state of GameServer.
type GameServerSpec struct {
	// profile references the GameProfile name.
	// +required
	Profile string `json:"profile"`

	// env provides environment overrides.
	// +optional
	Env map[string]string `json:"env,omitempty"`

	// lifecycle defines lifecycle settings.
	// +optional
	Lifecycle LifecycleSpec `json:"lifecycle,omitempty"`
}

// Endpoint defines a network endpoint for a GameServer.
type Endpoint struct {
	// name is the endpoint identifier (e.g., game, agent, filebrowser, sftp).
	Name string `json:"name"`

	// address is the reachable address.
	Address string `json:"address"`

	// port is the endpoint port.
	Port int32 `json:"port"`
}

// GameServerStatus defines the observed state of GameServer.
type GameServerStatus struct {
	// state reflects the current lifecycle state.
	// +optional
	State string `json:"state,omitempty"`

	// agentVersion is the version reported by the agent.
	// +optional
	AgentVersion string `json:"agentVersion,omitempty"`

	// players is the current number of online players.
	// +optional
	Players int32 `json:"players,omitempty"`

	// playerCapacity is the maximum number of players.
	// +optional
	PlayerCapacity int32 `json:"playerCapacity,omitempty"`

	// lastActivity is the timestamp of last player activity.
	// +optional
	LastActivity *metav1.Time `json:"lastActivity,omitempty"`

	// endpoints lists the available network endpoints for this server.
	// +listType=map
	// +listMapKey=name
	// +optional
	Endpoints []Endpoint `json:"endpoints,omitempty"`

	// conditions represent the current state of the GameServer resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GameServer is the Schema for the gameservers API.
type GameServer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of GameServer.
	// +required
	Spec GameServerSpec `json:"spec"`

	// status defines the observed state of GameServer.
	// +optional
	Status GameServerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// GameServerList contains a list of GameServer.
type GameServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []GameServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GameServer{}, &GameServerList{})
}
