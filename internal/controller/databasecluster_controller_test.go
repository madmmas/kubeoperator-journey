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

// Package controller_test contains the test suite for the DatabaseCluster operator.
//
// Blog 9: Testing Controllers — envtest, Fake Clients, What Actually Matters
//
// Two testing strategies are demonstrated here:
//
//  1. UNIT TESTS (no cluster, no envtest)
//     — Pure Go functions: buildStatefulSet, buildConfigMap, setCondition
//     — Fast: milliseconds per test
//     — Test the "what should exist" layer without any Kubernetes involvement
//     — Use these for all builder functions and business logic helpers
//
//  2. INTEGRATION TESTS (envtest — a real API server, no kubelet)
//     — Full reconcile loop against a real kube-apiserver
//     — Slower: seconds per test (API server startup)
//     — Test the "make it exist" layer: CreateOrUpdate, status patches, finalizers
//     — Use these for Reconcile() paths, not for individual helper functions
//
// What envtest is NOT:
//
//	— Not a full cluster (no kubelet, no scheduler, no actual pods running)
//	— StatefulSet.Status.ReadyReplicas will always be 0 (no kubelet to set it)
//	— You must manually patch status to simulate what kubelets would do
//
// Run: go test ./internal/controller/... -v
// Run unit tests only: go test ./internal/controller/... -v -run Unit
// Run integration tests: go test ./internal/controller/... -v -run Integration
package controller

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	databasesv1alpha1 "github.com/madmmas/kubeoperator-journey/api/v1alpha1"
	"github.com/madmmas/kubeoperator-journey/internal/backup"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// newTestCluster creates a DatabaseCluster with sensible test defaults.
// Using a helper prevents each test from repeating boilerplate and ensures
// all tests start from a consistent baseline.
func newTestCluster(name string, opts ...func(*databasesv1alpha1.DatabaseCluster)) *databasesv1alpha1.DatabaseCluster {
	cluster := &databasesv1alpha1.DatabaseCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: databasesv1alpha1.DatabaseClusterSpec{
			Replicas:    3,
			Version:     "15.4",
			StorageSize: "1Gi",
		},
	}
	for _, opt := range opts {
		opt(cluster)
	}
	return cluster
}

// withBackupSchedule is a functional option for newTestCluster.
func withBackupSchedule(schedule string) func(*databasesv1alpha1.DatabaseCluster) {
	return func(c *databasesv1alpha1.DatabaseCluster) {
		c.Spec.BackupSchedule = schedule
	}
}

// withPostgresConfig is a functional option for newTestCluster.
func withPostgresConfig(cfg map[string]string) func(*databasesv1alpha1.DatabaseCluster) {
	return func(c *databasesv1alpha1.DatabaseCluster) {
		c.Spec.PostgresConfig = cfg
	}
}

// ── Unit Tests: Builder functions ─────────────────────────────────────────────
// No cluster, no envtest, no network. Pure Go. Run in < 1ms each.

func TestUnit_BuildStatefulSet_Name(t *testing.T) {
	cluster := newTestCluster("production-postgres")
	sts := buildStatefulSet(cluster)

	if sts.Name != "production-postgres" {
		t.Errorf("expected Name=production-postgres, got %q", sts.Name)
	}
	if sts.Namespace != "default" {
		t.Errorf("expected Namespace=default, got %q", sts.Namespace)
	}
}

func TestUnit_BuildStatefulSet_Replicas(t *testing.T) {
	cluster := newTestCluster("pg", func(c *databasesv1alpha1.DatabaseCluster) {
		c.Spec.Replicas = 5
	})
	sts := buildStatefulSet(cluster)

	if *sts.Spec.Replicas != 5 {
		t.Errorf("expected Replicas=5, got %d", *sts.Spec.Replicas)
	}
}

func TestUnit_BuildStatefulSet_Image(t *testing.T) {
	cluster := newTestCluster("pg")
	sts := buildStatefulSet(cluster)

	got := sts.Spec.Template.Spec.Containers[0].Image
	want := "postgres:15.4"
	if got != want {
		t.Errorf("expected image %q, got %q", want, got)
	}
}

