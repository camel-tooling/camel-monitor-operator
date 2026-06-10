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
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/log"
)

const (
	OperatorWatchNamespaceEnvVariable     = "WATCH_NAMESPACE"
	OperatorNamespaceEnvVariable          = "NAMESPACE"
	createPrometheusPodMonitorEnvVariable = "CREATE_PROMETHEUS_POD_MONITOR"
	createPrometheusRulesEnvVariable      = "CREATE_PROMETHEUS_RULE"
	createGrafanaDashboardEnvVariable     = "CREATE_GRAFANA_DASHBOARD"
	PrometheusLabelEnvVariable            = "PROMETHEUS_LABEL"
	PrometheusRuleLabelEnvVariable        = "PROMETHEUS_RULE_LABEL"
	GrafanaLabelEnvVariable               = "GRAFANA_LABEL"
	GrafanaDatasourceEnvVariable          = "GRAFANA_DS"
	maxIdleEnvVariable                    = "MAX_IDLE_SEC"

	CamelMonitorLabelSelector = "LABEL_SELECTOR"

	CamelMonitorPollIntervalSeconds         = "POLL_INTERVAL_SECONDS"
	DefaultPollingIntervalSeconds           = 60
	SLIExchangeErrorPercentage              = "SLI_ERR_PERCENTAGE"
	defaultSLIExchangeErrorPercentage       = 5
	SLIExchangeWarningPercentage            = "SLI_WARN_PERCENTAGE"
	defaultSLIExchangeWarningPercentage     = 10
	CamelMonitorObservabilityPort           = "OBSERVABILITY_PORT"
	defaultObservabilityPort            int = 9876
	DefaultObservabilityMetrics             = "observe/metrics"
	DefaultObservabilityHealth              = "observe/health"
	defaultGrafanaDatasource                = "prometheus"
	defaultMaxIdleSec                   int = 60

	OperatorLockName = "camel-monitor-lock"
)

var defaultPrometheusLabels = map[string]string{"camel.apache.org/prometheus": "camel-monitor-operator"}
var defaultGrafanaLabels = map[string]string{"camel.apache.org/grafana": "camel-monitor-operator"}
var defaultPrometheusRuleLabels = map[string]string{"camel.apache.org/alerts": "camel-monitor-operator", "app": "camel-monitor"}

// IsCurrentOperatorGlobal returns true if the operator is configured to watch all namespaces.
func IsCurrentOperatorGlobal() bool {
	if watchNamespace, envSet := os.LookupEnv(OperatorWatchNamespaceEnvVariable); !envSet || strings.TrimSpace(watchNamespace) == "" {
		return true
	}

	return false
}

// GetOperatorNamespace returns the namespace where the current operator is located (if set).
func GetOperatorNamespace() string {
	if podNamespace, envSet := os.LookupEnv(OperatorNamespaceEnvVariable); envSet {
		return podNamespace
	}

	return ""
}

// GetCreatePrometheusPodMonitor returns the variable controlling the Prometheus Pod Monitor creation.
func GetCreatePrometheusPodMonitor() string {
	if create, envSet := os.LookupEnv(createPrometheusPodMonitorEnvVariable); envSet {
		return create
	}

	return ""
}

// GetCreatePrometheusRuleAlerts returns the variable controlling the PrometheusRule creation.
func GetCreatePrometheusRuleAlerts() string {
	if create, envSet := os.LookupEnv(createPrometheusRulesEnvVariable); envSet {
		return create
	}

	return ""
}

// GetCreateGrafanaDashboard returns the variable controlling the Grafana Dashboard creation.
func GetCreateGrafanaDashboard() string {
	if create, envSet := os.LookupEnv(createGrafanaDashboardEnvVariable); envSet {
		return create
	}

	return ""
}

// GetOperatorLockName returns the name of the lock lease that is electing a leader on the particular namespace.
func GetOperatorLockName(operatorID string) string {
	return operatorID + "-lock"
}

// GetMonitorLabelSelector returns the label selector used to determine a Camel application.
func GetMonitorLabelSelector() string {
	if labelSelector, envSet := os.LookupEnv(CamelMonitorLabelSelector); envSet && labelSelector != "" {
		return labelSelector
	}

	return v1alpha1.MonitorLabel
}

