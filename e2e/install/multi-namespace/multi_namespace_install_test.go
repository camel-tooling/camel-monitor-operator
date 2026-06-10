//go:build integration
// +build integration

// To enable compilation of this file in Goland, go to "Settings -> Go -> Vendoring & Build Tags -> Custom Tags" and add "integration"

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

package namespaced

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
)

/*
* The test will install the operator in the "operators" namespace and will monitor the applications
* deployed on "tenant-a" and "tenant-b" namespaces only. IMPORTANT: the operator and namespaces have to be created before
* running the test
 */
func TestSingleNamespaceInstallation(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		// Verify an app running in the outside namespace
		t.Run("simple Deployment (non-monitored)", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app --image="+CamelAppQuarkus()+" -n "+ns, " ")...,
				),
			)
			// Add the labels to discover it
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample -n "+ns, " ")...,
				),
			)
			g.Consistently(CamelMonitors(t, ctx, ns), TestTimeoutShort, 10*time.Second).Should(BeEmpty())
		})

		// Verify the app running in the tenant-a namespace, hence, we expect it to be monitored
		tenantANs := "tenant-a"
		t.Run("simple Deployment tenant a (monitored)", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app-tenant-a --image="+CamelAppQuarkus()+" -n "+tenantANs, " ")...,
				),
			)
			// Add the labels to discover it
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app-tenant-a camel.apache.org/monitor=camel-sample-a -n "+tenantANs, " ")...,
				),
			)
			// The name of the selector, "camel.apache.org/monitor: camel-sample-a"
			g.Eventually(CamelMonitor(t, ctx, tenantANs, "camel-sample-a")).Should(Not(BeNil()))
			g.Eventually(
				CamelMonitorStatus(t, ctx, tenantANs, "camel-sample-a"),
				TestTimeoutMedium,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase":       Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Replicas":    PointTo(Equal(int32(1))),
					"SuccessRate": Not(BeNil()),
				}),
			)
		})

		// Verify the app running in the tenant-b namespace, hence, we expect it to be monitored
		tenantBNs := "tenant-b"
		t.Run("simple Deployment tenant b (monitored)", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app-tenant-b --image="+CamelAppQuarkus()+" -n "+tenantBNs, " ")...,
				),
			)
			// Add the labels to discover it
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app-tenant-b camel.apache.org/monitor=camel-sample-b -n "+tenantBNs, " ")...,
				),
			)
			// The name of the selector, "camel.apache.org/monitor: camel-sample-b"
			g.Eventually(CamelMonitor(t, ctx, tenantBNs, "camel-sample-b")).Should(Not(BeNil()))
			g.Eventually(
				CamelMonitorStatus(t, ctx, tenantBNs, "camel-sample-b"),
				TestTimeoutMedium,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase":       Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Replicas":    PointTo(Equal(int32(1))),
					"SuccessRate": Not(BeNil()),
				}),
			)
		})

		t.Run("Clean monitored apps", func(t *testing.T) {
			// Delete deployment in tenant A
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("delete deployment camel-app-tenant-a -n "+tenantANs, " ")...,
				),
			)
			// Delete deployment in tenant B
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("delete deployment camel-app-tenant-b -n "+tenantBNs, " ")...,
				),
			)
			// No CamelMonitors around (garbage collected)
			g.Eventually(CamelMonitors(t, ctx, tenantANs)).Should(BeEmpty())
			g.Eventually(CamelMonitors(t, ctx, tenantBNs)).Should(BeEmpty())
		})
	})
}
