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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"
	res "github.com/kubernetes-app/redis-operator/controllers/resources"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	*res.RedisClient
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list
// +kubebuilder:rbac:groups=core,resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=core,resources=secrets;services,verbs=get;list;watch;create;update;patch;delete
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

	if instance.Spec.GlobalConfig.Password != nil {
		if err := r.RedisClient.CreateOrUpdateRedisSecret(instance); err != nil {
			return ctrl.Result{}, err
		}
	}
	if instance.Spec.Mode == "cluster" {
		if err := r.ReconcileRedisClusterNode(instance); err != nil {
			return ctrl.Result{}, err
		}
	} else if instance.Spec.Mode == "standalone" {
		if err := r.ReconcileRedisStandalone(instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	klog.Infof("Finished reconciling Redis: %s", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) ReconcileRedisClusterNode(instance *redisv1alpha1.Redis) error {
	klog.Info("Creating redis cluster nodes")
	if err := r.RedisClient.CreateOrUpdateRedisMaster(instance); err != nil {
		return err
	}
	if err := r.RedisClient.CreateOrUpdateMasterService(instance); err != nil {
		return err
	}
	if err := r.RedisClient.CreateOrUpdateMasterHeadlessService(instance); err != nil {
		return err
	}
	if err := r.RedisClient.CreateOrUpdateRedisSlave(instance); err != nil {
		return err
	}
	if err := r.RedisClient.CreateOrUpdateSlaveService(instance); err != nil {
		return err
	}
	if err := r.RedisClient.CreateOrUpdateSlaveHeadlessService(instance); err != nil {
		return err
	}

	klog.Info("Waiting for all redis cluster nodes ready")
	if err := r.WaitRedisClusterNodeReady(instance); err != nil {
		return err
	}

	numOfRedisNodes := r.RedisClient.GetRedisClusterNodes(instance).CountByFunc(res.AllNodes)
	// If number of redis nodes is 1, create redis cluster
	// If 1 < numOfRedisNodes < size*2, add left nodes(size*2 - numOfRedisNodes) to redis cluster
	if numOfRedisNodes == 1 {
		klog.Info("Creating redis cluster")
		if err := r.RedisClient.ExecuteRedisClusterCommand(instance); err != nil {
			return err
		}
		for podCount := 0; podCount < int(*instance.Spec.Size); podCount++ {
			if err := r.RedisClient.ExecuteRedisReplicationCommand(instance, podCount); err != nil {
				return err
			}
		}
	} else if numOfRedisNodes < int(*instance.Spec.Size)*2 {
		klog.Info("Adding node to redis cluster")
		startIndex := numOfRedisNodes / 2
		for podCount := startIndex; podCount < int(*instance.Spec.Size); podCount++ {
			if err := r.RedisClient.ExecuteAddRedisMasterCommand(instance, podCount); err != nil {
				return err
			}
			klog.Info("Waiting for master-%d node join cluster", podCount)
			if err := r.WaitRedisMasterJoin(instance); err != nil {
				return err
			}
			if err := r.RedisClient.ExecuteSlotReshardCommand(instance, podCount); err != nil {
				return err
			}
			if err := r.RedisClient.ExecuteRedisReplicationCommand(instance, podCount); err != nil {
				return err
			}
		}
	}
	klog.Info("Redis master count is desired")
	return nil
}

func (r *RedisReconciler) ReconcileRedisStandalone(instance *redisv1alpha1.Redis) error {
	if err := r.RedisClient.CreateOrUpdateRedisStandalone(instance); err != nil {
		return err
	}
	if err := r.RedisClient.CreateOrUpdateStandaloneService(instance); err != nil {
		return err
	}
	if err := r.RedisClient.CreateOrUpdateStandaloneHeadlessService(instance); err != nil {
		return err
	}
	return nil
}

// Waiting for redis cluster node ready
func (r *RedisReconciler) WaitRedisClusterNodeReady(instance *redisv1alpha1.Redis) error {
	if err := wait.PollImmediate(time.Second*30, time.Minute*5, func() (bool, error) {
		redisMasterInfo, err := r.RedisClient.AppsV1().StatefulSets(instance.Namespace).Get(context.TODO(), instance.ObjectMeta.Name+"-master", metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Cound not fetch redis master statefulset: %v", err)
			return false, err
		}
		redisSlaveInfo, err := r.RedisClient.AppsV1().StatefulSets(instance.Namespace).Get(context.TODO(), instance.ObjectMeta.Name+"-slave", metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Cound not fetch redis slave statefulset: %v", err)
			return false, err
		}
		if int(redisMasterInfo.Status.ReadyReplicas) != int(*instance.Spec.Size) && int(redisSlaveInfo.Status.ReadyReplicas) != int(*instance.Spec.Size) {
			klog.Infof("Redis master and slave nodes are not ready yet, Master.Ready.Replicas: %d, Slave.Ready.Replicas: %d", redisMasterInfo.Status.ReadyReplicas, redisSlaveInfo.Status.ReadyReplicas)
			return false, nil
		}
		klog.Infof("Redis master and slave nodes are ready, Master.Ready.Replicas: %d, Slave.Ready.Replicas: %d", redisMasterInfo.Status.ReadyReplicas, redisSlaveInfo.Status.ReadyReplicas)
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

// Waiting for redis cluster node ready
func (r *RedisReconciler) WaitRedisMasterJoin(instance *redisv1alpha1.Redis) error {
	if err := wait.PollImmediate(time.Second*10, time.Minute*2, func() (bool, error) {
		filterNodes := r.RedisClient.GetRedisClusterNodes(instance).FilterByFunc(res.IsMasterWithNoSlot)
		if len(filterNodes) == 0 {
			klog.Info("Waiting master node join cluster ...")
			return false, nil
		}
		klog.Info("Master node joined cluster")
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1alpha1.Redis{}).
		Complete(r)
}
