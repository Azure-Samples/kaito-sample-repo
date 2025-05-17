/*
Copyright 2025.

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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	kaitodevv1alpha1 "kaido.dev/BatchJobController/api/v1alpha1"
)

// BatchJobReconciler reconciles a BatchJob object
type BatchJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kaito.dev,resources=batchjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kaito.dev,resources=batchjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kaito.dev,resources=batchjobs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BatchJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *BatchJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	job := kaitodevv1alpha1.BatchJob{}
	if err := r.Get(ctx, req.NamespacedName, &job); err != nil {
		log.Error(err, "unable to fetch Job")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.Info("Reconcile BatchJob", "name", job.Name, "namespace", job.Namespace)

	// Set JobStartedAt if not already set
	if job.Status.JobStartedAt.IsZero() {
		job.Status.JobStartedAt = metav1.Now()
		if err := r.Status().Update(ctx, &job); err != nil {
			log.Error(err, "unable to update BatchJob status")
			return ctrl.Result{}, err
		}
		log.Info("Set JobStartedAt", "time", job.Status.JobStartedAt)
	}

	// Execute the single step (pod)
	podName := job.Name + "-step"
	pod := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Namespace: job.Namespace, Name: podName}, pod)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Pod does not exist, create it
			newPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: job.Namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: job.APIVersion,
						Kind:       job.Kind,
						Name:       job.Name,
						UID:        job.UID,
					}},
				},
				Spec: job.Spec.Step.Spec,
			}
			if err := r.Create(ctx, newPod); err != nil {
				log.Error(err, "unable to create Pod", "pod", podName)
				return ctrl.Result{}, err
			}
			log.Info("Created Pod for BatchJob step", "pod", podName)
		}
		return ctrl.Result{Requeue: true}, nil
	}
	// Pod exists, check status
	job.Status.StepStatus = pod.Status.Phase
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		if err := r.Status().Update(ctx, &job); err != nil {
			log.Error(err, "unable to update BatchJob status after step completion")
			return ctrl.Result{}, err
		}
		log.Info("Step completed", "phase", pod.Status.Phase)
		return ctrl.Result{}, nil
	}
	if err := r.Status().Update(ctx, &job); err != nil {
		log.Error(err, "unable to update BatchJob status after pod check")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BatchJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kaitodevv1alpha1.BatchJob{}).
		Named("batchjob").
		Complete(r)
}
