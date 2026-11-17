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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
)

const shutdownMetrics = `
# HELP app_info Application information
# TYPE app_info gauge
app_info{camel_runtime_provider="Main",camel_runtime_version="4.23.0",camel_version="4.23.0"} 1.0
# HELP camel_exchanges_total Total number of processed exchanges
# TYPE camel_exchanges_total counter
camel_exchanges_total{routeId="route1"} 3.0
# HELP camel_exchanges_succeeded_total Number of successfully completed exchanges
# TYPE camel_exchanges_succeeded_total counter
camel_exchanges_succeeded_total{routeId="route1"} 2.0
# HELP camel_exchanges_failed_total Number of failed exchanges
# TYPE camel_exchanges_failed_total counter
camel_exchanges_failed_total{routeId="route1"} 1.0
# HELP camel_exchanges_last_timestamp Last exchange timestamp
# TYPE camel_exchanges_last_timestamp gauge
camel_exchanges_last_timestamp{routeId="route1"} 1.7e12
# HELP process_cpu_usage The "recent cpu usage" for the Java Virtual Machine process
# TYPE process_cpu_usage gauge
process_cpu_usage 0.1
# HELP jvm_memory_max_bytes The maximum amount of memory in bytes
# TYPE jvm_memory_max_bytes gauge
jvm_memory_max_bytes{area="heap",id="G1 Old Gen"} 1000.0
# HELP jvm_memory_used_bytes The amount of used memory
# TYPE jvm_memory_used_bytes gauge
jvm_memory_used_bytes{area="heap",id="G1 Old Gen"} 950.0
`

const logLinePrefix = "2026-04-28 11:08:27.608  WARN 247539 --- [           main] crometer.prometheus.MicrometerPrometheus : "

// shutdownMetricsLog is shutdownMetrics as printed by Camel on shutdown: one log line per metric line.
var shutdownMetricsLog = toLogLines(shutdownMetrics)

// toLogLines wraps each non empty line into a log line, as printed by Camel on shutdown.
func toLogLines(metrics string) string {
	var sb strings.Builder

	for line := range strings.SplitSeq(metrics, "\n") {
		if line == "" {
			continue
		}

		sb.WriteString(logLinePrefix + logMetricStart + line + logMetricEnd + "\n")
	}

	return sb.String()
}

func newLogClientset(t *testing.T, handler http.HandlerFunc) k8s.Interface {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	clientset, err := k8s.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	return clientset
}

func logHandler(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}
}

func newLogTestPod() *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "my-ns"}}
}

func newLogTestObsConf() appObservabilityConf {
	return appObservabilityConf{logger: log.Log}
}

func TestInspectLogSetsMetrics(t *testing.T) {
	clientset := newLogClientset(t, logHandler(shutdownMetricsLog))
	podInfo := &v1alpha1.PodInfo{}

	inspectLog(t.Context(), clientset, newLogTestPod(), podInfo, newLogTestObsConf(), ptr.To("500"))

	require.NotNil(t, podInfo.Runtime)
	assert.Equal(t, "Main", podInfo.Runtime.RuntimeProvider)
	assert.Equal(t, "4.23.0", podInfo.Runtime.RuntimeVersion)
	assert.Equal(t, "4.23.0", podInfo.Runtime.CamelVersion)
	require.NotNil(t, podInfo.Runtime.Exchange)
	assert.Equal(t, 3, podInfo.Runtime.Exchange.Total)
	assert.Equal(t, 2, podInfo.Runtime.Exchange.Succeeded)
	assert.Equal(t, 1, podInfo.Runtime.Exchange.Failed)
	assert.Equal(t, 0, podInfo.Runtime.Exchange.Pending)
	require.NotNil(t, podInfo.Runtime.Exchange.LastTimestamp)

	require.NotNil(t, podInfo.JVMMemoryUsed)
	require.NotNil(t, podInfo.JVMMemoryMax)
	assert.Equal(t, int64(950), *podInfo.JVMMemoryUsed)
	assert.Equal(t, int64(1000), *podInfo.JVMMemoryMax)
	assert.True(t, podInfo.HasMemoryPressure)

	require.NotNil(t, podInfo.ProcessCPUUsed)
	require.NotNil(t, podInfo.ProcessCPUMax)
	assert.Equal(t, "100", *podInfo.ProcessCPUUsed)
	assert.Equal(t, "500", *podInfo.ProcessCPUMax)
	assert.False(t, podInfo.HasCPUPressure)
}

func TestInspectLogCPUPressure(t *testing.T) {
	clientset := newLogClientset(t, logHandler(shutdownMetricsLog))
	podInfo := &v1alpha1.PodInfo{}

	// 100 millicores used out of 110 is above the 90% threshold
	inspectLog(t.Context(), clientset, newLogTestPod(), podInfo, newLogTestObsConf(), ptr.To("110"))

	assert.Equal(t, "110", *podInfo.ProcessCPUMax)
	assert.True(t, podInfo.HasCPUPressure)
}

func TestInspectLogNoCPULimit(t *testing.T) {
	clientset := newLogClientset(t, logHandler(shutdownMetricsLog))

	for name, limit := range map[string]*string{"nil": nil, "empty": ptr.To("")} {
		t.Run(name, func(t *testing.T) {
			podInfo := &v1alpha1.PodInfo{}

			inspectLog(t.Context(), clientset, newLogTestPod(), podInfo, newLogTestObsConf(), limit)

			require.NotNil(t, podInfo.Runtime)
			assert.Equal(t, 3, podInfo.Runtime.Exchange.Total)
			assert.Nil(t, podInfo.ProcessCPUMax)
			assert.False(t, podInfo.HasCPUPressure)
		})
	}
}

