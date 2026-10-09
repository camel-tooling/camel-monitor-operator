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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/camel-tooling/camel-monitor-operator/e2e/support"
	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"

	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
)

// appUnderTest describes a Camel app the prometheus tests deploy and probe. The sample
// apps are deployed as plain Deployments and labelled for monitoring, so these tests do
// not require a Camel K operator.
type appUnderTest struct {
	// image is the sample image deployed as a plain Deployment.
	image string
	// monitorName is the name of the CamelMonitor (and PodMonitor) the operator creates.
	monitorName string
	// routeID is the Camel route id exposed in the camel_exchanges_total metric.
	routeID string
}

// The sample-db-app-* images all run a single timer route with id "route1". Each runtime
// has its own top-level Test function (see below) so smoke/downstream runs can skip a
// runtime by name, e.g. `-skip 'Main$'` (the Camel Main image is not published downstream).
func quarkusApp() appUnderTest {
	return appUnderTest{image: CamelAppQuarkus(), monitorName: "camel-sample", routeID: "route1"}
}

func springBootApp() appUnderTest {
	return appUnderTest{image: CamelAppSpringBoot(), monitorName: "camel-sample", routeID: "route1"}
}

func mainApp() appUnderTest {
	return appUnderTest{image: CamelAppMain(), monitorName: "camel-sample", routeID: "route1"}
}

// deploy creates the app as a Deployment and labels it so the operator monitors it.
func (a appUnderTest) deploy(t *testing.T, ctx context.Context, g *WithT, ns string) {
	ExpectExecSucceed(t, g,
		exec.Command("kubectl",
			strings.Split("create deployment camel-app --image="+a.image+" -n "+ns, " ")...,
		),
	)
	g.Eventually(PodStatusPhase(t, ctx, ns, "app=camel-app"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))
	ExpectExecSucceed(t, g,
		exec.Command("kubectl",
			strings.Split("label deployment camel-app camel.apache.org/monitor="+a.monitorName+" -n "+ns, " ")...,
		),
	)
}

// promEndpoint describes how to reach a Prometheus-compatible query API via a port-forward.
// Every field is configurable through environment variables so the exact same test runs
// unchanged on upstream Kubernetes and on downstream OpenShift; only the configuration differs.
type promEndpoint struct {
	pfNamespace string // namespace of the service to port-forward
	pfService   string // service to port-forward
	pfPort      int    // service port to forward to
	scheme      string // "http" or "https"
	token       string // bearer token for the query (empty = no Authorization header)
}

// promQueryEndpoint builds the endpoint from the environment, defaulting to the standalone
// Prometheus created by the e2e setup scripts (upstream / minikube / CI). To run against
// OpenShift User-Workload-Monitoring instead, point it at thanos-querier, e.g.:
//
//	PROMETHEUS_PF_NAMESPACE=openshift-monitoring
//	PROMETHEUS_PF_SERVICE=thanos-querier
//	PROMETHEUS_PF_PORT=9091
//	PROMETHEUS_SCHEME=https
//	PROMETHEUS_TOKEN=$(oc whoami -t)   # identity needs the cluster-monitoring-view ClusterRole
func promQueryEndpoint() promEndpoint {
	return promEndpoint{
		pfNamespace: envOrDefault("PROMETHEUS_PF_NAMESPACE", "prometheus"),
		pfService:   envOrDefault("PROMETHEUS_PF_SERVICE", "prometheus-operated"),
		pfPort:      envIntOrDefault("PROMETHEUS_PF_PORT", 9090),
		scheme:      envOrDefault("PROMETHEUS_SCHEME", "http"),
		token:       os.Getenv("PROMETHEUS_TOKEN"),
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

func envIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}

	return def
}

