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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	redisv1alpha1 "github.com/kubernetes-app/redis-operator/api/v1alpha1"
)

const (
	constRedisExpoterName = "redis-exporter"
	graceTime             = 15
)

// CreateOrUpdateRedisMaster will create a Redis Master
func (rc *K8sClient) CreateOrUpdateRedisMaster(cr *redisv1alpha1.Redis) error {
	if err := rc.CreateOrUpdateRedisServer(cr, "master"); err != nil {
		return err
	}
	return nil
}

// CreateOrUpdateRedisSlave will create a Redis Slave
func (rc *K8sClient) CreateOrUpdateRedisSlave(cr *redisv1alpha1.Redis) error {
	if err := rc.CreateOrUpdateRedisServer(cr, "slave"); err != nil {
		return err
	}
	return nil
}

// CreateOrUpdateRedisStandalone will create a Redis Standalone server
func (rc *K8sClient) CreateOrUpdateRedisStandalone(cr *redisv1alpha1.Redis) error {
	if err := rc.CreateOrUpdateRedisServer(cr, "standalone"); err != nil {
		return err
	}
	return nil
}

func (rc *K8sClient) CreateOrUpdateRedisServer(cr *redisv1alpha1.Redis, role string) error {
	labels := map[string]string{
		"app":  cr.ObjectMeta.Name + "-" + role,
		"role": role,
	}
	sts := &appsv1.StatefulSet{
		TypeMeta:   GenerateMetaInformation("StatefulSet", "apps/v1"),
		ObjectMeta: GenerateObjectMetaInformation(cr.ObjectMeta.Name+"-"+role, cr.Namespace, labels, GenerateStatefulSetsAnots()),
	}

	or, err := ctrl.CreateOrUpdate(context.TODO(), rc.Client, sts, func() error {
		return GenerateStateFulSetsDef(cr, sts, labels, role, rc.Scheme())
	})
	if err != nil {
		klog.Errorf("Create or Update redis %s server failed: %v", role, err)
		return err
	}
	klog.Infof("Create or Update redis %s server successful, statefulset %s", role, or)
	return nil
}

func (rc *K8sClient) FetchStatefulSet(cr *redisv1alpha1.Redis, role string) (*appsv1.StatefulSet, error) {
	redisSts := &appsv1.StatefulSet{}
	redisStsKey := types.NamespacedName{Name: cr.ObjectMeta.Name + "-" + role, Namespace: cr.Namespace}
	if err := rc.Get(context.Background(), redisStsKey, redisSts); err != nil {
		klog.Errorf("Could not fetch redis %s statefulset: %v", role, err)
		return nil, err
	}
	return redisSts, nil
}

// GenerateStateFulSetsDef generates the statefulsets definition
func GenerateStateFulSetsDef(cr *redisv1alpha1.Redis, sts *appsv1.StatefulSet, labels map[string]string, role string, scheme *runtime.Scheme) error {
	sts.Spec.Selector = LabelSelectors(labels)
	sts.Spec.ServiceName = cr.ObjectMeta.Name + "-" + role
	sts.Spec.Replicas = cr.Spec.Size
	if role == "standalone" {
		var standaloneReplica int32 = 1
		sts.Spec.Replicas = &standaloneReplica
	}
	sts.Spec.Template.ObjectMeta = metav1.ObjectMeta{
		Labels: labels,
	}
	sts.Spec.Template.Spec.Containers = FinalContainerDef(cr, role)
	sts.Spec.Template.Spec.NodeSelector = cr.Spec.NodeSelector
	sts.Spec.Template.Spec.SecurityContext = cr.Spec.SecurityContext
	sts.Spec.Template.Spec.PriorityClassName = cr.Spec.PriorityClassName
	sts.Spec.Template.Spec.Affinity = cr.Spec.Affinity
	// Storage don't support scale up or down
	if cr.Spec.Storage != nil && len(sts.Spec.VolumeClaimTemplates) == 0 {
		sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			GeneratePVCTemplate(cr, role),
		}
	}
	// Set Redis instance as the owner and controller
	if err := ctrl.SetControllerReference(cr, sts, scheme); err != nil {
		return err
	}
	return nil
}

