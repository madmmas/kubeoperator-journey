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

// resources.go — pure builder functions for all child resources.
//
// Blog 5: Creating Resources From a CRD — Idempotency and the Reconcile Problem
//
// Every function in this file is a pure builder: it takes a DatabaseCluster
// and returns a Kubernetes object with zero side effects. No API calls, no
// mutations to cluster state, no randomness.
//
// This matters for two reasons:
//  1. Pure builders are trivially unit-testable without a real cluster.
//  2. Separating "what should exist" (builders) from "make it exist"
//     (CreateOrUpdate calls in reconciler.go) makes idempotency easier
//     to reason about. Each builder is called on every reconcile; the
//     CreateOrUpdate in the reconciler decides whether an API call is
//     actually needed.
//
// The idempotency contract: calling any builder twice with the same input
// must return an identical output. No UUIDs, no timestamps, no random values.
package controller

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	databasesv1alpha1 "github.com/madmmas/kubeoperator-journey/api/v1alpha1"
)

// commonLabels returns the standard label set applied to all child resources.
//
// Using app.kubernetes.io/* labels (the Kubernetes recommended label schema)
// makes resources discoverable by standard tooling: Helm, Kustomize, dashboards,
// and label-based network policies all understand these labels.
//
// Critically: these labels are the selector for the StatefulSet. They must be
// identical between the StatefulSet's .spec.selector and the pod template's
// .metadata.labels — Kubernetes rejects StatefulSets where they don't match,
// and the selector is immutable after creation.
func commonLabels(cluster *databasesv1alpha1.DatabaseCluster) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "databasecluster",
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/managed-by": "kubeoperator-journey",
		"app.kubernetes.io/component":  "database",
	}
}

// ── StatefulSet ───────────────────────────────────────────────────────────────

// buildStatefulSet constructs the desired StatefulSet for a DatabaseCluster.
//
// Key idempotency decisions:
//   - Name is always cluster.Name — same input, same name, every time
//   - Labels come from commonLabels — deterministic, no dynamic values
//   - VolumeClaimTemplates use cluster.Spec.StorageSize with a safe default
//   - The postgres image is derived from cluster.Spec.Version deterministically
//
// What we deliberately do NOT set:
//   - updateStrategy — we let the StatefulSet default (RollingUpdate) apply
//   - podManagementPolicy — we let OrderedReady (default) apply
//   - revisionHistoryLimit — not our concern
//
// Leaving framework-managed fields at their defaults means our operator
// doesn't fight with admission controllers or users who patch those fields.
func buildStatefulSet(cluster *databasesv1alpha1.DatabaseCluster) *appsv1.StatefulSet {
	labels := commonLabels(cluster)

	storageSize := cluster.Spec.StorageSize
	if storageSize == "" {
		storageSize = "10Gi"
	}

	// configMapName is the name of the ConfigMap we manage (see buildConfigMap).
	// The StatefulSet mounts it — so if the ConfigMap doesn't exist yet,
	// pods will fail to start. The reconciler must create the ConfigMap BEFORE
	// the StatefulSet to avoid a pod-level error on first provision.
	configMapName := configMapName(cluster)

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &cluster.Spec.Replicas,
			ServiceName: headlessServiceName(cluster), // must match headless Service name
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "postgres",
						Image: postgresImage(cluster.Spec.Version),
						Ports: []corev1.ContainerPort{
							{Name: "postgres", ContainerPort: 5432, Protocol: corev1.ProtocolTCP},
						},
						Env: []corev1.EnvVar{
							{Name: "POSTGRES_PASSWORD", Value: "changeme"},
							// PGDATA must be a subdirectory of the mount point.
							// PostgreSQL refuses to start if PGDATA contains lost+found
							// (which ext4 volumes have at the root). This is a common
							// "my operator worked in CI but not on real nodes" bug.
							{Name: "PGDATA", Value: "/var/lib/postgresql/data/pgdata"},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"pg_isready", "-U", "postgres", "-d", "postgres"},
								},
							},
							InitialDelaySeconds: 10,
							PeriodSeconds:       5,
							FailureThreshold:    3,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "data", MountPath: "/var/lib/postgresql/data"},
							{
								Name:      "config",
								MountPath: "/etc/postgresql/conf.d",
								ReadOnly:  true,
							},
						},
					}},
					Volumes: []corev1.Volume{{
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: configMapName,
								},
							},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: "data"},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(storageSize),
						},
					},
				},
			}},
		},
	}
}

// ── Headless Service ──────────────────────────────────────────────────────────

