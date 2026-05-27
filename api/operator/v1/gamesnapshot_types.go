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

// SnapshotRetention defines retention policy for snapshots.
type SnapshotRetention struct {
	// count is the maximum number of snapshots to retain.
	// +optional
	Count int `json:"count,omitempty"`

	// duration is the maximum age of snapshots to retain.
	// +optional
	Duration string `json:"duration,omitempty"`
}

// GameSnapshotSpec defines the desired state of GameSnapshot.
type GameSnapshotSpec struct {
	// gameServerRef references the GameServer to snapshot.
	// +required
	GameServerRef string `json:"gameServerRef"`

	// schedule is a cron expression for periodic snapshots.
	// If empty, the snapshot runs immediately on creation.
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// retention defines the retention policy.
	// +optional
	Retention SnapshotRetention `json:"retention,omitempty"`
}

// SnapshotEntry defines a single snapshot entry.
type SnapshotEntry struct {
	// name is the snapshot name.
	Name string `json:"name"`

	// createdAt is when the snapshot was created.
	CreatedAt metav1.Time `json:"createdAt"`

	// volumeSnapshotRef references the VolumeSnapshot.
	VolumeSnapshotRef string `json:"volumeSnapshotRef"`

	// sizeBytes is the size of the snapshot.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// GameSnapshotStatus defines the observed state of GameSnapshot.
type GameSnapshotStatus struct {
	// snapshots is the list of created snapshots.
	// +optional
	Snapshots []SnapshotEntry `json:"snapshots,omitempty"`

	// lastSnapshotAt is when the last snapshot was taken.
	// +optional
	LastSnapshotAt *metav1.Time `json:"lastSnapshotAt,omitempty"`

	// conditions represent the current state of the GameSnapshot.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=gsnap

// GameSnapshot is the Schema for the gamesnapshots API.
type GameSnapshot struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of GameSnapshot.
	// +required
	Spec GameSnapshotSpec `json:"spec"`

	// status defines the observed state of GameSnapshot.
	// +optional
	Status GameSnapshotStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// GameSnapshotList contains a list of GameSnapshot.
type GameSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []GameSnapshot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GameSnapshot{}, &GameSnapshotList{})
}
