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

package synthetic

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	v1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestAllPodsReady(t *testing.T) {
	pods := []v1alpha1.PodInfo{
		{Ready: true},
		{Ready: true},
	}
	assert.True(t, allPodsReady(pods))

	pods[1].Ready = false
	assert.False(t, allPodsReady(pods))
}

func TestAllPodsUp(t *testing.T) {
	pods := []v1alpha1.PodInfo{
		{Runtime: &v1alpha1.RuntimeInfo{Status: v1alpha1.PodStatusUP}},
		{Runtime: &v1alpha1.RuntimeInfo{Status: v1alpha1.PodStatusUP}},
	}
	assert.True(t, allPodsUp(pods))

	pods[1].Runtime.Status = "DOWN"
	assert.False(t, allPodsUp(pods))
}

func TestSetHealthHttpError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	podInfo := &v1alpha1.PodInfo{}
	err := setHealth(t.Context(), *server.Client(), podInfo, "127.0.0.1", 0)
	require.Error(t, err)
}

func TestSetHealthStatusOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"Healthy"}`))
	}))
	defer server.Close()

	podInfo := &v1alpha1.PodInfo{
		ObservabilityService: &v1alpha1.ObservabilityServiceInfo{},
	}

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	err = setHealth(t.Context(), *server.Client(), podInfo, host, port)
	require.NoError(t, err)

	require.NotNil(t, podInfo.Runtime)
	require.Equal(t, "Healthy", podInfo.Runtime.Status)
}

func TestSetHealthStatus503(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"Degraded"}`))
	}))
	defer server.Close()

	podInfo := &v1alpha1.PodInfo{
		ObservabilityService: &v1alpha1.ObservabilityServiceInfo{},
	}

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	err = setHealth(t.Context(), *server.Client(), podInfo, host, port)
	require.NoError(t, err)

	require.Equal(t, "Degraded", podInfo.Runtime.Status)
}

func TestSetHealthStatusNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status":"Not found"}`))
	}))
	defer server.Close()

	podInfo := &v1alpha1.PodInfo{
		ObservabilityService: &v1alpha1.ObservabilityServiceInfo{},
	}

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	err = setHealth(t.Context(), *server.Client(), podInfo, host, port)
	require.NoError(t, err)

	require.Equal(t, "404 Not Found", podInfo.Runtime.Status)
}