// instantQuery runs a Prometheus instant query through the local port-forward on :9090 and
// returns the first result's value. It returns -1 when no value is available yet so callers
// can poll with Eventually.
func (e promEndpoint) instantQuery(query string) int {
	args := []string{"-s", "--max-time", "10"}
	if e.scheme == "https" {
		args = append(args, "-k")
	}
	if e.token != "" {
		args = append(args, "-H", "Authorization: Bearer "+e.token)
	}
	args = append(args, "--get", "--data-urlencode", "query="+query,
		e.scheme+"://localhost:9090/api/v1/query")

	out, err := exec.Command("curl", args...).Output()
	if err != nil {
		return -1
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil ||
		resp.Status != "success" || len(resp.Data.Result) == 0 ||
		len(resp.Data.Result[0].Value) != 2 {
		return -1
	}

	// The sample value is the second element of [ <timestamp>, "<value>" ], a JSON string.
	var valStr string
	if err := json.Unmarshal(resp.Data.Result[0].Value[1], &valStr); err != nil {
		return -1
	}
	// Prometheus values are floats; truncate to int for the comparison.
	v, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return -1
	}

	return int(v)
}

func TestVerifyPrometheusScrapeMetricsQuarkus(t *testing.T) {
	testVerifyPrometheusScrapeMetrics(t, quarkusApp())
}

func TestVerifyPrometheusScrapeMetricsSpringBoot(t *testing.T) {
	testVerifyPrometheusScrapeMetrics(t, springBootApp())
}

func TestVerifyPrometheusScrapeMetricsMain(t *testing.T) {
	testVerifyPrometheusScrapeMetrics(t, mainApp())
}

func testVerifyPrometheusScrapeMetrics(t *testing.T, app appUnderTest) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		app.deploy(t, ctx, g, ns)

		g.Eventually(
			CamelMonitorStatus(t, ctx, ns, app.monitorName),
			TestTimeoutMedium,
		).Should(
			MatchFields(IgnoreExtras, Fields{
				"Phase": Equal(v1alpha1.CamelMonitorPhaseRunning),
			}),
		)

		// We must verify pod monitor exist. Use the medium timeout: the operator only
		// creates it after detecting the app's metrics endpoint, which can lag the
		// Running phase on slower runtimes (e.g. Spring Boot).
		g.Eventually(PodMonitor(t, ctx, ns, app.monitorName), TestTimeoutMedium).ShouldNot(BeNil())

		// Start port-forward to the Prometheus query API (standalone or UWM, per env).
		endpoint := promQueryEndpoint()
		stopPortForward := PortForwardPrometheus(t, ctx, 9090, endpoint.pfPort, endpoint.pfNamespace, endpoint.pfService)
		defer stopPortForward()

		// Scope the query to this namespace so the different runtimes (and any stale
		// series left in the TSDB) cannot satisfy the assertion for one another.
		query := fmt.Sprintf(`camel_exchanges_total{routeId="%s",namespace="%s"}`, app.routeID, ns)

		// Test the prometheus has scraped correctly
		g.Eventually(func() int {
			return endpoint.instantQuery(query)
		}, TestTimeoutMedium, 15*time.Second).Should(gomega.BeNumerically(">", 5))
	})
}

func TestVerifyGrafanaDashboardQuarkus(t *testing.T) {
	testVerifyGrafanaDashboard(t, quarkusApp())
}

func TestVerifyGrafanaDashboardSpringBoot(t *testing.T) {
	testVerifyGrafanaDashboard(t, springBootApp())
}

func TestVerifyGrafanaDashboardMain(t *testing.T) {
	testVerifyGrafanaDashboard(t, mainApp())
}

func testVerifyGrafanaDashboard(t *testing.T, app appUnderTest) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		app.deploy(t, ctx, g, ns)

		g.Eventually(
			CamelMonitorStatus(t, ctx, ns, app.monitorName),
			TestTimeoutMedium,
		).Should(
			MatchFields(IgnoreExtras, Fields{
				"Phase": Equal(v1alpha1.CamelMonitorPhaseRunning),
			}),
		)

		// We must verify the dashboard exist. Use the medium timeout: the operator only
		// creates it after detecting the app's metrics endpoint, which can lag the
		// Running phase on slower runtimes (e.g. Spring Boot).
		g.Eventually(GrafanaDashboard(t, ctx, ns, app.monitorName), TestTimeoutMedium).ShouldNot(BeNil())
		// The struct has no conditions, so we check other fields
		Eventually(func() bool {
			gd, err := GrafanaDashboard(t, ctx, ns, app.monitorName)()
			if err != nil || gd == nil {
				return false
			}

			return !gd.Status.NoMatchingInstances &&
				!gd.Status.LastResync.IsZero() &&
				gd.Status.UID != ""
		}).Should(BeTrue())
	})
}
