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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	databasesv1alpha1 "github.com/madmmas/kubeoperator-journey/api/v1alpha1"
)

type DatabaseClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=databases.madmmas.dev,resources=databaseclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=databases.madmmas.dev,resources=databaseclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=databases.madmmas.dev,resources=databaseclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch

func (r *DatabaseClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Step 1: Fetch desired state
	var cluster databasesv1alpha1.DatabaseCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if errors.IsNotFound(err) {
			log.Info("DatabaseCluster not found — likely deleted", "key", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetching DatabaseCluster: %w", err)
	}

	log.Info("Reconciling DatabaseCluster",
		"name", cluster.Name,
		"replicas", cluster.Spec.Replicas,
		"version", cluster.Spec.Version,
		"phase", cluster.Status.Phase,
	)

	// Step 2: Set initial Phase on first creation
	if cluster.Status.Phase == "" {
		log.Info("New DatabaseCluster detected — setting initial phase")
		cluster.Status.Phase = databasesv1alpha1.PhaseProvisioning
		if err := r.Status().Update(ctx, &cluster); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting initial phase: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Step 3: Reconcile the StatefulSet
	sts := r.buildStatefulSet(&cluster)
	if err := controllerutil.SetControllerReference(&cluster, sts, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting owner reference: %w", err)
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		sts.Spec.Replicas = &cluster.Spec.Replicas
		sts.Spec.Template.Spec.Containers[0].Image = postgresImage(cluster.Spec.Version)
		return nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling StatefulSet: %w", err)
	}
	log.Info("StatefulSet reconciled", "result", result, "statefulset", sts.Name)

	// Step 4: Observe actual state
	var currentSts appsv1.StatefulSet
	if err := r.Get(ctx, types.NamespacedName{
		Name:      sts.Name,
		Namespace: sts.Namespace,
	}, &currentSts); err != nil {
		return ctrl.Result{}, fmt.Errorf("fetching StatefulSet status: %w", err)
	}

	readyReplicas   := currentSts.Status.ReadyReplicas
	desiredReplicas := cluster.Spec.Replicas

	log.Info("Observed StatefulSet state", "ready", readyReplicas, "desired", desiredReplicas)

	// Step 5: Update Status
	patch := client.MergeFrom(cluster.DeepCopy())

	cluster.Status.ReadyReplicas  = readyReplicas
	cluster.Status.CurrentVersion = cluster.Spec.Version

	switch {
	case readyReplicas == 0:
		cluster.Status.Phase = databasesv1alpha1.PhaseProvisioning
	case readyReplicas < desiredReplicas:
		cluster.Status.Phase = databasesv1alpha1.PhaseDegraded
	default:
		cluster.Status.Phase = databasesv1alpha1.PhaseRunning
	}

	setReadyCondition(&cluster, readyReplicas, desiredReplicas)

	if err := r.Status().Patch(ctx, &cluster, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	// Step 6: Decide whether to requeue
	if readyReplicas < desiredReplicas {
		log.Info("Waiting for replicas to become ready — requeuing",
			"ready", readyReplicas, "desired", desiredReplicas)
		return ctrl.Result{RequeueAfter: requeueAfterWaiting}, nil
	}

	log.Info("DatabaseCluster is in desired state ✓",
		"phase", cluster.Status.Phase, "ready", readyReplicas)
	return ctrl.Result{}, nil
}

func (r *DatabaseClusterReconciler) buildStatefulSet(cluster *databasesv1alpha1.DatabaseCluster) *appsv1.StatefulSet {
	labels := map[string]string{
		"app.kubernetes.io/name":       "databasecluster",
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/managed-by": "kubeoperator-journey",
	}
	storageSize := cluster.Spec.StorageSize
	if storageSize == "" {
		storageSize = "10Gi"
	}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &cluster.Spec.Replicas,
			ServiceName: cluster.Name,
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

func setReadyCondition(cluster *databasesv1alpha1.DatabaseCluster, readyReplicas, desiredReplicas int32) {
	condition := metav1.Condition{
		Type:               databasesv1alpha1.ConditionReady,
		ObservedGeneration: cluster.Generation,
	}
	if readyReplicas >= desiredReplicas {
		condition.Status  = metav1.ConditionTrue
		condition.Reason  = "AllReplicasReady"
		condition.Message = fmt.Sprintf("All %d replicas are ready", desiredReplicas)
	} else {
		condition.Status  = metav1.ConditionFalse
		condition.Reason  = "ReplicasNotReady"
		condition.Message = fmt.Sprintf("%d/%d replicas ready", readyReplicas, desiredReplicas)
	}
	metav1.SetStatusCondition(&cluster.Status.Conditions, condition)
}

func postgresImage(version string) string {
	return fmt.Sprintf("postgres:%s", version)
}

const requeueAfterWaiting = 10e9 // 10 * time.Second

func (r *DatabaseClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasesv1alpha1.DatabaseCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
