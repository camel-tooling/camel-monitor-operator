/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AppKind --.
	AppKind string = "CamelMonitor"
	// PodStatusUP --.
	PodStatusUP string = "UP"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make generate" to regenerate code after modifying this file

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=camelmonitors,scope=Namespaced,shortName=cmon,categories=camel
// +kubebuilder:printcolumn:name="Info",type=string,JSONPath=`.status.info`,description="The Camel App info"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="The Camel App phase"
// +kubebuilder:printcolumn:name="Replicas",type=string,JSONPath=`.status.replicas`,description="The Camel App Pods"
// +kubebuilder:printcolumn:name="Healthy",type=string,JSONPath=`.status.conditions[?(@.type=="Healthy")].status`
// +kubebuilder:printcolumn:name="Monitored",type=string,JSONPath=`.status.conditions[?(@.type=="Monitored")].status`
// +kubebuilder:printcolumn:name="Memory Pressure",type=string,JSONPath=`.status.conditions[?(@.type=="MemoryPressure")].status`
// +kubebuilder:printcolumn:name="CPU Pressure",type=string,JSONPath=`.status.conditions[?(@.type=="CPUPressure")].status`
// +kubebuilder:printcolumn:name="Exchange SLI",type=string,JSONPath=`.status.sliExchangeSuccessRate.status`,description="The success rate SLI"
// +kubebuilder:printcolumn:name="Last Exchange",type=date,JSONPath=`.status.sliExchangeSuccessRate.lastTimestamp`,description="Last exchange age"
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// CamelMonitor is the Schema for the Camel Applications API.
type CamelMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// the desired App specification
	Spec CamelMonitorSpec `json:"spec,omitempty"`
	// the status of the App
	Status CamelMonitorStatus `json:"status,omitempty"`
}

// CamelMonitorSpec specifies the configuration of an App.
type CamelMonitorSpec struct {
}

// CamelMonitorStatus defines the observed state of an App.
type CamelMonitorStatus struct {
	// the actual phase
	Phase CamelMonitorPhase `json:"phase,omitempty"`
	// the image used to run the application
	Image string `json:"image,omitempty"`
	// Some information about the pods backing the application
	Pods []PodInfo `json:"pods,omitempty"`
	// The number of replicas (pods running)
	Replicas *int32 `json:"replicas,omitempty"`
	// A resume of the main App parameters
	Info string `json:"info,omitempty"`
	// The percentage of success rate
	SuccessRate *SLIExchangeSuccessRate `json:"sliExchangeSuccessRate,omitempty"`
	// The conditions catching more detailed information
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// CamelMonitorList contains a list of Apps.
type CamelMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []CamelMonitor `json:"items"`
}

// CamelMonitorPhase --.
type CamelMonitorPhase string

const (
	// CamelMonitorPhaseRunning --.
	CamelMonitorPhaseRunning CamelMonitorPhase = "Running"
	// CamelMonitorPhaseError --.
	CamelMonitorPhaseError CamelMonitorPhase = "Error"
	// CamelMonitorPhasePaused likely scaled to 0.
	CamelMonitorPhasePaused CamelMonitorPhase = "Paused"
)

// PodInfo contains a set of information related to the Pod running the Camel application.
type PodInfo struct {
	// the Pod name
	Name string `json:"name,omitempty"`
	// the Pod ip
	InternalIP string `json:"internalIp,omitempty"`
	// the Pod status
	Status string `json:"status,omitempty"`
	// the Pod updtime timestamp
	UptimeTimestamp *metav1.Time `json:"uptimeTimestamp,omitempty"`
	// the Pod readiness
	Ready bool `json:"ready,omitempty"`
	// the Pod reason why it's not ready
	Reason string `json:"reason,omitempty"`
	// Observability services information
	ObservabilityService *ObservabilityServiceInfo `json:"observe,omitempty"`
	// Some information about the Camel runtime
	Runtime *RuntimeInfo `json:"runtime,omitempty"`
	// the Pod exposes the jolokia port
	JolokiaEnabled bool `json:"jolokiaEnabled,omitempty"`
	// How much CPU (in millicores) the process is consuming
	ProcessCPUUsed *string `json:"processCPUUsed,omitempty"`
	// How much CPU (in millicores) the process is allowed to consume
	ProcessCPUMax *string `json:"processCPUMax,omitempty"`
	// How much memory (in bytes) the process is consuming
	JVMMemoryUsed *int64 `json:"jvmMemoryUsed,omitempty"`
	// How much memory (in bytes) the process is allowed to consume
	JVMMemoryMax *int64 `json:"jvmMemoryMax,omitempty"`
	// If true indicates that the memory usage is approximating dangerously to the cap
	HasMemoryPressure bool `json:"hasMemoryPressure,omitempty"`
	// If true indicates that the CPU usage is approximating dangerously to the cap
	HasCPUPressure bool `json:"hasCPUPressure,omitempty"`
}

// RuntimeInfo contains a set of information related to the Camel application runtime.
type RuntimeInfo struct {
	// the status as reported by health endpoint
	Status string `json:"status,omitempty"`
	// the runtime provider
	RuntimeProvider string `json:"runtimeProvider,omitempty"`
	// the runtime version
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// the Camel core version
	CamelVersion string `json:"camelVersion,omitempty"`
	// Information about the exchange
	Exchange *ExchangeInfo `json:"exchange,omitempty"`
}

// ObservabilityServiceInfo contains the endpoints that can be possibly used to scrape more information.
type ObservabilityServiceInfo struct {
	// the health endpoint
	HealthEndpoint string `json:"healthEndpoint,omitempty"`
	// the health port
	HealthPort int `json:"healthPort,omitempty"`
	// the metrics endpoint
	MetricsEndpoint string `json:"metricsEndpoint,omitempty"`
	// the metrics port
	MetricsPort int `json:"metricsPort,omitempty"`
}

// ExchangeInfo contains the endpoints that can be possibly used to scrape more information.
type ExchangeInfo struct {
	// The total number of exchanges
	Total int `json:"total,omitempty"`
	// The total number of exchanges succeeded
	Succeeded int `json:"succeed,omitempty"`
	// The total number of exchanges failed
	Failed int `json:"failed,omitempty"`
	// The total number of exchanges pending (in Camel jargon, inflight exchanges)
	Pending int `json:"pending,omitempty"`
	// the last message timestamp
	LastTimestamp *metav1.Time `json:"lastTimestamp,omitempty"`
}

// SLIExchangeStatus --.
type SLIExchangeStatus string

const (
	// SLIExchangeStatusError --.
	SLIExchangeStatusError SLIExchangeStatus = "Error"
	// SLIExchangeStatusWarning --.
	SLIExchangeStatusWarning SLIExchangeStatus = "Warning"
	// SLIExchangeStatusSuccess --.
	SLIExchangeStatusSuccess SLIExchangeStatus = "Success"
)

// SLIExchangeSuccessRate contains the information related to the SLI.
type SLIExchangeSuccessRate struct {
	// the success percentage
	SuccessPercentage string `json:"successPercentage,omitempty"`
	// the interval time considered
	SamplingIntervalDuration *time.Duration `json:"samplingInterval,omitempty"`
	// the total exchanges in the interval time considered
	SamplingIntervalTotal int `json:"samplingIntervalTotal,omitempty"`
	// the failed exchanges in the interval time considered
	SamplingIntervalFailed int `json:"samplingIntervalFailed,omitempty"`
	// the last message timestamp
	LastTimestamp *metav1.Time `json:"lastTimestamp,omitempty"`
	// a human readable status information
	Status SLIExchangeStatus `json:"status,omitempty"`
}