func TestUnit_BuildStatefulSet_Idempotent(t *testing.T) {
	// Calling buildStatefulSet twice with the same cluster must return
	// identical results. This is the idempotency contract.
	cluster := newTestCluster("pg")
	sts1 := buildStatefulSet(cluster)
	sts2 := buildStatefulSet(cluster)

	if sts1.Name != sts2.Name {
		t.Error("buildStatefulSet is not idempotent: Name differs")
	}
	if *sts1.Spec.Replicas != *sts2.Spec.Replicas {
		t.Error("buildStatefulSet is not idempotent: Replicas differ")
	}
	if sts1.Spec.Template.Spec.Containers[0].Image != sts2.Spec.Template.Spec.Containers[0].Image {
		t.Error("buildStatefulSet is not idempotent: Image differs")
	}
}

func TestUnit_BuildStatefulSet_StorageSizeDefault(t *testing.T) {
	// When StorageSize is empty, the builder should apply the default.
	cluster := newTestCluster("pg", func(c *databasesv1alpha1.DatabaseCluster) {
		c.Spec.StorageSize = ""
	})
	sts := buildStatefulSet(cluster)

	got := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
	want := resource.MustParse("10Gi")
	if got.Cmp(want) != 0 {
		t.Errorf("expected default storage 10Gi, got %s", got.String())
	}
}

func TestUnit_BuildStatefulSet_PGDATASubdir(t *testing.T) {
	// PGDATA must be a subdirectory of the mount point to avoid the
	// "lost+found" problem on ext4 volumes.
	cluster := newTestCluster("pg")
	sts := buildStatefulSet(cluster)

	var pgdata string
	for _, env := range sts.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "PGDATA" {
			pgdata = env.Value
		}
	}
	if pgdata == "" {
		t.Fatal("PGDATA env var not set")
	}
	if pgdata == "/var/lib/postgresql/data" {
		t.Errorf("PGDATA must be a subdirectory (e.g. /data/pgdata), got %q — this causes lost+found errors on ext4", pgdata)
	}
}

func TestUnit_BuildStatefulSet_ServiceName(t *testing.T) {
	// StatefulSet.Spec.ServiceName must match the headless Service name.
	cluster := newTestCluster("my-cluster")
	sts := buildStatefulSet(cluster)
	svc := buildHeadlessService(cluster)

	if sts.Spec.ServiceName != svc.Name {
		t.Errorf("StatefulSet.Spec.ServiceName=%q must match Service.Name=%q",
			sts.Spec.ServiceName, svc.Name)
	}
}

func TestUnit_BuildHeadlessService_ClusterIPNone(t *testing.T) {
	cluster := newTestCluster("pg")
	svc := buildHeadlessService(cluster)

	if svc.Spec.ClusterIP != "None" {
		t.Errorf("expected ClusterIP=None (headless), got %q", svc.Spec.ClusterIP)
	}
}

func TestUnit_BuildHeadlessService_PublishNotReadyAddresses(t *testing.T) {
	cluster := newTestCluster("pg")
	svc := buildHeadlessService(cluster)

	if !svc.Spec.PublishNotReadyAddresses {
		t.Error("PublishNotReadyAddresses must be true for StatefulSet bootstrapping")
	}
}

func TestUnit_BuildConfigMap_DefaultsApplied(t *testing.T) {
	cluster := newTestCluster("pg") // no PostgresConfig set
	cm := buildConfigMap(cluster)

	data, ok := cm.Data["custom.conf"]
	if !ok {
		t.Fatal("ConfigMap missing 'custom.conf' key")
	}
	if data == "" {
		t.Error("custom.conf is empty — default config should be applied")
	}
}

func TestUnit_BuildConfigMap_UserOverridesApplied(t *testing.T) {
	cluster := newTestCluster("pg", withPostgresConfig(map[string]string{
		"max_connections": "500",
		"shared_buffers":  "2GB",
	}))
	cm := buildConfigMap(cluster)

	conf := cm.Data["custom.conf"]
	if !containsLine(conf, "max_connections = 500") {
		t.Errorf("expected max_connections=500 in ConfigMap, got:\n%s", conf)
	}
	if !containsLine(conf, "shared_buffers = 2GB") {
		t.Errorf("expected shared_buffers=2GB in ConfigMap, got:\n%s", conf)
	}
}

