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

// ── Spec ──────────────────────────────────────────────────────────────────────

// DatabaseClusterSpec defines the desired state of DatabaseCluster.
type DatabaseClusterSpec struct {
	// Replicas is the desired number of database replicas.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Replicas int32 `json:"replicas"`

	// Version is the PostgreSQL version to run (e.g. "15.4").
	// Changing this triggers a rolling upgrade.
	// +kubebuilder:validation:Pattern=`^\d+\.\d+$`
	Version string `json:"version"`

	// StorageSize is the PersistentVolume size per replica (e.g. "10Gi").
	// +optional
	// +kubebuilder:default="10Gi"
	StorageSize string `json:"storageSize,omitempty"`

	// BackupSchedule is a cron expression for automated backups (e.g. "0 2 * * *").
	// When set, the operator creates a backup bucket and schedules periodic backups.
	// When cleared, the bucket is NOT deleted — use kubectl delete to trigger cleanup
	// via the finalizer.
	// +optional
	BackupSchedule string `json:"backupSchedule,omitempty"`

	// PostgresConfig holds custom postgresql.conf key-value pairs.
	// These are mounted as a ConfigMap and override the default configuration.
	// +optional
	PostgresConfig map[string]string `json:"postgresConfig,omitempty"`
}

// ── Status ────────────────────────────────────────────────────────────────────

// DatabaseClusterStatus defines the observed state of DatabaseCluster.
// All fields are written by the operator — never by the user.
type DatabaseClusterStatus struct {
	// Phase is a high-level lifecycle summary.
	// +optional
	Phase DatabaseClusterPhase `json:"phase,omitempty"`

	// ReadyReplicas is the number of replicas currently passing their readiness probe.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// CurrentVersion is the PostgreSQL version currently running.
	// May differ from Spec.Version during a rolling upgrade.
	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`

	// LastBackupTime is when the most recent successful backup completed.
	// +optional
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`

	// BackupBucketProvisioned indicates whether the S3 backup bucket has been
	// created by the operator. Used by the finalizer to decide whether cleanup
	// is needed on deletion.
	// +optional
	BackupBucketProvisioned bool `json:"backupBucketProvisioned,omitempty"`

	// ObservedGeneration is the .metadata.generation the operator last reconciled.
	// Conditions whose ObservedGeneration < metadata.generation are stale.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions provides detailed, machine-readable per-aspect status.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ── Phase ─────────────────────────────────────────────────────────────────────

// DatabaseClusterPhase is a high-level lifecycle summary.
// +kubebuilder:validation:Enum=Provisioning;Running;Upgrading;Degraded;Failed
type DatabaseClusterPhase string

const (
	PhaseProvisioning DatabaseClusterPhase = "Provisioning"
	PhaseRunning      DatabaseClusterPhase = "Running"
	PhaseUpgrading    DatabaseClusterPhase = "Upgrading"
	PhaseDegraded     DatabaseClusterPhase = "Degraded"
	PhaseFailed       DatabaseClusterPhase = "Failed"
)

// ── Condition types ───────────────────────────────────────────────────────────

const (
	// ConditionReady is True when all desired replicas are ready.
	ConditionReady = "Ready"

	// ConditionDegraded is True when fewer than desired replicas are ready.
	// The cluster is still serving at reduced capacity.
	ConditionDegraded = "Degraded"

	// ConditionUpgrading is True when a version upgrade is in progress.
	ConditionUpgrading = "Upgrading"

	// ConditionBackupHealthy is True when the last scheduled backup succeeded.
	ConditionBackupHealthy = "BackupHealthy"
)

// ── Finalizer ─────────────────────────────────────────────────────────────────

// FinalizerName is added to a DatabaseCluster when a backup bucket is provisioned.
// The operator removes this finalizer only after successfully cleaning up the bucket.
// This prevents Kubernetes from deleting the CR until external cleanup is complete.
const FinalizerName = "databases.madmmas.dev/backup-cleanup"

// ── Root object ───────────────────────────────────────────────────────────────

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

