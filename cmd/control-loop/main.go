// cmd/control-loop — companion code for Blog 2
// Run: go run ./cmd/control-loop
package main

import (
	"fmt"
	"time"

	"github.com/madmmas/kubeoperator-journey/internal/reconciler"
	"github.com/madmmas/kubeoperator-journey/internal/watcher"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  KubeOperator Journey — Blog 2: The Control Loop             ║")
	fmt.Println("║  Building a Kubernetes-style controller from scratch          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	store := watcher.NewStore()
	ctrl  := reconciler.NewController(store)
	ctrl.Start()
	time.Sleep(100 * time.Millisecond)

	printScene("1", "User applies a DatabaseCluster resource")
	fmt.Println("  $ kubectl apply -f database-cluster.yaml")
	fmt.Println()
	store.Set("default/production-postgres", &reconciler.DesiredState{
		Name: "production-postgres", Replicas: 3, Version: "15.4",
	})
	time.Sleep(1500 * time.Millisecond)

	printScene("2", "User scales the cluster: replicas 3 → 5")
	fmt.Println("  $ kubectl patch databasecluster production-postgres --patch '{\"spec\":{\"replicas\":5}}'")
	fmt.Println()
	store.Set("default/production-postgres", &reconciler.DesiredState{
		Name: "production-postgres", Replicas: 5, Version: "15.4",
	})
	time.Sleep(1500 * time.Millisecond)

	printScene("3", "Security patch: upgrading version 15.4 → 15.5")
	fmt.Println("  $ kubectl patch databasecluster production-postgres --patch '{\"spec\":{\"version\":\"15.5\"}}'")
	fmt.Println()
	store.Set("default/production-postgres", &reconciler.DesiredState{
		Name: "production-postgres", Replicas: 5, Version: "15.5",
	})
	time.Sleep(2000 * time.Millisecond)

	printScene("4", "10 rapid updates — watch deduplication in action")
	fmt.Println("  Simulating: Helm upgrade touching the resource 10 times quickly")
	fmt.Println()
	for i := 1; i <= 10; i++ {
		store.Set("default/production-postgres", &reconciler.DesiredState{
			Name: "production-postgres", Replicas: 5, Version: "15.5",
		})
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(2000 * time.Millisecond)

	printScene("5", "Second resource added — controller manages all instances")
	fmt.Println("  $ kubectl apply -f staging-cluster.yaml")
	fmt.Println()
	store.Set("default/staging-postgres", &reconciler.DesiredState{
		Name: "staging-postgres", Replicas: 1, Version: "15.5",
	})
	time.Sleep(1500 * time.Millisecond)

	printScene("6", "Deleting the staging cluster")
	fmt.Println("  $ kubectl delete databasecluster staging-postgres")
	fmt.Println()
	store.Delete("default/staging-postgres")
	time.Sleep(1000 * time.Millisecond)

	ctrl.Stop()
	time.Sleep(100 * time.Millisecond)

	fmt.Println()
	fmt.Println("━━━ SUMMARY ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Total Reconcile() calls: %d\n\n", ctrl.ReconcileCount())

	if s, ok := ctrl.ActualStateFor("default/production-postgres"); ok {
		fmt.Printf("  production-postgres: replicas=%d  version=%s  healthy=%v\n",
			s.RunningReplicas, s.CurrentVersion, s.Healthy)
	}
	_, stagingExists := ctrl.ActualStateFor("default/staging-postgres")
	fmt.Printf("  staging-postgres:    exists=%v (deleted successfully)\n\n", stagingExists)

	fmt.Println("━━━ WHAT THIS TAUGHT US ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("  1. Watch → Enqueue → Reconcile is the entire pattern")
	fmt.Println("  2. Reconcile() works from current state, not from the event")
	fmt.Println("  3. Rapid updates get deduplicated — level-triggered, not edge-triggered")
	fmt.Println("  4. Failures requeue automatically with backoff")
	fmt.Println("  5. Reconcile() must be idempotent — it runs multiple times")
	fmt.Println()
	fmt.Println("  Kubebuilder generates all of this infrastructure for you.")
	fmt.Println("  You only write Reconcile().")
	fmt.Println()
	fmt.Println("  That's Blog 3.")
}

func printScene(num, description string) {
	fmt.Printf("\n━━━ SCENE %s: %s ━━━\n\n", num, description)
}
