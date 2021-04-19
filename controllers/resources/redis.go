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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"

	redis "github.com/go-redis/redis/v8"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
)

// RedisDetails will hold the information for Redis Pod
type RedisDetails struct {
	PodName   string
	Namespace string
}

// GetRedisServerIP will return the IP of redis service
func (rc *RedisClient) GetRedisServerIP(redisInfo RedisDetails) string {
	redisIP, _ := rc.Clientset.CoreV1().Pods(redisInfo.Namespace).
		Get(context.TODO(), redisInfo.PodName, metav1.GetOptions{})

	klog.Infof("Successfully got the ip for redis, ip: %s", redisIP.Status.PodIP)
	return redisIP.Status.PodIP
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
		pod := RedisDetails{
			PodName:   cr.ObjectMeta.Name + "-master-" + strconv.Itoa(podCount),
			Namespace: cr.Namespace,
		}
		cmd = append(cmd, rc.GetRedisServerIP(pod)+":6379")
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
	cmd := rc.GenerateAddRedisMasterNodeCommand(cr, strconv.Itoa(podCount))
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteSlotReshardCommand will execute the reshard command
func (rc *RedisClient) ExecuteSlotReshardCommand(cr *redisv1alpha1.Redis, podCount int) error {
	cmd := rc.GenerateSlotReshardCommand(cr, strconv.Itoa(podCount))
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// ExecuteRedisReplicationCommand will execute the replication command
func (rc *RedisClient) ExecuteRedisReplicationCommand(cr *redisv1alpha1.Redis, podCount int) error {
	cmd := rc.GenerateRedisReplicationCommand(cr, strconv.Itoa(podCount))
	if err := rc.ExecuteCommand(cr, cmd); err != nil {
		return err
	}
	return nil
}

// GenerateRedisReplicationCommand will create redis replication creation command
func (rc *RedisClient) GenerateRedisReplicationCommand(cr *redisv1alpha1.Redis, nodeNumber string) []string {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"add-node",
	}
	masterPod := RedisDetails{
		PodName:   cr.ObjectMeta.Name + "-master-" + nodeNumber,
		Namespace: cr.Namespace,
	}
	slavePod := RedisDetails{
		PodName:   cr.ObjectMeta.Name + "-slave-" + nodeNumber,
		Namespace: cr.Namespace,
	}
	cmd = append(cmd, rc.GetRedisServerIP(slavePod)+":6379")
	cmd = append(cmd, rc.GetRedisServerIP(masterPod)+":6379")
	cmd = append(cmd, "--cluster-slave")

	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Redis replication creation command: %s", cmd)
	return cmd
}

// GenerateSlotReshardCommand will create redis replication creation command
func (rc *RedisClient) GenerateSlotReshardCommand(cr *redisv1alpha1.Redis, nodeNumber string) []string {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"reshard",
	}
	// redis-cli --cluster reshard 192.168.8.124:7979 \
	// --cluster-from 2846540d8284538096f111a8ce7cf01c50199237,e0a9c3e60eeb951a154d003b9b28bbdc0be67d5b,692dec0ccd6bdf68ef5d97f145ecfa6d6bca6132 \
	// --cluster-to 46f0b68b3f605b3369d3843a89a2b4a164ed21e8 --cluster-slots 1024
	masterPod := RedisDetails{
		PodName:   cr.ObjectMeta.Name + "-master-" + nodeNumber,
		Namespace: cr.Namespace,
	}
	cmd = append(cmd, rc.GetRedisServerIP(masterPod)+":6379")
	cmd = append(cmd, "--cluster-from ")
	cmd = append(cmd, rc.GetRedisClusterNodes(cr).GetClusterFromNodeIds())
	cmd = append(cmd, "--cluster-to")
	cmd = append(cmd, rc.GetRedisClusterNodes(cr).GetClusterToNodeID())
	cmd = append(cmd, "--cluster-slots")
	cmd = append(cmd, "1024")

	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Redis replication creation command: %s", cmd)
	return cmd
}

