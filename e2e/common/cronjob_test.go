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
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/camel-tooling/camel-monitor-operator/e2e/support"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// cronJobManifest runs the Camel application every minute. The Job is stopped after a short deadline, so that the
// Pod terminates (printing the metrics on shutdown) and the operator can recover them from the Pod log.
const cronJobManifest = `
apiVersion: batch/v1
kind: CronJob
metadata:
  name: camel-app
spec:
  schedule: "* * * * *"
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 0
      activeDeadlineSeconds: 45
      template:
        metadata:
          labels:
            app: camel-app
        spec:
          restartPolicy: Never
          containers:
            - name: camel-app
              image: %s
`

func TestVerifyCronJobQuarkus(t *testing.T) {
	testVerifyCronJob(t, CamelAppQuarkus(), "Quarkus")
}

func TestVerifyCronJobSpringBoot(t *testing.T) {
	testVerifyCronJob(t, CamelAppSpringBoot(), "Spring-Boot")
}

func TestVerifyCronJobMain(t *testing.T) {
	testVerifyCronJob(t, CamelAppMain(), "Main")
}

func testVerifyCronJob(t *testing.T, image string, expectedRuntimeProvider string) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		t.Run("simple CronJob", func(t *testing.T) {
			applyCmd := exec.Command("kubectl", "apply", "-f", "-", "-n", ns)
			applyCmd.Stdin = strings.NewReader(fmt.Sprintf(cronJobManifest, image))
			ExpectExecSucceed(t, g, applyCmd)

			// As there is no label, there is not yet any CamelMonitor CR
			g.Consistently(CamelMonitors(t, ctx, ns), TestTimeoutShort, 10*time.Second).Should(BeEmpty())

			// Add the labels to discover it
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label cronjob camel-app camel.apache.org/monitor=camel-sample -n "+ns, " ")...,
				),
			)
			// The name of the selector, "camel.apache.org/monitor: camel-sample"
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample")).Should(Not(BeNil()))

			// The CronJob is monitored and healthy
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample"),
				TestTimeoutLong,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Conditions": And(
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
						ContainElement(
							MatchFields(IgnoreExtras, Fields{
								"Type":   Equal("CronJobExecution"),
								"Status": Equal(metav1.ConditionTrue),
							}),
						),
					),
				}),
			)

			// The metrics printed on shutdown are recovered from the Pod log: we check the runtime,
			// the exchanges and the information required to calculate the memory and cpu pressures.
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample"),
				TestTimeoutLong,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Pods": ContainElement(
						MatchFields(IgnoreExtras, Fields{
							"Runtime": PointTo(MatchFields(IgnoreExtras, Fields{
								"RuntimeProvider": Equal(expectedRuntimeProvider),
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

			// Delete the CronJob
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("delete cronjob camel-app -n "+ns, " ")...,
				),
			)
			// No CamelMonitors around (garbage collected)
			g.Eventually(CamelMonitors(t, ctx, ns)).Should(BeEmpty())
		})
	})
}
