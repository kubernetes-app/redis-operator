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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
const (
	// DefaultRedisPort define the default Redis Port
	DefaultRedisPort string = "6379"
	// RedisMasterRole redis role master
	RedisMasterRole string = "master"
	// RedisSlaveRole redis role slave
	RedisSlaveRole string = "slave"
)

// RedisSpec defines the desired state of Redis
type RedisSpec struct {
	Mode              string                     `json:"mode"`
	Size              *int32                     `json:"size,omitempty"`
	GlobalConfig      GlobalConfig               `json:"global"`
	Master            RedisConfig                `json:"master,omitempty"`
	Slave             RedisConfig                `json:"slave,omitempty"`
	Standalone        RedisConfig                `json:"standalone,omitempty"`
	RedisExporter     *RedisExporter             `json:"redisExporter,omitempty"`
	Storage           *Storage                   `json:"storage,omitempty"`
	NodeSelector      map[string]string          `json:"nodeSelector,omitempty"`
	SecurityContext   *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	PriorityClassName string                     `json:"priorityClassName,omitempty"`
	Affinity          *corev1.Affinity           `json:"affinity,omitempty"`
}

// RedisStatus defines the observed state of Redis
type RedisStatus struct {
	// The desired number of member Nodes for the redis cluster
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Size",xDescriptors="urn:alm:descriptor:com.tectonic.ui:podCount"
	Size *int32 `json:"size,omitempty"`
	// The desired all redis nodes info for the redis cluster
	// +optional
	RedisNodes Nodes `json:"redisNodes,omitempty"`
	// Conditions represents the current state of the Request Service.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Conditions",xDescriptors="urn:alm:descriptor:io.kubernetes.conditions"
	Conditions []Condition `json:"conditions,omitempty"`
	// Phase is the redis cluster running phase.
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Phase",xDescriptors="urn:alm:descriptor:io.kubernetes.phase"
	// +optional
	Phase ClusterPhase `json:"phase,omitempty"`
}

// Slot represent a Redis Cluster slot
type Slot uint64

// Node storage redis node info
type Node struct {
	Name           string            `json:"name,omitempty"`
	Namespace      string            `json:"namespace,omitempty"`
	ID             string            `json:"id,omitempty"`
	IP             string            `json:"ip,omitempty"`
	Port           string            `json:"port,omitempty"`
	Role           string            `json:"role,omitempty"`
	LinkState      string            `json:"linkState,omitempty"`
	MasterReferent string            `json:"masterReferent,omitempty"`
	FailStatus     []string          `json:"failStatus,omitempty"`
	PingSent       int64             `json:"pingSent,omitempty"`
	PongRecv       int64             `json:"pongRecv,omitempty"`
	ConfigEpoch    int64             `json:"configEpoch,omitempty"`
	Slots          []Slot            `json:"slots,omitempty"`
	MigratingSlots map[string]string `json:"migratingSlots,omitempty"`
	ImportingSlots map[string]string `json:"importingSlots,omitempty"`
}

// Nodes represent a Node slice
type Nodes []Node

// ConditionType is the condition of a service.
type ConditionType string

// ClusterPhase is the phase of the installation.
type ClusterPhase string

// Constants are used for state.
const (
	ConditionCreating   ConditionType = "Creating"
	ConditionUpdating   ConditionType = "Updating"
	ConditionDeleting   ConditionType = "Deleting"
	ConditionNotFound   ConditionType = "NotFound"
	ConditionOutofScope ConditionType = "OutofScope"
	ConditionReady      ConditionType = "Ready"

	ClusterPhaseDeleting ClusterPhase = "Deleting"
	ClusterPhaseCreating ClusterPhase = "Creating"
	ClusterPhaseAdding   ClusterPhase = "Adding"
	ClusterPhaseUpdating ClusterPhase = "Updating"
	ClusterPhaseRunning  ClusterPhase = "Running"
	ClusterPhaseFailed   ClusterPhase = "Failed"
)

// Condition represents the current state of the Request Service.
// A condition might not show up if it is not happening.
type Condition struct {
	// Type of condition.
	Type ConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	// +optional
	LastUpdateTime string `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// Storage is the inteface to add pvc and pv support in redis
type Storage struct {
	VolumeClaimTemplate corev1.PersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// RedisConfig interface will have the different redis role configuration
type RedisConfig struct {
	Resources Resources         `json:"resources,omitempty"`
	Config    map[string]string `json:"redisConfig,omitempty"`
	Service   Service           `json:"service,omitempty"`
}

// RedisExporter interface will have the information for redis exporter related stuff
type RedisExporter struct {
	Enabled         bool              `json:"enabled,omitempty"`
	Image           string            `json:"image"`
	Resources       *Resources        `json:"resources,omitempty"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// GlobalConfig will be the JSON struct for Basic Redis Config
type GlobalConfig struct {
	Image           string            `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	Password        *string           `json:"password,omitempty"`
	Resources       *Resources        `json:"resources,omitempty"`
}

// ResourceDescription describes CPU and memory resources defined for a cluster.
type ResourceDescription struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// Service is the struct for service definition
type Service struct {
	Type string `json:"type"`
}

// Resources describes requests and limits for the cluster resouces.
type Resources struct {
	ResourceRequests ResourceDescription `json:"requests,omitempty"`
	ResourceLimits   ResourceDescription `json:"limits,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Redis is the Schema for the redis API
type Redis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisSpec   `json:"spec,omitempty"`
	Status RedisStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RedisList contains a list of Redis
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Redis `json:"items"`
}

// SetPhase sets the current Phase status
func (r *Redis) SetPhase(p ClusterPhase) {
	r.Status.Phase = p
}

// SetSize sets the current cluster size status
func (r *Redis) SetSize(s *int32) {
	r.Status.Size = s
}

func init() {
	SchemeBuilder.Register(&Redis{}, &RedisList{})
}