// buildHeadlessService constructs the headless Service for a DatabaseCluster.
//
// Why a headless Service (ClusterIP: None)?
//
// A regular Service load-balances traffic across pods. That's wrong for
// databases — you need to connect to a SPECIFIC replica (the primary for
// writes, a specific secondary for read routing). A headless Service creates
// DNS A records for each pod individually:
//
//	production-postgres-0.production-postgres.default.svc.cluster.local → pod IP
//	production-postgres-1.production-postgres.default.svc.cluster.local → pod IP
//	production-postgres-2.production-postgres.default.svc.cluster.local → pod IP
//
// StatefulSets require a headless Service — it's what gives pods their stable
// DNS identity. Without it, pod DNS names don't resolve and replication between
// replicas breaks.
//
// Note: StatefulSet.Spec.ServiceName must match this Service's name.
func buildHeadlessService(cluster *databasesv1alpha1.DatabaseCluster) *corev1.Service {
	labels := commonLabels(cluster)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      headlessServiceName(cluster),
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			// ClusterIP: None makes this a headless Service.
			// Kubernetes skips kube-proxy for headless Services and creates
			// individual DNS records per pod instead of a single virtual IP.
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{{
				Name:     "postgres",
				Port:     5432,
				Protocol: corev1.ProtocolTCP,
			}},
			// PublishNotReadyAddresses: true means DNS records are created
			// even for pods that haven't passed their readiness probe yet.
			// This is needed for StatefulSet bootstrapping — pods need to
			// discover each other before they're all ready (e.g. primary
			// must be reachable before secondaries can start replication).
			PublishNotReadyAddresses: true,
		},
	}
}

// headlessServiceName returns the deterministic name for the headless Service.
// Centralising this prevents the StatefulSet's ServiceName and the actual
// Service name from drifting apart.
func headlessServiceName(cluster *databasesv1alpha1.DatabaseCluster) string {
	return cluster.Name
}

// ── ConfigMap ─────────────────────────────────────────────────────────────────

// buildConfigMap constructs the postgresql.conf ConfigMap for a DatabaseCluster.
//
// Why a ConfigMap for config?
//
// Baking postgresql.conf into the container image means every config change
// requires a new image build. Storing it in a ConfigMap means:
//   - Config changes are applied without rebuilding the image
//   - Config is version-controlled alongside the DatabaseCluster CR
//   - Different clusters can have different configs (OLTP vs analytics)
//   - kubectl describe configmap shows the current running config
//
// Idempotency note: the ConfigMap data is derived entirely from
// cluster.Spec.PostgresConfig. Same spec → same ConfigMap, always.
// When Spec.PostgresConfig changes, CreateOrUpdate detects the diff
// and patches the ConfigMap — which triggers a pod restart via the
// volume mount (Kubernetes re-mounts ConfigMap volumes automatically).
func buildConfigMap(cluster *databasesv1alpha1.DatabaseCluster) *corev1.ConfigMap {
	labels := commonLabels(cluster)

	// Start with safe production defaults.
	// These are conservative values that work for most workloads.
	// Users override them via Spec.PostgresConfig.
	config := map[string]string{
		"max_connections":              "100",
		"shared_buffers":               "128MB",
		"effective_cache_size":         "512MB",
		"maintenance_work_mem":         "64MB",
		"checkpoint_completion_target": "0.9",
		"wal_buffers":                  "16MB",
		"default_statistics_target":    "100",
		"random_page_cost":             "1.1",
		"effective_io_concurrency":     "200",
		"log_timezone":                 "UTC",
		"timezone":                     "UTC",
	}

	// Apply user overrides — these take precedence over defaults.
	// We do a shallow merge: user values overwrite defaults, defaults
	// fill in anything the user didn't specify.
	for k, v := range cluster.Spec.PostgresConfig {
		config[k] = v
	}

	// Render to postgresql.conf format: one "key = value" per line.
	// ConfigMap data is a map[string]string — we use a single "custom.conf"
	// key containing the full conf file content.
	confContent := ""
	for k, v := range config {
		confContent += k + " = " + v + "\n"
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName(cluster),
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"custom.conf": confContent,
		},
	}
}

// configMapName returns the deterministic name for the postgresql ConfigMap.
func configMapName(cluster *databasesv1alpha1.DatabaseCluster) string {
	return cluster.Name + "-config"
}

// ── Image helper ──────────────────────────────────────────────────────────────

// postgresImage maps a version string to a container image reference.
// Centralised here so all builders use the same resolution logic.
func postgresImage(version string) string {
	return "postgres:" + version
}

