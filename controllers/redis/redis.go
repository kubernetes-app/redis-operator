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
package redis

import (
	"context"
	"net"
	"strconv"
	"strings"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"

	redis "github.com/go-redis/redis/v8"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

const (
	// DefaultRedisPort define the default Redis Port
	DefaultRedisPort = "6379"
	// RedisMasterRole redis role master
	RedisMasterRole = "master"
	// RedisSlaveRole redis role slave
	RedisSlaveRole = "slave"
)

// GetRedisServerIP will return the IP of redis service
func (r *Client) GetRedisServerIP(cr *redisv1alpha1.Redis, role, nodeNum string) (string, error) {
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{
		Name:      cr.ObjectMeta.Name + "-" + role + "-" + nodeNum,
		Namespace: cr.Namespace,
	}
	if err := r.Get(context.Background(), podKey, pod); err != nil {
		klog.Errorf("Could not get pod info: %v", err)
		return "", err
	}
	klog.V(1).Infof("Successfully got the pod ip for redis, IP: %s", pod.Status.PodIP)
	return pod.Status.PodIP, nil
}

// ExecuteRedisClusterClusterCommand will execute redis cluster creation command
func (r *Client) ExecuteRedisClusterClusterCommand(cr *redisv1alpha1.Redis) error {
	// replicas := cr.Spec.Size
	cmd := []string{
		"redis-cli",
		"--cluster",
		"create",
	}
	cmd = append(cmd, cr.Status.RedisNodes.GetIPPortsByRole(redisv1alpha1.RedisMasterRole)...)
	cmd = append(cmd, "--cluster-yes")
	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Redis cluster creation command: %s", cmd)
	if err := r.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteAddRedisMasterCommand will execute add redis master node command
func (r *Client) ExecuteAddRedisMasterCommand(cr *redisv1alpha1.Redis, masterNodeName string) error {
	master0NodeName := cr.ObjectMeta.Name + "-" + redisv1alpha1.RedisMasterRole + "-0"
	cmd, err := r.AddNodeCommand(cr, RedisMasterRole, masterNodeName, master0NodeName)
	if err != nil {
		return err
	}
	if err := r.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteAddRedisSlaveCommand will execute the replication command
func (r *Client) ExecuteAddRedisSlaveCommand(cr *redisv1alpha1.Redis, slaveNodeName, masterNodeName string) error {
	cmd, err := r.AddNodeCommand(cr, RedisSlaveRole, slaveNodeName, masterNodeName)
	if err != nil {
		return err
	}
	if err := r.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteDeleteRedisNodeCommand will execute add redis master node command
func (r *Client) ExecuteDeleteRedisNodeCommand(cr *redisv1alpha1.Redis, nodeName string) error {
	cmd, err := r.DeleteNodeCommand(cr, nodeName)
	if err != nil {
		return err
	}
	if err := r.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteReshardCommand will execute the reshard command
func (r *Client) ExecuteReshardCommand(cr *redisv1alpha1.Redis, nodeName, fromNodeIds, toNodeID, slots string) error {
	cmd, err := r.ReshardCommand(cr, nodeName, fromNodeIds, toNodeID, slots)
	if err != nil {
		return nil
	}
	if err := r.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ReshardCommand will create redis replication creation command
func (r *Client) ReshardCommand(cr *redisv1alpha1.Redis, nodeName, clusterFromNodeIds, clusterToNodeID, clusterSlots string) ([]string, error) {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"reshard",
	}
	cmd = append(cmd, cr.Status.RedisNodes.GetIPPortByName(nodeName))
	cmd = append(cmd, "--cluster-from")
	cmd = append(cmd, clusterFromNodeIds)
	cmd = append(cmd, "--cluster-to")
	cmd = append(cmd, clusterToNodeID)
	cmd = append(cmd, "--cluster-slots")
	cmd = append(cmd, clusterSlots)
	cmd = append(cmd, "--cluster-yes")

	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Redis reshard command: %s", cmd)
	return cmd, nil
}

// AddNodeCommand will generate a command for add redis node
func (r *Client) AddNodeCommand(cr *redisv1alpha1.Redis, role, newNodeName, existingNodeName string) ([]string, error) {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"add-node",
	}

	newNode := cr.Status.RedisNodes.GetNodeByName(newNodeName)
	existingNode := cr.Status.RedisNodes.GetNodeByName(existingNodeName)
	cmd = append(cmd, newNode.IPPort())
	cmd = append(cmd, existingNode.IPPort())
	if role == redisv1alpha1.RedisSlaveRole {
		cmd = append(cmd, "--cluster-slave")
	}
	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Add redis %s node command: %s", role, cmd)
	return cmd, nil
}

// DeleteNodeCommand will generate a command for add redis master node
func (r *Client) DeleteNodeCommand(cr *redisv1alpha1.Redis, nodeName string) ([]string, error) {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"del-node",
	}
	node := cr.Status.RedisNodes.GetNodeByName(nodeName)

	cmd = append(cmd, node.IPPort())
	cmd = append(cmd, node.ID)

	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Delete redis node command: %s", cmd)
	return cmd, nil
}

// ExecuteCommand will execute the commands in pod
func (r *Client) ExecuteCommand(cr *redisv1alpha1.Redis, cmd []string) error {
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{Name: cr.ObjectMeta.Name + "-master-0", Namespace: cr.Namespace}
	if err := r.Get(context.Background(), podKey, pod); err != nil {
		klog.Errorf("Could not get pod info: %v", err)
		return err
	}
	if err := r.Execer.ExecCommandInPod(pod, cmd...); err != nil {
		return err
	}
	return nil
}

func (r *Client) GetRedisClusterNodes(cr *redisv1alpha1.Redis) (*redisv1alpha1.Nodes, error) {
	klog.Info("Geting redis cluster nodes info with redis api")
	ctx := context.Background()

	masterIP, err := r.GetRedisServerIP(cr, RedisMasterRole, "0")
	if err != nil {
		return nil, err
	}

	var client *redis.Client
	password := ""
	if cr.Spec.GlobalConfig.Password != nil {
		password = *cr.Spec.GlobalConfig.Password
	}
	client = redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(masterIP, DefaultRedisPort),
		Password: password,
		DB:       0,
	})
	cmd := client.ClusterNodes(ctx)
	if err := client.Process(ctx, cmd); err != nil {
		klog.Errorf("Redis command failed with this error: %v", err)
	}

	output, err := cmd.Result()
	if err != nil {
		klog.Errorf("Redis command failed with this error: %v", err)
	}
	klog.V(2).Infof("Redis cluster nodes: \n%s", output)
	return ParseNodeInfos(&output), nil
}

func ParseNodeInfos(input *string) *redisv1alpha1.Nodes {
	nodes := redisv1alpha1.Nodes{}
	lines := strings.Split(*input, "\n")
	for _, line := range lines {
		values := strings.Split(line, " ")
		if len(values) < 8 {
			// last line is always empty
			klog.V(2).Infof("Not enough values in line split, ignoring line: %s", line)
			continue
		} else {
			node := &redisv1alpha1.Node{
				Port:           DefaultRedisPort,
				MigratingSlots: map[string]string{},
				ImportingSlots: map[string]string{},
			}

			node.ID = values[0]
			//remove trailing port for cluster internal protocol
			ipPort := strings.Split(values[1], "@")
			if ip, port, err := net.SplitHostPort(ipPort[0]); err == nil {
				node.IP = ip
				node.Port = port
			} else {
				klog.Errorf("Error while decoding node info for node %s, cannot split ip:port (%s): %v", node.ID, values[1], err)
			}
			node.SetRole(values[2])
			node.SetFailureStatus(values[2])
			node.SetReferentMaster(values[3])
			if i, err := strconv.Atoi(values[4]); err == nil {
				node.PingSent = i
			}
			if i, err := strconv.Atoi(values[5]); err == nil {
				node.PongRecv = i
			}
			if i, err := strconv.Atoi(values[6]); err == nil {
				node.ConfigEpoch = i
			}
			node.SetLinkStatus(values[7])
			node.Slots = strings.Join(values[8:], " ")
			for _, slot := range values[8:] {
				if s, importing, migrating, err := redisv1alpha1.DecodeSlotRange(slot); err == nil {
					node.SlotsNum += len(s)
					if importing != nil {
						node.ImportingSlots[importing.SlotID.String()] = importing.FromNodeID
					}
					if migrating != nil {
						node.MigratingSlots[migrating.SlotID.String()] = migrating.ToNodeID
					}
				}
			}
			nodes = append(nodes, *node)
		}
	}
	return &nodes
}
