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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

const (
	redisPort = 6379
)

// ServiceInterface is the interface to pass service information accross methods
type ServiceInterface struct {
	ExistingService      *corev1.Service
	NewServiceDefinition *corev1.Service
	ServiceType          string
}

// GenerateHeadlessServiceDef generate service definition
func GenerateHeadlessServiceDef(cr *redisv1alpha1.Redis, labels map[string]string, portNumber int32, role string, serviceName string, clusterIP string) *corev1.Service {
	var redisExporterPort int32 = 9121
	service := &corev1.Service{
		TypeMeta:   GenerateMetaInformation("Service", "core/v1"),
		ObjectMeta: GenerateObjectMetaInformation(serviceName, cr.Namespace, labels, GenerateServiceAnots()),
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterIP,
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Name:       cr.ObjectMeta.Name + "-" + role,
					Port:       portNumber,
					TargetPort: intstr.FromInt(int(portNumber)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	if !cr.Spec.RedisExporter.Enabled {
		service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{
			Name:       "redis-exporter",
			Port:       redisExporterPort,
			TargetPort: intstr.FromInt(int(redisExporterPort)),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	AddOwnerRefToObject(service, AsOwner(cr))
	return service
}

// GenerateServiceDef generate service definition
func GenerateServiceDef(cr *redisv1alpha1.Redis, labels map[string]string, portNumber int32, role string, serviceName string, typeService string) *corev1.Service {
	var redisExporterPort int32 = 9121
	var serviceType corev1.ServiceType

	if typeService == "LoadBalancer" {
		serviceType = corev1.ServiceTypeLoadBalancer
	} else if typeService == "NodePort" {
		serviceType = corev1.ServiceTypeNodePort
	} else {
		serviceType = corev1.ServiceTypeClusterIP
	}

	service := &corev1.Service{
		TypeMeta:   GenerateMetaInformation("Service", "core/v1"),
		ObjectMeta: GenerateObjectMetaInformation(serviceName, cr.Namespace, labels, GenerateServiceAnots()),
		Spec: corev1.ServiceSpec{
			Type:     serviceType,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       cr.ObjectMeta.Name + "-" + role,
					Port:       portNumber,
					TargetPort: intstr.FromInt(int(portNumber)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	if !cr.Spec.RedisExporter.Enabled {
		service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{
			Name:       "redis-exporter",
			Port:       redisExporterPort,
			TargetPort: intstr.FromInt(int(redisExporterPort)),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	AddOwnerRefToObject(service, AsOwner(cr))
	return service
}

// CreateMasterHeadlessService creates master headless service
func (rc *RedisClient) CreateMasterHeadlessService(cr *redisv1alpha1.Redis) {
	labels := map[string]string{
		"app":  cr.ObjectMeta.Name + "-master",
		"role": "master",
	}
	serviceDefinition := GenerateHeadlessServiceDef(cr, labels, int32(redisPort), "master", cr.ObjectMeta.Name+"-master-headless", "None")
	serviceBody, err := rc.Clientset.CoreV1().Services(cr.Namespace).Get(context.TODO(), cr.ObjectMeta.Name+"-master-headless", metav1.GetOptions{})
	service := ServiceInterface{
		ExistingService:      serviceBody,
		NewServiceDefinition: serviceDefinition,
		ServiceType:          "master",
	}
	rc.CompareAndCreateService(cr, service, err)
}

// CreateMasterService creates different services for master
func (rc *RedisClient) CreateMasterService(cr *redisv1alpha1.Redis) {
	labels := map[string]string{
		"app":  cr.ObjectMeta.Name + "-master",
		"role": "master",
	}
	serviceDefinition := GenerateServiceDef(cr, labels, int32(redisPort), "master", cr.ObjectMeta.Name+"-master", cr.Spec.Master.Service.Type)
	serviceBody, err := rc.Clientset.CoreV1().Services(cr.Namespace).Get(context.TODO(), cr.ObjectMeta.Name+"-master", metav1.GetOptions{})
	service := ServiceInterface{
		ExistingService:      serviceBody,
		NewServiceDefinition: serviceDefinition,
		ServiceType:          "master",
	}
	rc.CompareAndCreateService(cr, service, err)
}

// CreateSlaveHeadlessService creates slave headless service
func (rc *RedisClient) CreateSlaveHeadlessService(cr *redisv1alpha1.Redis) {
	labels := map[string]string{
		"app":  cr.ObjectMeta.Name + "-slave",
		"role": "slave",
	}
	serviceDefinition := GenerateHeadlessServiceDef(cr, labels, int32(redisPort), "slave", cr.ObjectMeta.Name+"-slave-headless", "None")
	serviceBody, err := rc.Clientset.CoreV1().Services(cr.Namespace).Get(context.TODO(), cr.ObjectMeta.Name+"-slave-headless", metav1.GetOptions{})
	service := ServiceInterface{
		ExistingService:      serviceBody,
		NewServiceDefinition: serviceDefinition,
		ServiceType:          "slave",
	}
	rc.CompareAndCreateService(cr, service, err)
}

// CreateSlaveService creates different services for slave
func (rc *RedisClient) CreateSlaveService(cr *redisv1alpha1.Redis) {
	labels := map[string]string{
		"app":  cr.ObjectMeta.Name + "-slave",
		"role": "slave",
	}
	serviceDefinition := GenerateServiceDef(cr, labels, int32(redisPort), "slave", cr.ObjectMeta.Name+"-slave", cr.Spec.Slave.Service.Type)
	serviceBody, err := rc.Clientset.CoreV1().Services(cr.Namespace).Get(context.TODO(), cr.ObjectMeta.Name+"-slave", metav1.GetOptions{})
	service := ServiceInterface{
		ExistingService:      serviceBody,
		NewServiceDefinition: serviceDefinition,
		ServiceType:          "slave",
	}
	rc.CompareAndCreateService(cr, service, err)
}

// CreateStandaloneService creates redis standalone service
func (rc *RedisClient) CreateStandaloneService(cr *redisv1alpha1.Redis) {
	labels := map[string]string{
		"app":  cr.ObjectMeta.Name + "-" + "standalone",
		"role": "standalone",
	}
	serviceDefinition := GenerateServiceDef(cr, labels, int32(redisPort), "standalone", cr.ObjectMeta.Name, cr.Spec.Service.Type)
	serviceBody, err := rc.Clientset.CoreV1().Services(cr.Namespace).Get(context.TODO(), cr.ObjectMeta.Name, metav1.GetOptions{})

	service := ServiceInterface{
		ExistingService:      serviceBody,
		NewServiceDefinition: serviceDefinition,
		ServiceType:          "standalone",
	}
	rc.CompareAndCreateService(cr, service, err)
}

// CreateStandaloneHeadlessService creates redis standalone service
func (rc *RedisClient) CreateStandaloneHeadlessService(cr *redisv1alpha1.Redis) {
	labels := map[string]string{
		"app":  cr.ObjectMeta.Name + "-" + "standalone",
		"role": "standalone",
	}
	serviceDefinition := GenerateHeadlessServiceDef(cr, labels, int32(redisPort), "standalone", cr.ObjectMeta.Name+"-headless", "None")
	serviceBody, err := rc.Clientset.CoreV1().Services(cr.Namespace).Get(context.TODO(), cr.ObjectMeta.Name+"-headless", metav1.GetOptions{})

	service := ServiceInterface{
		ExistingService:      serviceBody,
		NewServiceDefinition: serviceDefinition,
		ServiceType:          "standalone",
	}
	rc.CompareAndCreateService(cr, service, err)
}

// CompareAndCreateService compares and creates service
func (rc *RedisClient) CompareAndCreateService(cr *redisv1alpha1.Redis, service ServiceInterface, err error) {
	if err != nil {
		klog.Infof("Creating redis service, Redis.Name: %s, Service.Type: %s", cr.ObjectMeta.Name+"-"+service.ServiceType, service.ServiceType)
		_, err := rc.Clientset.CoreV1().Services(cr.Namespace).Create(context.TODO(), service.NewServiceDefinition, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Failed in creating service for redis: %v", err)
		}
	} else if service.ExistingService != service.NewServiceDefinition {
		klog.Infof("Reconciling redis service, Redis.Name: %s, Service.Type: %s", cr.ObjectMeta.Name+"-"+service.ServiceType, service.ServiceType)
		_, err := rc.Clientset.CoreV1().Services(cr.Namespace).Update(context.TODO(), service.NewServiceDefinition, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("Failed in updating service for redis: %v", err)
		}
	} else {
		klog.Infof("Redis service is in sync, Redis.Name: %s, Service.Type: %s", cr.ObjectMeta.Name+"-"+service.ServiceType, service.ServiceType)
	}
}
