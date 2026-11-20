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
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVerifyDeploymentQuarkus(t *testing.T) {
	testVerifyDeployment(t, CamelAppQuarkus())
}

func TestVerifyDeploymentSpringBoot(t *testing.T) {
	testVerifyDeployment(t, CamelAppSpringBoot())
}

func TestVerifyDeploymentMain(t *testing.T) {
	testVerifyDeployment(t, CamelAppMain())
}

func TestVerifyDeploymentLabelInLabelOut(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		t.Run("simple Deployment", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app --image="+CamelAppQuarkus()+" -n "+ns, " ")...,
				),
			)
			g.Eventually(PodStatusPhase(t, ctx, ns, "app=camel-app"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))
			// Label in: the operator should discover
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample -n "+ns, " ")...,
				),
			)
			// The name of the selector, "camel.apache.org/monitor: camel-sample"
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample")).Should(Not(BeNil()))
			// Label out: the operator has to remove the CR
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor- -n "+ns, " ")...,
				),
			)
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample")).Should(BeNil())
		})
	})
}

func TestVerifyDeploymentLabelValueChange(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		t.Run("simple Deployment", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app --image="+CamelAppQuarkus()+" -n "+ns, " ")...,
				),
			)
			g.Eventually(PodStatusPhase(t, ctx, ns, "app=camel-app"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))
			// Label in: the operator should discover
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample -n "+ns, " ")...,
				),
			)
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample")).Should(Not(BeNil()))
			// Change the label value: the CR named after the old value is replaced by one named after the new value
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample-renamed --overwrite -n "+ns, " ")...,
				),
			)
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample-renamed")).Should(Not(BeNil()))
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample")).Should(BeNil())
			g.Eventually(CamelMonitors(t, ctx, ns)).Should(HaveLen(1))
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample-renamed"),
				TestTimeoutMedium,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase":    Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Replicas": PointTo(Equal(int32(1))),
				}),
			)
			// Label out: the operator has to remove the CR
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor- -n "+ns, " ")...,
				),
			)
			g.Eventually(CamelMonitors(t, ctx, ns)).Should(BeEmpty())
		})
	})
}

func TestVerifyDeploymentNoMetrics(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		t.Run("no metrics Deployment", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app --image=busybox -n "+ns+" -- sleep 3600", " ")...,
				),
			)
			g.Eventually(PodStatusPhase(t, ctx, ns, "app=camel-app"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))
			// Label in: the operator should discover the deployment even without metrics or health endpoints
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample -n "+ns, " ")...,
				),
			)
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample")).Should(Not(BeNil()))
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample"),
				TestTimeoutMedium,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase":          Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Replicas":       PointTo(Equal(int32(1))),
					"RuntimeSummary": BeNil(),
					"SuccessRate":    BeNil(),
					"Conditions": And(
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type":    Equal(v1alpha1.ConditionTypeMonitored),
								"Status":  Equal(metav1.ConditionFalse),
								"Reason":  Equal("MonitoringError"),
								"Message": Equal("Could not scrape health or metrics endpoints. See specific Pods status messages"),
							}),
						),
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type":    Equal(v1alpha1.ConditionTypeHealthy),
								"Status":  Equal(metav1.ConditionFalse),
								"Reason":  Equal("HealthCheckCompleted"),
								"Message": Equal("Could not verify health status. See specific Pods status messages"),
							}),
						),
					),
				}),
			)
			// Scale to 0
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("scale deployment camel-app --replicas 0 -n "+ns, " ")...,
				),
			)
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample"),
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase":          Equal(v1alpha1.CamelMonitorPhasePaused),
					"Replicas":       PointTo(Equal(int32(0))),
					"RuntimeSummary": BeNil(),
					"SuccessRate":    BeNil(),
					"Conditions": And(
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type":    Equal(v1alpha1.ConditionTypeMonitored),
								"Status":  Equal(metav1.ConditionUnknown),
								"Message": Equal("No active Pod available"),
							}),
						),
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type":    Equal(v1alpha1.ConditionTypeHealthy),
								"Status":  Equal(metav1.ConditionUnknown),
								"Message": Equal("No active Pod available"),
							}),
						),
					),
				}),
			)
			// Label out: the operator has to remove the CR
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor- -n "+ns, " ")...,
				),
			)
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample")).Should(BeNil())
		})
	})
}

func testVerifyDeployment(t *testing.T, image string) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		t.Run("simple Deployment", func(t *testing.T) {
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
					"Phase":          Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Replicas":       PointTo(Equal(int32(1))),
					"RuntimeSummary": runtimeSummaryWithExchanges(),
					"SuccessRate":    Not(BeNil()),
					"Conditions": And(
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type":   Equal(v1alpha1.ConditionTypeMonitored),
								"Status": Equal(metav1.ConditionTrue),
							}),
						),
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type":   Equal(v1alpha1.ConditionTypeHealthy),
								"Status": Equal(metav1.ConditionTrue),
							}),
						),
					),
				}),
			)
			// Scale up
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("scale deployment camel-app --replicas 2 -n "+ns, " ")...,
				),
			)
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample"),
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase":          Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Replicas":       PointTo(Equal(int32(2))),
					"RuntimeSummary": runtimeSummaryWithExchanges(),
					"SuccessRate":    Not(BeNil()),
				}),
			)
			// Scale to 0
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("scale deployment camel-app --replicas 0 -n "+ns, " ")...,
				),
			)
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample"),
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase":          Equal(v1alpha1.CamelMonitorPhasePaused),
					"Replicas":       PointTo(Equal(int32(0))),
					"RuntimeSummary": runtimeSummaryWithExchanges(),
					"SuccessRate":    BeNil(),
				}),
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

// runtimeSummaryWithExchanges matches a runtime summary that has already processed at least one exchange.
func runtimeSummaryWithExchanges() types.GomegaMatcher {
	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Exchange": PointTo(MatchFields(IgnoreExtras, Fields{
			"Total": BeNumerically(">", 0),
		})),
	}))
}
