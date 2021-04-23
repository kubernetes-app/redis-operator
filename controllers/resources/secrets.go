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
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

// GenerateSecretDef is a method that will generate a secret interface
func GenerateSecretDef(cr *redisv1alpha1.Redis, secret *corev1.Secret, scheme *runtime.Scheme) error {
	password := []byte(*cr.Spec.GlobalConfig.Password)
	secret.Data = map[string][]byte{
		"password": password,
	}
	// Set Redis instance as the owner and controller
	if err := ctrl.SetControllerReference(cr, secret, scheme); err != nil {
		return err
	}
	return nil
}

func (rc *K8sClient) CreateOrUpdateRedisSecret(cr *redisv1alpha1.Redis) error {
	labels := map[string]string{
		"app": cr.ObjectMeta.Name,
	}
	secret := &corev1.Secret{
		TypeMeta:   GenerateMetaInformation("Secret", "v1"),
		ObjectMeta: GenerateObjectMetaInformation(cr.ObjectMeta.Name, cr.Namespace, labels, GenerateSecretAnots()),
	}

	or, err := ctrl.CreateOrUpdate(context.TODO(), rc.Client, secret, func() error {
		return GenerateSecretDef(cr, secret, rc.Scheme())
	})
	if err != nil {
		klog.Errorf("Create or Update redis secret failed: %v", err)
		return err
	}
	klog.Infof("Create or Update redis secret successful, secret %s", or)
	return nil
}
