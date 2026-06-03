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

package support

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/google/uuid"
	integreatlyv1beta1 "github.com/grafana/grafana-operator/v5/api/v1beta1"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	. "github.com/onsi/gomega"
)

var (
	testContext        = context.TODO()
	testClient         *kubernetes.Clientset
	camelMonitorClient *client.Client

	TestTimeoutShort  = 1 * time.Minute
	TestTimeoutMedium = 3 * time.Minute
	TestTimeoutLong   = 5 * time.Minute
)

func init() {
	// Change default to longer periods (we're in kubernetes, so reconciliations can take seconds)
	SetDefaultEventuallyTimeout(TestTimeoutShort)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
}

func WithNewTestNamespace(t *testing.T, doRun func(context.Context, *gomega.WithT, string)) {
	ns := NewTestNamespace(t, testContext)
	defer deleteTestNamespace(t, testContext, ns)

	invokeUserTestCode(t, testContext, ns.GetName(), doRun)
}

func NewTestNamespace(t *testing.T, ctx context.Context) ctrl.Object {
	name := os.Getenv("CAMEL_DASHBOARD_TEST_NS")
	if name == "" {
		name = "test-" + uuid.New().String()
	}

	if exists, err := testNamespaceExists(t, ctx, name); err != nil {
		failTest(t, err)
	} else if exists {
		fmt.Println("Warning: namespace ", name, " already exists so using different namespace name")
		name = fmt.Sprintf("%s-%d", name, time.Now().Second())
	}

	return NewNamedTestNamespace(t, ctx, name)
}

func testNamespaceExists(t *testing.T, ctx context.Context, ns string) (bool, error) {
	_, err := TestClient(t).CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		} else {
			return false, err
		}
	}

	return true, nil
}

func deleteTestNamespace(t *testing.T, ctx context.Context, ns ctrl.Object) {
	if err := TestClient(t).CoreV1().Namespaces().Delete(ctx, ns.GetName(), metav1.DeleteOptions{}); err != nil {
		t.Logf("Warning: cannot delete test namespace %q", ns.GetName())
	}
}

func invokeUserTestCode(t *testing.T, ctx context.Context, ns string, doRun func(context.Context, *gomega.WithT, string)) {
	defer func() {
		if t.Failed() {
			DumpNamespace(t, ctx, ns)
			// Also dump the operator namespace in case it's common
			DumpNamespace(t, ctx, "camel-monitor")
			DumpNamespace(t, ctx, "camel-k")
		}
	}()

	g := gomega.NewWithT(t)
	doRun(ctx, g, ns)
}

// Only panic the test if absolutely necessary and there is
// no test locus. In most cases, the test should fail gracefully
// using the test locus to error out and fail now.
func failTest(t *testing.T, err error) {
	if t != nil {
		t.Helper()
		t.Error(err)
		t.FailNow()
	} else {
		panic(err)
	}
}

func NewNamedTestNamespace(t *testing.T, ctx context.Context, name string) ctrl.Object {
	namespace := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if _, err := TestClient(t).CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{}); err != nil {
		failTest(t, err)
	}
	return namespace
}

func CamelMonitorClient(t *testing.T) *client.Client {
	if camelMonitorClient != nil {
		return camelMonitorClient
	}

	var err error
	cfg, err := config.GetConfig()
	camelMonitorClient, err := NewClientWithConfig(cfg)
	if err != nil {
		failTest(t, err)
	}
	return &camelMonitorClient
}

func TestClient(t *testing.T) *kubernetes.Clientset {
	if testClient != nil {
		return testClient
	}

	var err error
	testClient, err = NewClient()
	if err != nil {
		failTest(t, err)
	}
	return testClient
}

// Pod return the first pod filtered by namespace ns and a given label selector (eg, app=my-deployment).
func Pod(t *testing.T, ctx context.Context, ns string, labelSelector string) func() (*corev1.Pod, error) {
	return func() (*corev1.Pod, error) {
		podList, err := TestClient(t).CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}

		if len(podList.Items) == 0 {
			return nil, nil
		}

		return &podList.Items[0], nil
	}
}

// PodStatusPhase return the first Pod status phase filtered by namespace ns and a given label selector (eg, app=my-deployment).
func PodStatusPhase(t *testing.T, ctx context.Context, ns string, labelSelector string) func() (corev1.PodPhase, error) {
	return func() (corev1.PodPhase, error) {
		pod, err := Pod(t, ctx, ns, labelSelector)()
		if err != nil || pod == nil {
			return "", err
		}

		return pod.Status.Phase, nil
	}
}

// CamelMonitor return the CamelMonitor with the name provided in that namespace.
func CamelMonitor(t *testing.T, ctx context.Context, ns string, appName string) func() (*v1alpha1.CamelMonitor, error) {
	return func() (*v1alpha1.CamelMonitor, error) {
		cli := *CamelMonitorClient(t)
		cmon, err := cli.CamelV1alpha1().CamelMonitors(ns).Get(ctx, appName, v1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, err
		}

		return cmon, nil
	}
}

// CamelMonitor return the CamelMonitor with the name provided in that namespace.
func CamelMonitorStatus(t *testing.T, ctx context.Context, ns string, appName string) func() (v1alpha1.CamelMonitorStatus, error) {
	return func() (v1alpha1.CamelMonitorStatus, error) {
		cmon, err := CamelMonitor(t, ctx, ns, appName)()
		if err != nil || cmon == nil {
			return v1alpha1.CamelMonitorStatus{}, nil
		}

		return cmon.Status, nil
	}
}