// GetPrometheusRuleLabels returns the label selector used to link a Prometheus PodMonitor to a Prometheus instance.
func GetPrometheusRuleLabels() map[string]string {
	return getLabelFromEnvVar(PrometheusRuleLabelEnvVariable, defaultPrometheusRuleLabels)
}

// GetPrometheusLabels returns the label selector used to link a Prometheus PodMonitor to a Prometheus instance.
func GetPrometheusLabels() map[string]string {
	return getLabelFromEnvVar(PrometheusLabelEnvVariable, defaultPrometheusLabels)
}

// GetGrafanaLabels returns the label selector used to link a GrafanaDashboard to a Grafana instance.
func GetGrafanaLabels() map[string]string {
	return getLabelFromEnvVar(GrafanaLabelEnvVariable, defaultGrafanaLabels)
}

func getLabelFromEnvVar(envVar string, defaultReturnLabels map[string]string) map[string]string {
	if varValue, envSet := os.LookupEnv(envVar); envSet && varValue != "" {
		split := strings.Split(varValue, "=")
		if len(split) == 2 {
			defaultReturnLabels[split[0]] = split[1]
		} else {
			log.Errorf(errors.New("could not parse"), "could not properly parse label environment variable %s, "+
				"fallback to default value %s", envVar, defaultReturnLabels)
		}
	}

	return defaultReturnLabels
}

// GetGrafanaDatasource returns the datasource to use for a GrafanaDashboard.
func GetGrafanaDatasource() string {
	if grafanaDatasourceEnvVar, envSet := os.LookupEnv(GrafanaDatasourceEnvVariable); envSet && grafanaDatasourceEnvVar != "" {
		return grafanaDatasourceEnvVar
	}

	return defaultGrafanaDatasource
}

// GetMaxIdle returns the max time expected for an application to be idle.
func GetMaxIdle() int {
	return getOperatorEnvAsInt(maxIdleEnvVariable, "max idle sec", defaultMaxIdleSec)
}

// getOperatorEnvAsInt returns a generic operator environment variable as an it. It fallbacks to default value if the env var is missing.
func getOperatorEnvAsInt(envVar, envVarDescription string, defaultValue int) int {
	if envVarVal, envSet := os.LookupEnv(envVar); envSet && envVarVal != "" {
		v, err := strconv.Atoi(envVarVal)
		if err == nil {
			return v
		} else {
			log.Errorf(err, "could not properly parse Operator env var %s, "+
				"fallback to default value %d", envVarDescription, defaultValue)
		}
	}

	return defaultValue
}

// getPollingIntervalSeconds returns the polling interval (in seconds) for the operator. It fallbacks to default value.
func getPollingIntervalSeconds() int {
	return getOperatorEnvAsInt(CamelMonitorPollIntervalSeconds, "polling interval configuration", DefaultPollingIntervalSeconds)
}

// GetPollingInterval returns the polling interval for the operator. It fallbacks to default value.
func GetPollingInterval() time.Duration {
	return time.Duration(getPollingIntervalSeconds()) * time.Second
}

// GetObservabilityPort returns the observability port set for the operator. It fallbacks to default value.
func GetObservabilityPort() int {
	return getOperatorEnvAsInt(CamelMonitorObservabilityPort, "observability port configuration", defaultObservabilityPort)
}

// GetSLIExchangeErrorThreshold returns the SLI Exchange error threshold configuration. It fallbacks to default value.
func GetSLIExchangeErrorThreshold() int {
	return getOperatorEnvAsInt(SLIExchangeErrorPercentage, "SLI exchange error threshold", defaultSLIExchangeErrorPercentage)
}

// GetSLIExchangeWarningThreshold returns the SLI Exchange warning threshold configuration. It fallbacks to default value.
func GetSLIExchangeWarningThreshold() int {
	return getOperatorEnvAsInt(SLIExchangeWarningPercentage, "SLI exchange warning threshold", defaultSLIExchangeWarningPercentage)
}
