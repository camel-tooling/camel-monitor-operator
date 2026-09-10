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

package monitor

import (
	"context"
	"testing"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAddPrometheusPodMonitor_Success(t *testing.T) {
	target := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
			UID:       "12345",
		},
		Status: v1alpha1.CamelMonitorStatus{
			Pods: []v1alpha1.PodInfo{
				{
					ObservabilityService: &v1alpha1.ObservabilityServiceInfo{
						MetricsEndpoint: "/metrics",
						MetricsPort:     "8080",
					},
				},
			},
		},
	}

	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	err = addPrometheusPodMonitor(context.TODO(), fakeClient, target, nil)
	assert.NoError(t, err)

	pm := &monitoringv1.PodMonitor{}
	err = fakeClient.Get(context.TODO(), ctrl.ObjectKey{
		Name:      "test-app",
		Namespace: "default",
	}, pm)

	assert.NoError(t, err)
	assert.Equal(t, "/metrics", pm.Spec.PodMetricsEndpoints[0].Path)
	assert.Len(t, pm.Spec.PodMetricsEndpoints[0].RelabelConfigs, 1)
	assert.Equal(t, "${1}:8080", *pm.Spec.PodMetricsEndpoints[0].RelabelConfigs[0].Replacement)
	require.NotNil(t, target.Status.GetCondition("PrometheusPodMonitor"))
	assert.Equal(t, metav1.ConditionTrue, target.Status.GetCondition("PrometheusPodMonitor").Status)
}

func TestAddPrometheusPodMonitor_NoPods(t *testing.T) {
	target := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Status: v1alpha1.CamelMonitorStatus{
			Pods: []v1alpha1.PodInfo{},
		},
	}

	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	err = addPrometheusPodMonitor(context.TODO(), fakeClient, target, nil)
	require.NoError(t, err)

	pm := &monitoringv1.PodMonitor{}
	err = fakeClient.Get(context.TODO(), ctrl.ObjectKey{
		Name:      "test-app",
		Namespace: "default",
	}, pm)

	require.Error(t, err)
	assert.Equal(t, "podmonitors.monitoring.coreos.com \"test-app\" not found", err.Error())
}

func TestAddPrometheusPodMonitor_NoObservabilityServices(t *testing.T) {
	target := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Status: v1alpha1.CamelMonitorStatus{
			Pods: []v1alpha1.PodInfo{
				{},
			},
		},
	}

	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	err = addPrometheusPodMonitor(context.TODO(), fakeClient, target, nil)
	require.NoError(t, err)

	pm := &monitoringv1.PodMonitor{}
	err = fakeClient.Get(context.TODO(), ctrl.ObjectKey{
		Name:      "test-app",
		Namespace: "default",
	}, pm)

	require.Error(t, err)
	assert.Equal(t, "podmonitors.monitoring.coreos.com \"test-app\" not found", err.Error())
}

func TestAddPrometheusPodMonitor_NoMetrics(t *testing.T) {
	target := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Status: v1alpha1.CamelMonitorStatus{
			Pods: []v1alpha1.PodInfo{
				{ObservabilityService: &v1alpha1.ObservabilityServiceInfo{HealthEndpoint: "/health"}},
			},
		},
	}

	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	err = addPrometheusPodMonitor(context.TODO(), fakeClient, target, nil)
	require.NoError(t, err)

	pm := &monitoringv1.PodMonitor{}
	err = fakeClient.Get(context.TODO(), ctrl.ObjectKey{
		Name:      "test-app",
		Namespace: "default",
	}, pm)

	require.Error(t, err)
	assert.Equal(t, "podmonitors.monitoring.coreos.com \"test-app\" not found", err.Error())
}

func TestAddPrometheusPodMonitor_UpdateExisting(t *testing.T) {
	target := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
			UID:       "12345",
		},
		Status: v1alpha1.CamelMonitorStatus{
			Pods: []v1alpha1.PodInfo{
				{
					ObservabilityService: &v1alpha1.ObservabilityServiceInfo{
						MetricsEndpoint: "/metrics-new",
						MetricsPort:     "9090",
					},
				},
			},
		},
	}

	// Pre-existing PodMonitor
	existing := &monitoringv1.PodMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
	}

	fakeClient, err := internal.NewFakeClient(existing)
	require.NoError(t, err)

	err = addPrometheusPodMonitor(context.TODO(), fakeClient, target, nil)
	require.NoError(t, err)

	pm := &monitoringv1.PodMonitor{}
	err = fakeClient.Get(context.TODO(), ctrl.ObjectKey{
		Name:      "test-app",
		Namespace: "default",
	}, pm)

	require.NoError(t, err)
	assert.Equal(t, "/metrics-new", pm.Spec.PodMetricsEndpoints[0].Path)
	assert.Len(t, pm.Spec.PodMetricsEndpoints[0].RelabelConfigs, 1)
	assert.Equal(t, "${1}:9090", *pm.Spec.PodMetricsEndpoints[0].RelabelConfigs[0].Replacement)
}
