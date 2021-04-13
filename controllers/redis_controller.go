/*
Copyright 2021 kubernetes-app Solutions.

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
	"strconv"
	"time"

	"github.com/kubernetes-app/redis-operator/k8sutils"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	*k8sutils.RedisClient
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operators.cloud.tencent.com,resources=redis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operators.cloud.tencent.com,resources=redis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operators.cloud.tencent.com,resources=redis/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Redis object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.V(1).Infof("Reconciling Redis: %s", req.NamespacedName)
	instance := &redisv1alpha1.Redis{}

	if err := r.Client.Get(context.TODO(), req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := controllerutil.SetControllerReference(instance, instance, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	sts := &appsv1.StatefulSet{}
	stsKey := req.NamespacedName
	if err := r.Client.Get(context.TODO(), stsKey, sts); err != nil && errors.IsNotFound(err) {
		if instance.Spec.GlobalConfig.Password != nil {
			r.RedisClient.CreateRedisSecret(instance)
		}
		if instance.Spec.Mode == "cluster" {
			r.RedisClient.CreateRedisMaster(instance)
			r.RedisClient.CreateMasterService(instance)
			r.RedisClient.CreateMasterHeadlessService(instance)
			r.RedisClient.CreateRedisSlave(instance)
			r.RedisClient.CreateSlaveService(instance)
			r.RedisClient.CreateSlaveHeadlessService(instance)
			redisMasterInfo, err := r.RedisClient.AppsV1().StatefulSets(instance.Namespace).Get(context.TODO(), instance.ObjectMeta.Name+"-master", metav1.GetOptions{})
			if err != nil {
				return ctrl.Result{}, err
			}
			redisSlaveInfo, err := r.RedisClient.AppsV1().StatefulSets(instance.Namespace).Get(context.TODO(), instance.ObjectMeta.Name+"-slave", metav1.GetOptions{})
			if err != nil {
				return ctrl.Result{}, err
			}
			if int(redisMasterInfo.Status.ReadyReplicas) != int(*instance.Spec.Size) && int(redisSlaveInfo.Status.ReadyReplicas) != int(*instance.Spec.Size) {
				klog.Info("Redis master and slave nodes are not ready yet", "Ready.Replicas", strconv.Itoa(int(redisMasterInfo.Status.ReadyReplicas)))
				return ctrl.Result{RequeueAfter: time.Second * 120}, nil
			}
			klog.Info("Creating redis cluster by executing cluster creation command", "Ready.Replicas", strconv.Itoa(int(redisMasterInfo.Status.ReadyReplicas)))
			if r.RedisClient.CheckRedisCluster(instance) != int(*instance.Spec.Size)*2 {
				r.RedisClient.ExecuteRedisClusterCommand(instance)
				r.RedisClient.ExecuteRedisReplicationCommand(instance)
			} else {
				klog.Info("Redis master count is desired")
				return ctrl.Result{RequeueAfter: time.Second * 120}, nil
			}
		} else if instance.Spec.Mode == "standalone" {
			r.RedisClient.CreateRedisStandalone(instance)
			r.RedisClient.CreateStandaloneService(instance)
			r.RedisClient.CreateStandaloneHeadlessService(instance)
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}

	klog.Info("Will reconcile in again 10 seconds")
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1alpha1.Redis{}).
		Complete(r)
}
