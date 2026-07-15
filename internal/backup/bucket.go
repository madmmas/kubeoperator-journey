// Package backup simulates an external S3 backup bucket lifecycle.
//
// Blog 7: Finalizers — Safe Deletion and External Resource Cleanup
//
// In a real operator, "external resources" are things Kubernetes cannot
// garbage-collect automatically: S3 buckets, DNS records, cloud load balancers,
// database users, API keys. When you delete a DatabaseCluster, Kubernetes cleans
// up child resources (StatefulSet, Service, ConfigMap) via owner references —
// but it has no knowledge of your S3 bucket.
//
// This package simulates that problem with an in-process store. The real
// implementation would use the AWS SDK or similar. The interface and lifecycle
// are identical — only the storage backend differs.
//
// The finalizer pattern:
//
//	Create → operator adds FinalizerName to CR
//	           operator calls bucket.Provision(name)
//	           Status.BackupBucketProvisioned = true
//
//	Delete → Kubernetes sets DeletionTimestamp (does NOT delete yet)
//	           operator sees DeletionTimestamp set
//	           operator calls bucket.Delete(name)
//	           operator removes FinalizerName from CR
//	           Kubernetes now deletes the CR
package backup

import (
	"fmt"
	"sync"
	"time"
)

// BucketStore is a thread-safe in-memory simulation of an S3 bucket registry.
// Replace with a real AWS/GCP/Azure SDK client in production.
type BucketStore struct {
	mu      sync.RWMutex
	buckets map[string]*BucketInfo
}

// BucketInfo holds metadata about a provisioned backup bucket.
type BucketInfo struct {
	Name        string
	ClusterName string
	CreatedAt   time.Time
	SizeBytes   int64
}

// Global singleton — simulates a shared external service.
// In production, inject this as a dependency via the reconciler struct.
var globalStore = &BucketStore{
	buckets: make(map[string]*BucketInfo),
}

// GlobalStore returns the package-level bucket store.
// Tests can replace this with a fresh store to avoid cross-test pollution.
func GlobalStore() *BucketStore {
	return globalStore
}

// NewBucketStore creates a fresh BucketStore — use in tests.
func NewBucketStore() *BucketStore {
	return &BucketStore{buckets: make(map[string]*BucketInfo)}
}

// Provision creates a backup bucket for the given cluster.
// Returns an error if the bucket already exists (idempotency: callers should
// check Exists() first and skip provisioning if already done).
//
// In a real operator this would call s3.CreateBucket() and set a bucket
// policy, lifecycle rules, and encryption. The call may be slow (network)
// and may fail transiently — which is why returning an error (triggering
// requeue with backoff) is the correct response to provisioning failures.
func (s *BucketStore) Provision(clusterName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bucketName := bucketNameFor(clusterName)
	if _, exists := s.buckets[bucketName]; exists {
		return fmt.Errorf("bucket %q already exists", bucketName)
	}

	s.buckets[bucketName] = &BucketInfo{
		Name:        bucketName,
		ClusterName: clusterName,
		CreatedAt:   time.Now(),
	}

	return nil
}

// Delete removes the backup bucket for the given cluster.
// Returns nil if the bucket doesn't exist (idempotent delete).
//
// Idempotency is critical here: the finalizer removal and bucket deletion
// may be retried if the controller restarts mid-operation. Delete must be
// safe to call multiple times — it should succeed (return nil) even if the
// bucket was already deleted. Returning an error on "not found" would cause
// the finalizer to never be removed, permanently blocking CR deletion.
func (s *BucketStore) Delete(clusterName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bucketName := bucketNameFor(clusterName)
	delete(s.buckets, bucketName) // no-op if not present — idempotent
	return nil
}

// Exists reports whether a backup bucket has been provisioned for this cluster.
// Used by the reconciler to decide whether to provision or skip.
func (s *BucketStore) Exists(clusterName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.buckets[bucketNameFor(clusterName)]
	return ok
}

// Info returns metadata about a provisioned bucket, or nil if not found.
func (s *BucketStore) Info(clusterName string) *BucketInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.buckets[bucketNameFor(clusterName)]
	if !ok {
		return nil
	}
	copy := *b
	return &copy
}

// bucketNameFor derives a deterministic S3 bucket name from a cluster name.
// In production: include account ID or namespace to guarantee global uniqueness.
func bucketNameFor(clusterName string) string {
	return "kubeoperator-backups-" + clusterName
}