// GenerateAddRedisMasterNodeCommand will generate a command for add redis master node
func (rc *RedisClient) GenerateAddRedisMasterNodeCommand(cr *redisv1alpha1.Redis, nodeNumber string) []string {
	cmd := []string{
		"redis-cli",
		"--cluster",
		"add-node",
	}
	master0Pod := RedisDetails{
		PodName:   cr.ObjectMeta.Name + "-master-0",
		Namespace: cr.Namespace,
	}
	masterNPod := RedisDetails{
		PodName:   cr.ObjectMeta.Name + "-master-" + nodeNumber,
		Namespace: cr.Namespace,
	}
	// MasterNPod is a new redis node, it will added to redis cluster
	cmd = append(cmd, rc.GetRedisServerIP(masterNPod)+":6379")
	cmd = append(cmd, rc.GetRedisServerIP(master0Pod)+":6379")

	if cr.Spec.GlobalConfig.Password != nil {
		cmd = append(cmd, "-a")
		cmd = append(cmd, *cr.Spec.GlobalConfig.Password)
	}
	klog.Infof("Add redis master node command: %s", cmd)
	return cmd
}

// FetchRedisClusterNodesNum will check the redis cluster have sufficient nodes or not
func (rc *RedisClient) FetchRedisClusterNodesNum(cr *redisv1alpha1.Redis) int {
	klog.Info("Checking redis cluster nodes")
	ctx := context.Background()
	var client *redis.Client

	redisInfo := RedisDetails{
		PodName:   cr.ObjectMeta.Name + "-master-0",
		Namespace: cr.Namespace,
	}

	if cr.Spec.GlobalConfig.Password != nil {
		client = redis.NewClient(&redis.Options{
			Addr:     rc.GetRedisServerIP(redisInfo) + ":6379",
			Password: *cr.Spec.GlobalConfig.Password,
			DB:       0,
		})
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:     rc.GetRedisServerIP(redisInfo) + ":6379",
			Password: "",
			DB:       0,
		})
	}
	cmd := client.ClusterNodes(ctx)
	err := client.Process(ctx, cmd)
	if err != nil {
		klog.Errorf("Redis command failed with this error: %v", err)
	}

	output, err := cmd.Result()
	if err != nil {
		klog.Errorf("Redis command failed with this error: %v", err)
	}
	klog.Infof("Redis cluster nodes are listed, Output: \n%s", output)
	scanner := bufio.NewScanner(strings.NewReader(output))

	count := 0
	for scanner.Scan() {
		count++
	}
	klog.Infof("Total number of redis nodes: %d", count)
	return count
}

// ExecuteCommand will execute the commands in pod
func (rc *RedisClient) ExecuteCommand(cr *redisv1alpha1.Redis, cmd []string) error {
	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)

	pod, err := rc.Clientset.CoreV1().Pods(cr.Namespace).Get(context.TODO(), cr.ObjectMeta.Name+"-master-0", metav1.GetOptions{})

	if err != nil {
		klog.Error("Could not get pod info: %v", err)
		return err
	}

	targetContainerIndex := -1
	for i, tr := range pod.Spec.Containers {
		if tr.Name == cr.ObjectMeta.Name+"-master" {
			klog.Infof("Pod Counted successfully, Count: %d, Container Name: %s", i, tr.Name)
			targetContainerIndex = i
			break
		}
	}

	if targetContainerIndex < 0 {
		klog.Errorf("Could not find pod to execute: %v", err)
		return err
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

func (rc *RedisClient) GetRedisClusterNodes(cr *redisv1alpha1.Redis) Nodes {
	ctx := context.Background()
	var client *redis.Client

	redisInfo := RedisDetails{
		PodName:   cr.ObjectMeta.Name + "-master-0",
		Namespace: cr.Namespace,
	}

	if cr.Spec.GlobalConfig.Password != nil {
		client = redis.NewClient(&redis.Options{
			Addr:     rc.GetRedisServerIP(redisInfo) + ":6379",
			Password: *cr.Spec.GlobalConfig.Password,
			DB:       0,
		})
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:     rc.GetRedisServerIP(redisInfo) + ":6379",
			Password: "",
			DB:       0,
		})
	}
	cmd := client.ClusterNodes(ctx)
	err := client.Process(ctx, cmd)
	if err != nil {
		klog.Errorf("Redis command failed with this error: %v", err)
	}

	output, err := cmd.Result()
	if err != nil {
		klog.Errorf("Redis command failed with this error: %v", err)
	}
	return DecodeNodeInfos(&output)
}

// DecodeNodeInfos decode from the cmd output the Redis nodes info.
func DecodeNodeInfos(input *string) Nodes {
	nodes := Nodes{}
	lines := strings.Split(*input, "\n")
	for _, line := range lines {
		klog.Infof("line: %s", line)
		values := strings.Split(line, " ")
		if len(values) < 8 {
			// last line is always empty
			klog.Infof("not enough values in line split, ignoring line: %s", line)
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
				klog.Errorf("error while decoding node info for node '%s', cannot split ip:port ('%s'): %v", node.ID, values[1], err)
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