// GenerateContainerDef generates container definition
func GenerateContainerDef(cr *redisv1alpha1.Redis, role string) corev1.Container {
	containerDefinition := corev1.Container{
		Name:            cr.ObjectMeta.Name + "-" + role,
		Image:           cr.Spec.GlobalConfig.Image,
		ImagePullPolicy: cr.Spec.GlobalConfig.ImagePullPolicy,
		Env: []corev1.EnvVar{
			{
				Name:  "SERVER_MODE",
				Value: role,
			},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{},
		},
		VolumeMounts: []corev1.VolumeMount{},
		ReadinessProbe: &corev1.Probe{
			InitialDelaySeconds: graceTime,
			PeriodSeconds:       15,
			FailureThreshold:    5,
			TimeoutSeconds:      5,
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash",
						"/usr/bin/healthcheck.sh",
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash",
						"/usr/bin/healthcheck.sh",
					},
				},
			},
		},
	}
	if cr.Spec.GlobalConfig.Resources != nil {
		containerDefinition.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(cr.Spec.GlobalConfig.Resources.ResourceLimits.CPU)
		containerDefinition.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cr.Spec.GlobalConfig.Resources.ResourceRequests.CPU)
		containerDefinition.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(cr.Spec.GlobalConfig.Resources.ResourceLimits.Memory)
		containerDefinition.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(cr.Spec.GlobalConfig.Resources.ResourceRequests.Memory)
	}
	if cr.Spec.Storage != nil {
		VolumeMounts := corev1.VolumeMount{
			Name:      cr.ObjectMeta.Name + "-" + role,
			MountPath: "/data",
		}
		containerDefinition.VolumeMounts = append(containerDefinition.VolumeMounts, VolumeMounts)
	}
	if cr.Spec.GlobalConfig.Password != nil {
		containerDefinition.Env = append(containerDefinition.Env, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.ObjectMeta.Name,
					},
					Key: "password",
				},
			},
		})
	}
	if cr.Spec.Mode != "cluster" {
		containerDefinition.Env = append(containerDefinition.Env, corev1.EnvVar{
			Name:  "SETUP_MODE",
			Value: "standalone",
		})
	} else {
		containerDefinition.Env = append(containerDefinition.Env, corev1.EnvVar{
			Name:  "SETUP_MODE",
			Value: "cluster",
		})
	}

	if cr.Spec.Storage != nil {
		containerDefinition.Env = append(containerDefinition.Env, corev1.EnvVar{
			Name:  "PERSISTENCE_ENABLED",
			Value: "true",
		})
	}
	return containerDefinition
}

// FinalContainerDef will generate the final statefulset definition
func FinalContainerDef(cr *redisv1alpha1.Redis, role string) []corev1.Container {
	var containerDefinition []corev1.Container
	var exporterDefinition corev1.Container
	var exporterEnvDetails []corev1.EnvVar

	containerDefinition = append(containerDefinition, GenerateContainerDef(cr, role))

	if !cr.Spec.RedisExporter.Enabled {
		return containerDefinition
	}

	if cr.Spec.GlobalConfig.Password != nil {
		exporterEnvDetails = []corev1.EnvVar{
			{
				Name: "REDIS_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cr.ObjectMeta.Name,
						},
						Key: "password",
					},
				},
			}, {
				Name:  "REDIS_ADDR",
				Value: "redis://localhost:6379",
			},
		}
	} else {
		exporterEnvDetails = []corev1.EnvVar{
			{
				Name:  "REDIS_ADDR",
				Value: "redis://localhost:6379",
			},
		}
	}
	exporterDefinition = corev1.Container{
		Name:            constRedisExpoterName,
		Image:           cr.Spec.RedisExporter.Image,
		ImagePullPolicy: cr.Spec.RedisExporter.ImagePullPolicy,
		Env:             exporterEnvDetails,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{},
		},
	}

	if cr.Spec.RedisExporter.Resources != nil {
		exporterDefinition.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(cr.Spec.RedisExporter.Resources.ResourceLimits.CPU)
		exporterDefinition.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cr.Spec.RedisExporter.Resources.ResourceRequests.CPU)
		exporterDefinition.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(cr.Spec.RedisExporter.Resources.ResourceLimits.Memory)
		exporterDefinition.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(cr.Spec.RedisExporter.Resources.ResourceRequests.Memory)
	}

	containerDefinition = append(containerDefinition, exporterDefinition)
	return containerDefinition
}

// GeneratePVCTemplate will create the persistent volume claim template
func GeneratePVCTemplate(cr *redisv1alpha1.Redis, role string) corev1.PersistentVolumeClaim {
	storageSpec := cr.Spec.Storage
	var pvcTemplate corev1.PersistentVolumeClaim

	if storageSpec == nil {
		klog.Infof("No storage is defined for redis", "Redis.Name", cr.ObjectMeta.Name)
	} else {
		pvcTemplate = storageSpec.VolumeClaimTemplate
		pvcTemplate.CreationTimestamp = metav1.Time{}
		pvcTemplate.Name = cr.ObjectMeta.Name + "-" + role
		if storageSpec.VolumeClaimTemplate.Spec.AccessModes == nil {
			pvcTemplate.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		} else {
			pvcTemplate.Spec.AccessModes = storageSpec.VolumeClaimTemplate.Spec.AccessModes
		}
		pvcTemplate.Spec.Resources = storageSpec.VolumeClaimTemplate.Spec.Resources
		pvcTemplate.Spec.Selector = storageSpec.VolumeClaimTemplate.Spec.Selector
		pvcTemplate.Spec.Selector = storageSpec.VolumeClaimTemplate.Spec.Selector
	}
	return pvcTemplate
}
