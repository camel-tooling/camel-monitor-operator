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

package monitor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetSLIExchangeSuccessRate(t *testing.T) {
	app := v1alpha1.RuntimeInfo{
		Exchange: &v1alpha1.ExchangeInfo{
			Total:  100,
			Failed: 5,
		},
	}

	target := v1alpha1.RuntimeInfo{
		Exchange: &v1alpha1.ExchangeInfo{
			Total:  200,
			Failed: 15,
		},
	}

	interval := 30 * time.Second

	// 10% failure rate: warning when error threshold is 20 and warning threshold is 5
	result := getSLIExchangeSuccessRate(app, target, &interval, 20, 5)

	assert.NotNil(t, result)
	assert.Equal(t, 100, result.SamplingIntervalTotal)
	assert.Equal(t, 10, result.SamplingIntervalFailed)
	assert.Equal(t, "90.00", result.SuccessPercentage)
	assert.Equal(t, v1alpha1.SLIExchangeStatusWarning, result.Status)

	// 10% failure rate: error when error threshold is 5
	result = getSLIExchangeSuccessRate(app, target, &interval, 5, 1)
	assert.Equal(t, v1alpha1.SLIExchangeStatusError, result.Status)

	// 10% failure rate: success when both thresholds are above it
	result = getSLIExchangeSuccessRate(app, target, &interval, 20, 10)
	assert.Equal(t, v1alpha1.SLIExchangeStatusSuccess, result.Status)
}

func TestGetSummary(t *testing.T) {
	now := metav1.NewTime(time.Now())
	later := metav1.NewTime(time.Now().Add(1 * time.Minute))

	tests := []struct {
		name     string
		pods     []v1alpha1.PodInfo
		expected *v1alpha1.RuntimeInfo
	}{
		{
			name: "no runtime info at all -> nil",
			pods: []v1alpha1.PodInfo{
				{Ready: true},
				{Ready: true},
			},
			expected: nil,
		},
		{
			name: "aggregate exchange info",
			pods: []v1alpha1.PodInfo{
				{
					Runtime: &v1alpha1.RuntimeInfo{
						RuntimeProvider: "quarkus",
						RuntimeVersion:  "3.2.0",
						CamelVersion:    "4.0.0",
						Exchange: &v1alpha1.ExchangeInfo{
							Total:         100,
							Failed:        5,
							Pending:       10,
							Succeeded:     85,
							LastTimestamp: &now,
						},
					},
				},
				{
					Runtime: &v1alpha1.RuntimeInfo{
						Exchange: &v1alpha1.ExchangeInfo{
							Total:         50,
							Failed:        2,
							Pending:       5,
							Succeeded:     43,
							LastTimestamp: &later,
						},
					},
				},
			},
			expected: &v1alpha1.RuntimeInfo{
				RuntimeProvider: "quarkus",
				RuntimeVersion:  "3.2.0",
				CamelVersion:    "4.0.0",
				Exchange: &v1alpha1.ExchangeInfo{
					Total:         150,
					Failed:        7,
					Pending:       15,
					Succeeded:     128,
					LastTimestamp: &later, // latest timestamp wins
				},
			},
		},
		{
			name: "first pod without runtime info is skipped",
			pods: []v1alpha1.PodInfo{
				{Ready: false},
				{
					Runtime: &v1alpha1.RuntimeInfo{
						RuntimeProvider: "main",
						RuntimeVersion:  "4.5.0",
						CamelVersion:    "4.5.0",
						Exchange: &v1alpha1.ExchangeInfo{
							Total:         10,
							Failed:        1,
							Pending:       2,
							Succeeded:     7,
							LastTimestamp: &now,
						},
					},
				},
			},
			expected: &v1alpha1.RuntimeInfo{
				RuntimeProvider: "main",
				RuntimeVersion:  "4.5.0",
				CamelVersion:    "4.5.0",
				Exchange: &v1alpha1.ExchangeInfo{
					Total:         10,
					Failed:        1,
					Pending:       2,
					Succeeded:     7,
					LastTimestamp: &now,
				},
			},
		},
		{
			name: "runtime present but no exchange data",
			pods: []v1alpha1.PodInfo{
				{
					Runtime: &v1alpha1.RuntimeInfo{
						RuntimeProvider: "springboot",
						RuntimeVersion:  "3.0",
						CamelVersion:    "4.1",
					},
				},
			},
			expected: &v1alpha1.RuntimeInfo{
				RuntimeProvider: "springboot",
				RuntimeVersion:  "3.0",
				CamelVersion:    "4.1",
				Exchange:        &v1alpha1.ExchangeInfo{},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The summary is stored in the app status, as done by the monitor action
			status := v1alpha1.CamelMonitorStatus{RuntimeSummary: getRuntimeSummary(tt.pods)}
			result := status.RuntimeSummary

			if tt.expected == nil {
				assert.Nil(t, status.RuntimeSummary)
				return
			}

			require.NotNil(t, status.RuntimeSummary)
			require.NotNil(t, result.Exchange)
			assert.Equal(t, tt.expected.RuntimeProvider, result.RuntimeProvider)
			assert.Equal(t, tt.expected.RuntimeVersion, result.RuntimeVersion)
			assert.Equal(t, tt.expected.CamelVersion, result.CamelVersion)

			assert.Equal(t, tt.expected.Exchange.Total, result.Exchange.Total)
			assert.Equal(t, tt.expected.Exchange.Failed, result.Exchange.Failed)
			assert.Equal(t, tt.expected.Exchange.Pending, result.Exchange.Pending)
			assert.Equal(t, tt.expected.Exchange.Succeeded, result.Exchange.Succeeded)

			if tt.expected.Exchange.LastTimestamp != nil {
				assert.True(t,
					tt.expected.Exchange.LastTimestamp.Equal(result.Exchange.LastTimestamp),
				)
			}
		})
	}
}

