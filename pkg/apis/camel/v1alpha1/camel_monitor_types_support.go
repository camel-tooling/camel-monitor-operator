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
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types set in the CamelMonitor status.
// NOTE: the kubebuilder printcolumn markers in camel_monitor_types.go cannot reference these constants,
// so they must be kept in sync manually if a value is ever changed.
const (
	// ConditionTypeMetricsAvailable reports whether the application metrics endpoint is available. It is rewritten on every
	// monitoring cycle, so its LastTransitionTime is also used as the "last time the application was polled" marker.
	ConditionTypeMetricsAvailable = "MetricsAvailable"
	// ConditionTypeHealthy reports the health status of the application Pods.
	ConditionTypeHealthy = "Healthy"
	// ConditionTypeMemoryPressure reports whether any Pod has JVM memory pressure.
	ConditionTypeMemoryPressure = "MemoryPressure"
	// ConditionTypeCPUPressure reports whether any Pod has CPU pressure.
	ConditionTypeCPUPressure = "CPUPressure"
	// ConditionTypeUpgradeAvailable reports whether a newer runtime version is available.
	ConditionTypeUpgradeAvailable = "UpgradeAvailable"
	// ConditionTypeCronJobExecution reports the scheduling information of a CronJob application.
	ConditionTypeCronJobExecution = "CronJobExecution"
	// ConditionTypePrometheusPodMonitor reports the outcome of the Prometheus PodMonitor creation.
	ConditionTypePrometheusPodMonitor = "PrometheusPodMonitor"
	// ConditionTypeGrafanaDashboard reports the outcome of the Grafana dashboard creation.
	ConditionTypeGrafanaDashboard = "GrafanaDashboard"
)

const (
	// camelPrefix is used to identify Camel prefix labels/annotations.
	camelPrefix = "camel.apache.org"
	// MonitorLabel is used to tag k8s object created by a given Camel Application.
	MonitorLabel = camelPrefix + "/monitor"
	// MonitorImportedKindLabel specifies from what kind of resource an App was imported.
	MonitorImportedKindLabel = camelPrefix + "/imported-from-kind"
	// MonitorImportedNameLabel specifies from what resource an App was imported.
	MonitorImportedNameLabel = camelPrefix + "/imported-from-name"
	// MonitorPollingIntervalSecondsAnnotation is used to instruct a given application to poll interval.
	MonitorPollingIntervalSecondsAnnotation = camelPrefix + "/polling-interval-seconds"
	// MonitorObservabilityServicesHealthPorts is used to instruct an application to use specific ports (comma separated) for health scraping.
	MonitorObservabilityServicesHealthPorts = camelPrefix + "/health-ports"
	// MonitorObservabilityServicesMetricsPorts is used to instruct an application to use specific ports (comma separated) for metrics scraping.
	MonitorObservabilityServicesMetricsPorts = camelPrefix + "/metrics-ports"
	// MonitorObservabilityServicesMetricsEndpoint is used to instruct an application to use specific endpoints (comma separated) for metrics scraping.
	MonitorObservabilityServicesMetricsEndpoint = camelPrefix + "/metrics-endpoints"
	// MonitorObservabilityServicesHealthEndpoint is used to instruct an application to use a specific endpoints (comma separated) for health scraping.
	MonitorObservabilityServicesHealthEndpoint = camelPrefix + "/health-endpoints"
	// MonitorSLIExchangeErrorPercentageAnnotation is used to instruct a given application error percentage SLI Exchange.
	// The status is Error when the failure rate is above this value.
	MonitorSLIExchangeErrorPercentageAnnotation = camelPrefix + "/sli-exchange-error-percentage"
	// MonitorSLIExchangeWarningPercentageAnnotation is used to instruct a given application warning percentage SLI Exchange.
	// The status is Warning when the failure rate is above this value (and not above the error percentage).
	MonitorSLIExchangeWarningPercentageAnnotation = camelPrefix + "/sli-exchange-warning-percentage"
)

func NewCamelMonitor(namespace string, name string) CamelMonitor {
	return CamelMonitor{
		APIVersion: SchemeGroupVersion.String(),
		Kind:       AppKind,
		Namespace:  namespace,
		Name:       name,
	}
}

// AddCondition adds the condition, or updates the existing one of the same type. As per Kubernetes API conventions,
// LastTransitionTime is the last time the condition changed from one status to another, so it is set only when the
// condition is new or its Status changed (the given LastTransitionTime is used if set, otherwise now). If the Status
// is the same, the previous LastTransitionTime is kept and only Reason and Message are updated.
func (cmonStatus *CamelMonitorStatus) AddCondition(condition metav1.Condition) {
	meta.SetStatusCondition(&cmonStatus.Conditions, condition)
}

// RemoveCondition removes the conditions of the given types. Conditions are kept between monitoring cycles, so
// those that do not apply in the current cycle (for instance, a feature that is not enabled or a Pod related
// condition when there is no Pod available) must be explicitly removed in order not to report a stale value.
func (cmonStatus *CamelMonitorStatus) RemoveCondition(condTypes ...string) {
	for _, condType := range condTypes {
		meta.RemoveStatusCondition(&cmonStatus.Conditions, condType)
	}
}

// ImportCamelAnnotations copies all camel annotations from the deployment to the App.
func (monitor *CamelMonitor) ImportCamelAnnotations(annotations map[string]string) {
	for k, v := range annotations {
		if strings.HasPrefix(k, camelPrefix) {
			monitor.Annotations[k] = v
		}
	}
}

func (cmonStatus *CamelMonitorStatus) GetCondition(condType string) *metav1.Condition {
	for _, cond := range cmonStatus.Conditions {
		if cond.Type == condType {
			return &cond
		}
	}

	return nil
}

// DoesExposeMetrics returns true if the app was reconciled and has metrics availability.
func (cmonStatus *CamelMonitorStatus) DoesExposeMetrics() bool {
	return len(cmonStatus.Pods) > 0 &&
		cmonStatus.Pods[0].ObservabilityService != nil &&
		cmonStatus.Pods[0].ObservabilityService.MetricsEndpoint != "" &&
		cmonStatus.Pods[0].ObservabilityService.MetricsPort != ""
}

// GetOwnerReferences returns the owner references to this app.
func (monitor *CamelMonitor) GetOwnerReferences() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         monitor.APIVersion,
			Kind:               monitor.Kind,
			Name:               monitor.Name,
			UID:                monitor.UID,
			Controller:         new(true),
			BlockOwnerDeletion: new(true),
		},
	}
}
