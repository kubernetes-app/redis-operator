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
	"context"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// GenerateSecret is a method that will generate a secret interface
func GenerateSecret(cr *redisv1alpha1.Redis) *corev1.Secret {
	password := []byte(*cr.Spec.GlobalConfig.Password)
	labels := map[string]string{
		"app": cr.ObjectMeta.Name,
	}
	secret := &corev1.Secret{
		TypeMeta:   GenerateMetaInformation("Secret", "v1"),
		ObjectMeta: GenerateObjectMetaInformation(cr.ObjectMeta.Name, cr.Namespace, labels, GenerateSecretAnots()),
		Data: map[string][]byte{
			"password": password,
		},
	}
	AddOwnerRefToObject(secret, AsOwner(cr))
	return secret
}

// CreateRedisSecret method will create a redis secret
func (rc *RedisClient) CreateRedisSecret(cr *redisv1alpha1.Redis) {
	secretBody := GenerateSecret(cr)
	secretName, err := rc.Clientset.CoreV1().Secrets(cr.Namespace).Get(context.TODO(), cr.ObjectMeta.Name, metav1.GetOptions{})
	if err != nil {
		klog.Infof("Creating secret for redis, Secret.Name: %s", cr.ObjectMeta.Name)
		_, err := rc.Clientset.CoreV1().Secrets(cr.Namespace).Create(context.TODO(), secretBody, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Failed in creating secret for redis: %v", err)
		}
	} else if secretBody != secretName {
		klog.Infof("Reconciling secret for redis, Secret.Name: %s", cr.ObjectMeta.Name)
		_, err := rc.Clientset.CoreV1().Secrets(cr.Namespace).Update(context.TODO(), secretBody, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("Failed in updating secret for redis: %v", err)
		}
	} else {
		klog.Infof("Secret for redis are in sync, Secret.Name: %s", cr.ObjectMeta.Name)
	}
}