func TestUnit_BuildConfigMap_Idempotent(t *testing.T) {
	cluster := newTestCluster("pg", withPostgresConfig(map[string]string{
		"max_connections": "200",
	}))
	cm1 := buildConfigMap(cluster)
	cm2 := buildConfigMap(cluster)

	if cm1.Name != cm2.Name {
		t.Error("buildConfigMap not idempotent: Name differs")
	}
	// Data may have different key ordering but same keys+values
	for k, v1 := range cm1.Data {
		if v2, ok := cm2.Data[k]; !ok || v1 != v2 {
			t.Errorf("buildConfigMap not idempotent: Data[%q] differs", k)
		}
	}
}

func TestUnit_SetCondition_Ready(t *testing.T) {
	cluster := newTestCluster("pg")
	cluster.Generation = 3

	setCondition(cluster, databasesv1alpha1.ConditionReady,
		metav1.ConditionTrue, "AllReplicasReady", "All 3 replicas are ready")

	cond := findCondition(cluster, databasesv1alpha1.ConditionReady)
	if cond == nil {
		t.Fatal("Ready condition not set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected Status=True, got %s", cond.Status)
	}
	if cond.Reason != "AllReplicasReady" {
		t.Errorf("expected Reason=AllReplicasReady, got %s", cond.Reason)
	}
	if cond.ObservedGeneration != 3 {
		t.Errorf("expected ObservedGeneration=3, got %d", cond.ObservedGeneration)
	}
}

