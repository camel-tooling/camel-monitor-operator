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

package olm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/camel-tooling/camel-monitor-operator/e2e/support"
	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
)

func TestOLMNamespacedInstallation(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		bundleImageName, ok := os.LookupEnv("BUNDLE_IMAGE_NAME")
		g.Expect(ok).To(BeTrue(), "Missing bundle image: you need to build and push to a container registry and set BUNDLE_IMAGE_NAME env var")
		os.Setenv("CAMEL_MONITOR_OPERATOR_TEST_MAKE_DIR", "../../../")
		// Install staged bundle (it must be available by building it before running the test)
		// You can build it locally via `make bundle-push` action
		ExpectExecSucceedWithTimeout(t, g,
			Make(t,
				"bundle-test",
				fmt.Sprintf("BUNDLE_IMAGE_NAME=%s", bundleImageName),
				fmt.Sprintf("NAMESPACE=%s", ns),
				fmt.Sprintf("OLM_INSTALL_MODE=%s", "OwnNamespace"),
			),
			"300s",
		)

		// Check the operator pod is running
		g.Eventually(PodStatusPhase(t, ctx, ns, "camel.apache.org/component=operator"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))

		// Verify an app running in the same namespace
		t.Run("simple Deployment (monitored)", func(t *testing.T) {
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
					strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample-monitored -n "+ns, " ")...,
				),
			)
			// The name of the selector, "camel.apache.org/monitor: camel-sample-monitored"
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample-monitored")).Should(Not(BeNil()))
			g.Eventually(
				CamelMonitorStatus(t, ctx, ns, "camel-sample-monitored"),
				TestTimeoutMedium,
			).Should(
				MatchFields(IgnoreExtras, Fields{
					"Phase":       Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Replicas":    PointTo(Equal(int32(1))),
					"SuccessRate": Not(BeNil()),
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

		// Verify the app running in another namespace is not monitored
		WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns2 string) {
			// Verify an app running in the same namespace
			t.Run("simple Deployment (non monitored)", func(t *testing.T) {
				ExpectExecSucceed(t, g,
					exec.Command(
						"kubectl",
						strings.Split("create deployment camel-app --image="+CamelAppQuarkus()+" -n "+ns2, " ")...,
					),
				)
				// Add the labels to discover it
				ExpectExecSucceed(t, g,
					exec.Command(
						"kubectl",
						strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample-non-monitored -n "+ns2, " ")...,
					),
				)
				// No CamelMonitors in this namespace
				g.Consistently(CamelMonitors(t, ctx, ns2), TestTimeoutShort, 10*time.Second).Should(BeEmpty())
			})
		})
	})
}

func TestOLMGlobalInstallation(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		bundleImageName, ok := os.LookupEnv("BUNDLE_IMAGE_NAME")
		g.Expect(ok).To(BeTrue(), "Missing bundle image: you need to build and push to a container registry and set BUNDLE_IMAGE_NAME env var")
		os.Setenv("CAMEL_MONITOR_OPERATOR_TEST_MAKE_DIR", "../../../")
		// Install staged bundle (it must be available by building it before running the test)
		// You can build it locally via `make bundle-push` action
		ExpectExecSucceedWithTimeout(t, g,
			Make(t,
				"bundle-test",
				fmt.Sprintf("BUNDLE_IMAGE_NAME=%s", bundleImageName),
				fmt.Sprintf("NAMESPACE=%s", ns),
				fmt.Sprintf("OLM_INSTALL_MODE=%s", "AllNamespaces"),
			),
			"300s",
		)

		// Check the operator pod is running
		g.Eventually(PodStatusPhase(t, ctx, ns, "camel.apache.org/component=operator"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))

		// Verify the app running in another namespace is monitored
		WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns2 string) {
			// Verify an app running in the same namespace
			t.Run("simple Deployment (monitored)", func(t *testing.T) {
				ExpectExecSucceed(t, g,
					exec.Command(
						"kubectl",
						strings.Split("create deployment camel-app --image="+CamelAppQuarkus()+" -n "+ns2, " ")...,
					),
				)
				// Add the labels to discover it
				ExpectExecSucceed(t, g,
					exec.Command(
						"kubectl",
						strings.Split("label deployment camel-app camel.apache.org/monitor=camel-global-monitored -n "+ns2, " ")...,
					),
				)
				// The name of the selector, "camel.apache.org/monitor: camel-sample-monitored"
				g.Eventually(CamelMonitor(t, ctx, ns2, "camel-global-monitored")).Should(Not(BeNil()))
				g.Eventually(
					CamelMonitorStatus(t, ctx, ns2, "camel-global-monitored"),
					TestTimeoutMedium,
				).Should(
					MatchFields(IgnoreExtras, Fields{
						"Phase":       Equal(v1alpha1.CamelMonitorPhaseRunning),
						"Replicas":    PointTo(Equal(int32(1))),
						"SuccessRate": Not(BeNil()),
					}),
				)
				// Delete deployment
				ExpectExecSucceed(t, g,
					exec.Command(
						"kubectl",
						strings.Split("delete deployment camel-app -n "+ns2, " ")...,
					),
				)
				// No CamelMonitors around (garbage collected)
				g.Eventually(CamelMonitors(t, ctx, ns2)).Should(BeEmpty())
			})
		})
	})
}
