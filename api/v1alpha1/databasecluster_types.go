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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DatabaseClusterSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Pattern=`^\d+\.\d+$`
	Version string `json:"version"`

	// +optional
	StorageSize string `json:"storageSize,omitempty"`

	// +optional
	BackupSchedule string `json:"backupSchedule,omitempty"`
}

type DatabaseClusterStatus struct {
	// +optional
	Phase DatabaseClusterPhase `json:"phase,omitempty"`

	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`

	// +optional
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:validation:Enum=Provisioning;Running;Upgrading;Degraded;Failed
type DatabaseClusterPhase string

const (
	PhaseProvisioning DatabaseClusterPhase = "Provisioning"
	PhaseRunning      DatabaseClusterPhase = "Running"
	PhaseUpgrading    DatabaseClusterPhase = "Upgrading"
	PhaseDegraded     DatabaseClusterPhase = "Degraded"
	PhaseFailed       DatabaseClusterPhase = "Failed"
)

const (
	ConditionReady     = "Ready"
	ConditionDegraded  = "Degraded"
	ConditionUpgrading = "Upgrading"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.currentVersion`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type DatabaseCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseClusterSpec   `json:"spec,omitempty"`
	Status DatabaseClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type DatabaseClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseCluster{}, &DatabaseClusterList{})
}
