// Package diagnostics provides debugging tools for Kubernetes operators.
//
// Blog 8: Why Is My Controller Not Reconciling? — Debugging Guide
//
// This package is a standalone diagnostic tool — it does NOT import
// controller-runtime or k8s API packages so it can be used without
// the full operator dependency tree. Real cluster interaction is done
// via kubectl commands documented in DebugCommands().
package diagnostics

import (
	"fmt"
	"io"
	"strings"
)

// Finding represents a single diagnostic observation.
type Finding struct {
	Severity Severity
	Category string
	Message  string
	Hint     string
}

// Severity classifies the impact of a finding.
type Severity string

const (
	SeverityError   Severity = "ERROR"
	SeverityWarning Severity = "WARNING"
	SeverityInfo    Severity = "INFO"
)

// ReconcileChecker holds context for diagnostic checks.
type ReconcileChecker struct {
	Namespace string
	Out       io.Writer
}

// CheckRBACHints returns diagnostic findings for common RBAC misconfigurations.
//
// Blog 8: RBAC is the most common reason a controller appears to run but
// doesn't create resources. The operator pod starts, Reconcile() is called,
// but the API server returns 403 Forbidden when trying to create a StatefulSet.
//
// Symptoms:
//   - "FailedCreate" events on the DatabaseCluster
//   - "forbidden: User ... cannot create resource" in operator logs
//   - kubectl describe shows no child resources created
//
// Root causes:
//   - Missing //+kubebuilder:rbac marker for a resource type you added
//   - Ran `make manifests` but forgot `make install` to apply the new ClusterRole
//   - ServiceAccount name in Deployment doesn't match the one in ClusterRoleBinding
//   - Operator deployed to a different namespace than the ClusterRoleBinding expects
func (c *ReconcileChecker) CheckRBACHints(resource, serviceAccount string) []Finding {
	return []Finding{
		{
			Severity: SeverityInfo,
			Category: "RBAC",
			Message:  "Verify operator permissions with kubectl auth can-i",
			Hint: fmt.Sprintf(`kubectl auth can-i create statefulsets \
  --as=system:serviceaccount:%s:%s -n %s
kubectl auth can-i update %s/status \
  --as=system:serviceaccount:%s:%s -n %s
kubectl auth can-i update %s/finalizers \
  --as=system:serviceaccount:%s:%s -n %s`,
				c.Namespace, serviceAccount, c.Namespace,
				resource, c.Namespace, serviceAccount, c.Namespace,
				resource, c.Namespace, serviceAccount, c.Namespace,
			),
		},
		{
			Severity: SeverityInfo,
			Category: "RBAC",
			Message:  "If auth can-i returns 'no', regenerate and reapply RBAC manifests",
			Hint: `make manifests   # regenerate ClusterRole from //+kubebuilder:rbac markers
make install     # apply CRDs
kubectl apply -f config/rbac/  # apply updated ClusterRole and ClusterRoleBinding`,
		},
	}
}

// CheckWatchHints returns findings for missing watch registrations.
//
// Blog 8: If a child resource changes but Reconcile() isn't triggered,
// the most likely cause is a missing Owns() in SetupWithManager.
//
// Example: you added a ConfigMap in Blog 5, but forgot to add
// .Owns(&corev1.ConfigMap{}) in SetupWithManager. The ConfigMap is created
// correctly, but if someone deletes it manually, the controller never notices
// and doesn't recreate it.
func (c *ReconcileChecker) CheckWatchHints() []Finding {
	return []Finding{
		{
			Severity: SeverityInfo,
			Category: "WatchRegistration",
			Message:  "Verify all owned resource types are registered with Owns()",
			Hint: `// In SetupWithManager, confirm you have:
ctrl.NewControllerManagedBy(mgr).
    For(&databasesv1alpha1.DatabaseCluster{}).
    Owns(&appsv1.StatefulSet{}).   // triggers reconcile when StatefulSet changes
    Owns(&corev1.Service{}).       // triggers reconcile when Service changes  
    Owns(&corev1.ConfigMap{}).     // triggers reconcile when ConfigMap changes
    Complete(r)`,
		},
		{
			Severity: SeverityInfo,
			Category: "WatchRegistration",
			Message:  "Test watch by manually deleting a child resource",
			Hint: `kubectl delete statefulset <name> -n ` + c.Namespace + `
# Watch operator logs — should see "Reconciling" within 1-2 seconds
# If nothing happens, the watch is not registered`,
		},
	}
}

// CheckStatusLoopHints returns findings for infinite reconcile loops.
//
// Blog 8: Status loops are subtle. The controller runs continuously,
// Reconcile() succeeds every time, but the resource never settles.
// Metrics show reconcile_total climbing without bound.
//
// Root causes:
//  1. Using r.Update() for status (should be r.Status().Patch())
//     r.Update() changes resourceVersion → triggers MODIFIED watch event → infinite loop
//  2. Setting a time-dependent field unconditionally
//     status.lastChecked = time.Now() on every reconcile always produces a diff
//  3. Not using client.MergeFrom() snapshot
//     Patch without a snapshot sends the whole object → always triggers an update
func (c *ReconcileChecker) CheckStatusLoopHints() []Finding {
	return []Finding{
		{
			Severity: SeverityWarning,
			Category: "StatusLoop",
			Message:  "Check reconcile rate — should be near-zero when cluster is steady",
			Hint: `# Port-forward to operator metrics
kubectl port-forward <operator-pod> 8080:8080 -n ` + c.Namespace + ` &
# Check reconcile rate — high rate with result="success" = loop
curl -s localhost:8080/metrics | grep controller_runtime_reconcile_total`,
		},
		{
			Severity: SeverityInfo,
			Category: "StatusLoop",
			Message:  "Common fixes for status loops",
			Hint: strings.Join([]string{
				"1. Replace r.Status().Update() with r.Status().Patch()",
				"   patch := client.MergeFrom(cluster.DeepCopy())  // snapshot BEFORE changes",
				"   cluster.Status.Phase = PhaseRunning",
				"   r.Status().Patch(ctx, cluster, patch)  // sends only the diff",
				"",
				"2. Never set time-dependent fields unconditionally",
				"   BAD:  status.lastChecked = time.Now()",
				"   GOOD: set lastBackupTime only when a backup actually completes",
				"",
				"3. Use metav1.SetStatusCondition — it skips the update if nothing changed",
			}, "\n"),
		},
	}
}

