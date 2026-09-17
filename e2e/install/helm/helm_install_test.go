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

package helm

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
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/defaults"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
)

func TestHelmInstallation(t *testing.T) {
	chartPath := os.Getenv("CAMEL_MONITOR_HELM_CHART")
	if chartPath == "" {
		chartPath = fmt.Sprintf("../../../docs/charts/camel-monitor-operator-%s.tgz", defaults.Version)
	}
	operatorImage := os.Getenv("CAMEL_MONITOR_OPERATOR_IMAGE")
	if operatorImage == "" {
		t.Fatal("CAMEL_MONITOR_OPERATOR_IMAGE env var is required (use 'make test-install-helm')")
	}

	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		ExpectExecSucceedWithTimeout(t, g,
			exec.Command(
				"helm", "install", "camel-monitor", chartPath,
				"--namespace", ns,
				"--set", "operator.image="+operatorImage,
				"--set", "operator.global=false",
			),
			"120s",
		)

		g.Eventually(PodStatusPhase(t, ctx, ns, "name=camel-monitor-operator"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))

		t.Run("simple Deployment (monitored)", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app --image="+CamelAppQuarkus()+" -n "+ns, " ")...,
				),
			)
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
					"Phase":       Equal(v1alpha1.CamelMonitorPhaseRunning),
					"Replicas":    PointTo(Equal(int32(1))),
					"SuccessRate": Not(BeNil()),
				}),
			)
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("delete deployment camel-app -n "+ns, " ")...,
				),
			)
			g.Eventually(CamelMonitors(t, ctx, ns)).Should(BeEmpty())
		})
	})

	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		t.Run("simple Deployment (non monitored)", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app --image="+CamelAppQuarkus()+" -n "+ns, " ")...,
				),
			)
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample -n "+ns, " ")...,
				),
			)
			g.Consistently(CamelMonitors(t, ctx, ns), TestTimeoutShort, 10*time.Second).Should(BeEmpty())
		})
	})
}
