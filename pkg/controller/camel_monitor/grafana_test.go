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
	"fmt"
	"testing"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	integreatlyv1beta1 "github.com/grafana/grafana-operator/v5/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAddGrafanaDashboard_Success(t *testing.T) {
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

	err = addGrafanaDashboard(context.TODO(), fakeClient, target, nil)
	require.NoError(t, err)

	gd := &integreatlyv1beta1.GrafanaDashboard{}
	err = fakeClient.Get(context.TODO(), ctrl.ObjectKey{
		Name:      "test-app",
		Namespace: "default",
	}, gd)

	require.NoError(t, err)
	assert.Len(t, gd.Spec.InstanceSelector.MatchLabels, 1)
	assert.Equal(t, "camel-monitor-operator", gd.Spec.InstanceSelector.MatchLabels["camel.apache.org/grafana"])
	assert.Contains(t, gd.Spec.JSON,
		fmt.Sprintf("sum(rate(camel_exchanges_total{job=\\\"%s/%s\\\", eventType=\\\"route\\\"}[5m]))", target.Namespace, target.Name))
	require.NotNil(t, target.Status.GetCondition("GrafanaDashboard"))
	assert.Equal(t, metav1.ConditionTrue, target.Status.GetCondition("GrafanaDashboard").Status)
}

func TestAddGrafanaDashboardUpdateExisting(t *testing.T) {
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

	// Pre-existing GrafanaDashboard
	existing := &integreatlyv1beta1.GrafanaDashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-app",
			Namespace:       "default",
			ResourceVersion: "123",
		},
		// This part will be replaced
		Spec: integreatlyv1beta1.GrafanaDashboardSpec{
			FolderTitle: "olderFolder",
		},
	}

	fakeClient, err := internal.NewFakeClient(existing)
	require.NoError(t, err)

	err = addGrafanaDashboard(context.TODO(), fakeClient, target, nil)
	require.NoError(t, err)

	db := &integreatlyv1beta1.GrafanaDashboard{}
	err = fakeClient.Get(context.TODO(), ctrl.ObjectKey{
		Name:      "test-app",
		Namespace: "default",
	}, db)

	require.NoError(t, err)
	assert.NotEqual(t, existing.Spec.FolderTitle, db.Spec.FolderTitle)
}
