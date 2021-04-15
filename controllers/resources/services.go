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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

type ServiceMode string

const (
	redisPort                     = 6379
	redisExporterPort             = 9121
	HeadService       ServiceMode = "head"
	HeadlessService   ServiceMode = "headless"
)

// CreateOrUpdateMasterHeadlessService creates master headless service
func (rc *RedisClient) CreateOrUpdateMasterHeadlessService(cr *redisv1alpha1.Redis) error {
	if err := rc.CreateRedisService(cr, HeadlessService, "master"); err != nil {
		return err
	}
	return nil
}

// CreateOrUpdateMasterService creates different services for master
func (rc *RedisClient) CreateOrUpdateMasterService(cr *redisv1alpha1.Redis) error {
	if err := rc.CreateRedisService(cr, HeadService, "master"); err != nil {
		return err
	}
	return nil
}

// CreateOrUpdateSlaveHeadlessService creates slave headless service
func (rc *RedisClient) CreateOrUpdateSlaveHeadlessService(cr *redisv1alpha1.Redis) error {
	if err := rc.CreateRedisService(cr, HeadlessService, "slave"); err != nil {
		return err
	}
	return nil
}

// CreateOrUpdateSlaveService creates different services for slave
func (rc *RedisClient) CreateOrUpdateSlaveService(cr *redisv1alpha1.Redis) error {
	if err := rc.CreateRedisService(cr, HeadService, "slave"); err != nil {
		return err
	}
	return nil
}

// CreateOrUpdateStandaloneHeadlessService creates redis standalone service
func (rc *RedisClient) CreateOrUpdateStandaloneHeadlessService(cr *redisv1alpha1.Redis) error {
	if err := rc.CreateRedisService(cr, HeadlessService, "standalone"); err != nil {
		return err
	}
	return nil
}

// CreateOrUpdateStandaloneService creates redis standalone service
func (rc *RedisClient) CreateOrUpdateStandaloneService(cr *redisv1alpha1.Redis) error {
	if err := rc.CreateRedisService(cr, HeadService, "standalone"); err != nil {
		return err
	}
	return nil
}

func (rc *RedisClient) CreateRedisService(cr *redisv1alpha1.Redis, serviceMode ServiceMode, role string) error {
	labels := map[string]string{
		"app":  cr.ObjectMeta.Name + "-" + role,
		"role": role,
	}
	var serviceName string
	if serviceMode == HeadService {
		if role == "standalone" {
			serviceName = cr.ObjectMeta.Name
		} else {
			serviceName = cr.ObjectMeta.Name + "-" + role
		}
	} else {
		if role == "standalone" {
			serviceName = cr.ObjectMeta.Name + "-" + string(serviceMode)
		} else {
			serviceName = cr.ObjectMeta.Name + "-" + role + "-" + string(serviceMode)
		}
	}
	svc := &corev1.Service{
		TypeMeta:   GenerateMetaInformation("Service", "core/v1"),
		ObjectMeta: GenerateObjectMetaInformation(serviceName, cr.Namespace, labels, GenerateServiceAnots()),
	}

	or, err := ctrl.CreateOrUpdate(context.TODO(), rc.Client, svc, func() error {
		return GenerateServiceDef(cr, svc, serviceMode, labels, role, rc.Scheme())
	})
	if err != nil {
		klog.Errorf("Create or Update redis %s service failed: %v", role, err)
		return err
	}
	klog.Infof("Create or Update redis %s service successful, service %s", role, or)
	return nil
}

// GenerateServiceDef generate service definition
func GenerateServiceDef(cr *redisv1alpha1.Redis, svc *corev1.Service, serviceMode ServiceMode, labels map[string]string, role string, scheme *runtime.Scheme) error {
	if serviceMode == HeadService {
		var serviceType string
		if role == "master" {
			serviceType = cr.Spec.Master.Service.Type
		} else if role == "slave" {
			serviceType = cr.Spec.Slave.Service.Type
		} else {
			serviceType = cr.Spec.Standalone.Service.Type
		}
		svc.Spec.Type = corev1.ServiceType(serviceType)
	} else {
		svc.Spec.ClusterIP = "None"
	}

	svc.Spec.Selector = labels
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       cr.ObjectMeta.Name + "-" + role,
			Port:       redisPort,
			TargetPort: intstr.FromInt(int(redisPort)),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	if !cr.Spec.RedisExporter.Enabled {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:       "redis-exporter",
			Port:       redisExporterPort,
			TargetPort: intstr.FromInt(int(redisExporterPort)),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	// Set Redis instance as the owner and controller
	if err := ctrl.SetControllerReference(cr, svc, scheme); err != nil {
		return err
	}
	return nil
}
