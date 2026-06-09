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

package platform

import (
	"testing"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestIsCurrentOperatorGlobal(t *testing.T) {
	t.Run("empty env => global", func(t *testing.T) {
		t.Setenv(OperatorWatchNamespaceEnvVariable, "")
		assert.True(t, IsCurrentOperatorGlobal())
	})

	t.Run("whitespace env => global", func(t *testing.T) {
		t.Setenv(OperatorWatchNamespaceEnvVariable, "   ")
		assert.True(t, IsCurrentOperatorGlobal())
	})

	t.Run("set env => local", func(t *testing.T) {
		t.Setenv(OperatorWatchNamespaceEnvVariable, "my-namespace")
		assert.False(t, IsCurrentOperatorGlobal())
	})
}

func TestGetOperatorNamespace(t *testing.T) {
	t.Setenv(OperatorNamespaceEnvVariable, "operator-ns")

	assert.Equal(t, "operator-ns", GetOperatorNamespace())
}

func TestGetOperatorEnvAsInt(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		expected     int
		defaultValue int
	}{
		{"valid int", "42", 42, 10},
		{"invalid int", "abc", 10, 10},
		{"empty value", "", 10, 10},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_INT_ENV", tt.envValue)

			val := getOperatorEnvAsInt("TEST_INT_ENV", "test int env", tt.defaultValue)
			assert.Equal(t, tt.expected, val)
		})
	}
}

func TestGetPollingInterval(t *testing.T) {
	t.Setenv(CamelMonitorPollIntervalSeconds, "30")

	assert.Equal(t, 30*time.Second, GetPollingInterval())
}

func TestGetObservabilityPort_Default(t *testing.T) {
	t.Setenv(CamelMonitorObservabilityPort, "")

	assert.Equal(t, defaultObservabilityPort, GetObservabilityPort())
}

func TestSLIThresholds(t *testing.T) {
	t.Setenv(SLIExchangeErrorPercentage, "7")
	assert.Equal(t, 7, GetSLIExchangeErrorThreshold())

	t.Setenv(SLIExchangeWarningPercentage, "15")
	assert.Equal(t, 15, GetSLIExchangeWarningThreshold())
}

func TestGetCreatePrometheusPodMonitor(t *testing.T) {
	t.Setenv(createPrometheusPodMonitorEnvVariable, "true")

	assert.Equal(t, "true", GetCreatePrometheusPodMonitor())
}

func TestGetCreatePrometheusRuleAlerts(t *testing.T) {
	t.Setenv(createPrometheusRulesEnvVariable, "true")

	assert.Equal(t, "true", GetCreatePrometheusRuleAlerts())
}

func TestGetCreateGrafanaDashboard(t *testing.T) {
	t.Setenv(createGrafanaDashboardEnvVariable, "true")

	assert.Equal(t, "true", GetCreateGrafanaDashboard())
}

func TestGetOperatorLockName(t *testing.T) {
	assert.Equal(t, "camel-lock", GetOperatorLockName("camel"))
}

func TestGetMonitorLabelSelector(t *testing.T) {
	t.Run("returns env value", func(t *testing.T) {
		t.Setenv(CamelMonitorLabelSelector, "custom-selector")

		assert.Equal(t, "custom-selector", GetMonitorLabelSelector())
	})

	t.Run("falls back to default", func(t *testing.T) {
		t.Setenv(CamelMonitorLabelSelector, "")

		assert.Equal(t, v1alpha1.MonitorLabel, GetMonitorLabelSelector())
	})
}

func TestGetLabelFromEnvVar(t *testing.T) {
	t.Run("valid label", func(t *testing.T) {
		env := "TEST_LABEL"

		t.Setenv(env, "team=camel")

		labels := getLabelFromEnvVar(env, map[string]string{})

		assert.Equal(t, map[string]string{
			"team": "camel",
		}, labels)
	})

	t.Run("invalid label format", func(t *testing.T) {
		env := "TEST_LABEL"

		t.Setenv(env, "invalid")

		labels := getLabelFromEnvVar(env, map[string]string{
			"existing": "value",
		})

		assert.Equal(t, map[string]string{
			"existing": "value",
		}, labels)
	})

	t.Run("env not set", func(t *testing.T) {
		labels := getLabelFromEnvVar(
			"NON_EXISTING_ENV",
			map[string]string{"a": "b"},
		)

		assert.Equal(t, map[string]string{"a": "b"}, labels)
	})
}

func TestGetGrafanaDatasource(t *testing.T) {
	t.Run("env value", func(t *testing.T) {
		t.Setenv(GrafanaDatasourceEnvVariable, "thanos")

		assert.Equal(t, "thanos", GetGrafanaDatasource())
	})

	t.Run("default", func(t *testing.T) {
		t.Setenv(GrafanaDatasourceEnvVariable, "")

		assert.Equal(t, defaultGrafanaDatasource, GetGrafanaDatasource())
	})
}

func TestGetPrometheusRuleLabels(t *testing.T) {
	t.Setenv(PrometheusRuleLabelEnvVariable, "team=camel")

	labels := GetPrometheusRuleLabels()

	assert.Equal(t, "camel", labels["team"])
}

func TestGetPrometheusLabels(t *testing.T) {
	t.Setenv(PrometheusLabelEnvVariable, "team=camel")

	labels := GetPrometheusLabels()

	assert.Equal(t, "camel", labels["team"])
}

func TestGetGrafanaLabels(t *testing.T) {
	t.Setenv(GrafanaLabelEnvVariable, "team=camel")

	labels := GetGrafanaLabels()

	assert.Equal(t, "camel", labels["team"])
}
