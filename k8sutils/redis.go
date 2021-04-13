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
package k8sutils

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"

	"github.com/go-redis/redis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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
func (rc *RedisClient) ExecuteRedisClusterCommand(cr *redisv1alpha1.Redis) {
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
	klog.Infof("Redis cluster creation command is %s", cmd)
	rc.ExecuteCommand(cr, cmd)
}

// CreateRedisReplicationCommand will create redis replication creation command
func (rc *RedisClient) CreateRedisReplicationCommand(cr *redisv1alpha1.Redis, nodeNumber string) []string {
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
	klog.Infof("Redis replication creation command is %s", cmd)
	return cmd
}

// ExecuteRedisReplicationCommand will execute the replication command
func (rc *RedisClient) ExecuteRedisReplicationCommand(cr *redisv1alpha1.Redis) {
	replicas := cr.Spec.Size
	for podCount := 0; podCount <= int(*replicas)-1; podCount++ {
		cmd := rc.CreateRedisReplicationCommand(cr, strconv.Itoa(podCount))
		rc.ExecuteCommand(cr, cmd)
	}
}

// CheckRedisCluster will check the redis cluster have sufficient nodes or not
func (rc *RedisClient) CheckRedisCluster(cr *redisv1alpha1.Redis) int {
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
	cmd := redis.NewStringCmd("cluster", "nodes")
	err := client.Process(cmd)
	if err != nil {
		klog.Errorf("Redis command failed with this error: %v", err)
	}

	output, err := cmd.Result()
	if err != nil {
		klog.Errorf("Redis command failed with this error: %v", err)
	}
	klog.Infof("Redis cluster nodes are listed, Output: %s", output)
	scanner := bufio.NewScanner(strings.NewReader(output))

	count := 0
	for scanner.Scan() {
		count++
	}
	klog.Infof("Total number of redis nodes are %d", strconv.Itoa(count))
	return count
}

// ExecuteCommand will execute the commands in pod
func (rc *RedisClient) ExecuteCommand(cr *redisv1alpha1.Redis, cmd []string) {
	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)

	config, _ := rest.InClusterConfig()

	pod, err := rc.Clientset.CoreV1().Pods(cr.Namespace).Get(context.TODO(), cr.ObjectMeta.Name+"-master-0", metav1.GetOptions{})

	if err != nil {
		klog.Error("Could not get pod info: %v", err)
	}

	targetContainer := -1
	for i, tr := range pod.Spec.Containers {
		klog.Infof("Pod Counted successfully, Count: %d, Container Name: %s", i, tr.Name)
		if tr.Name == cr.ObjectMeta.Name+"-master" {
			targetContainer = i
			break
		}
	}

	if targetContainer < 0 {
		klog.Errorf("Could not find pod to execute: %v", err)
	}

	req := rc.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(cr.ObjectMeta.Name + "-master-0").
		Namespace(cr.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: pod.Spec.Containers[targetContainer].Name,
		Command:   cmd,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		klog.Errorf("Failed to init executor", err)
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	})

	if err != nil {
		klog.Errorf("Could not execute command: %v", err)
	}
	klog.Infof("Successfully executed the command, Command: %s, Output: %s", cmd, execOut.String())
}
