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

	. "github.com/camel-tooling/camel-monitor-operator/e2e/support"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega/gstruct"
)

func TestVerifyCamelKIntegrationCron(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		// Test a simple route which sends a messages to log
		t.Run("simple cron job", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					"apply",
					"-f",
					"files/sample-cron-it.yaml",
					"-n",
					ns,
				),
			)

			// We check the app is monitored and healthy
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "sample-cron-it"),
				TestTimeoutMedium,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Conditions": ContainElements(
						MatchFields(IgnoreExtras, Fields{
							"Type":   Equal("Monitored"),
							"Status": Equal(metav1.ConditionTrue),
							"Reason": Equal("MonitoringComplete"),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Type":   Equal("Healthy"),
							"Status": Equal(metav1.ConditionTrue),
							"Reason": Equal("HealthCheckCompleted"),
						}),
					),
				}),
			)

			// The metrics are recovered from the Pod log: we check the exchanges and the
			// information required to calculate the memory and cpu pressures are available
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "sample-cron-it"),
				TestTimeoutMedium,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Pods": ContainElement(
						MatchFields(IgnoreExtras, Fields{
							"Runtime": PointTo(MatchFields(IgnoreExtras, Fields{
								"Exchange": PointTo(MatchFields(IgnoreExtras, Fields{
									"Total":     BeNumerically(">", 0),
									"Succeeded": BeNumerically(">", 0),
								})),
							})),
							"JVMMemoryUsed":  PointTo(BeNumerically(">", 0)),
							"JVMMemoryMax":   PointTo(BeNumerically(">", 0)),
							"ProcessCPUUsed": PointTo(Not(BeEmpty())),
						}),
					),
				}),
			)
		})
	})
}
