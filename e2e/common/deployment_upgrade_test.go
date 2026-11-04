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
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/camel-tooling/camel-monitor-operator/e2e/support"
	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// These tests assert the actual UpgradeAvailable condition (status + reason + message).
// They deploy the sample app as a plain Deployment (no Camel K).
//
// The expected latest is supplied externally via a per-runtime environment variable.
// The test compares the version the operator detected on the running app against that target:
//
// actual == target -> the app runs the latest -> UpgradeAvailable must be False/UpToDate
// actual != target -> a newer version exists  -> UpgradeAvailable must be True/NewVersionAvailable
//
// If the variable for a runtime is unset the test is skipped.
//
// Note the version scheme differs per runtime:
// Quarkus compares the Quarkus platform (runtime) version, e.g. 3.40.1 - hence the RUNTIME_ prefix
// While Spring-Boot and Main compare the Camel version, e.g. 4.23.0.
const (
	quarkusTargetUpgradeVersionEnv    = "RUNTIME_QUARKUS_TARGET_UPGRADE_VERSION"
	springBootTargetUpgradeVersionEnv = "CAMEL_SB_TARGET_UPGRADE_VERSION"
	mainTargetUpgradeVersionEnv       = "CAMEL_MAIN_TARGET_UPGRADE_VERSION"
)

func TestVerifyUpgradeAvailableQuarkus(t *testing.T) {
	testVerifyUpgradeAvailable(t, CamelAppQuarkus(), quarkusTargetUpgradeVersionEnv)
}

func TestVerifyUpgradeAvailableSpringBoot(t *testing.T) {
	testVerifyUpgradeAvailable(t, CamelAppSpringBoot(), springBootTargetUpgradeVersionEnv)
}

func TestVerifyUpgradeAvailableMain(t *testing.T) {
	testVerifyUpgradeAvailable(t, CamelAppMain(), mainTargetUpgradeVersionEnv)
}

func testVerifyUpgradeAvailable(t *testing.T, image, targetVersionEnv string) {
	target := os.Getenv(targetVersionEnv)
	if target == "" {
		t.Skipf("set %s to the expected latest version to run this test", targetVersionEnv)
	}

	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		t.Run("upgrade available condition", func(t *testing.T) {
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("create deployment camel-app --image="+image+" -n "+ns, " ")...,
				),
			)

			g.Eventually(PodStatusPhase(t, ctx, ns, "app=camel-app"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))

			// As there is no label, there is not yet any CamelMonitor CR
			g.Consistently(CamelMonitors(t, ctx, ns), TestTimeoutShort, 10*time.Second).Should(BeEmpty())

			// Add the label to discover it
			ExpectExecSucceed(t, g,
				exec.Command(
					"kubectl",
					strings.Split("label deployment camel-app camel.apache.org/monitor=camel-sample -n "+ns, " ")...,
				),
			)
			// The name of the selector, "camel.apache.org/monitor: camel-sample"
			g.Eventually(CamelMonitor(t, ctx, ns, "camel-sample")).Should(Not(BeNil()))

			// Wait until the operator is Running and has detected the runtime (provider + version).
			g.Eventually(func() bool {
				st, err := CamelMonitorStatus(t, ctx, ns, "camel-sample")()
				return err == nil &&
					st.Phase == v1alpha1.CamelMonitorPhaseRunning &&
					len(st.Pods) > 0 && st.Pods[0].Runtime != nil &&
					st.Pods[0].Runtime.RuntimeProvider != ""
			}, TestTimeoutMedium).Should(BeTrue())

			st, err := CamelMonitorStatus(t, ctx, ns, "camel-sample")()
			g.Expect(err).NotTo(HaveOccurred())
			runtime := st.Pods[0].Runtime

			actual := comparedVersion(runtime)
			t.Logf("runtime=%s actual=%s target=%s", runtime.RuntimeProvider, actual, target)

			assertUpgradeCondition(t, ctx, g, ns, "camel-sample", actual, target)
		})
	})
}

// comparedVersion returns the version the operator compares against the Maven metadata for this runtime:
// Quarkus uses the runtime version, Spring-Boot/Main use the Camel version.
func comparedVersion(runtime *v1alpha1.RuntimeInfo) string {
	if runtime.RuntimeProvider == "Quarkus" {
		return runtime.RuntimeVersion
	}
	return runtime.CamelVersion
}

// assertUpgradeCondition polls the CamelMonitor until its UpgradeAvailable condition matches what
// the actual-vs-target comparison implies: if the app runs the target version there must be no
// upgrade (False/UpToDate), otherwise one is available (True/NewVersionAvailable). The message must
// always be non-empty. It fails (with a got-vs-want message) if the condition never matches.
func assertUpgradeCondition(t *testing.T, ctx context.Context, g *WithT, ns, name, actual, target string) {
	t.Helper()

	var wantStatus metav1.ConditionStatus
	var wantReason string
	if actual != target {
		wantStatus = metav1.ConditionTrue
		wantReason = "NewVersionAvailable"
	} else {
		wantStatus = metav1.ConditionFalse
		wantReason = "UpToDate"
	}

	var got metav1.Condition
	g.Eventually(func() error {
		status, err := CamelMonitorStatus(t, ctx, ns, name)()
		if err != nil {
			return err
		}
		cond := apimeta.FindStatusCondition(status.Conditions, "UpgradeAvailable")
		if cond == nil {
			return fmt.Errorf("UpgradeAvailable condition not populated on %s yet", name)
		}
		got = *cond
		if cond.Status != wantStatus || cond.Reason != wantReason || cond.Message == "" {
			return fmt.Errorf("unexpected UpgradeAvailable: got status=%s reason=%s message=%q; want status=%s reason=%s with a non-empty message",
				cond.Status, cond.Reason, cond.Message, wantStatus, wantReason)
		}

		return nil
	}, TestTimeoutMedium).Should(Succeed())

	t.Logf("UpgradeAvailable verified: actual=%s target=%s want(status=%s reason=%s) -> found(status=%s reason=%s message=%q)",
		actual, target, wantStatus, wantReason, got.Status, got.Reason, got.Message)
}