// CheckFinalizerHints returns findings for stuck finalizers.
//
// Blog 8: A stuck finalizer permanently blocks CR deletion.
// kubectl delete hangs, the CR shows DeletionTimestamp but never disappears.
//
// Causes:
//   - The cleanup function (reconcileDeletion) returns an error on every call
//   - The operator pod crashed and isn't running
//   - The finalizer string in the CR doesn't match FinalizerName constant
func (c *ReconcileChecker) CheckFinalizerHints(resourceName string) []Finding {
	return []Finding{
		{
			Severity: SeverityInfo,
			Category: "Finalizer",
			Message:  "Check if a finalizer is blocking deletion",
			Hint: fmt.Sprintf(
				`kubectl get databasecluster %s -n %s \
  -o jsonpath='{.metadata.finalizers}'
# Expected when backup is configured: ["databases.madmmas.dev/backup-cleanup"]
# Expected when deletion is clean: []`, resourceName, c.Namespace),
		},
		{
			Severity: SeverityWarning,
			Category: "Finalizer",
			Message:  "If the operator is down and CR is stuck, check operator logs first",
			Hint: fmt.Sprintf(
				`# Check if operator is running
kubectl get pods -n %s | grep operator
# Check deletion error in logs
kubectl logs <operator-pod> -n %s | grep "deletion\|finalizer\|ERROR"
# LAST RESORT — force remove finalizer (risks orphaning external resources)
# kubectl patch databasecluster %s -n %s \
#   -p '{"metadata":{"finalizers":[]}}' --type=merge`,
				c.Namespace, c.Namespace, resourceName, c.Namespace),
		},
	}
}

// PrintReport writes a human-readable diagnostic report.
func (c *ReconcileChecker) PrintReport(findings []Finding) {
	fmt.Fprintf(c.Out, "\n━━━ Operator Diagnostic Report ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	counts := map[Severity]int{}
	for _, f := range findings {
		counts[f.Severity]++
		icon := map[Severity]string{
			SeverityError:   "✗",
			SeverityWarning: "⚠",
			SeverityInfo:    "ℹ",
		}[f.Severity]

		fmt.Fprintf(c.Out, "%s [%s] %s\n", icon, f.Category, f.Message)
		if f.Hint != "" {
			for _, line := range strings.Split(f.Hint, "\n") {
				fmt.Fprintf(c.Out, "    %s\n", line)
			}
		}
		fmt.Fprintln(c.Out)
	}

	fmt.Fprintf(c.Out, "Summary: %d errors, %d warnings, %d info\n\n",
		counts[SeverityError], counts[SeverityWarning], counts[SeverityInfo])
}

// DebugCommands returns the standard kubectl debugging commands.
func DebugCommands(namespace, resourceName, operatorPodName string) string {
	return fmt.Sprintf(`
━━━ Standard Operator Debugging Checklist ━━━━━━━━━━━━━━━━━━━━━

# 1. Is the operator pod running?
kubectl get pods -n %[1]s | grep operator

# 2. What are the operator logs showing?
kubectl logs %[3]s -n %[1]s --tail=100 -f

# 3. What events has Kubernetes recorded?
kubectl describe databasecluster %[2]s -n %[1]s | tail -20

# 4. What is the current status and conditions?
kubectl get databasecluster %[2]s -n %[1]s -o yaml | grep -A 40 "status:"

# 5. Do child resources exist?
kubectl get statefulset,service,configmap -n %[1]s \
  -l app.kubernetes.io/instance=%[2]s

# 6. Are pods healthy?
kubectl get pods -n %[1]s -l app.kubernetes.io/instance=%[2]s
kubectl describe pod %[2]s-0 -n %[1]s | grep -A 10 "Events:"

# 7. RBAC check
kubectl auth can-i create statefulsets \
  --as=system:serviceaccount:%[1]s:kubeoperator-journey -n %[1]s
kubectl auth can-i update databaseclusters/status \
  --as=system:serviceaccount:%[1]s:kubeoperator-journey -n %[1]s

# 8. Finalizer check
kubectl get databasecluster %[2]s -n %[1]s \
  -o jsonpath='{.metadata.finalizers}'

# 9. Reconcile metrics (is it looping?)
kubectl port-forward %[3]s 8080:8080 -n %[1]s &
curl -s localhost:8080/metrics | grep reconcile_total

# 10. Watch events in real time
kubectl get events -n %[1]s -w --field-selector involvedObject.name=%[2]s
`, namespace, resourceName, operatorPodName)
}

