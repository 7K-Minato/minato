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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AgentSpec defines the per-game agent image details.
type AgentSpec struct {
	// image is the agent container image.
	// +required
	Image string `json:"image"`

	// version is the agent version string.
	// +required
	Version string `json:"version"`
}

// PortSpec defines a game port exposed by the container.
type PortSpec struct {
	// name is a friendly port name.
	// +required
	Name string `json:"name"`

	// containerPort is the port exposed by the container.
	// +required
	ContainerPort int32 `json:"containerPort"`

	// protocol for this port (TCP or UDP).
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`
}

// EnvironmentSpec defines a configurable environment variable.
type EnvironmentSpec struct {
	// key is the environment variable name.
	// +required
	Key string `json:"key"`

	// default is the default value if not provided.
	// +optional
	Default string `json:"default,omitempty"`

	// required indicates whether this env var must be provided.
	// +optional
	Required bool `json:"required,omitempty"`
}

// StorageSpec defines minimal storage configuration.
type StorageSpec struct {
	// mountPath is where the volume is mounted.
	// +required
	MountPath string `json:"mountPath"`

	// sizeDefault is the default PVC size.
	// +required
	SizeDefault string `json:"sizeDefault"`
}

// CapabilitiesSpec defines optional sidecar capabilities for a game profile.
type CapabilitiesSpec struct {
	// files enables the filebrowser sidecar.
	// +optional
	Files bool `json:"files,omitempty"`

	// sftp enables the sftp sidecar.
	// +optional
	SFTP bool `json:"sftp,omitempty"`

	// backup enables the backup action.
	// +optional
	Backup bool `json:"backup,omitempty"`

	// restoreFromSnapshot enables restoring from a snapshot.
	// +optional
	RestoreFromSnapshot bool `json:"restoreFromSnapshot,omitempty"`
}

// GameProfileSpec defines the desired state of GameProfile
type GameProfileSpec struct {
	// displayName is a human-friendly name for the profile.
	// +required
	DisplayName string `json:"displayName"`

	// image is the game container image.
	// +required
	Image string `json:"image"`

	// imagePullPolicy is the pull policy for the game container image.
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ports defines the game ports.
	// +optional
	Ports []PortSpec `json:"ports,omitempty"`

	// environment defines configurable environment variables.
	// +optional
	Environment []EnvironmentSpec `json:"environment,omitempty"`

	// resources defines default container resources.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// storage defines the default persistent storage settings.
	// +required
	Storage StorageSpec `json:"storage"`

	// agent defines the per-game agent sidecar.
	// +required
	Agent AgentSpec `json:"agent"`

	// actions defines the declared action catalog.
	// +optional
	Actions []ActionDecl `json:"actions,omitempty"`

	// capabilities defines optional sidecar capabilities.
	// +optional
	Capabilities *CapabilitiesSpec `json:"capabilities,omitempty"`
}

// GameProfileStatus defines the observed state of GameProfile.
type GameProfileStatus struct {
	// conditions represent the current state of the GameProfile resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status

// GameProfile is the Schema for the gameprofiles API
type GameProfile struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of GameProfile
	// +required
	Spec GameProfileSpec `json:"spec"`

	// status defines the observed state of GameProfile
	// +optional
	Status GameProfileStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// GameProfileList contains a list of GameProfile
type GameProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []GameProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GameProfile{}, &GameProfileList{})
}
