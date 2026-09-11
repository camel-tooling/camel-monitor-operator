//go:build integration
// +build integration

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

package common

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/camel-tooling/camel-monitor-operator/e2e/support"
	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVerifyAltDeploymentQuarkus(t *testing.T) {
	testVerifyAltDeployment(t, CamelRegularAppQuarkus(), "8080", "q/health", "q/metrics")
}

func TestVerifyAltDeploymentSpringBoot(t *testing.T) {
	testVerifyAltDeployment(t, CamelRegularAppSpringBoot(), "8080", "actuator/health", "actuator/prometheus")
}

func TestVerifyAltDeploymentMain(t *testing.T) {
	testVerifyAltDeployment(t, CamelRegularAppMain(), "8080", "observe/health", "observe/metrics")
}

func testVerifyAltDeployment(t *testing.T, image string, expectedPort string, expectedHealthEndpoint string, expectedMetricsEndpoint string) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		t.Run("alternative Deployment", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app --image="+image+" -n "+ns, " ")...,
				),
			)
			g.Eventually(PodStatusPhase(t, ctx, ns, "app=camel-app"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))
			// As there is no label, there is not yet any CamelMonitor CR
			g.Consistently(CamelMonitors(t, ctx, ns), TestTimeoutShort, 10*time.Second).Should(BeEmpty())

			// Add the labels to discover it
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample -n "+ns, " ")...,
				),
			)
			// The name of the selector, "camel.apache.org/monitor: camel-sample"
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample")).Should(Not(BeNil()))
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample"),
				TestTimeoutMedium,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase":       Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Replicas":    PointTo(Equal(int32(1))),
					"SuccessRate": Not(BeNil()),
					"Conditions": And(
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type": Equal("UpgradeAvailable"),
								// Make sure that the check is not failing, it could be either true or false.
								"Status": Not(Equal(metav1.ConditionUnknown)),
							}),
						),
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type":   Equal("Monitored"),
								"Status": Equal(metav1.ConditionTrue),
							}),
						),
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type":   Equal("Healthy"),
								"Status": Equal(metav1.ConditionTrue),
							}),
						),
					),
				}),
			)

			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample"),
				TestTimeoutMedium,
			).Should(
				WithTransform(
					func(s v1alpha1.CamelMonitorStatus) bool {
						return len(s.Pods) > 0 && s.Pods[0].ObservabilityService != nil &&
							s.Pods[0].ObservabilityService.MetricsPort == expectedPort &&
							s.Pods[0].ObservabilityService.MetricsEndpoint == expectedMetricsEndpoint &&
							s.Pods[0].ObservabilityService.HealthEndpoint == expectedHealthEndpoint

					},
					BeTrue(),
				),
			)

			// Delete deployment
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("delete deployment camel-app -n "+ns, " ")...,
				),
			)
			// No CamelMonitors around (garbage collected)
			g.Eventually(CamelMonitors(t, ctx, ns)).Should(BeEmpty())
		})
	})
}
