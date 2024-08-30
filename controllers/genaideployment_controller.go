/*
Copyright 2021.

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

package controllers

import (
	"context"
	"fmt"
	rayv1 "github.com/splunk/splunk-operator/controllers/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	enterpriseApi "github.com/splunk/splunk-operator/api/v4"
)

// GenAIDeploymentReconciler reconciles a GenAIDeployment object
type GenAIDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=enterprise.splunk.com,resources=genaideployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=enterprise.splunk.com,resources=genaideployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=enterprise.splunk.com,resources=genaideployments/finalizers,verbs=update

func (r *GenAIDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	reqLogger := log.FromContext(ctx)
	reqLogger = reqLogger.WithValues("indexercluster", req.NamespacedName)

	// Fetch the GenAIDeployment instance
	genAIDeployment := &enterpriseApi.GenAIDeployment{}
	err := r.Client.Get(ctx, req.NamespacedName, genAIDeployment)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			reqLogger.Error(err, "Failed to get GenAIDeployment")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle RayCluster creation/update
	if genAIDeployment.Spec.RayService.Enabled {
		rayCluster := &rayv1.RayCluster{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: req.Name + "-raycluster", Namespace: req.Namespace}, rayCluster)
		if err != nil {
			// Create RayCluster if not found
			newRayCluster := r.constructRayCluster(ctx, genAIDeployment)
			if err := r.Client.Create(ctx, newRayCluster); err != nil {
				reqLogger.Error(err, "Failed to create RayCluster")
				return ctrl.Result{}, err
			}
		} else {
			// Update existing RayCluster if necessary
			updatedRayCluster := r.updateRayCluster(ctx, rayCluster, genAIDeployment)
			if err := r.Client.Update(ctx, updatedRayCluster); err != nil {
				reqLogger.Error(err, "Failed to update RayCluster")
				return ctrl.Result{}, err
			}
		}

		// Update Status with RayCluster information
		r.updateRayClusterStatus(ctx, genAIDeployment, rayCluster)
	}

	// Reconcile SaisService Deployment
	if err := r.reconcileSaisServiceDeployment(ctx, genAIDeployment); err != nil {
		reqLogger.Error(err, "Failed to reconcile SaisService Deployment")
		return ctrl.Result{}, err
	}

	// Reconcile VectorDb Deployment
	if err := r.reconcileVectorDbDeployment(ctx, genAIDeployment); err != nil {
		reqLogger.Error(err, "Failed to reconcile VectorDb Deployment")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *GenAIDeploymentReconciler) constructRayCluster(ctx context.Context, genAIDeployment *enterpriseApi.GenAIDeployment) *rayv1.RayCluster {
	// Create RayCluster object based on GenAIDeployment spec
	return &rayv1.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      genAIDeployment.Name + "-raycluster",
			Namespace: genAIDeployment.Namespace,
		},
		Spec: rayv1.RayClusterSpec{
			HeadGroupSpec: rayv1.HeadGroupSpec{
				RayStartParams: map[string]string{
					"num-cpus": genAIDeployment.Spec.RayService.HeadGroup.NumCpus,
				},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:      "ray-head",
								Image:     genAIDeployment.Spec.RayService.Image,
								Resources: genAIDeployment.Spec.RayService.HeadGroup.Resources,
							},
						},
					},
				},
			},
			WorkerGroupSpecs: []rayv1.WorkerGroupSpec{
				{
					GroupName: "ray-worker",
					Replicas:  &genAIDeployment.Spec.RayService.Replicas,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:      "ray-worker",
									Image:     genAIDeployment.Spec.RayService.Image,
									Resources: genAIDeployment.Spec.RayService.WorkerGroup.Resources,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *GenAIDeploymentReconciler) updateRayClusterStatus(ctx context.Context, genAIDeployment *enterpriseApi.GenAIDeployment, rayCluster *rayv1.RayCluster) {
	reqLogger := log.FromContext(ctx)
	reqLogger = reqLogger.WithValues("updateRayClusterStatus")

	// Fetch RayCluster status and update GenAIDeployment status
	genAIDeployment.Status.RayClusterStatus = enterpriseApi.RayClusterStatus{
		ClusterName: rayCluster.Name,
		State:       string(rayCluster.Status.State),
		Conditions:  rayCluster.Status.Conditions,
	}
	err := r.Client.Status().Update(context.Background(), genAIDeployment)
	if err != nil {
		reqLogger.Error(err, "Failed to update GenAIDeployment status")
	}
}

func (r *GenAIDeploymentReconciler) updateRayCluster(ctx context.Context, existingCluster *rayv1.RayCluster, genAIDeployment *enterpriseApi.GenAIDeployment) *rayv1.RayCluster {
	// Update RayCluster spec if necessary
	// ...
	return existingCluster
}

func (r *GenAIDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&enterpriseApi.GenAIDeployment{}).
		Owns(&rayv1.RayCluster{}).
		Complete(r)
}

func (r *GenAIDeploymentReconciler) reconcileSaisServiceDeployment(ctx context.Context, genAIDeployment *enterpriseApi.GenAIDeployment) error {
	log := log.FromContext(ctx)

	// Define the desired Deployment object
	desiredDeployment := r.constructSaisServiceDeployment(genAIDeployment)

	// Check if the Deployment already exists
	existingDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: desiredDeployment.Name, Namespace: desiredDeployment.Namespace}, existingDeployment)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}

		// Create the Deployment if it does not exist
		log.Info("Creating new Deployment", "Deployment.Namespace", desiredDeployment.Namespace, "Deployment.Name", desiredDeployment.Name)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			return fmt.Errorf("failed to create new Deployment: %w", err)
		}
	} else {
		// Update the existing Deployment if necessary
		if !isEqual(desiredDeployment, existingDeployment) {
			log.Info("Updating existing Deployment", "Deployment.Namespace", existingDeployment.Namespace, "Deployment.Name", existingDeployment.Name)
			existingDeployment.Spec = desiredDeployment.Spec
			if err := r.Update(ctx, existingDeployment); err != nil {
				return fmt.Errorf("failed to update Deployment: %w", err)
			}
		}
	}

	return nil
}

func (r *GenAIDeploymentReconciler) constructSaisServiceDeployment(genAIDeployment *enterpriseApi.GenAIDeployment) *appsv1.Deployment {
	labels := map[string]string{
		"app":        "sais-service",
		"deployment": genAIDeployment.Name,
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-sais-service", genAIDeployment.Name),
			Namespace: genAIDeployment.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &genAIDeployment.Spec.SaisService.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:      "sais-service-container",
							Image:     genAIDeployment.Spec.SaisService.Image,
							Resources: genAIDeployment.Spec.SaisService.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      genAIDeployment.Spec.SaisService.Volume.Name,
									MountPath: "/data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						genAIDeployment.Spec.SaisService.Volume,
					},
					SchedulerName: genAIDeployment.Spec.SaisService.SchedulerName,
					Affinity:      &genAIDeployment.Spec.SaisService.Affinity,
					Tolerations:   genAIDeployment.Spec.SaisService.Tolerations,
				},
			},
		},
	}

	// Set the owner reference to enable garbage collection
	ctrl.SetControllerReference(genAIDeployment, deployment, r.Scheme)
	return deployment
}

func isEqual(desired, existing *appsv1.Deployment) bool {
	// Compare important fields for determining if an update is necessary
	// This is a simplified example; you may need a more thorough comparison
	return desired.Spec.Replicas == existing.Spec.Replicas &&
		desired.Spec.Template.Spec.Containers[0].Image == existing.Spec.Template.Spec.Containers[0].Image
}

func (r *GenAIDeploymentReconciler) reconcileVectorDbDeployment(ctx context.Context, genAIDeployment *enterpriseApi.GenAIDeployment) error {
	log := log.FromContext(ctx)

	// Define the desired Deployment object for the VectorDb service
	desiredDeployment := r.constructVectorDbDeployment(genAIDeployment)

	// Check if the Deployment already exists
	existingDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: desiredDeployment.Name, Namespace: desiredDeployment.Namespace}, existingDeployment)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}

		// Create the Deployment if it does not exist
		log.Info("Creating new VectorDb Deployment", "Deployment.Namespace", desiredDeployment.Namespace, "Deployment.Name", desiredDeployment.Name)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			return fmt.Errorf("failed to create new VectorDb Deployment: %w", err)
		}
	} else {
		// Update the existing Deployment if necessary
		if !isEqual(desiredDeployment, existingDeployment) {
			log.Info("Updating existing VectorDb Deployment", "Deployment.Namespace", existingDeployment.Namespace, "Deployment.Name", existingDeployment.Name)
			existingDeployment.Spec = desiredDeployment.Spec
			if err := r.Update(ctx, existingDeployment); err != nil {
				return fmt.Errorf("failed to update VectorDb Deployment: %w", err)
			}
		}
	}

	return nil
}

func (r *GenAIDeploymentReconciler) constructVectorDbDeployment(genAIDeployment *enterpriseApi.GenAIDeployment) *appsv1.Deployment {
	labels := map[string]string{
		"app":        "vectordb-service",
		"deployment": genAIDeployment.Name,
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-vectordb-service", genAIDeployment.Name),
			Namespace: genAIDeployment.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &genAIDeployment.Spec.VectorDbService.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:      "vectordb-container",
							Image:     genAIDeployment.Spec.VectorDbService.Image,
							Resources: genAIDeployment.Spec.VectorDbService.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      genAIDeployment.Spec.VectorDbService.Volume.Name,
									MountPath: "/data", // Adjust mount path as necessary
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						genAIDeployment.Spec.VectorDbService.Volume,
					},
					Affinity:                  &genAIDeployment.Spec.VectorDbService.Affinity,
					Tolerations:               genAIDeployment.Spec.VectorDbService.Tolerations,
					TopologySpreadConstraints: genAIDeployment.Spec.VectorDbService.TopologySpreadConstraints,
				},
			},
		},
	}

	// Set the owner reference to enable garbage collection
	ctrl.SetControllerReference(genAIDeployment, deployment, r.Scheme)
	return deployment
}