func TestSetMetricsStatusOK(t *testing.T) {
	metricsPayload := `
# HELP app_info Application info
# TYPE app_info gauge
app_info{runtime="quarkus",version="1.0.0"} 1

# TYPE camel_exchanges_total counter
camel_exchanges_total 5

# TYPE camel_exchanges_failed_total counter
camel_exchanges_failed_total 1

# TYPE camel_exchanges_succeeded_total counter
camel_exchanges_succeeded_total 4

# TYPE camel_exchanges_inflight gauge
camel_exchanges_inflight 2

# TYPE camel_exchanges_last_timestamp gauge
camel_exchanges_last_timestamp 123456
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.Header.Get("Accept"), "text/plain")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(metricsPayload))
	}))
	defer server.Close()

	podInfo := &v1alpha1.PodInfo{
		ObservabilityService: &v1alpha1.ObservabilityServiceInfo{},
	}

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	err = setMetrics(t.Context(), *server.Client(), podInfo, host, port)
	require.NoError(t, err)

	// Verify endpoint + port set
	require.Equal(t, platform.DefaultObservabilityMetrics, podInfo.ObservabilityService.MetricsEndpoint)
	require.Equal(t, port, podInfo.ObservabilityService.MetricsPort)

	// Verify runtime + exchange initialized
	require.NotNil(t, podInfo.Runtime)
	require.NotNil(t, podInfo.Runtime.Exchange)

	assert.Equal(t, 5, podInfo.Runtime.Exchange.Total)
	assert.Equal(t, 1, podInfo.Runtime.Exchange.Failed)
	assert.Equal(t, 4, podInfo.Runtime.Exchange.Succeeded)
	assert.Equal(t, 2, podInfo.Runtime.Exchange.Pending)
}

func TestSetMetricsStatusNotOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	podInfo := &v1alpha1.PodInfo{
		ObservabilityService: &v1alpha1.ObservabilityServiceInfo{},
	}

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	err = setMetrics(t.Context(), *server.Client(), podInfo, host, port)
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP status not OK")
}

func TestGetObservabilityPort(t *testing.T) {
	defaultPort := platform.GetObservabilityPort()

	tests := []struct {
		name        string
		annotations map[string]string
		expected    int
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    defaultPort,
		},
		{
			name:        "missing annotation",
			annotations: map[string]string{},
			expected:    defaultPort,
		},
		{
			name: "valid port",
			annotations: map[string]string{
				v1alpha1.MonitorObservabilityServicesPort: "9090",
			},
			expected: 9090,
		},
		{
			name: "invalid port",
			annotations: map[string]string{
				v1alpha1.MonitorObservabilityServicesPort: "not-a-number",
			},
			expected: defaultPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := getObservabilityPort(tt.annotations)
			assert.Equal(t, tt.expected, port)
		})
	}
}

func TestInspectPods(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	podInfo := &v1alpha1.PodInfo{}
	httpClient := http.Client{
		Timeout: time.Second,
	}
	// Use localhost with a wrong port to simulate failure
	badPort := -1
	inspectPod(t.Context(), httpClient, pod, podInfo, "127.0.0.1", badPort, nil)

	assert.NotNil(t, podInfo.ObservabilityService)
	assert.False(t, podInfo.Ready)
	assert.Contains(t, podInfo.Reason, "Could not scrape health endpoint")
	assert.Contains(t, podInfo.Reason, "Could not scrape metrics endpoint")
}

func TestGetPodsWithInspectionFailure(t *testing.T) {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.1",
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod2",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			PodIP: "10.0.0.2",
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	fakeClient, err := internal.NewFakeClient(pod1, pod2)
	require.NoError(t, err)

	podsInfo, err := getPods(http.Client{}, context.Background(), fakeClient, "default",
		map[string]string{"app": "test"}, -1, true, nil)

	assert.NoError(t, err)
	assert.Len(t, podsInfo, 2)
	assert.True(t, podsInfo[0].Ready)
	assert.Contains(t, podsInfo[0].Reason, "Could not scrape health endpoint")
	assert.Contains(t, podsInfo[0].Reason, "Could not scrape metrics endpoint")
	assert.False(t, podsInfo[1].Ready)
	assert.Equal(t, "", podsInfo[1].Reason)
	assert.Equal(t, "", podsInfo[1].Reason)
}

func TestSetUptimeMetricsOK(t *testing.T) {
	metricsPayload := `
# HELP process_cpu_usage The "recent cpu usage" for the Java Virtual Machine process
# TYPE process_cpu_usage gauge
process_cpu_usage 0.013952569169960474
# HELP jvm_memory_max_bytes The maximum amount of memory in bytes that can be used for memory management
# TYPE jvm_memory_max_bytes gauge
jvm_memory_max_bytes{area="heap",id="G1 Eden Space"} -1.0
jvm_memory_max_bytes{area="heap",id="G1 Old Gen"} 8.371830784E9
jvm_memory_max_bytes{area="heap",id="G1 Survivor Space"} -1.0
jvm_memory_max_bytes{area="nonheap",id="CodeHeap 'non-nmethods'"} 5840896.0
jvm_memory_max_bytes{area="nonheap",id="CodeHeap 'non-profiled nmethods'"} 1.22908672E8
jvm_memory_max_bytes{area="nonheap",id="CodeHeap 'profiled nmethods'"} 1.22908672E8
jvm_memory_max_bytes{area="nonheap",id="Compressed Class Space"} 1.073741824E9
jvm_memory_max_bytes{area="nonheap",id="Metaspace"} -1.0
# HELP jvm_memory_used_bytes The amount of used memory
# TYPE jvm_memory_used_bytes gauge
jvm_memory_used_bytes{area="heap",id="G1 Eden Space"} 1.2582912E7
jvm_memory_used_bytes{area="heap",id="G1 Old Gen"} 2.837832E7
jvm_memory_used_bytes{area="heap",id="G1 Survivor Space"} 2481872.0
jvm_memory_used_bytes{area="nonheap",id="CodeHeap 'non-nmethods'"} 1720320.0
jvm_memory_used_bytes{area="nonheap",id="CodeHeap 'non-profiled nmethods'"} 7263744.0
jvm_memory_used_bytes{area="nonheap",id="CodeHeap 'profiled nmethods'"} 1.8410368E7
jvm_memory_used_bytes{area="nonheap",id="Compressed Class Space"} 6795488.0
jvm_memory_used_bytes{area="nonheap",id="Metaspace"} 5.7801568E7
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.Header.Get("Accept"), "text/plain")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(metricsPayload))
	}))
	defer server.Close()

	podInfo := &v1alpha1.PodInfo{
		ObservabilityService: &v1alpha1.ObservabilityServiceInfo{},
	}

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	err = setMetrics(t.Context(), *server.Client(), podInfo, host, port)
	require.NoError(t, err)

	assert.Equal(t, "14", *podInfo.ProcessCPUUsed)
	assert.Equal(t, int64(float64(1.2582912e7)+float64(2.837832e7)+float64(2481872.0)), *podInfo.JVMMemoryUsed)
	assert.Equal(t, int64(float64(-1.0)+float64(8.371830784e9)+float64(-1.0)), *podInfo.JVMMemoryMax)
	assert.False(t, podInfo.HasMemoryPressure)
}

