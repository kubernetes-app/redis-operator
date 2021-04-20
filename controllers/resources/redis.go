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
package resources

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

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
func (rc *RedisClient) GetRedisServerIP(podKey types.NamespacedName) (string, error) {
	pod := &corev1.Pod{}
	if err := rc.Get(context.Background(), podKey, pod); err != nil {
		klog.Errorf("Could not get pod info: %v", err)
		return "", err
	}
	klog.V(1).Infof("Successfully got the pod ip for redis, IP: %s", pod.Status.PodIP)
	return pod.Status.PodIP, nil
}

// ExecuteRedisClusterCommand will execute redis cluster creation command
func (rc *RedisClient) ExecuteRedisClusterCommand(cr *redisv1alpha1.Redis) error {
	replicas := cr.Spec.Size
	cmd := []string{
		"redis-cli",
		"--cluster",
		"create",
	}
	for podCount := 0; podCount <= int(*replicas)-1; podCount++ {
		masterIp, err := rc.GetRedisServerIP(types.NamespacedName{
			Name:      cr.ObjectMeta.Name + "-master-" + strconv.Itoa(podCount),
			Namespace: cr.Namespace,
		})
		if err != nil {
			return err
		}
		cmd = append(cmd, masterIp+":"+DefaultRedisPort)
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
	cmd, err := rc.GenerateAddRedisMasterNodeCommand(cr, strconv.Itoa(podCount))
	if err != nil {
		return err
	}
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteSlotReshardCommand will execute the reshard command
func (rc *RedisClient) ExecuteSlotReshardCommand(cr *redisv1alpha1.Redis, podCount int) error {
	cmd, err := rc.GenerateSlotReshardCommand(cr, strconv.Itoa(podCount))
	if err != nil {
		return nil
	}
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteRedisReplicationCommand will execute the replication command
func (rc *RedisClient) ExecuteRedisReplicationCommand(cr *redisv1alpha1.Redis, podCount int) error {
	cmd, err := rc.GenerateRedisReplicationCommand(cr, strconv.Itoa(podCount))
	if err != nil {
		return err
	}
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// GenerateRedisReplicationCommand will create redis replication creation command
func (rc *RedisClient) GenerateRedisReplicationCommand(cr *redisv1alpha1.Redis, nodeNumber string) ([]string, error) {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"add-node",
	}
	masterIp, err := rc.GetRedisServerIP(types.NamespacedName{
		Name:      cr.ObjectMeta.Name + "-master-" + nodeNumber,
		Namespace: cr.Namespace,
	})
	if err != nil {
		return nil, err
	}
	slaveIp, err := rc.GetRedisServerIP(types.NamespacedName{
		Name:      cr.ObjectMeta.Name + "-slave-" + nodeNumber,
		Namespace: cr.Namespace,
	})
	if err != nil {
		return nil, err
	}
	cmd = append(cmd, slaveIp+":"+DefaultRedisPort)
	cmd = append(cmd, masterIp+":"+DefaultRedisPort)
	cmd = append(cmd, "--cluster-slave")

	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Redis replication creation command: %s", cmd)
	return cmd, nil
}

// GenerateSlotReshardCommand will create redis replication creation command
func (rc *RedisClient) GenerateSlotReshardCommand(cr *redisv1alpha1.Redis, nodeNumber string) ([]string, error) {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"reshard",
	}

	masterIp, err := rc.GetRedisServerIP(types.NamespacedName{
		Name:      cr.ObjectMeta.Name + "-master-" + nodeNumber,
		Namespace: cr.Namespace})
	if err != nil {
		return nil, err
	}
	nodes, err := rc.GetRedisClusterNodes(cr)
	if err != nil {
		return nil, err
	}
	cmd = append(cmd, masterIp+":"+DefaultRedisPort)
	cmd = append(cmd, "--cluster-from")
	cmd = append(cmd, nodes.GetClusterFromNodeIds())
	cmd = append(cmd, "--cluster-to")
	cmd = append(cmd, nodes.GetClusterToNodeID())
	cmd = append(cmd, "--cluster-slots")
	cmd = append(cmd, "1024")

	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Redis replication creation command: %s", cmd)
	return cmd, nil
}

// GenerateAddRedisMasterNodeCommand will generate a command for add redis master node
func (rc *RedisClient) GenerateAddRedisMasterNodeCommand(cr *redisv1alpha1.Redis, nodeNumber string) ([]string, error) {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"add-node",
	}
	masterIp, err := rc.GetRedisServerIP(types.NamespacedName{
		Name:      cr.ObjectMeta.Name + "-master-0",
		Namespace: cr.Namespace,
	})
	if err != nil {
		return nil, err
	}
	slaveIp, err := rc.GetRedisServerIP(types.NamespacedName{
		Name:      cr.ObjectMeta.Name + "-master-" + nodeNumber,
		Namespace: cr.Namespace,
	})
	if err != nil {
		return nil, err
	}
	cmd = append(cmd, slaveIp+":"+DefaultRedisPort)
	cmd = append(cmd, masterIp+":"+DefaultRedisPort)

	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Add redis master node command: %s", cmd)
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
		klog.Errorf("Failed to execute command, Command: %s, \nerr: \n%v, \nStdout: \n%s, \nStderr: \n%s", cmd, err, execOut.String(), execErr.String())
		return err
	}
	klog.Infof("Successfully executed the command, Command: %s, \nStdout: \n%s", cmd, execOut.String())
	return nil
}

func (rc *RedisClient) GetRedisClusterNodes(cr *redisv1alpha1.Redis) (Nodes, error) {
	ctx := context.Background()
	var client *redis.Client
	masterIp, err := rc.GetRedisServerIP(types.NamespacedName{
		Name:      cr.ObjectMeta.Name + "-master-0",
		Namespace: cr.Namespace,
	})
	if err != nil {
		return nil, err
	}

	if cr.Spec.GlobalConfig.Password != nil {
		client = redis.NewClient(&redis.Options{
			Addr:     masterIp + ":" + DefaultRedisPort,
			Password: *cr.Spec.GlobalConfig.Password,
			DB:       0,
		})
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:     masterIp + ":" + DefaultRedisPort,
			Password: "",
			DB:       0,
		})
	}
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

// DecodeNodeInfos decode from the cmd output the Redis nodes info.
func DecodeNodeInfos(input *string) Nodes {
	nodes := Nodes{}
	lines := strings.Split(*input, "\n")
	for _, line := range lines {
		values := strings.Split(line, " ")
		if len(values) < 8 {
			// last line is always empty
			continue
		} else {
			node := NewDefaultNode()
			node.ID = values[0]
			//remove trailing port for cluster internal protocol
			ipPort := strings.Split(values[1], "@")
			if ip, port, err := splitHostPort(ipPort[0]); err == nil {
				node.IP = ip
				node.Port = port
			} else {
				klog.Errorf("error while decoding node info for node %s, cannot split ip:port (%s): %v", node.ID, values[1], err)
			}
			node.SetRole(values[2])
			node.SetReferentMaster(values[3])
			for _, slot := range values[8:] {
				node.Slot = slot
			}
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func splitHostPort(address string) (string, string, error) {
	i := strings.LastIndex(address, ":")
	if i < 0 {
		return "", "", fmt.Errorf("splitHostPort failed, invalid address %s", address)
	}
	host := address[:i]
	port := address[i+1:]
	return host, port, nil
}
