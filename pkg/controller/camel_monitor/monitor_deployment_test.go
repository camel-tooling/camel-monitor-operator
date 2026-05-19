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
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestMonitorActionBakingDeploymentMissing(t *testing.T) {
	app := &v1alpha1.CamelMonitor{}
	app.Name = "test-app"
	app.Namespace = "default"
	app.Annotations = map[string]string{
		v1alpha1.MonitorImportedKindLabel: "Deployment",
		v1alpha1.MonitorImportedNameLabel: "test-deployment",
	}

	fakeClient, err := internal.NewFakeClient(app)
	require.NoError(t, err)

	action := &monitorAction{}
	action.InjectClient(fakeClient)

	_, err = action.Handle(context.TODO(), app)

	require.Error(t, err)
	require.Equal(t, "baking deployment does not exist for App default/test-app", err.Error())
}

func TestMonitorActionDeploymentScaledTo0(t *testing.T) {
	app := &v1alpha1.CamelMonitor{}
	app.Name = "test-app"
	app.Namespace = "default"
	app.Annotations = map[string]string{
		v1alpha1.MonitorImportedKindLabel: "Deployment",
		v1alpha1.MonitorImportedNameLabel: "my-test-deploy",
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-test-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Image: "my-camel-image"},
				},
			}},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "my-camel-app"}},
			Replicas: ptr.To(int32(0)),
		},
	}

	fakeClient, err := internal.NewFakeClient(app, deployment)
	require.NoError(t, err)

	action := &monitorAction{}
	action.InjectClient(fakeClient)

	target, err := action.Handle(context.TODO(), app)

	require.NoError(t, err)
	require.NotNil(t, target)
	assert.Equal(t, "my-camel-image", target.Status.Image)
	assert.Equal(t, ptr.To(int32(0)), target.Status.Replicas)
	assert.Equal(t, v1alpha1.CamelMonitorPhasePaused, target.Status.Phase)
	monitored := target.Status.GetCondition("Monitored")
	assert.NotNil(t, monitored)
	assert.Equal(t, metav1.ConditionUnknown, monitored.Status)
	assert.Equal(t, "No active Pod available", monitored.Message)
	healthy := target.Status.GetCondition("Healthy")
	assert.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionUnknown, healthy.Status)
	assert.Equal(t, "No active Pod available", healthy.Message)
}

func TestMonitorActionDeploymentWithStatusScaledTo0(t *testing.T) {
	lastExchangeTime := time.Now()
	app := &v1alpha1.CamelMonitor{}
	app.Name = "test-app"
	app.Namespace = "default"
	app.Annotations = map[string]string{
		v1alpha1.MonitorImportedKindLabel: "Deployment",
		v1alpha1.MonitorImportedNameLabel: "my-test-deploy",
	}
	// NOTE: these information should still exists when the app is scaled to 0
	app.Status = v1alpha1.CamelMonitorStatus{
		Info: "existing info",
		SuccessRate: &v1alpha1.SLIExchangeSuccessRate{
			LastTimestamp: &metav1.Time{Time: lastExchangeTime},
		},
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-test-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Image: "my-camel-image"},
				},
			}},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "my-camel-app"}},
			Replicas: ptr.To(int32(0)),
		},
	}

	fakeClient, err := internal.NewFakeClient(app, deployment)
	require.NoError(t, err)

	action := &monitorAction{}
	action.InjectClient(fakeClient)

	target, err := action.Handle(context.TODO(), app)

	require.NoError(t, err)
	require.NotNil(t, target)
	assert.Equal(t, "my-camel-image", target.Status.Image)
	assert.Equal(t, ptr.To(int32(0)), target.Status.Replicas)
	assert.Equal(t, v1alpha1.CamelMonitorPhasePaused, target.Status.Phase)
	monitored := target.Status.GetCondition("Monitored")
	assert.NotNil(t, monitored)
	assert.Equal(t, metav1.ConditionUnknown, monitored.Status)
	assert.Equal(t, "No active Pod available", monitored.Message)
	healthy := target.Status.GetCondition("Healthy")
	assert.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionUnknown, healthy.Status)
	assert.Equal(t, "No active Pod available", healthy.Message)
	assert.Equal(t, "existing info", target.Status.Info)
	assert.Equal(t, lastExchangeTime, target.Status.SuccessRate.LastTimestamp.Time)
}

