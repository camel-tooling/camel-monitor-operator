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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	v1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/kubernetes"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/log"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

// getPods returns the pods backing the Camel application. You can provide an inspect flag to scrape health and metrics.
func getPods(httpClient http.Client, ctx context.Context, c client.Client, namespace string,
	matchLabelsSelector map[string]string, observabilityPort int, inspect bool, cpuLimit *string) ([]v1alpha1.PodInfo, error) {
	var podsInfo []v1alpha1.PodInfo
	pods := &corev1.PodList{}
	err := c.List(ctx, pods,
		ctrl.InNamespace(namespace),
		ctrl.MatchingLabels(matchLabelsSelector),
	)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		readyCondition := kubernetes.GetPodCondition(pod, corev1.PodReady)
		isPodReady := readyCondition != nil && readyCondition.Status == corev1.ConditionTrue
		podIp := pod.Status.PodIP
		podInfo := v1alpha1.PodInfo{
			Name:           pod.GetName(),
			Status:         string(pod.Status.Phase),
			Ready:          isPodReady,
			InternalIP:     podIp,
			JolokiaEnabled: kubernetes.JolokiaEnabled(pod),
		}
		if readyCondition != nil {
			podInfo.UptimeTimestamp = &metav1.Time{Time: readyCondition.LastTransitionTime.Time}
		}

		if isPodReady && inspect {
			inspectPod(httpClient, &pod, &podInfo, podIp, observabilityPort, cpuLimit)
		}

		podsInfo = append(podsInfo, podInfo)
	}

	return podsInfo, nil
}

// inspectPod scan a ready Pod and scrape health and metrics which it stores on podInfo resource.
func inspectPod(httpClient http.Client, pod *corev1.Pod, podInfo *v1alpha1.PodInfo, podIp string, observabilityPort int, cpuLimit *string) {
	podInfo.ObservabilityService = &v1alpha1.ObservabilityServiceInfo{}
	if err := setHealth(podInfo, podIp, observabilityPort); err != nil {
		reason := fmt.Sprintf("Could not scrape health endpoint: %s", err.Error())
		log.Infof("Pod %s/%s: %s", pod.GetNamespace(), pod.GetName(), reason)
		podInfo.Reason = reason
	}
	if err := setMetrics(httpClient, podInfo, podIp, observabilityPort); err != nil {
		reason := fmt.Sprintf("Could not scrape metrics endpoint: %s", err.Error())
		log.Infof("Pod %s/%s: %s", pod.GetNamespace(), pod.GetName(), reason)
		if podInfo.Reason != "" {
			podInfo.Reason += ". "
		}
		podInfo.Reason += reason
	}
	if err := setCPUPressure(podInfo, cpuLimit); err != nil {
		log.Error(err, "Could not parse cpu usage/max value, skipping")
	}
}

func setCPUPressure(podInfo *v1alpha1.PodInfo, cpuLimit *string) error {
	if cpuLimit != nil && *cpuLimit != "" {
		podInfo.ProcessCPUMax = cpuLimit
		// At this stage we should have already the cpu usage metric collected (if it exists)
		// therefore we can calculate the cpu pressure flag
		if podInfo.ProcessCPUUsed != nil {
			val, err1 := strconv.ParseFloat(*podInfo.ProcessCPUUsed, 64)
			if err1 != nil {
				return err1
			}
			max, err2 := strconv.ParseFloat(*podInfo.ProcessCPUMax, 64)
			if err2 != nil {
				return err2
			}
			cpuPerc := val / max * 100
			if cpuPerc >= 90 {
				podInfo.HasCPUPressure = true
			}
		}
	}

	return nil
}

func getObservabilityPort(appAnnotations map[string]string) int {
	defaultPort := platform.GetObservabilityPort()
	if appAnnotations == nil || appAnnotations[v1alpha1.MonitorObservabilityServicesPort] == "" {
		return defaultPort
	}

	port, err := strconv.Atoi(appAnnotations[v1alpha1.MonitorObservabilityServicesPort])
	if err == nil {
		return port
	} else {
		log.Error(err, "could not properly parse application observability services port configuration, "+
			"fallback to default operator value %d", defaultPort)
	}

	return defaultPort
}