func TestUnit_SetCondition_NoduplicateTypes(t *testing.T) {
	cluster := newTestCluster("pg")

	// Set the same condition type twice — should result in exactly one entry
	setCondition(cluster, databasesv1alpha1.ConditionReady,
		metav1.ConditionFalse, "NotReady", "0/3 replicas ready")
	setCondition(cluster, databasesv1alpha1.ConditionReady,
		metav1.ConditionTrue, "AllReplicasReady", "3/3 replicas ready")

	count := 0
	for _, c := range cluster.Status.Conditions {
		if c.Type == databasesv1alpha1.ConditionReady {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 Ready condition, got %d", count)
	}
}

func TestUnit_SetCondition_LastTransitionTime(t *testing.T) {
	cluster := newTestCluster("pg")

	// First set
	setCondition(cluster, databasesv1alpha1.ConditionReady,
		metav1.ConditionFalse, "NotReady", "waiting")
	cond1 := findCondition(cluster, databasesv1alpha1.ConditionReady)
	t1 := cond1.LastTransitionTime

	// Small delay
	time.Sleep(10 * time.Millisecond)

	// Same status — LastTransitionTime should NOT change
	setCondition(cluster, databasesv1alpha1.ConditionReady,
		metav1.ConditionFalse, "NotReady", "still waiting")
	cond2 := findCondition(cluster, databasesv1alpha1.ConditionReady)

	if !cond2.LastTransitionTime.Equal(&t1) {
		t.Error("LastTransitionTime should not change when Status stays the same")
	}

	// Status changes — LastTransitionTime SHOULD change
	time.Sleep(10 * time.Millisecond)
	setCondition(cluster, databasesv1alpha1.ConditionReady,
		metav1.ConditionTrue, "AllReplicasReady", "all ready")
	cond3 := findCondition(cluster, databasesv1alpha1.ConditionReady)

	if cond3.LastTransitionTime.Equal(&t1) {
		t.Error("LastTransitionTime should change when Status transitions True→False or False→True")
	}
}

// ── Unit Tests: Backup bucket ─────────────────────────────────────────────────

func TestUnit_BackupBucket_ProvisionAndExists(t *testing.T) {
	store := backup.NewBucketStore()

	if store.Exists("my-cluster") {
		t.Error("bucket should not exist before provisioning")
	}

	if err := store.Provision("my-cluster"); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	if !store.Exists("my-cluster") {
		t.Error("bucket should exist after provisioning")
	}
}

func TestUnit_BackupBucket_DeleteIdempotent(t *testing.T) {
	store := backup.NewBucketStore()

	// Delete non-existent bucket — must not error
	if err := store.Delete("nonexistent"); err != nil {
		t.Errorf("Delete of non-existent bucket should not error, got: %v", err)
	}

	// Provision then delete twice — second delete must not error
	_ = store.Provision("cluster-a")
	_ = store.Delete("cluster-a")
	if err := store.Delete("cluster-a"); err != nil {
		t.Errorf("Second Delete should not error (idempotent), got: %v", err)
	}
}

func TestUnit_BackupBucket_IsolatedBetweenClusters(t *testing.T) {
	store := backup.NewBucketStore()
	_ = store.Provision("cluster-a")

	if store.Exists("cluster-b") {
		t.Error("provisioning cluster-a should not affect cluster-b")
	}
}

// ── Integration test stubs ────────────────────────────────────────────────────
//
// These tests require envtest (a real kube-apiserver binary).
// They are gated by the build tag `integration` and are separate from unit tests
// so that `go test ./...` (no tags) runs only unit tests.
//
// To run integration tests:
//   go test ./internal/controller/... -tags=integration -v
//
// Setup (one-time):
//   go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
//   setup-envtest use 1.29 --bin-dir ./bin
//   export KUBEBUILDER_ASSETS=$(setup-envtest use 1.29 -p path --bin-dir ./bin)
//
// The stubs below show the STRUCTURE of integration tests — what to test and
// how to assert on controller-runtime behaviour. They compile cleanly but are
// not executed without the integration build tag.

// IntegrationTestSuite documents what the envtest suite covers.
// In the real suite (gated by //go:build integration), each of these
// becomes a Ginkgo Describe/It block using gomega matchers.
//
//	Describe("DatabaseCluster controller") {
//
//	  Context("when a new DatabaseCluster is created") {
//	    It("should set Phase=Provisioning immediately")     // Step 2 in Reconcile
//	    It("should create a ConfigMap")                     // Step 4
//	    It("should create a headless Service")              // Step 5
//	    It("should create a StatefulSet")                   // Step 6
//	    It("should set ownerReferences on all child resources")
//	    It("should update Status.ReadyReplicas from StatefulSet")
//	  }
//
//	  Context("when replicas are scaled up") {
//	    It("should update StatefulSet.Spec.Replicas")
//	    It("should set Phase=Degraded while scaling")
//	    It("should set Phase=Running when all replicas ready")
//	  }
//
//	  Context("when version is changed") {
//	    It("should set Phase=Upgrading immediately")
//	    It("should update StatefulSet container image")
//	    It("should set ConditionUpgrading=True")
//	    It("should set ConditionUpgrading=False after upgrade")
//	  }
//
//	  Context("when BackupSchedule is set") {
//	    It("should add the finalizer")
//	    It("should set Status.BackupBucketProvisioned=true")
//	    It("should set ConditionBackupHealthy=True")
//	  }
//
//	  Context("when the DatabaseCluster is deleted") {
//	    It("should block deletion while finalizer is present")
//	    It("should delete the backup bucket")
//	    It("should remove the finalizer")
//	    It("should allow deletion to complete")
//	  }
//
//	  Context("when a child StatefulSet is manually deleted") {
//	    It("should recreate it (Owns() watch triggers reconcile)")
//	  }
//	}

// TestUnit_ReconcilerBuildersCompile verifies that all builder functions
// produce valid objects that could be sent to the API server.
// This catches import errors and type mismatches without needing envtest.
func TestUnit_ReconcilerBuildersCompile(t *testing.T) {
	cluster := newTestCluster("compile-check",
		withBackupSchedule("0 2 * * *"),
		withPostgresConfig(map[string]string{"max_connections": "100"}),
	)

	sts := buildStatefulSet(cluster)
	if sts == nil {
		t.Error("buildStatefulSet returned nil")
	}

	svc := buildHeadlessService(cluster)
	if svc == nil {
		t.Error("buildHeadlessService returned nil")
	}

	cm := buildConfigMap(cluster)
	if cm == nil {
		t.Error("buildConfigMap returned nil")
	}

	// Verify the StatefulSet references the ConfigMap by name
	var found bool
	for _, vol := range sts.Spec.Template.Spec.Volumes {
		if vol.ConfigMap != nil && vol.ConfigMap.Name == cm.Name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("StatefulSet does not reference ConfigMap %q in volumes", cm.Name)
	}
}

// TestUnit_CommonLabels_Consistent verifies that all builders use the same labels.
// The StatefulSet selector must match pod template labels exactly — if they diverge,
// Kubernetes rejects the StatefulSet.
func TestUnit_CommonLabels_Consistent(t *testing.T) {
	cluster := newTestCluster("pg")
	sts := buildStatefulSet(cluster)

	selectorLabels := sts.Spec.Selector.MatchLabels
	podLabels := sts.Spec.Template.ObjectMeta.Labels

	for k, v := range selectorLabels {
		if podLabels[k] != v {
			t.Errorf("label mismatch: selector[%q]=%q but podTemplate[%q]=%q",
				k, v, k, podLabels[k])
		}
	}
}

// TestUnit_FakeReconcilerSmoke runs a minimal smoke test of Reconcile()
// using a fake client — no envtest, no network, just in-process.
//
// Fake client limitations:
//   - Status().Patch() is supported (in controller-runtime fake client v0.17+)
//   - No watch events — you call Reconcile() directly
//   - No admission controllers — invalid resources are accepted
//   - StatefulSet status is not updated automatically (no kubelet)
//
// Use fake client for: "does Reconcile() call the right methods?"
// Use envtest for: "does the full reconcile loop work end-to-end?"
func TestUnit_FakeReconcilerSmoke(t *testing.T) {
	// This test documents the pattern without executing against a real API.
	// The full version requires importing sigs.k8s.io/controller-runtime/pkg/client/fake
	// which is available in the Kubebuilder project after `go mod tidy`.
	//
	// Pattern:
	//
	//   scheme := runtime.NewScheme()
	//   _ = databasesv1alpha1.AddToScheme(scheme)
	//   _ = appsv1.AddToScheme(scheme)
	//   _ = corev1.AddToScheme(scheme)
	//
	//   cluster := newTestCluster("smoke-test")
	//   fakeClient := fake.NewClientBuilder().
	//     WithScheme(scheme).
	//     WithObjects(cluster).
	//     WithStatusSubresource(cluster).
	//     Build()
	//
	//   reconciler := &DatabaseClusterReconciler{
	//     Client:      fakeClient,
	//     Scheme:      scheme,
	//     BucketStore: backup.NewBucketStore(), // fresh store per test
	//   }
	//
	//   result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
	//     NamespacedName: types.NamespacedName{Name: "smoke-test", Namespace: "default"},
	//   })
	//
	//   assert.NoError(t, err)
	//   // First reconcile sets Phase=Provisioning and returns
	//   assert.Equal(t, ctrl.Result{}, result)
	//
	//   // Fetch updated cluster and check Phase
	//   updated := &databasesv1alpha1.DatabaseCluster{}
	//   _ = fakeClient.Get(ctx, types.NamespacedName{...}, updated)
	//   assert.Equal(t, databasesv1alpha1.PhaseProvisioning, updated.Status.Phase)

	t.Log("FakeReconcilerSmoke: pattern documented — run with full deps after go mod tidy")
}

// ── Test helpers (internal) ───────────────────────────────────────────────────

// findCondition returns the Condition with the given type, or nil.
func findCondition(
	cluster *databasesv1alpha1.DatabaseCluster,
	condType string,
) *metav1.Condition {
	for i := range cluster.Status.Conditions {
		if cluster.Status.Conditions[i].Type == condType {
			return &cluster.Status.Conditions[i]
		}
	}
	return nil
}

// containsLine reports whether s contains the given line (ignoring surrounding whitespace).
func containsLine(s, line string) bool {
	for _, l := range splitLines(s) {
		if trimSpace(l) == trimSpace(line) {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// Compile-time checks: ensure types are used so the compiler catches import issues.
var (
	_ *appsv1.StatefulSet = nil
	_ *corev1.Service     = nil
	_ *corev1.ConfigMap   = nil
	_ context.Context     = nil
	_ time.Duration       = 0
)