func TestMonitorActionDeploymentNonActivePods(t *testing.T) {
	app := &v1alpha1.CamelMonitor{}
	app.Name = "test-app"
	app.Namespace = "default"
	app.Annotations = map[string]string{
		v1alpha1.MonitorImportedKindLabel: "Deployment",
		v1alpha1.MonitorImportedNameLabel: "my-test-deploy",
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-test-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Image: "my-camel-image"},
				},
			}},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "my-camel-app"}},
			Replicas: ptr.To(int32(2)),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          2,
			AvailableReplicas: 2,
		},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod-1", Namespace: "default", Labels: map[string]string{"app": "my-camel-app"}},
		Status:     v1.PodStatus{Phase: corev1.PodPending},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod-2", Namespace: "default", Labels: map[string]string{"app": "my-camel-app"}},
		Status:     v1.PodStatus{Phase: corev1.PodPending},
	}
	pod3 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod-3", Namespace: "default", Labels: map[string]string{"app": "my-camel-app"}},
		Status:     v1.PodStatus{Phase: corev1.PodFailed},
	}

	fakeClient, err := internal.NewFakeClient(app, deployment, pod1, pod2, pod3)
	require.NoError(t, err)

	action := &monitorAction{}
	action.InjectClient(fakeClient)

	target, err := action.Handle(context.TODO(), app)

	require.NoError(t, err)
	require.NotNil(t, target)
	assert.Equal(t, "my-camel-image", target.Status.Image)
	assert.Equal(t, ptr.To(int32(2)), target.Status.Replicas)
	assert.Equal(t, v1alpha1.CamelMonitorPhaseRunning, target.Status.Phase)
	assert.Len(t, target.Status.Pods, 3)
	assert.Contains(t, target.Status.Pods, v1alpha1.PodInfo{Name: "my-pod-1", Status: "Pending"})
	assert.Contains(t, target.Status.Pods, v1alpha1.PodInfo{Name: "my-pod-2", Status: "Pending"})
	assert.Contains(t, target.Status.Pods, v1alpha1.PodInfo{Name: "my-pod-3", Status: "Failed"})

	monitored := target.Status.GetCondition("Monitored")
	assert.NotNil(t, monitored)
	assert.Equal(t, metav1.ConditionFalse, monitored.Status)
	healthy := target.Status.GetCondition("Healthy")
	assert.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionFalse, healthy.Status)
}

func TestMonitorActionDeploymentPodsRunning(t *testing.T) {
	server := mockServer()
	defer server.Close()
	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	app := &v1alpha1.CamelMonitor{}
	app.Name = "test-app"
	app.Namespace = "default"
	app.Annotations = map[string]string{
		v1alpha1.MonitorImportedKindLabel: "Deployment",
		v1alpha1.MonitorImportedNameLabel: "my-test-deploy",
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-test-deploy",
			Namespace: "default",
			Annotations: map[string]string{
				// Use the mock server for testing purposes
				v1alpha1.MonitorObservabilityServicesPort: portStr,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Image: "my-camel-image"},
				},
			}},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "my-camel-app"}},
			Replicas: ptr.To(int32(1)),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          1,
			AvailableReplicas: 1,
		},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod-1", Namespace: "default", Labels: map[string]string{"app": "my-camel-app"}},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			Phase:      corev1.PodRunning,
			PodIP:      host,
		},
	}

	fakeClient, err := internal.NewFakeClient(app, deployment, pod1)
	require.NoError(t, err)

	action := &monitorAction{}
	action.InjectClient(fakeClient)

	target, err := action.Handle(context.TODO(), app)

	require.NoError(t, err)
	require.NotNil(t, target)
	assert.Equal(t, "my-camel-image", target.Status.Image)
	assert.Equal(t, ptr.To(int32(1)), target.Status.Replicas)
	assert.Equal(t, v1alpha1.CamelMonitorPhaseRunning, target.Status.Phase)
	assert.Len(t, target.Status.Pods, 1)
	assert.True(t, target.Status.Pods[0].Ready)

	monitored := target.Status.GetCondition("Monitored")
	assert.NotNil(t, monitored)
	assert.Equal(t, metav1.ConditionTrue, monitored.Status)
	healthy := target.Status.GetCondition("Healthy")
	assert.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionTrue, healthy.Status)
}

func mockServer() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "health"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"UP"}`))

		case strings.Contains(r.URL.Path, "metrics"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`# HELP app_info Application info
# TYPE app_info gauge
app_info{runtime="quarkus",version="1.0.0"} 1
`))

		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	return server
}
