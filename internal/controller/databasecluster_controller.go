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

package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	databasesv1alpha1 "github.com/madmmas/kubeoperator-journey/api/v1alpha1"
	"github.com/madmmas/kubeoperator-journey/internal/backup"
)

type DatabaseClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	BucketStore *backup.BucketStore
}

//+kubebuilder:rbac:groups=databases.madmmas.dev,resources=databaseclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=databases.madmmas.dev,resources=databaseclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=databases.madmmas.dev,resources=databaseclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *DatabaseClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. Fetch
	var cluster databasesv1alpha1.DatabaseCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetching DatabaseCluster: %w", err)
	}

	log.Info("Reconciling",
		"name", cluster.Name,
		"generation", cluster.Generation,
		"phase", cluster.Status.Phase,
		"deletionTimestamp", cluster.DeletionTimestamp,
	)

	// 2. Deletion path — handle finalizer cleanup first
	if !cluster.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, &cluster)
	}

	// 3. Initial Phase on first creation
	if cluster.Status.Phase == "" {
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Phase = databasesv1alpha1.PhaseProvisioning
		cluster.Status.ObservedGeneration = cluster.Generation
		if err := r.Status().Patch(ctx, &cluster, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting initial phase: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// 4. ConfigMap — must be created before StatefulSet
	cm := buildConfigMap(&cluster)
	if err := controllerutil.SetControllerReference(&cluster, cm, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("owner ref ConfigMap: %w", err)
	}
	cmResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Data = buildConfigMap(&cluster).Data
		return nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling ConfigMap: %w", err)
	}
	log.Info("ConfigMap reconciled", "result", cmResult)

	// 5. Headless Service — must exist before StatefulSet pods schedule
	svc := buildHeadlessService(&cluster)
	if err := controllerutil.SetControllerReference(&cluster, svc, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("owner ref Service: %w", err)
	}
	svcResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Spec.Selector = commonLabels(&cluster)
		svc.Spec.PublishNotReadyAddresses = true
		return nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling Service: %w", err)
	}
	log.Info("Service reconciled", "result", svcResult)

	// 6. Detect version upgrade before touching StatefulSet
	upgrading := cluster.Status.CurrentVersion != "" &&
		cluster.Status.CurrentVersion != cluster.Spec.Version

	if upgrading {
		log.Info("Version upgrade detected",
			"from", cluster.Status.CurrentVersion,
			"to", cluster.Spec.Version,
		)
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Phase = databasesv1alpha1.PhaseUpgrading
		setCondition(&cluster, databasesv1alpha1.ConditionUpgrading,
			metav1.ConditionTrue, "VersionUpgradeInProgress",
			fmt.Sprintf("Upgrading from %s to %s",
				cluster.Status.CurrentVersion, cluster.Spec.Version),
		)
		if err := r.Status().Patch(ctx, &cluster, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting upgrading status: %w", err)
		}
	}

	// StatefulSet
	sts := buildStatefulSet(&cluster)
	if err := controllerutil.SetControllerReference(&cluster, sts, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("owner ref StatefulSet: %w", err)
	}
	stsResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		sts.Spec.Replicas = &cluster.Spec.Replicas
		sts.Spec.Template.Spec.Containers[0].Image = postgresImage(cluster.Spec.Version)
		sts.Spec.Template.Spec.Volumes[0].ConfigMap.Name = configMapName(&cluster)
		return nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling StatefulSet: %w", err)
	}
	log.Info("StatefulSet reconciled", "result", stsResult)

	// 7. Observe actual state
	var currentSts appsv1.StatefulSet
	if err := r.Get(ctx, types.NamespacedName{
		Name:      sts.Name,
		Namespace: sts.Namespace,
	}, &currentSts); err != nil {
		return ctrl.Result{}, fmt.Errorf("fetching StatefulSet status: %w", err)
	}

	readyReplicas := currentSts.Status.ReadyReplicas
	desiredReplicas := cluster.Spec.Replicas
	log.Info("Observed", "ready", readyReplicas, "desired", desiredReplicas)

	// 8. Backup bucket (finalizer + external resource)
	if cluster.Spec.BackupSchedule != "" && !cluster.Status.BackupBucketProvisioned {
		if err := r.provisionBackupBucket(ctx, &cluster); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 9. Update full status
	if err := r.updateStatus(ctx, &cluster, &currentSts, upgrading); err != nil {
		return ctrl.Result{}, err
	}

	// 10. Return
	if readyReplicas < desiredReplicas {
		log.Info("Waiting for replicas", "ready", readyReplicas, "desired", desiredReplicas)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	log.Info("DatabaseCluster in desired state ✓", "phase", cluster.Status.Phase)
	return ctrl.Result{}, nil
}

// reconcileDeletion handles cleanup when DeletionTimestamp is set.
func (r *DatabaseClusterReconciler) reconcileDeletion(
	ctx context.Context,
	cluster *databasesv1alpha1.DatabaseCluster,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(cluster, databasesv1alpha1.FinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Running deletion cleanup", "cluster", cluster.Name)

	if err := r.bucketStore().Delete(cluster.Name); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting backup bucket: %w", err)
	}
	log.Info("Backup bucket deleted")

	patch := client.MergeFrom(cluster.DeepCopy())
	controllerutil.RemoveFinalizer(cluster, databasesv1alpha1.FinalizerName)
	if err := r.Patch(ctx, cluster, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
	}

	log.Info("Finalizer removed — deletion will proceed")
	return ctrl.Result{}, nil
}

// provisionBackupBucket adds the finalizer then creates the external bucket.
func (r *DatabaseClusterReconciler) provisionBackupBucket(
	ctx context.Context,
	cluster *databasesv1alpha1.DatabaseCluster,
) error {
	log := log.FromContext(ctx)

	// Add finalizer BEFORE provisioning — so we track the bucket even if we crash
	if !controllerutil.ContainsFinalizer(cluster, databasesv1alpha1.FinalizerName) {
		patch := client.MergeFrom(cluster.DeepCopy())
		controllerutil.AddFinalizer(cluster, databasesv1alpha1.FinalizerName)
		if err := r.Patch(ctx, cluster, patch); err != nil {
			return fmt.Errorf("adding finalizer: %w", err)
		}
		log.Info("Finalizer added")
	}

	// Provision bucket (idempotent — skips if already exists)
	buckets := r.bucketStore()
	if !buckets.Exists(cluster.Name) {
		if err := buckets.Provision(cluster.Name); err != nil {
			return fmt.Errorf("provisioning backup bucket: %w", err)
		}
		log.Info("Backup bucket provisioned")
	}

	// Record in status
	patch := client.MergeFrom(cluster.DeepCopy())
	cluster.Status.BackupBucketProvisioned = true
	setCondition(cluster, databasesv1alpha1.ConditionBackupHealthy,
		metav1.ConditionTrue, "BucketProvisioned", "Backup bucket is ready",
	)
	if err := r.Status().Patch(ctx, cluster, patch); err != nil {
		return fmt.Errorf("updating backup status: %w", err)
	}
	return nil
}

// updateStatus patches the full Status block from observed facts.
func (r *DatabaseClusterReconciler) updateStatus(
	ctx context.Context,
	cluster *databasesv1alpha1.DatabaseCluster,
	currentSts *appsv1.StatefulSet,
	upgrading bool,
) error {
	patch := client.MergeFrom(cluster.DeepCopy())

	readyReplicas := currentSts.Status.ReadyReplicas
	desiredReplicas := cluster.Spec.Replicas

	cluster.Status.ReadyReplicas = readyReplicas
	cluster.Status.CurrentVersion = cluster.Spec.Version
	cluster.Status.ObservedGeneration = cluster.Generation

	switch {
	case upgrading && readyReplicas < desiredReplicas:
		cluster.Status.Phase = databasesv1alpha1.PhaseUpgrading
	case readyReplicas == 0:
		cluster.Status.Phase = databasesv1alpha1.PhaseProvisioning
	case readyReplicas < desiredReplicas:
		cluster.Status.Phase = databasesv1alpha1.PhaseDegraded
	default:
		cluster.Status.Phase = databasesv1alpha1.PhaseRunning
	}

	if readyReplicas >= desiredReplicas {
		setCondition(cluster, databasesv1alpha1.ConditionReady,
			metav1.ConditionTrue, "AllReplicasReady",
			fmt.Sprintf("All %d replicas are ready", desiredReplicas),
		)
		setCondition(cluster, databasesv1alpha1.ConditionDegraded,
			metav1.ConditionFalse, "ReplicasReady", "All replicas are healthy",
		)
		if upgrading {
			setCondition(cluster, databasesv1alpha1.ConditionUpgrading,
				metav1.ConditionFalse, "UpgradeComplete",
				fmt.Sprintf("Upgrade to %s complete", cluster.Spec.Version),
			)
		}
	} else {
		setCondition(cluster, databasesv1alpha1.ConditionReady,
			metav1.ConditionFalse, "ReplicasNotReady",
			fmt.Sprintf("%d/%d replicas ready", readyReplicas, desiredReplicas),
		)
		setCondition(cluster, databasesv1alpha1.ConditionDegraded,
			metav1.ConditionTrue, "InsufficientReplicas",
			fmt.Sprintf("Only %d of %d replicas are ready", readyReplicas, desiredReplicas),
		)
	}

	if err := r.Status().Patch(ctx, cluster, patch); err != nil {
		return fmt.Errorf("patching status: %w", err)
	}
	return nil
}

// setCondition is a thin wrapper around metav1.SetStatusCondition.
func setCondition(
	cluster *databasesv1alpha1.DatabaseCluster,
	condType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	metav1.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cluster.Generation,
	})
}

func (r *DatabaseClusterReconciler) bucketStore() *backup.BucketStore {
	if r.BucketStore != nil {
		return r.BucketStore
	}
	return backup.GlobalStore()
}

func (r *DatabaseClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasesv1alpha1.DatabaseCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