func TestMemoryPressure(t *testing.T) {
	metricsPayload := `
# HELP process_cpu_usage The "recent cpu usage" for the Java Virtual Machine process
# TYPE process_cpu_usage gauge
process_cpu_usage 0.013952569169960474
# HELP jvm_memory_max_bytes The maximum amount of memory in bytes that can be used for memory management
# TYPE jvm_memory_max_bytes gauge
jvm_memory_max_bytes{area="heap",id="G1 Eden Space"} 2.2582912E7
jvm_memory_max_bytes{area="heap",id="G1 Old Gen"} 2.837832E7
jvm_memory_max_bytes{area="heap",id="G1 Survivor Space"} 2481872.0
# HELP jvm_memory_used_bytes The amount of used memory
# TYPE jvm_memory_used_bytes gauge
jvm_memory_used_bytes{area="heap",id="G1 Eden Space"} 2.2582912E7
jvm_memory_used_bytes{area="heap",id="G1 Old Gen"} 2.837832E7
jvm_memory_used_bytes{area="heap",id="G1 Survivor Space"} 2081872.0
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.Header.Get("Accept"), "text/plain")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(metricsPayload))
	}))
	defer server.Close()

	podInfo := &v1alpha1.PodInfo{
		ObservabilityService: &v1alpha1.ObservabilityServiceInfo{},
	}

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	err = setMetrics(t.Context(), *server.Client(), podInfo, host, port)
	require.NoError(t, err)

	assert.True(t, podInfo.HasMemoryPressure)
}

func TestCPUPressure(t *testing.T) {
	metricsPayload := `
# HELP process_cpu_usage The "recent cpu usage" for the Java Virtual Machine process
# TYPE process_cpu_usage gauge
process_cpu_usage 0.1
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.Header.Get("Accept"), "text/plain")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(metricsPayload))
	}))
	defer server.Close()

	podInfo := &v1alpha1.PodInfo{
		ObservabilityService: &v1alpha1.ObservabilityServiceInfo{},
	}

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	err = setMetrics(t.Context(), *server.Client(), podInfo, host, port)
	require.NoError(t, err)
	// value is in millicores
	err = setCPUPressure(podInfo, ptr.To("500"))
	require.NoError(t, err)

	assert.Equal(t, "500", *podInfo.ProcessCPUMax)
	assert.Equal(t, "100", *podInfo.ProcessCPUUsed)
	// 0.1 equals 100 millicores
	assert.False(t, podInfo.HasCPUPressure)

	err = setCPUPressure(podInfo, ptr.To("110"))
	require.NoError(t, err)

	assert.Equal(t, "110", *podInfo.ProcessCPUMax)
	assert.Equal(t, "100", *podInfo.ProcessCPUUsed)
	assert.True(t, podInfo.HasCPUPressure)
}

func TestCPUPressureNoMax(t *testing.T) {
	podInfo := &v1alpha1.PodInfo{
		ObservabilityService: &v1alpha1.ObservabilityServiceInfo{},
	}
	// empty value
	err := setCPUPressure(podInfo, ptr.To(""))
	require.NoError(t, err)
	assert.False(t, podInfo.HasCPUPressure)
}
