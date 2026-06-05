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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// MonitorObservabilityServicesPort is used to instruct an application to use a specific port for metrics scraping.
	MonitorObservabilityServicesPort = camelPrefix + "/observability-services-port"
	// MonitorSLIExchangeErrorPercentageAnnotation is used to instruct a given application error percentage SLI Exchange.
	MonitorSLIExchangeErrorPercentageAnnotation = camelPrefix + "/sli-exchange-error-percentage"
	// MonitorSLIExchangeWarningPercentageAnnotation is used to instruct a given application warning percentage SLI Exchange.
	MonitorSLIExchangeWarningPercentageAnnotation = camelPrefix + "/sli-exchange-warning-percentage"
)

func NewCamelMonitor(namespace string, name string) CamelMonitor {
	return CamelMonitor{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       AppKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func (cmonStatus *CamelMonitorStatus) AddCondition(condition metav1.Condition) {
	if cmonStatus.Conditions == nil {
		cmonStatus.Conditions = []metav1.Condition{}
	}

	cmonStatus.Conditions = append(cmonStatus.Conditions, condition)
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
		cmonStatus.Pods[0].ObservabilityService.MetricsPort != 0
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