func TestCheckVersionUpgradeEmpty(t *testing.T) {
	cleanCache(t)
	t.Setenv("CHECK_VERSION_UPGRADE", "true")
	status := &v1alpha1.CamelMonitorStatus{}
	checkVersionUpgrade(t.Context(), &v1alpha1.RuntimeInfo{}, status)
	assert.Empty(t, status.Conditions)
}

func TestCheckVersionUpgradeNoEnvVar(t *testing.T) {
	cleanCache(t)
	status := &v1alpha1.CamelMonitorStatus{}
	checkVersionUpgrade(t.Context(), &v1alpha1.RuntimeInfo{
		RuntimeProvider: "Quarkus",
	}, status)
	assert.Empty(t, status.Conditions)
}

func TestCheckVersionUpgradeUnknownRuntime(t *testing.T) {
	cleanCache(t)
	t.Setenv("CHECK_VERSION_UPGRADE", "true")
	status := &v1alpha1.CamelMonitorStatus{}
	checkVersionUpgrade(t.Context(), &v1alpha1.RuntimeInfo{
		RuntimeProvider: "bogus",
	}, status)

	require.Len(t, status.Conditions, 1)
	assert.Equal(t, metav1.ConditionUnknown, status.Conditions[0].Status)
	assert.Equal(t, "could not fetch repository: unknown runtime provider bogus", status.Conditions[0].Message)
}

func TestCheckVersionUpgradeMainIsTrue(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `
			<metadata>
				<groupId>org.apache.camel</groupId>
				<artifactId>camel-core</artifactId>
				<versioning>
					<latest>4.22.0</latest>
					<release>4.22.0</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	t.Setenv("CHECK_VERSION_UPGRADE", "true")
	t.Setenv("MAVEN_BASE_REPO_URL", server.URL)
	status := &v1alpha1.CamelMonitorStatus{}
	checkVersionUpgrade(t.Context(), &v1alpha1.RuntimeInfo{
		RuntimeProvider: "Main",
		RuntimeVersion:  "4.21.0",
		CamelVersion:    "4.21.0",
	}, status)

	require.Len(t, status.Conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, status.Conditions[0].Status)
}

func TestCheckVersionUpgradeMainIsFalse(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `
			<metadata>
				<groupId>org.apache.camel</groupId>
				<artifactId>camel-core</artifactId>
				<versioning>
					<latest>4.22.0</latest>
					<release>4.22.0</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	t.Setenv("CHECK_VERSION_UPGRADE", "true")
	t.Setenv("MAVEN_BASE_REPO_URL", server.URL)
	status := &v1alpha1.CamelMonitorStatus{}
	checkVersionUpgrade(t.Context(), &v1alpha1.RuntimeInfo{
		RuntimeProvider: "Main",
		RuntimeVersion:  "4.22.0",
		CamelVersion:    "4.22.0",
	}, status)

	require.Len(t, status.Conditions, 1)
	assert.Equal(t, metav1.ConditionFalse, status.Conditions[0].Status)
}