func setMetrics(httpClient http.Client, podInfo *v1alpha1.PodInfo, podIp string, port int) error {
	// NOTE: we're not using a proxy as a design choice in order
	// to have a faster turnaround.
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/%s", podIp, port, platform.DefaultObservabilityMetrics), nil)
	if err != nil {
		return err
	}
	// Quarkus runtime specific, see https://github.com/apache/camel-quarkus/issues/7405
	req.Header.Add("Accept", "text/plain, */*")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		podInfo.ObservabilityService.MetricsEndpoint = platform.DefaultObservabilityMetrics
		podInfo.ObservabilityService.MetricsPort = port

		if podInfo.Runtime == nil {
			podInfo.Runtime = &v1alpha1.RuntimeInfo{}
		}
		if podInfo.Runtime.Exchange == nil {
			podInfo.Runtime.Exchange = &v1alpha1.ExchangeInfo{}
		}

		metrics, err := parseMetrics(resp.Body)
		if err != nil {
			return err
		}
		if metric, ok := metrics[v1alpha1.Metric_app_info]; ok {
			populateRuntimeInfo(metric, v1alpha1.Metric_app_info, podInfo)
		}

		podInfo.Runtime.Exchange.Total = int((ptr.Deref(getCounter(metrics, v1alpha1.Metric_camel_exchanges_total), 0)))
		podInfo.Runtime.Exchange.Failed = int((ptr.Deref(getCounter(metrics, v1alpha1.Metric_camel_exchanges_failed_total), 0)))
		podInfo.Runtime.Exchange.Succeeded = int((ptr.Deref(getCounter(metrics, v1alpha1.Metric_camel_exchanges_succeeded_total), 0)))
		// Note: camel is reporting this as a gauge
		podInfo.Runtime.Exchange.Pending = int((ptr.Deref(getGauge(metrics, v1alpha1.Metric_camel_exchanges_inflight), 0)))

		exchangeLastTimestamp := getGauge(metrics, v1alpha1.Metric_camel_exchanges_last_timestamp)
		if exchangeLastTimestamp != nil {
			timeUnixMilli := time.UnixMilli(int64(math.Round(*exchangeLastTimestamp)))
			podInfo.Runtime.Exchange.LastTimestamp = &metav1.Time{Time: timeUnixMilli}
		}

		processFloatVal := getGauge(metrics, v1alpha1.Metric_system_cpu_usage)
		if processFloatVal != nil {
			// values is expressed in cores in Prometheus, whilst we want millicores
			podInfo.ProcessCPUUsed = ptr.To(strconv.FormatFloat(*processFloatVal*1000, 'f', 0, 64))
		}

		podInfo.JVMMemoryUsed = ptr.To(int64(*getGaugeWithLabel(metrics, v1alpha1.Metric_jvm_memory_used, "area", "heap")))
		podInfo.JVMMemoryMax = ptr.To(int64(*getGaugeWithLabel(metrics, v1alpha1.Metric_jvm_memory_max, "area", "heap")))
		if podInfo.JVMMemoryUsed != nil && podInfo.JVMMemoryMax != nil && *podInfo.JVMMemoryMax > 0 {
			memoryPercentage := float64(*podInfo.JVMMemoryUsed) / float64(*podInfo.JVMMemoryMax) * 100
			if memoryPercentage >= 90 {
				podInfo.HasMemoryPressure = true
			}
		}

		return nil
	}

	return fmt.Errorf("HTTP status not OK, it was %d", resp.StatusCode)
}

func parseMetrics(reader io.Reader) (map[string]*dto.MetricFamily, error) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	mf, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}

	return mf, nil
}

func populateRuntimeInfo(metric *dto.MetricFamily, metricName string, podInfo *v1alpha1.PodInfo) {
	if len(metric.GetMetric()) != 1 {
		log.Infof("WARN: expected exactly one %s metric, got %d", metricName, len(metric.GetMetric()))
		return
	}

	for _, label := range metric.GetMetric()[0].GetLabel() {
		switch ptr.Deref(label.Name, "") {
		case "camel_runtime_provider":
			podInfo.Runtime.RuntimeProvider = ptr.Deref(label.Value, "")
		case "camel_runtime_version":
			podInfo.Runtime.RuntimeVersion = ptr.Deref(label.Value, "")
		case "camel_version":
			podInfo.Runtime.CamelVersion = ptr.Deref(label.Value, "")
		}
	}
}

func getCounter(metrics map[string]*dto.MetricFamily, metricName string) *float64 {
	if metric, ok := metrics[metricName]; ok {
		if len(metric.GetMetric()) == 0 {
			log.Debugf("expected at least 1 %s metric, got %d", metricName, len(metric.GetMetric()))
			return nil
		}
		if metric.GetMetric()[0].GetCounter() == nil {
			log.Debugf("expected %s metric to be a counter", metricName)
			return nil
		}

		return metric.GetMetric()[0].GetCounter().Value
	}

	return nil
}

func getGauge(metrics map[string]*dto.MetricFamily, metricName string) *float64 {
	return getGaugeInternal(metrics, metricName, "", "")
}

// getGaugeWithLabel filter the gauge with the label provided.
func getGaugeWithLabel(metrics map[string]*dto.MetricFamily, metricName, labelName, labelValue string) *float64 {
	return getGaugeInternal(metrics, metricName, labelName, labelValue)
}

func getGaugeInternal(metrics map[string]*dto.MetricFamily, metricName, labelName, labelValue string) *float64 {
	var total float64
	if metric, ok := metrics[metricName]; ok {
		if len(metric.GetMetric()) == 0 {
			log.Debugf("expected at least 1 %s metric, got %d", metricName, len(metric.GetMetric()))
			return nil
		}
		for _, g := range metric.GetMetric() {
			if g.GetGauge() != nil && accept(g.Label, labelName, labelValue) {
				total += *g.GetGauge().Value
			}
		}
	}

	return &total
}

