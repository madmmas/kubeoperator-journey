// Package problem demonstrates the manual toil that Kubernetes Operators replace.
//
// This is NOT how you'd build real infrastructure. This is a faithful simulation
// of what engineers actually do before operators exist: write scripts, run them
// manually, hope nothing drifts, get paged at 2am when it does.
package problem

import (
	"fmt"
	"math/rand"
	"time"
)

// DatabaseCluster represents a stateful app that needs careful lifecycle management.
type DatabaseCluster struct {
	Name      string
	Replicas  int
	Version   string
	BackupURL string
	Status    ClusterStatus
}

// ClusterStatus is what an operator would track automatically.
type ClusterStatus string

const (
	StatusUnknown      ClusterStatus = "Unknown"
	StatusProvisioning ClusterStatus = "Provisioning"
	StatusRunning      ClusterStatus = "Running"
	StatusDegraded     ClusterStatus = "Degraded"
	StatusFailed       ClusterStatus = "Failed"
)

// ManualOperator simulates what a human operator (or a fragile shell script)
// does to manage a database cluster without a Kubernetes Operator.
type ManualOperator struct {
	clusters map[string]*DatabaseCluster
}

func NewManualOperator() *ManualOperator {
	return &ManualOperator{clusters: make(map[string]*DatabaseCluster)}
}

func (m *ManualOperator) Provision(name string, replicas int, version string) error {
	fmt.Printf("[MANUAL] Provisioning cluster %q — %d replicas, version %s\n", name, replicas, version)
	fmt.Println("[MANUAL]   Step 1: SSH into node, create PersistentVolume manually...")
	time.Sleep(200 * time.Millisecond)
	fmt.Println("[MANUAL]   Step 2: Apply StatefulSet YAML (hope it's the right version)...")
	time.Sleep(200 * time.Millisecond)
	fmt.Println("[MANUAL]   Step 3: Wait for pods... (checking every 10s like it's 2014)...")
	time.Sleep(300 * time.Millisecond)
	fmt.Println("[MANUAL]   Step 4: Configure replication manually between nodes...")
	time.Sleep(200 * time.Millisecond)

	if rand.Float32() < 0.3 {
		fmt.Println("[MANUAL]   !! FAILED: Node not ready. Check Slack. Good luck.")
		return fmt.Errorf("provisioning failed for cluster %q: node not ready", name)
	}

	m.clusters[name] = &DatabaseCluster{
		Name: name, Replicas: replicas, Version: version, Status: StatusRunning,
	}
	fmt.Printf("[MANUAL]   ✓ Cluster %q is running (took ~2 hours in real life)\n", name)
	return nil
}

func (m *ManualOperator) CheckHealth(name string) ClusterStatus {
	cluster, ok := m.clusters[name]
	if !ok {
		return StatusUnknown
	}
	fmt.Printf("[MANUAL] Checking health of %q...\n", name)
	fmt.Println("[MANUAL]   Connecting to each replica... (no readiness probe, just hope)")
	time.Sleep(150 * time.Millisecond)

	if rand.Float32() < 0.2 {
		cluster.Status = StatusDegraded
		fmt.Printf("[MANUAL]   !! Replica drift detected in %q. Add to backlog.\n", name)
	}
	fmt.Printf("[MANUAL]   Status: %s\n", cluster.Status)
	return cluster.Status
}

func (m *ManualOperator) ScaleUp(name string, additional int) error {
	cluster, ok := m.clusters[name]
	if !ok {
		return fmt.Errorf("cluster %q not found", name)
	}
	fmt.Printf("[MANUAL] Scaling %q from %d to %d replicas...\n", name, cluster.Replicas, cluster.Replicas+additional)
	fmt.Println("[MANUAL]   Editing StatefulSet YAML by hand...")
	fmt.Println("[MANUAL]   Hoping replicas join cluster automatically (they often don't)...")
	time.Sleep(300 * time.Millisecond)
	cluster.Replicas += additional
	fmt.Printf("[MANUAL]   ✓ Scaled. Probably. Check again in an hour.\n")
	return nil
}

func (m *ManualOperator) ScheduleBackup(name, s3URL string) error {
	cluster, ok := m.clusters[name]
	if !ok {
		return fmt.Errorf("cluster %q not found", name)
	}
	fmt.Printf("[MANUAL] Scheduling backup for %q → %s\n", name, s3URL)
	fmt.Println("[MANUAL]   SSHing into each node to configure cron...")
	fmt.Println("[MANUAL]   Creating IAM role manually (or using a shared key, let's be honest)...")
	time.Sleep(200 * time.Millisecond)
	cluster.BackupURL = s3URL
	fmt.Printf("[MANUAL]   ✓ Backup scheduled. Probably.\n")
	return nil
}

func (m *ManualOperator) UpgradeVersion(name, newVersion string) error {
	cluster, ok := m.clusters[name]
	if !ok {
		return fmt.Errorf("cluster %q not found", name)
	}
	fmt.Printf("[MANUAL] Upgrading %q from %s to %s\n", name, cluster.Version, newVersion)
	fmt.Println("[MANUAL]   Step 1: Read 47-step upgrade runbook...")
	fmt.Println("[MANUAL]   Step 2: Take manual snapshot (hope there's enough disk space)...")
	fmt.Println("[MANUAL]   Step 3: Rolling restart, praying for no data loss...")
	time.Sleep(500 * time.Millisecond)

	if rand.Float32() < 0.4 {
		cluster.Status = StatusFailed
		return fmt.Errorf("upgrade failed at step 3: replica 2 didn't rejoin cluster. Page the DBA")
	}
	cluster.Version = newVersion
	cluster.Status = StatusRunning
	fmt.Printf("[MANUAL]   ✓ Upgraded to %s. Incident report not required (this time).\n", newVersion)
	return nil
}

func (m *ManualOperator) PrintSummary() {
	fmt.Println("\n[MANUAL] ── Current Cluster Inventory ──────────────────────────")
	if len(m.clusters) == 0 {
		fmt.Println("[MANUAL]   No clusters tracked (or the tracking sheet was lost)")
		return
	}
	for _, c := range m.clusters {
		fmt.Printf("[MANUAL]   %-20s  replicas=%-3d  version=%-10s  status=%s\n",
			c.Name, c.Replicas, c.Version, c.Status)
	}
	fmt.Println("[MANUAL] ────────────────────────────────────────────────────────")
}
