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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// RedisClient is the struct for ODLM controllers
type RedisClient struct {
	client.Client
	*rest.Config
	*kubernetes.Clientset
}

// NewRedisClient is the method to initialize an Operator struct
func NewRedisClient(mgr manager.Manager) *RedisClient {
	clientset, _ := kubernetes.NewForConfig(mgr.GetConfig())
	return &RedisClient{
		Client:    mgr.GetClient(),
		Config:    mgr.GetConfig(),
		Clientset: clientset,
	}
}
