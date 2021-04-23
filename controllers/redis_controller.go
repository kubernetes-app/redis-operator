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
	"reflect"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"
	"github.com/kubernetes-app/redis-operator/controllers/redis"
	res "github.com/kubernetes-app/redis-operator/controllers/resources"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	*res.K8sClient
	*redis.RedisClient
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
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
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, err error) {
	klog.Infof("Reconciling Redis: %s", req.NamespacedName)
	instance := &redisv1alpha1.Redis{}

	if err := r.Get(context.TODO(), req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	originalInstance := instance.DeepCopy()

	// Always attempt to patch the status after each reconciliation.
	defer func() {
		if err != nil {
			instance.SetPhase(redisv1alpha1.ClusterPhaseFailed)
		}
		if reflect.DeepEqual(originalInstance.Status, instance.Status) {
			return
		}
		if err := r.Status().Update(ctx, instance, &client.UpdateOptions{}); err != nil {
			klog.Error("Update status failed.")
		}
	}()

	if instance.Spec.GlobalConfig.Password != nil {
		if err := r.K8sClient.CreateOrUpdateRedisSecret(instance); err != nil {
			return ctrl.Result{}, err
		}
	}
	if instance.Spec.Mode == "cluster" {
		if instance.Status.Size == nil {
			klog.Info("Creating redis cluster")
			instance.SetPhase(redisv1alpha1.ClusterPhaseCreating)
			if err := r.CreateOrUpdateResisStatefulSet(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.CreateOrUpdateResisService(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.WaitRedisNodesReady(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.CreateResisCluster(instance); err != nil {
				return ctrl.Result{}, err
			}
		} else if *instance.Spec.Size > *instance.Status.Size {
			klog.Info("Adding node into redis cluster")
			instance.SetPhase(redisv1alpha1.ClusterPhaseAdding)
			if err := r.CreateOrUpdateResisStatefulSet(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.WaitRedisNodesReady(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.AddRedisNodes(instance); err != nil {
				return ctrl.Result{}, err
			}
		} else if *instance.Spec.Size < *instance.Status.Size {
			klog.Info("Removing node from redis cluster")
			instance.SetPhase(redisv1alpha1.ClusterPhaseDeleting)
			if err := r.DeleteRedisNodes(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.CreateOrUpdateResisStatefulSet(instance); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else if instance.Spec.Mode == "standalone" {
		instance.SetPhase(redisv1alpha1.ClusterPhaseCreating)
		if err := r.ReconcileRedisStandalone(instance); err != nil {
			return ctrl.Result{}, err
		}
	}
	instance.SetPhase(redisv1alpha1.ClusterPhaseRunning)
	instance.SetSize(instance.Spec.Size)
	klog.Infof("Finished reconciling Redis: %s", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) CreateOrUpdateResisStatefulSet(instance *redisv1alpha1.Redis) error {
	if err := r.K8sClient.CreateOrUpdateRedisMaster(instance); err != nil {
		return err
	}
	if err := r.K8sClient.CreateOrUpdateRedisSlave(instance); err != nil {
		return err
	}
	return nil
}

func (r *RedisReconciler) CreateOrUpdateResisService(instance *redisv1alpha1.Redis) error {
	if err := r.K8sClient.CreateOrUpdateMasterService(instance); err != nil {
		return err
	}
	if err := r.K8sClient.CreateOrUpdateMasterHeadlessService(instance); err != nil {
		return err
	}
	if err := r.K8sClient.CreateOrUpdateSlaveService(instance); err != nil {
		return err
	}
	if err := r.K8sClient.CreateOrUpdateSlaveHeadlessService(instance); err != nil {
		return err
	}
	return nil
}

func (r *RedisReconciler) CreateResisCluster(instance *redisv1alpha1.Redis) error {
	klog.Info("Creating redis cluster")
	if err := r.RedisClient.ExecuteRedisClusterClusterCommand(instance); err != nil {
		return err
	}
	klog.Info("Adding slave nodes")
	for podCount := 0; podCount < int(*instance.Spec.Size); podCount++ {
		if err := r.RedisClient.ExecuteAddRedisSlaveCommand(instance, podCount); err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisReconciler) AddRedisNodes(instance *redisv1alpha1.Redis) error {
	for podCount := int(*instance.Status.Size); podCount < int(*instance.Spec.Size); podCount++ {
		klog.Info("Adding a new node as a master")
		if err := r.RedisClient.ExecuteAddRedisMasterCommand(instance, podCount); err != nil {
			return err
		}
		klog.Infof("Waiting for new master-%d node join cluster", podCount)
		if err := r.WaitRedisMasterJoin(instance); err != nil {
			return err
		}
		klog.Info("Resharding the cluster for the new master node")
		nodes, err := r.RedisClient.GetRedisClusterNodes(instance)
		if err != nil {
			return err
		}
		masterIp, err := r.RedisClient.GetRedisServerIP(instance, redis.RedisMasterRole, strconv.Itoa(podCount))
		if err != nil {
			return err
		}
		fromNodeIds := nodes.GetNodeIds(redis.IsMasterWithSlot)
		toNodeId := nodes.GetNodeByIpPort(masterIp, redis.DefaultRedisPort).ID
		if err := r.RedisClient.ExecuteReshardCommand(instance, podCount, strings.Join(fromNodeIds, ","), toNodeId, "1024"); err != nil {
			return err
		}
		klog.Info("Adding a new node as a slave")
		if err := r.RedisClient.ExecuteAddRedisSlaveCommand(instance, podCount); err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisReconciler) DeleteRedisNodes(instance *redisv1alpha1.Redis) error {
	for podCount := int(*instance.Spec.Size); podCount < int(*instance.Status.Size); podCount++ {
		klog.Info("Deleting slave node")
		if err := r.RedisClient.ExecuteDeleteRedisSlaveCommand(instance, podCount); err != nil {
			return err
		}
		klog.Info("Magerating master node slot to another nodes")
		nodes, err := r.RedisClient.GetRedisClusterNodes(instance)
		if err != nil {
			return err
		}
		masterIp, err := r.RedisClient.GetRedisServerIP(instance, redis.RedisMasterRole, strconv.Itoa(podCount))
		if err != nil {
			return err
		}
		fromNode := nodes.GetNodeByIpPort(masterIp, redis.DefaultRedisPort)
		toNodes := nodes.GetNodeWithNoIpPort(redis.IsMasterWithSlot, masterIp, redis.DefaultRedisPort)
		slotsLen := len(fromNode.Slots)
		slots, remainSlots := slotsLen/len(toNodes), slotsLen%len(toNodes)
		klog.Infof("slotsLen: %s, fromNode.Slots: %s", fromNode.Slots, slotsLen)
		for i, toNode := range toNodes {
			if i == (len(toNodes) - 1) {
				slots = slots + remainSlots
			}
			if err := r.RedisClient.ExecuteReshardCommand(instance, podCount, fromNode.ID, toNode.ID, strconv.Itoa(slots)); err != nil {
				return err
			}
		}

		klog.Info("Deleting master node")
		if err := r.RedisClient.ExecuteDeleteRedisMasterCommand(instance, podCount); err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisReconciler) ReconcileRedisStandalone(instance *redisv1alpha1.Redis) error {
	if err := r.K8sClient.CreateOrUpdateRedisStandalone(instance); err != nil {
		return err
	}
	if err := r.K8sClient.CreateOrUpdateStandaloneService(instance); err != nil {
		return err
	}
	if err := r.K8sClient.CreateOrUpdateStandaloneHeadlessService(instance); err != nil {
		return err
	}
	return nil
}

// Waiting for redis nodes ready
func (r *RedisReconciler) WaitRedisNodesReady(instance *redisv1alpha1.Redis) error {
	klog.Info("Waiting for redis nodes ready")
	if err := wait.PollImmediate(time.Second*30, time.Minute*5, func() (bool, error) {
		redisMasterSts, err := r.FetchStatefulSet(instance, "master")
		if err != nil {
			klog.Errorf("Could not fetch redis master statefulset: %v", err)
			return false, client.IgnoreNotFound(err)
		}

		redisSlaveSts, err := r.FetchStatefulSet(instance, "slave")
		if err != nil {
			klog.Errorf("Could not fetch redis slave statefulset: %v", err)
			return false, client.IgnoreNotFound(err)
		}
		clusterSize := int32(*instance.Spec.Size)
		masterReady, slaveReady := redisMasterSts.Status.ReadyReplicas, redisSlaveSts.Status.ReadyReplicas
		if masterReady != clusterSize && slaveReady != clusterSize {
			klog.Infof("Redis master and slave nodes are not ready yet, Master: [%d/%d], Slave: [%d/%d]", masterReady, clusterSize, slaveReady, clusterSize)
			return false, nil
		}
		klog.Infof("Redis master and slave nodes are ready, Master: [%d/%d], Slave: [%d/%d]", masterReady, clusterSize, slaveReady, clusterSize)
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

// Waiting for redis cluster node ready
func (r *RedisReconciler) WaitRedisMasterJoin(instance *redisv1alpha1.Redis) error {
	if err := wait.PollImmediate(time.Second*10, time.Minute*2, func() (bool, error) {
		allNodes, err := r.RedisClient.GetRedisClusterNodes(instance)
		if err != nil {
			return false, err
		}
		filterNodes := allNodes.FilterByFunc(redis.IsMasterWithNoSlot)
		if len(filterNodes) == 0 {
			klog.Info("Still waiting for the master node to join ...")
			return false, nil
		}
		klog.Info("The master node has joined the cluster")
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
