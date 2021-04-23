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
	"bytes"
	"context"
	"fmt"
	"net"
	"strconv"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"

	redis "github.com/go-redis/redis/v8"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
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
func (rc *RedisClient) GetRedisServerIP(cr *redisv1alpha1.Redis, role, nodeNum string) (string, error) {
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{
		Name:      cr.ObjectMeta.Name + "-" + role + "-" + nodeNum,
		Namespace: cr.Namespace,
	}
	if err := rc.Get(context.Background(), podKey, pod); err != nil {
		klog.Errorf("Could not get pod info: %v", err)
		return "", err
	}
	klog.V(1).Infof("Successfully got the pod ip for redis, IP: %s", pod.Status.PodIP)
	return pod.Status.PodIP, nil
}

// ExecuteRedisClusterClusterCommand will execute redis cluster creation command
func (rc *RedisClient) ExecuteRedisClusterClusterCommand(cr *redisv1alpha1.Redis) error {
	replicas := cr.Spec.Size
	cmd := []string{
		"redis-cli",
		"--cluster",
		"create",
	}
	for podCount := 0; podCount <= int(*replicas)-1; podCount++ {
		masterIp, err := rc.GetRedisServerIP(cr, RedisMasterRole, strconv.Itoa(podCount))
		if err != nil {
			return err
		}
		cmd = append(cmd, net.JoinHostPort(masterIp, DefaultRedisPort))
	}
	cmd = append(cmd, "--cluster-yes")
	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Redis cluster creation command: %s", cmd)
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteAddRedisMasterCommand will execute add redis master node command
func (rc *RedisClient) ExecuteAddRedisMasterCommand(cr *redisv1alpha1.Redis, podCount int) error {
	cmd, err := rc.AddNodeCommand(cr, RedisMasterRole, RedisMasterRole, strconv.Itoa(podCount), "0")
	if err != nil {
		return err
	}
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteAddRedisSlaveCommand will execute the replication command
func (rc *RedisClient) ExecuteAddRedisSlaveCommand(cr *redisv1alpha1.Redis, podCount int) error {
	cmd, err := rc.AddNodeCommand(cr, RedisSlaveRole, RedisMasterRole, strconv.Itoa(podCount), strconv.Itoa(podCount))
	if err != nil {
		return err
	}
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteDeleteRedisMasterCommand will execute add redis master node command
func (rc *RedisClient) ExecuteDeleteRedisMasterCommand(cr *redisv1alpha1.Redis, podCount int) error {
	cmd, err := rc.DeleteNodeCommand(cr, RedisMasterRole, strconv.Itoa(podCount))
	if err != nil {
		return err
	}
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteDeleteRedisSlaveCommand will execute add redis master node command
func (rc *RedisClient) ExecuteDeleteRedisSlaveCommand(cr *redisv1alpha1.Redis, podCount int) error {
	cmd, err := rc.DeleteNodeCommand(cr, RedisSlaveRole, strconv.Itoa(podCount))
	if err != nil {
		return err
	}
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteReshardCommand will execute the reshard command
func (rc *RedisClient) ExecuteReshardCommand(cr *redisv1alpha1.Redis, podCount int, fromNodeIds, toNodeId, slots string) error {
	cmd, err := rc.ReshardCommand(cr, strconv.Itoa(podCount), fromNodeIds, toNodeId, slots)
	if err != nil {
		return nil
	}
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ReshardCommand will create redis replication creation command
func (rc *RedisClient) ReshardCommand(cr *redisv1alpha1.Redis, nodeNum, clusterFromNodeIds, clusterToNodeId, clusterSlots string) ([]string, error) {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"reshard",
	}
	masterIp, err := rc.GetRedisServerIP(cr, RedisMasterRole, nodeNum)
	if err != nil {
		return nil, err
	}
	cmd = append(cmd, masterIp+":"+DefaultRedisPort)
	cmd = append(cmd, "--cluster-from")
	cmd = append(cmd, clusterFromNodeIds)
	cmd = append(cmd, "--cluster-to")
	cmd = append(cmd, clusterToNodeId)
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
func (rc *RedisClient) AddNodeCommand(cr *redisv1alpha1.Redis, newRole, existingRole, newNodeNum, existingNodeNum string) ([]string, error) {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"add-node",
	}
	newIp, err := rc.GetRedisServerIP(cr, newRole, newNodeNum)
	if err != nil {
		return nil, err
	}
	existingIp, err := rc.GetRedisServerIP(cr, existingRole, existingNodeNum)
	if err != nil {
		return nil, err
	}

	cmd = append(cmd, net.JoinHostPort(newIp, DefaultRedisPort))
	cmd = append(cmd, net.JoinHostPort(existingIp, DefaultRedisPort))
	if newRole == RedisSlaveRole {
		cmd = append(cmd, "--cluster-slave")
	}
	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Add redis %s node command: %s", newRole, cmd)
	return cmd, nil
}

// DeleteNodeCommand will generate a command for add redis master node
func (rc *RedisClient) DeleteNodeCommand(cr *redisv1alpha1.Redis, role, nodeNum string) ([]string, error) {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"del-node",
	}

	nodeIp, err := rc.GetRedisServerIP(cr, role, nodeNum)
	if err != nil {
		return nil, err
	}
	nodes, err := rc.GetRedisClusterNodes(cr)
	if err != nil {
		return nil, err
	}
	nodeId := nodes.GetNodeByIpPort(nodeIp, DefaultRedisPort).ID

	cmd = append(cmd, net.JoinHostPort(nodeIp, DefaultRedisPort))
	cmd = append(cmd, nodeId)

	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Delete redis %s node command: %s", role, cmd)
	return cmd, nil
}

// ExecuteCommand will execute the commands in pod
func (rc *RedisClient) ExecuteCommand(cr *redisv1alpha1.Redis, cmd []string) error {
	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{Name: cr.ObjectMeta.Name + "-master-0", Namespace: cr.Namespace}
	if err := rc.Get(context.Background(), podKey, pod); err != nil {
		klog.Errorf("Could not get pod info: %v", err)
		return err
	}

	targetContainerIndex := -1
	for i, tr := range pod.Spec.Containers {
		if tr.Name == cr.ObjectMeta.Name+"-master" {
			klog.V(1).Infof("Pod Counted successfully, Count: %d, Container Name: %s", i, tr.Name)
			targetContainerIndex = i
			break
		}
	}

	if targetContainerIndex < 0 {
		klog.Error("Could not find pod container to execute")
		return fmt.Errorf("Could not find pod container to execute")
	}
	req := rc.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(cr.ObjectMeta.Name + "-master-0").
		Namespace(cr.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: pod.Spec.Containers[targetContainerIndex].Name,
		Command:   cmd,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(rc.Config, "POST", req.URL())
	if err != nil {
		klog.Errorf("Failed to init executor: %v", err)
		return err
	}

	if err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	}); err != nil {
		klog.Errorf("Failed to execute command, Command: %s, err: \n%v, \nStdout: \n%s, \nStderr: \n%s", cmd, err, execOut.String(), execErr.String())
		return err
	}
	klog.V(1).Infof("Successfully executed the command, Command: %s, Stdout: \n%s", cmd, execOut.String())
	return nil
}

func (rc *RedisClient) GetRedisClusterNodes(cr *redisv1alpha1.Redis) (Nodes, error) {
	ctx := context.Background()

	masterIp, err := rc.GetRedisServerIP(cr, RedisMasterRole, "0")
	if err != nil {
		return nil, err
	}

	var client *redis.Client
	password := ""
	if cr.Spec.GlobalConfig.Password != nil {
		password = *cr.Spec.GlobalConfig.Password
	}
	client = redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(masterIp, DefaultRedisPort),
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
	return DecodeNodeInfos(&output), nil
}
