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
	"strconv"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/kubernetes"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

// addPrometheusPodMonitor will include a Prometeus PodMonitor resource bound to the CamelMonitor resource.
func addPrometheusPodMonitor(ctx context.Context, c client.Client, target *v1alpha1.CamelMonitor,
	matchLabelSelector map[string]string) error {
	// Verify the existence of the Prometheus metrics endpoint
	if target.Status.DoesExposeMetrics() {
		// We assume all Pods expose the same port and metrics endpoint configuration
		metricsEndpoint := target.Status.Pods[0].ObservabilityService.MetricsEndpoint
		metricsPortNumber := target.Status.Pods[0].ObservabilityService.MetricsPort
		references := target.GetOwnerReferences()
		podMonitor := monitoringv1.PodMonitor{
			TypeMeta: metav1.TypeMeta{
				Kind:       "PodMonitor",
				APIVersion: monitoringv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            target.GetName(),
				Namespace:       target.GetNamespace(),
				OwnerReferences: references,
				Labels:          platform.GetPrometheusLabels(),
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: matchLabelSelector,
				},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						// NOTE: we must add a relabel configuration as we are not sure the
						// Pod is publicly exposing the prometheus metrics (it is probably not).
						// With the relabeling, we're making sure the Prometheus instance can
						// access to the metrics endpoint, rewriting the Pod IP address.
						Path: metricsEndpoint,
						RelabelConfigs: []monitoringv1.RelabelConfig{
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_pod_ip"},
								TargetLabel:  "__address__",
								Replacement:  new("${1}:" + strconv.Itoa(metricsPortNumber)),
							},
						},
					},
				},
			},
		}

		err := replacePodMonitor(ctx, c, &podMonitor)
		addCamelMonitorPrometheusCondition(target, err)

		return err
	}

	return nil
}

func addCamelMonitorPrometheusCondition(target *v1alpha1.CamelMonitor, err error) {
	statusCond := metav1.ConditionTrue
	message := "Created a PodMonitor with the same name of this CamelMonitor"

	if err != nil {
		statusCond = metav1.ConditionFalse
		message = "Some error happened while creating PodMonitor: " + err.Error()
	}

	target.Status.AddCondition(metav1.Condition{
		Type:               "PrometheusPodMonitor",
		Status:             statusCond,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "PodMonitorAdded",
		Message:            message,
	})
}

func replacePodMonitor(ctx context.Context, c client.Client, pm *monitoringv1.PodMonitor) error {
	existing := &monitoringv1.PodMonitor{}

	err := c.Get(ctx, ctrl.ObjectKey{
		Name:      pm.Name,
		Namespace: pm.Namespace,
	}, existing)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return c.Create(ctx, pm)
		}

		return err
	}

	pm.ResourceVersion = existing.ResourceVersion

	return c.Update(ctx, pm)
}

func prometheusCRDExists(c client.Client) (bool, error) {
	return kubernetes.IsAPIResourceInstalled(
		c,
		"monitoring.coreos.com/v1",
		"Prometheus",
	)

}
