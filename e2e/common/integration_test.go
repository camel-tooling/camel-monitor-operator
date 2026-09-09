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
	"testing"
	"time"

	. "github.com/camel-tooling/camel-monitor-operator/e2e/support"
	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	. "github.com/onsi/gomega"

	. "github.com/onsi/gomega/gstruct"
)

func TestVerifyCamelKIntegrationCountMessages(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		// Test a simple route which sends 5 messages to log
		t.Run("5 messages it", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					"apply",
					"-f",
					"files/sample-it.yaml",
					"-n",
					ns,
				),
			)

			// The first time the number of messages is 5
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "sample-it"),
				TestTimeoutMedium,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase": Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Pods": And(
						HaveLen(1),
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Status": Equal("Running"),
								"Runtime": PointTo(MatchFields(IgnoreExtras, Fields{
									"Exchange": PointTo(MatchFields(IgnoreExtras, Fields{
										"Succeeded": Equal(5),
										"Total":     Equal(5),
									})),
								})),
							}),
						),
					),
				}),
			)
			// With this integration the number of exchanges has to stick to 5 consistently
			g.Consistently(
				CamelMonitorStatus(t, ctx, ns, "sample-it"),
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase": Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Pods": And(
						HaveLen(1),
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Status": Equal("Running"),
								"Runtime": PointTo(MatchFields(IgnoreExtras, Fields{
									"Exchange": PointTo(MatchFields(IgnoreExtras, Fields{
										"Succeeded": Equal(5),
										"Total":     Equal(5),
									})),
								})),
							}),
						),
					),
				}),
			)
		})
	})
}

func TestVerifyCamelKIntegrationTimerToLog(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		// Test a simple route with indefinite message delivery (all success)
		t.Run("all success messages", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					"apply",
					"-f",
					"files/timer-to-log.yaml",
					"-n",
					ns,
				),
			)

			// Verify health status is UP
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "timer-to-log"),
				TestTimeoutMedium,
			).Should(
				WithTransform(
					func(s v1alpha1.CamelMonitorStatus) bool {
						return isCamelMonitorHealthStatusUP(s)
					},
					BeTrue(),
				),
			)
			// We check the success rate is not reporting weird results
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "timer-to-log"),
				TestTimeoutMedium,
			).Should(
				WithTransform(
					func(s v1alpha1.CamelMonitorStatus) bool {
						return isCamelMonitorMetricsHealthy(s)
					},
					BeTrue(),
				),
			)
		})
	})
}

func TestVerifyQuarkusDefaultAlternativeEndpoints(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		// Test a simple route with indefinite message delivery (all success)
		t.Run("all success messages", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					"apply",
					"-f",
					"files/timer-to-log-default-quarkus.yaml",
					"-n",
					ns,
				),
			)

			// Verify health status is UP (on /q/health)
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "timer-to-log-default-quarkus"),
				TestTimeoutMedium,
			).Should(
				WithTransform(
					func(s v1alpha1.CamelMonitorStatus) bool {
						isUp := isCamelMonitorHealthStatusUP(s)
						quarkusHealthEndpoint := len(s.Pods) > 0 && s.Pods[0].ObservabilityService.HealthEndpoint == "q/health"

						return isUp && quarkusHealthEndpoint
					},
					BeTrue(),
				),
			)
			// We check the /q/metrics are read fine
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "timer-to-log-default-quarkus"),
				TestTimeoutMedium,
			).Should(
				WithTransform(
					func(s v1alpha1.CamelMonitorStatus) bool {
						isHealthy := isCamelMonitorMetricsHealthy(s)
						quarkusMetricsEndpoint := len(s.Pods) > 0 && s.Pods[0].ObservabilityService.MetricsEndpoint == "q/metrics"

						return isHealthy && quarkusMetricsEndpoint
					},
					BeTrue(),
				),
			)
		})
	})
}

func isCamelMonitorMetricsHealthy(s v1alpha1.CamelMonitorStatus) bool {
	sr := s.SuccessRate
	if sr == nil {
		return false
	}

	// We need tolerance as the check may be slightly higher than 1 minute,
	// ie, time to reconcile.
	tolerance := *sr.SamplingIntervalDuration / 10
	elapsed := time.Since(sr.LastTimestamp.Time)
	if elapsed >= *sr.SamplingIntervalDuration+tolerance {
		return false
	}

	if sr.SamplingIntervalFailed != 0 {
		return false
	}
	if sr.SamplingIntervalTotal <= 0 {
		return false
	}
	if sr.Status != "Success" {
		return false
	}
	if sr.SuccessPercentage != "100.00" {
		return false
	}

	return true
}

func isCamelMonitorHealthStatusUP(s v1alpha1.CamelMonitorStatus) bool {
	if len(s.Pods) == 0 {
		return false
	}

	if s.Pods[0].Runtime.Status != "UP" {
		return false
	}

	return true
}
