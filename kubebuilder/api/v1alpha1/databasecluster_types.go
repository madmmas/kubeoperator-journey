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

// DatabaseClusterSpec defines the DESIRED state of a DatabaseCluster.
// This is what the user writes in their YAML under `spec:`.
type DatabaseClusterSpec struct {
	// Replicas is the desired number of database replicas.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Replicas int32 `json:"replicas"`

	// Version is the PostgreSQL version to run (e.g. "15.4").
	// +kubebuilder:validation:Pattern=`^\d+\.\d+$`
	Version string `json:"version"`

	// StorageSize is the PersistentVolume size per replica (e.g. "100Gi").
	// +optional
	StorageSize string `json:"storageSize,omitempty"`

	// BackupSchedule is a cron expression for automated backups.
	// +optional
	BackupSchedule string `json:"backupSchedule,omitempty"`
}

// DatabaseClusterStatus defines the OBSERVED state of a DatabaseCluster.
// The operator writes this — users and tooling read it.
type DatabaseClusterStatus struct {
	// Phase is a high-level summary of the cluster state.
	// +optional
	Phase DatabaseClusterPhase `json:"phase,omitempty"`

	// ReadyReplicas is the number of replicas currently ready to serve traffic.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// CurrentVersion is the PostgreSQL version currently running.
	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`

	// LastBackupTime is when the most recent successful backup completed.
	// +optional
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`

	// Conditions provides detailed, machine-readable status.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DatabaseClusterPhase is a high-level summary of cluster state.
// +kubebuilder:validation:Enum=Provisioning;Running;Upgrading;Degraded;Failed
type DatabaseClusterPhase string

const (
	PhaseProvisioning DatabaseClusterPhase = "Provisioning"
	PhaseRunning      DatabaseClusterPhase = "Running"
	PhaseUpgrading    DatabaseClusterPhase = "Upgrading"
	PhaseDegraded     DatabaseClusterPhase = "Degraded"
	PhaseFailed       DatabaseClusterPhase = "Failed"
)

// Standard condition type constants.
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

// DatabaseCluster is the Schema for the databaseclusters API.
type DatabaseCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseClusterSpec   `json:"spec,omitempty"`
	Status DatabaseClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseClusterList contains a list of DatabaseCluster.
type DatabaseClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseCluster{}, &DatabaseClusterList{})
}