func TestInspectLogInvalidCPULimit(t *testing.T) {
	clientset := newLogClientset(t, logHandler(shutdownMetricsLog))
	podInfo := &v1alpha1.PodInfo{}

	// The error is only logged: metrics are kept, no pressure is flagged
	inspectLog(t.Context(), clientset, newLogTestPod(), podInfo, newLogTestObsConf(), ptr.To("not-a-number"))

	require.NotNil(t, podInfo.Runtime)
	assert.Equal(t, 3, podInfo.Runtime.Exchange.Total)
	assert.False(t, podInfo.HasCPUPressure)
}

func TestInspectLogDumpFailure(t *testing.T) {
	clientset := newLogClientset(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "pod not found", http.StatusNotFound)
	})
	podInfo := &v1alpha1.PodInfo{}

	inspectLog(t.Context(), clientset, newLogTestPod(), podInfo, newLogTestObsConf(), ptr.To("500"))

	assert.Equal(t, &v1alpha1.PodInfo{}, podInfo)
}

func TestInspectLogInvalidPrometheusContent(t *testing.T) {
	clientset := newLogClientset(t, logHandler("2024-01-01 INFO this is not a prometheus metric line\n"))
	podInfo := &v1alpha1.PodInfo{}

	inspectLog(t.Context(), clientset, newLogTestPod(), podInfo, newLogTestObsConf(), ptr.To("500"))

	// Nothing scraped, and the CPU pressure is not evaluated
	assert.Equal(t, &v1alpha1.PodInfo{}, podInfo)
}

func TestInspectLogRequestsTailOfPodLog(t *testing.T) {
	var gotPath, gotTail string

	clientset := newLogClientset(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTail = r.URL.Query().Get("tailLines")
		_, _ = w.Write([]byte(shutdownMetricsLog))
	})

	inspectLog(t.Context(), clientset, newLogTestPod(), &v1alpha1.PodInfo{}, newLogTestObsConf(), nil)

	assert.Equal(t, "/api/v1/namespaces/my-ns/pods/my-pod/log", gotPath)
	assert.Equal(t, "128", gotTail)
}

func TestInspectLogAwryContent(t *testing.T) {
	tests := map[string]string{
		"log lines around the metrics": "2026-04-28 11:08:27.574  WARN 247539 --- [           main] crometer.prometheus.MicrometerPrometheus : " +
			"Micrometer service is stopping, here a list of metrics collected so far.\n" +
			shutdownMetricsLog +
			"2026-04-28 11:08:27.900  INFO 247539 --- [           main] org.apache.camel.main.MainSupport : Apache Camel stopped\n",
		"log lines in the middle of the metrics": strings.Replace(shutdownMetricsLog, "\n", "\nnot a metric line\n", 3),
		"marker without end":                     shutdownMetricsLog + logLinePrefix + logMetricStart + "camel_exchanges_total{routeId=\"rou\n",
		"empty metric lines":                     shutdownMetricsLog + logLinePrefix + logMetricStart + logMetricEnd + "\n",
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			clientset := newLogClientset(t, logHandler(content))
			podInfo := &v1alpha1.PodInfo{}

			inspectLog(t.Context(), clientset, newLogTestPod(), podInfo, newLogTestObsConf(), ptr.To("500"))

			// Only the content between the markers is scraped
			require.NotNil(t, podInfo.Runtime)
			require.NotNil(t, podInfo.Runtime.Exchange)
			assert.Equal(t, 3, podInfo.Runtime.Exchange.Total)
			assert.Equal(t, int64(950), *podInfo.JVMMemoryUsed)
		})
	}
}

func TestInspectLogRawPrometheusWithoutMarkers(t *testing.T) {
	// Content which is not wrapped by the markers is not considered
	clientset := newLogClientset(t, logHandler(shutdownMetrics))
	podInfo := &v1alpha1.PodInfo{}

	inspectLog(t.Context(), clientset, newLogTestPod(), podInfo, newLogTestObsConf(), ptr.To("500"))

	assert.Equal(t, &v1alpha1.PodInfo{}, podInfo)
}

func TestExtractLogMetrics(t *testing.T) {
	logContent := `2026-04-28 11:08:27.574  WARN 247539 --- [           main] crometer.prometheus.MicrometerPrometheus : Micrometer service is stopping, here a list of metrics collected so far.
2026-04-28 11:08:27.608  WARN 247539 --- [           main] crometer.prometheus.MicrometerPrometheus : #METRIC-START## HELP app_info  #METRIC-END#
2026-04-28 11:08:27.608  WARN 247539 --- [           main] crometer.prometheus.MicrometerPrometheus : #METRIC-START## TYPE app_info gauge#METRIC-END#
2026-04-28 11:08:27.608  WARN 247539 --- [           main] crometer.prometheus.MicrometerPrometheus : #METRIC-START#app_info{camel_context="camel-1",camel_runtime_provider="Main",camel_runtime_version="4.21.0-SNAPSHOT",camel_version="4.21.0-SNAPSHOT"} 1#METRIC-END#
2026-04-28 11:08:27.611  WARN 247539 --- [           main] crometer.prometheus.MicrometerPrometheus : #METRIC-START## HELP jvm_memory_max_bytes The maximum amount of memory in bytes that can be used for memory management#METRIC-END#
`

	expected := `# HELP app_info
# TYPE app_info gauge
app_info{camel_context="camel-1",camel_runtime_provider="Main",camel_runtime_version="4.21.0-SNAPSHOT",camel_version="4.21.0-SNAPSHOT"} 1
# HELP jvm_memory_max_bytes The maximum amount of memory in bytes that can be used for memory management
`

	assert.Equal(t, expected, extractLogMetrics(logContent))
	assert.Empty(t, extractLogMetrics("just a log line\nanother one\n"))
	assert.Empty(t, extractLogMetrics(""))
}
