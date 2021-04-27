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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"
	"github.com/kubernetes-app/redis-operator/controllers/redis"
	res "github.com/kubernetes-app/redis-operator/controllers/resources"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	K8sClient   *res.K8sClient
	RedisClient *redis.Client
	Scheme      *runtime.Scheme
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
		if instance.Status.PodSize == nil {
			klog.Info("Creating redis cluster")
			instance.SetPhase(redisv1alpha1.ClusterPhaseCreating)
			if err := r.CreateOrUpdateResisStatefulSet(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.CreateOrUpdateResisService(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.CreateResisCluster(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.WaitRedisClusterReady(instance); err != nil {
				return ctrl.Result{}, err
			}
		} else if instance.GetSize() > instance.StatusMinSize() {
			klog.Info("Adding node into redis cluster")
			instance.SetPhase(redisv1alpha1.ClusterPhaseAdding)
			if err := r.CreateOrUpdateResisStatefulSet(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.AddRedisNodes(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.WaitRedisClusterReady(instance); err != nil {
				return ctrl.Result{}, err
			}
		} else if instance.GetSize() < instance.StatusMaxSize() {
			klog.Info("Deleting node from redis cluster")
			instance.SetPhase(redisv1alpha1.ClusterPhaseDeleting)
			if err := r.DeleteRedisNodes(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.CreateOrUpdateResisStatefulSet(instance); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.WaitRedisClusterReady(instance); err != nil {
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
	klog.Infof("Finished reconciling Redis: %s", req.NamespacedName)
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) CreateOrUpdateResisStatefulSet(instance *redisv1alpha1.Redis) error {
	if err := r.K8sClient.CreateOrUpdateRedisServer(instance, redisv1alpha1.RedisMasterRole); err != nil {
		return err
	}
	if err := r.K8sClient.CreateOrUpdateRedisServer(instance, redisv1alpha1.RedisSlaveRole); err != nil {
		return err
	}
	if err := r.WaitRedisNodesReady(instance); err != nil {
		return err
	}
	instance.SetPodSize(instance.Spec.Size)
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
	klog.Info("Adding slave nodes to cluster")
	for podCount := 0; podCount < int(*instance.Spec.Size); podCount++ {
		masterNodeName := instance.ObjectMeta.Name + "-" + redisv1alpha1.RedisMasterRole + "-" + strconv.Itoa(podCount)
		slaveNodeName := instance.ObjectMeta.Name + "-" + redisv1alpha1.RedisSlaveRole + "-" + strconv.Itoa(podCount)
		if err := r.RedisClient.ExecuteAddRedisSlaveCommand(instance, slaveNodeName, masterNodeName); err != nil {
			return err
		}
	}
	instance.SetClusterSize(instance.Spec.Size)
	if err := r.UpdateRedisNodesStatus(instance); err != nil {
		return err
	}
	return nil
}

func (r *RedisReconciler) AddRedisNodes(instance *redisv1alpha1.Redis) error {
	for podCount := instance.GetClusterSize(); podCount < instance.GetSize(); podCount++ {
		masterNodeName := instance.ObjectMeta.Name + "-" + redisv1alpha1.RedisMasterRole + "-" + strconv.Itoa(podCount)
		slaveNodeName := instance.ObjectMeta.Name + "-" + redisv1alpha1.RedisSlaveRole + "-" + strconv.Itoa(podCount)
		if instance.Status.RedisNodes.GetNodeByName(masterNodeName).ID == "" {
			klog.Info("Adding a new node as a master")
			if err := r.RedisClient.ExecuteAddRedisMasterCommand(instance, masterNodeName); err != nil {
				return err
			}
			klog.Infof("Waiting for new node %s join cluster", masterNodeName)
			if err := r.WaitRedisNodeJoinCluster(instance, masterNodeName); err != nil {
				return err
			}
		}

		klog.Info("Resharding the cluster for the new master node")
		fromNodeIds := instance.Status.RedisNodes.GetMasterNodesWithSlot().GetNodeIds()
		toNodeID := instance.Status.RedisNodes.GetNodeByName(masterNodeName).ID
		if err := r.RedisClient.ExecuteReshardCommand(instance, masterNodeName, strings.Join(fromNodeIds, ","), toNodeID, "1024"); err != nil {
			return err
		}

		if instance.Status.RedisNodes.GetNodeByName(slaveNodeName).ID == "" {
			klog.Info("Adding a new node as a slave")
			if err := r.RedisClient.ExecuteAddRedisSlaveCommand(instance, slaveNodeName, masterNodeName); err != nil {
				return err
			}
			klog.Infof("Waiting for new node %s join cluster", slaveNodeName)
			if err := r.WaitRedisNodeJoinCluster(instance, slaveNodeName); err != nil {
				return err
			}
		}
	}
	instance.SetClusterSize(instance.Spec.Size)
	return nil
}

func (r *RedisReconciler) DeleteRedisNodes(instance *redisv1alpha1.Redis) error {
	for podCount := instance.GetSize(); podCount < instance.GetClusterSize(); podCount++ {
		slaveNodeName := instance.ObjectMeta.Name + "-" + redisv1alpha1.RedisSlaveRole + "-" + strconv.Itoa(podCount)
		masterNodeName := instance.ObjectMeta.Name + "-" + redisv1alpha1.RedisMasterRole + "-" + strconv.Itoa(podCount)
		if instance.Status.RedisNodes.GetNodeByName(slaveNodeName).ID != "" {
			klog.Info("Deleting slave node")
			if err := r.RedisClient.ExecuteDeleteRedisNodeCommand(instance, slaveNodeName); err != nil {
				return err
			}
			if err := r.WaitRedisClusterNodeDelete(instance, slaveNodeName); err != nil {
				return err
			}
		}
		if instance.Status.RedisNodes.GetNodeByName(masterNodeName).ID != "" {
			klog.Info("Magerating master node slot to another nodes")
			fromNode := instance.Status.RedisNodes.GetNodeByName(masterNodeName)
			toNodes := instance.Status.RedisNodes.GetMasterNodes().GetNodeWithNoName(masterNodeName)
			slots, remainSlots := fromNode.SlotsNum/toNodes.CountNodes(), fromNode.SlotsNum%toNodes.CountNodes()
			klog.V(1).Infof("fromNode.SlotsNum: %d, toNodesNum: %d, slots: %d, remainSlots: %d", fromNode.SlotsNum, toNodes.CountNodes(), slots, remainSlots)
			for i, toNode := range *toNodes {
				if i == (len(*toNodes) - 1) {
					slots = slots + remainSlots
				}
				if err := r.RedisClient.ExecuteReshardCommand(instance, masterNodeName, fromNode.ID, toNode.ID, strconv.Itoa(int(slots))); err != nil {
					return err
				}
			}
			klog.Info("Waiting for master node data migrate complete")
			if err := r.WaitRedisNodeSlotEmpty(instance, masterNodeName); err != nil {
				return err
			}
			klog.Info("Deleting master node")
			if err := r.RedisClient.ExecuteDeleteRedisNodeCommand(instance, masterNodeName); err != nil {
				return err
			}
			if err := r.WaitRedisClusterNodeDelete(instance, slaveNodeName); err != nil {
				return err
			}
		}
	}
	instance.SetClusterSize(instance.Spec.Size)
	return nil
}

func (r *RedisReconciler) ReconcileRedisStandalone(instance *redisv1alpha1.Redis) error {
	if err := r.K8sClient.CreateOrUpdateRedisServer(instance, redisv1alpha1.RedisStandaloneRole); err != nil {
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
		redisMasterSts, err := r.K8sClient.FetchStatefulSet(instance, redisv1alpha1.RedisMasterRole)
		if err != nil {
			klog.Errorf("Could not fetch redis master statefulset: %v", err)
			return false, client.IgnoreNotFound(err)
		}

		redisSlaveSts, err := r.K8sClient.FetchStatefulSet(instance, redisv1alpha1.RedisSlaveRole)
		if err != nil {
			klog.Errorf("Could not fetch redis slave statefulset: %v", err)
			return false, client.IgnoreNotFound(err)
		}
		clusterSize := *instance.Spec.Size
		masterReady, slaveReady := redisMasterSts.Status.ReadyReplicas, redisSlaveSts.Status.ReadyReplicas
		if masterReady == clusterSize && slaveReady == clusterSize {
			klog.Infof("Redis master and slave nodes are ready, Master: [%d/%d], Slave: [%d/%d]", masterReady, clusterSize, slaveReady, clusterSize)
			return true, nil
		}
		klog.Infof("Redis master and slave nodes are not ready yet, Master: [%d/%d], Slave: [%d/%d]", masterReady, clusterSize, slaveReady, clusterSize)
		return false, nil
	}); err != nil {
		return err
	}
	if err := r.UpdateRedisNodesStatus(instance); err != nil {
		return err
	}
	return nil
}

// Waiting for redis cluster node ready
func (r *RedisReconciler) WaitRedisNodeJoinCluster(instance *redisv1alpha1.Redis, nodeName string) error {
	if err := wait.PollImmediate(time.Second*10, time.Minute*2, func() (bool, error) {
		if err := r.UpdateRedisNodesStatus(instance); err != nil {
			return false, err
		}
		if instance.Status.RedisNodes.GetNodeByName(nodeName).ID == "" {
			klog.Infof("Still waiting for the node %s to join cluster...", nodeName)
			return false, nil
		}
		klog.Infof("The node %s has joined the cluster", nodeName)
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

// Waiting for redis cluster node ready
func (r *RedisReconciler) WaitRedisClusterNodeDelete(instance *redisv1alpha1.Redis, nodeName string) error {
	if err := wait.PollImmediate(time.Second*10, time.Minute*2, func() (bool, error) {
		if err := r.UpdateRedisNodesStatus(instance); err != nil {
			return false, err
		}
		if instance.Status.RedisNodes.GetNodeByName(nodeName).ID != "" {
			klog.Infof("Still wait for the node %s to be deleted...", nodeName)
			return false, nil
		}
		klog.Infof("The node %s has been deleted from the cluster", nodeName)
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

// Waiting for redis cluster ready
func (r *RedisReconciler) WaitRedisClusterReady(instance *redisv1alpha1.Redis) error {
	if err := wait.PollImmediate(time.Second*10, time.Minute*2, func() (bool, error) {
		if err := r.UpdateRedisNodesStatus(instance); err != nil {
			return false, err
		}
		for _, node := range instance.Status.RedisNodes {
			if node.ID == "" || len(node.FailStatus) != 0 {
				klog.Infof("Still waiting for node %s ready in the Redis cluster...", node.Name)
				return false, nil
			}
		}
		klog.Info("The Redis cluster has ready")
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

// Waiting node slots migrate complete
func (r *RedisReconciler) WaitRedisNodeSlotEmpty(instance *redisv1alpha1.Redis, nodeName string) error {
	if err := wait.PollImmediate(time.Second*10, time.Minute*2, func() (bool, error) {
		if err := r.UpdateRedisNodesStatus(instance); err != nil {
			return false, err
		}
		if instance.Status.RedisNodes.GetNodeByName(nodeName).Slots != "" {
			klog.Infof("Still waiting for the node data migrating...", nodeName)
			return false, nil
		}
		klog.Infof("The data of node %s is empty", nodeName)
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

func (r *RedisReconciler) UpdateRedisNodesStatus(cr *redisv1alpha1.Redis) error {
	klog.Info("Updating redis nodes status")
	nodesFromPod, err := r.K8sClient.GetRedisClusterNodes(cr)
	if err != nil {
		return err
	}
	nodesFromApi, err := r.RedisClient.GetRedisClusterNodes(cr)
	if err != nil {
		return err
	}
	newNodes := redisv1alpha1.Nodes{}
	for _, nfp := range *nodesFromPod {
		node := redisv1alpha1.Node{}
		node.Name = nfp.Name
		node.Namespace = nfp.Namespace
		node.IP = nfp.IP
		node.Port = nfp.Port
		node.Role = nfp.Role
		nfa := nodesFromApi.GetNodeWithIPPort(nfp.IP, nfp.Port)
		if nfa != nil {
			node.Role = nfa.Role
			node.ID = nfa.ID
			node.MasterReferent = nfa.MasterReferent
			node.ConfigEpoch = nfa.ConfigEpoch
			node.FailStatus = nfa.FailStatus
			node.LinkState = nfa.LinkState
			node.PingSent = nfa.PingSent
			node.PongRecv = nfa.PongRecv
			node.Slots = nfa.Slots
			node.SlotsNum = nfa.SlotsNum
			node.ImportingSlots = nfa.ImportingSlots
			node.MigratingSlots = nfa.MigratingSlots
		}
		newNodes = append(newNodes, node)
	}
	cr.Status.RedisNodes = newNodes
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1alpha1.Redis{}, builder.WithPredicates(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldObject := e.ObjectOld.(*redisv1alpha1.Redis)
				newObject := e.ObjectNew.(*redisv1alpha1.Redis)
				return !reflect.DeepEqual(oldObject.Spec, newObject.Spec)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Evaluates to false if the object has been confirmed deleted.
				return !e.DeleteStateUnknown
			},
		})).
		Complete(r)
}