// CamelMonitors returns all the apps available in the namespace.
func CamelMonitors(t *testing.T, ctx context.Context, ns string) func() ([]v1alpha1.CamelMonitor, error) {
	return func() ([]v1alpha1.CamelMonitor, error) {
		cli := *CamelMonitorClient(t)
		cmonList, err := cli.CamelV1alpha1().CamelMonitors(ns).List(ctx, v1.ListOptions{})
		if err != nil {
			return nil, err
		}

		return cmonList.Items, nil
	}
}

// PodMonitor returns a PodMonitor with the given name.
func PodMonitor(t *testing.T, ctx context.Context, ns string, name string) func() (*monitoringv1.PodMonitor, error) {
	return func() (*monitoringv1.PodMonitor, error) {
		pm := &monitoringv1.PodMonitor{}
		cli := *CamelMonitorClient(t)
		err := cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, pm)
		if err != nil {
			return nil, nil
		}
		return pm, nil
	}
}

// GrafanaDashboard returns a GrafanaDashboard with the given name.
func GrafanaDashboard(t *testing.T, ctx context.Context, ns string, name string) func() (*integreatlyv1beta1.GrafanaDashboard, error) {
	return func() (*integreatlyv1beta1.GrafanaDashboard, error) {
		gd := &integreatlyv1beta1.GrafanaDashboard{}
		cli := *CamelMonitorClient(t)
		err := cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, gd)
		if err != nil {
			return nil, nil
		}
		return gd, nil
	}
}

func ExpectExecSucceed(t *testing.T, g *WithT, command *exec.Cmd) {
	ExpectExecSucceedWithTimeout(t, g, command, "")
}

func ExpectExecSucceedWithTimeout(t *testing.T, g *WithT, command *exec.Cmd, timeout string) {
	t.Helper()

	var cmdOut strings.Builder
	var cmdErr strings.Builder

	defer func() {
		t.Logf(`Executing "%s" ...`, command)
		t.Logf("[OUT] %s\n", cmdOut.String())
		if t.Failed() {
			t.Logf("[ERR] %s\n", cmdErr.String())
		}
	}()

	RegisterTestingT(t)
	session, err := gexec.Start(command, &cmdOut, &cmdErr)
	if timeout != "" {
		session.Wait(timeout)
	} else {
		session.Wait()
	}

	g.Eventually(session).Should(gexec.Exit(0))
	require.NoError(t, err)
	assert.NotContains(t, strings.ToUpper(cmdErr.String()), "ERROR")
}

func DumpNamespace(t *testing.T, ctx context.Context, ns string) {
	if t.Failed() {
		if err := Dump(ctx, TestClient(t), ns, t); err != nil {
			t.Logf("Error while dumping namespace %s: %v\n", ns, err)
		}
	}
}

// PortForwardPrometheus is used to temporarily port-forward and return a function to stop after it's used.
func PortForwardPrometheus(t *testing.T, ctx context.Context, localPort, remotePort int, namespace, svcName string) func() {
	fmt.Println("** Started Kubernetes port forwarding " + strconv.Itoa(localPort) + ":" + strconv.Itoa(remotePort))
	cmd := exec.CommandContext(ctx,
		"kubectl", "port-forward",
		"svc/"+svcName,
		fmt.Sprintf("%d:%d", localPort, remotePort),
		"-n", namespace,
	)

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start port-forward: %v", err)
	}

	// Give port-forward a moment to establish
	time.Sleep(2 * time.Second)

	// Return a cleanup function
	return func() {
		fmt.Println("** Stopping Kubernetes port forwarding " + strconv.Itoa(localPort) + ":" + strconv.Itoa(remotePort))
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("failed to kill port-forward: %v", err)
		}
	}
}

func Make(t *testing.T, rule string, args ...string) *exec.Cmd {
	return MakeWithContext(t, rule, args...)
}

func MakeWithContext(t *testing.T, rule string, args ...string) *exec.Cmd {
	makeArgs := os.Getenv("CAMEL_MONITOR_OPERATOR_TEST_MAKE_ARGS")
	defaultArgs := strings.Fields(makeArgs)
	args = append(defaultArgs, args...)

	defaultDir := "."
	makeDir := os.Getenv("CAMEL_MONITOR_OPERATOR_TEST_MAKE_DIR")
	if makeDir == "" {
		makeDir = defaultDir
	} else if makeDir != defaultDir {
		fmt.Printf("Using alternative make directory on path: %s\n", makeDir)
	}

	if fi, e := os.Stat(makeDir); e != nil && os.IsNotExist(e) {
		failTest(t, e)
	} else if !fi.Mode().IsDir() {
		failTest(t, e)
	}

	args = append([]string{"-C", makeDir, rule}, args...)
	fmt.Println("Running make with arguments:", args)
	return exec.Command("make", args...)
}

func CamelAppMain() string {
	camelAppVersion := getCamelAppVersion()

	return "docker.io/squakez/db-app-main:" + camelAppVersion
}

func getCamelAppVersion() string {
	camelAppVersion := os.Getenv("CAMEL_APP_VERSION")
	if camelAppVersion == "" {
		camelAppVersion = "4.20.0"
	}

	return camelAppVersion
}
