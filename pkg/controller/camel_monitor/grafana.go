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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/kubernetes"
	integreatlyv1beta1 "github.com/grafana/grafana-operator/v5/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

func grafanaCRDExists(c client.Client) (bool, error) {
	_, err := c.Discovery().ServerResourcesForGroupVersion("grafana.integreatly.org/v1beta1")
	if err != nil && k8serrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// addGrafanaDashboard will include a GrafanaDashboard resource bound to the CamelMonitor resource.
func addGrafanaDashboard(ctx context.Context, c client.Client, target *v1alpha1.CamelMonitor, limits corev1.ResourceList) error {
	// Verify the existence of the Prometheus metrics endpoint
	if target.Status.DoesExposeMetrics() {
		references := target.GetOwnerReferences()

		dashboardJson, err := buildGrafanaDashboardJSON(target, limits)
		if err != nil {
			return err
		}

		dashboard := &integreatlyv1beta1.GrafanaDashboard{
			TypeMeta: metav1.TypeMeta{
				Kind:       "GrafanaDashboard",
				APIVersion: integreatlyv1beta1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            target.GetName(),
				Namespace:       target.GetNamespace(),
				OwnerReferences: references,
			},
			Spec: integreatlyv1beta1.GrafanaDashboardSpec{
				GrafanaCommonSpec: integreatlyv1beta1.GrafanaCommonSpec{
					AllowCrossNamespaceImport: true,
					InstanceSelector:          &metav1.LabelSelector{MatchLabels: platform.GetGrafanaLabels()},
				},
				GrafanaContentSpec: integreatlyv1beta1.GrafanaContentSpec{
					JSON: dashboardJson,
				},
				FolderTitle: "camel-dashboard",
			},
		}

		err = replaceGrafanaDashboard(ctx, c, dashboard)
		addCamelMonitorGrafanaCondition(target, err)

		return err
	}

	return nil
}

func replaceGrafanaDashboard(ctx context.Context, c client.Client, dashboard *integreatlyv1beta1.GrafanaDashboard) error {
	existing := &integreatlyv1beta1.GrafanaDashboard{}

	err := c.Get(ctx, ctrl.ObjectKey{
		Name:      dashboard.Name,
		Namespace: dashboard.Namespace,
	}, existing)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return c.Create(ctx, dashboard)
		}

		return err
	}

	dashboard.ResourceVersion = existing.ResourceVersion

	return c.Update(ctx, dashboard)
}

func addCamelMonitorGrafanaCondition(target *v1alpha1.CamelMonitor, err error) {
	statusCond := metav1.ConditionTrue
	message := "Created a GrafanaDashboard with the same name of this CamelMonitor"

	if err != nil {
		statusCond = metav1.ConditionFalse
		message = "Some error happened while creating GrafanaDashboard: " + err.Error()
	}

	target.Status.AddCondition(metav1.Condition{
		Type:               "GrafanaDashboard",
		Status:             statusCond,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "GrafanaDashboardAdded",
		Message:            message,
	})
}

