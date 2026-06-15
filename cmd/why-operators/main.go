// cmd/why-operators — companion code for Blog 1
// Run: go run ./cmd/why-operators
package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/madmmas/kubeoperator-journey/internal/problem"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  KubeOperator Journey — Blog 1: Why Operators Exist          ║")
	fmt.Println("║  Simulating life WITHOUT a Kubernetes Operator                ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	ops := problem.NewManualOperator()

	fmt.Println("━━━ STEP 1: Provisioning ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	clusters := []struct{ name string; replicas int; version string }{
		{"production-postgres", 3, "15.3"},
		{"staging-postgres", 1, "15.3"},
		{"analytics-postgres", 2, "14.8"},
	}
	for _, c := range clusters {
		if err := ops.Provision(c.name, c.replicas, c.version); err != nil {
			fmt.Printf("\n⚠  Provisioning failed: %v\n", err)
			fmt.Println("   With an operator: it retries automatically and reports status.")
			fmt.Println()
		}
		fmt.Println()
	}

	fmt.Println("━━━ STEP 2: Health Checks ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	ops.CheckHealth("production-postgres")
	ops.CheckHealth("staging-postgres")
	fmt.Println()

	fmt.Println("━━━ STEP 3: Scaling ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if err := ops.ScaleUp("production-postgres", 2); err != nil {
		fmt.Printf("   !! Scale failed: %v\n\n", err)
	}
	fmt.Println()

	fmt.Println("━━━ STEP 4: Backup Configuration ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	ops.ScheduleBackup("production-postgres", "s3://company-backups/prod-postgres")
	fmt.Println()

	fmt.Println("━━━ STEP 5: Version Upgrade ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if err := ops.UpgradeVersion("production-postgres", "15.4"); err != nil {
		fmt.Printf("   !! UPGRADE FAILED: %v\n", err)
		fmt.Println("   With an operator: automatic rollback, status update, alert.")
	}
	fmt.Println()

	ops.PrintSummary()

	fmt.Println()
	fmt.Println("━━━ THE OPERATOR ALTERNATIVE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("Open: internal/crd/database-cluster-example.yaml")
	fmt.Println("You declare what you want. The operator figures out how.")
}