// accept filters the labels and return true if there is no filter (labelName=="") or it
// matches any label provided.
func accept(labelPair []*dto.LabelPair, labelName, labelValue string) bool {
	if labelName == "" {
		return true
	}
	for _, lp := range labelPair {
		if *lp.Name == labelName && *lp.Value == labelValue {
			return true
		}
	}

	return false
}

func setHealth(podInfo *v1alpha1.PodInfo, podIp string, port int) error {
	// NOTE: we're not using a proxy as a design choice in order
	// to have a faster turnaround.
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/%s", podIp, port, platform.DefaultObservabilityHealth))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	status := resp.Status
	// The endpoint reports 503 when the service is down, but still provide the
	// health information
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusServiceUnavailable {
		podInfo.ObservabilityService.HealthPort = port
		podInfo.ObservabilityService.HealthEndpoint = platform.DefaultObservabilityHealth

		status, err = parseHealthStatus(resp.Body)
		if err != nil {
			return err
		}
	}
	if podInfo.Runtime == nil {
		podInfo.Runtime = &v1alpha1.RuntimeInfo{
			Status: status,
		}
	}

	return nil
}

func parseHealthStatus(reader io.Reader) (string, error) {
	var healthContent map[string]any
	err := json.NewDecoder(reader).Decode(&healthContent)
	if err != nil {
		return "", err
	}
	status, ok := healthContent["status"].(string)
	if !ok {
		return "", errors.New("health endpoint syntax error: missing .status property")
	}

	return string(status), nil
}

func setMonitoringCondition(app, targetApp *v1alpha1.CamelMonitor, pods []v1alpha1.PodInfo) {
	if len(pods) == 0 {
		targetApp.Status.AddCondition(metav1.Condition{
			Type:               "Monitored",
			Status:             metav1.ConditionUnknown,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "MonitoringComplete",
			Message:            "No active Pod available",
		})
		targetApp.Status.AddCondition(metav1.Condition{
			Type:               "Healthy",
			Status:             metav1.ConditionUnknown,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "HealthCheckCompleted",
			Message:            "No active Pod available",
		})

		return
	}

	monitoredMessage := "Some Pod is not ready. See specific Pods status messages"
	monitoredCondition := metav1.ConditionFalse

	if allPodsReady(pods) {
		monitoredCondition = metav1.ConditionTrue
		monitoredMessage = "Success"
		if app.Status.Replicas != nil && len(pods) != int(*app.Status.Replicas) {
			monitoredMessage = fmt.Sprintf("%d out of %d pods available", len(pods), int(*app.Status.Replicas))
		}
	}
	targetApp.Status.AddCondition(metav1.Condition{
		Type:               "Monitored",
		Status:             monitoredCondition,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "MonitoringComplete",
		Message:            monitoredMessage,
	})

	healthMessage := "Some Pod is not ready. See specific Pods status messages"
	healthCondition := metav1.ConditionFalse

	if allPodsUp(pods) {
		healthCondition = metav1.ConditionTrue
		healthMessage = "All Pods are reported as healthy"
	}
	targetApp.Status.AddCondition(metav1.Condition{
		Type:               "Healthy",
		Status:             healthCondition,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "HealthCheckCompleted",
		Message:            healthMessage,
	})

	memoryPressureCondition := metav1.ConditionFalse
	memoryPressureMessage := "No JVM memory pressure detected"
	if podMemoryPressure(pods) {
		memoryPressureCondition = metav1.ConditionTrue
		memoryPressureMessage = "At least one Pod has JVM memory pressure"
	}

	targetApp.Status.AddCondition(metav1.Condition{
		Type:               "MemoryPressure",
		Status:             memoryPressureCondition,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "JVMMemoryPressure",
		Message:            memoryPressureMessage,
	})

	cpuPressureCondition := metav1.ConditionFalse
	cpuPressureMessage := "No CPU pressure detected"
	if podCpuPressure(pods) {
		cpuPressureCondition = metav1.ConditionTrue
		cpuPressureMessage = "At least one Pod has JVM memory pressure"
	}

	targetApp.Status.AddCondition(metav1.Condition{
		Type:               "CPUPressure",
		Status:             cpuPressureCondition,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "CPUPressure",
		Message:            cpuPressureMessage,
	})
}

func podMemoryPressure(pods []v1alpha1.PodInfo) bool {
	for _, pod := range pods {
		if pod.HasMemoryPressure {
			return true
		}
	}

	return false
}

func podCpuPressure(pods []v1alpha1.PodInfo) bool {
	for _, pod := range pods {
		if pod.HasCPUPressure {
			return true
		}
	}

	return false
}

func allPodsReady(pods []v1alpha1.PodInfo) bool {
	for _, pod := range pods {
		if !pod.Ready {
			return false
		}
	}

	return true
}

func countPodsWithStatus(pods []v1alpha1.PodInfo, status string) int {
	podsCount := 0
	for _, pod := range pods {
		if status == pod.Status {
			podsCount++
		}
	}

	return podsCount
}

func allPodsUp(pods []v1alpha1.PodInfo) bool {
	for _, pod := range pods {
		if pod.Runtime == nil || pod.Runtime.Status != v1alpha1.PodStatusUP {
			return false
		}
	}

	return true
}