// buildGrafanaDashboardJSON is in charge to generate a JSON configuration of the dashboard.
func buildGrafanaDashboardJSON(target *v1alpha1.CamelMonitor, limits corev1.ResourceList) (string, error) {
	dashboard := v1alpha1.Dashboard{
		Title: "Camel exchange metrics: " + target.GetName(),
		Panels: []v1alpha1.Panel{
			getTimeSeriesPanel(v1alpha1.Metric_camel_exchanges_total, target.GetNamespace(), target.GetName(), "route", "5m",
				v1alpha1.GridPos{H: 9, W: 11, X: 11, Y: 0}),
			getTimeSeriesPanel(v1alpha1.Metric_camel_exchanges_failed_total, target.GetNamespace(), target.GetName(), "route", "5m",
				v1alpha1.GridPos{H: 11, W: 11, X: 11, Y: 9}),
			getLastExchangeGaugePanel(target.GetNamespace(), target.GetName(),
				v1alpha1.GridPos{H: 8, W: 11, X: 0, Y: 0}),
			getCPUUsagePanel(target.GetNamespace(), target.GetName(), kubernetes.GetResourcesLimitInMillis(limits, corev1.ResourceCPU),
				v1alpha1.GridPos{H: 6, W: 11, X: 0, Y: 8}),
			getJVMMemoryUsagePanel(target.GetNamespace(), target.GetName(), kubernetes.GetResourcesLimitInMillis(limits, corev1.ResourceMemory),
				v1alpha1.GridPos{H: 6, W: 11, X: 0, Y: 14}),
		},
		SchemaVersion: 36,
		Version:       1,
	}

	bytes, err := json.Marshal(dashboard)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func getTimeSeriesPanel(metric, jobNamespace, jobName, eventType, sample string, pos v1alpha1.GridPos) v1alpha1.Panel {
	panelTitle, panelExpressions := getRateExpressions(metric, jobNamespace, jobName, eventType, sample)

	panel := v1alpha1.Panel{
		Datasource: platform.GetGrafanaDatasource(),
		Type:       "timeseries",
		Title:      panelTitle,
		Targets:    make([]v1alpha1.Target, 0, len(panelExpressions)),
		GridPos:    pos,
	}
	for _, expr := range panelExpressions {
		panel.Targets = append(panel.Targets, v1alpha1.Target{Expr: expr})
	}

	return panel
}

func getLastExchangeGaugePanel(jobNamespace, jobName string, pos v1alpha1.GridPos) v1alpha1.Panel {
	panel := v1alpha1.Panel{
		Datasource: platform.GetGrafanaDatasource(),
		Type:       "gauge",
		Title:      "Last exchange delay (in seconds)",
		Targets: []v1alpha1.Target{
			{
				Expr: fmt.Sprintf(`time() - (%s{job="%s/%s"} / 1000)`,
					v1alpha1.Metric_camel_exchanges_last_timestamp, jobNamespace, jobName),
				LegendFormat: "{{pod}}",
			},
		},
		GridPos: pos,
	}
	panel.FieldConfig = getFieldConfigWithThresholds(float64(platform.GetMaxIdle()), "seconds", "")

	return panel
}

func getCPUUsagePanel(jobNamespace, jobName string, maxValue float64, pos v1alpha1.GridPos) v1alpha1.Panel {
	panel := v1alpha1.Panel{
		Datasource: platform.GetGrafanaDatasource(),
		Type:       "timeseries",
		Title:      "CPU usage (in millicores)",
		Targets: []v1alpha1.Target{
			{
				Expr: fmt.Sprintf(`avg(system_cpu_usage{job="%s/%s"} * 1000) by (pod)`, jobNamespace, jobName),
			},
		},
		GridPos: pos,
	}
	if maxValue > 0 {
		panel.FieldConfig = getFieldConfigWithThresholds(maxValue, "millicores", "dashed+area")
	}

	return panel
}

func getFieldConfigWithThresholds(maxValue float64, unit, thresholdStyleMode string) v1alpha1.FieldConfig {
	// TODO: we could make these as parameters
	warnThreshold := maxValue * .8
	errThreshold := maxValue * .9

	fc := v1alpha1.FieldConfig{
		Defaults: v1alpha1.FieldDefaults{
			Unit: unit,
			Min:  0,
			Max:  maxValue,
			Thresholds: &v1alpha1.Thresholds{
				Mode: "absolute",
				Steps: []v1alpha1.ThresholdStep{
					{Color: "green", Value: nil},
					{Color: "yellow", Value: new(warnThreshold)},
					{Color: "red", Value: new(errThreshold)},
				},
			},
		},
	}
	if thresholdStyleMode != "" {
		fc.Defaults.Custom = &v1alpha1.CustomOptions{
			ThresholdsStyle: &v1alpha1.ThresholdsStyle{
				Mode: thresholdStyleMode,
			},
		}
	}

	return fc
}

func getJVMMemoryUsagePanel(jobNamespace, jobName string, maxValue float64, pos v1alpha1.GridPos) v1alpha1.Panel {
	panel := v1alpha1.Panel{
		Datasource: platform.GetGrafanaDatasource(),
		Type:       "timeseries",
		Title:      "JVM Heap memory (in Mi)",
		Targets: []v1alpha1.Target{
			{
				Expr: fmt.Sprintf(`avg(jvm_memory_used_bytes{area="heap", job="%s/%s"} / 1024 / 1024) by (pod)`, jobNamespace, jobName),
			},
		},
		GridPos: pos,
	}

	if maxValue > 0 {
		// the max value is expressed in millis
		panel.FieldConfig = getFieldConfigWithThresholds(maxValue/1024/1024/1000, "Mi", "dashed+area")
	}

	return panel
}

// getRateExpressions return an array of expressions with the format expected for a rate count grouped and by pod.
func getRateExpressions(metric, jobNamespace, jobName, eventType, sample string) (string, []string) {
	metricTitle := strings.ReplaceAll(metric, "_", " ") + " per second"

	return metricTitle, []string{
		fmt.Sprintf("sum(rate(%s{job=\"%s/%s\", eventType=\"%s\"}[%s]))", metric, jobNamespace, jobName, eventType, sample),
		fmt.Sprintf("sum(rate(%s{job=\"%s/%s\", eventType=\"%s\"}[%s])) by (pod)", metric, jobNamespace, jobName, eventType, sample),
	}
}