func TestCheckVersionUpgradeQuarkusIsTrue(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		if strings.HasSuffix(r.URL.Path, ".pom") {
			fmt.Fprint(w, `
		<project xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
			<dependencyManagement>
				<dependencies>
					<dependency>
						<groupId>org.apache.camel.quarkus</groupId>
						<artifactId>camel-quarkus-core</artifactId>
						<version>4.40.0</version>
					</dependency>
				</dependencies>
			</dependencyManagement>
		</project>
		`)

			return
		}

		fmt.Fprint(w, `
			<metadata>
				<groupId>quarkus</groupId>
				<artifactId>camel-quarkus</artifactId>
				<versioning>
					<latest>4.40.1</latest>
					<release>4.40.1</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	t.Setenv("CHECK_VERSION_UPGRADE", "true")
	t.Setenv("MAVEN_BASE_REPO_URL", server.URL)

	status := &v1alpha1.CamelMonitorStatus{}
	checkVersionUpgrade(t.Context(), &v1alpha1.RuntimeInfo{
		RuntimeProvider: "Quarkus",
		RuntimeVersion:  "3.38.0",
		CamelVersion:    "4.20.0",
	}, status)

	require.Len(t, status.Conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, status.Conditions[0].Status)
	assert.Equal(t, "New Camel Quarkus runtime version 4.40.0 is available", status.Conditions[0].Message)
}

func TestCheckVersionUpgradeSpringBootIsTrue(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `
			<metadata>
				<groupId>csb</groupId>
				<artifactId>camel-csb</artifactId>
				<versioning>
					<latest>4.22.0</latest>
					<release>4.22.0</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	t.Setenv("CHECK_VERSION_UPGRADE", "true")
	t.Setenv("MAVEN_BASE_REPO_URL", server.URL)
	status := &v1alpha1.CamelMonitorStatus{}
	checkVersionUpgrade(t.Context(), &v1alpha1.RuntimeInfo{
		RuntimeProvider: "Spring-Boot",
		RuntimeVersion:  "4.0.5",
		CamelVersion:    "4.21.0",
	}, status)

	require.Len(t, status.Conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, status.Conditions[0].Status)
}

func TestRemoveDisabledFeaturesConditions(t *testing.T) {
	newStatus := func() *v1alpha1.CamelMonitorStatus {
		status := &v1alpha1.CamelMonitorStatus{}
		for _, condType := range []string{
			v1alpha1.ConditionTypeUpgradeAvailable,
			v1alpha1.ConditionTypePrometheusPodMonitor,
			v1alpha1.ConditionTypeGrafanaDashboard,
		} {
			status.AddCondition(metav1.Condition{Type: condType, Status: metav1.ConditionTrue})
		}

		return status
	}

	t.Run("disabled features are removed", func(t *testing.T) {
		t.Setenv("CHECK_VERSION_UPGRADE", "false")
		t.Setenv("CREATE_PROMETHEUS_POD_MONITOR", "false")
		t.Setenv("CREATE_GRAFANA_DASHBOARD", "false")

		status := newStatus()
		(&monitorAction{hasPrometheusCRDs: true, hasGrafanaCRDs: true}).removeDisabledFeaturesConditions(status)
		assert.Empty(t, status.Conditions)
	})

	t.Run("missing CRDs are removed", func(t *testing.T) {
		t.Setenv("CHECK_VERSION_UPGRADE", "true")
		t.Setenv("CREATE_PROMETHEUS_POD_MONITOR", "true")
		t.Setenv("CREATE_GRAFANA_DASHBOARD", "true")

		status := newStatus()
		(&monitorAction{}).removeDisabledFeaturesConditions(status)
		assert.Len(t, status.Conditions, 1)
		assert.NotNil(t, status.GetCondition(v1alpha1.ConditionTypeUpgradeAvailable))
	})

	t.Run("enabled features are kept", func(t *testing.T) {
		t.Setenv("CHECK_VERSION_UPGRADE", "true")
		t.Setenv("CREATE_PROMETHEUS_POD_MONITOR", "true")
		t.Setenv("CREATE_GRAFANA_DASHBOARD", "true")

		status := newStatus()
		(&monitorAction{hasPrometheusCRDs: true, hasGrafanaCRDs: true}).removeDisabledFeaturesConditions(status)
		assert.Len(t, status.Conditions, 3)
	})
}
