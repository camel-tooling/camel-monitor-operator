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
	"testing"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func monitorWithAnnotations(annotations map[string]string) *v1alpha1.CamelMonitor {
	return &v1alpha1.CamelMonitor{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}}
}

func TestGetPollingInterval(t *testing.T) {
	t.Setenv(platform.CamelMonitorPollIntervalSeconds, "30")

	assert.Equal(t, 30*time.Second, getPollingInterval(monitorWithAnnotations(nil)))
	assert.Equal(t, 30*time.Second, getPollingInterval(monitorWithAnnotations(map[string]string{
		v1alpha1.MonitorPollingIntervalSecondsAnnotation: "",
	})))
	assert.Equal(t, 5*time.Second, getPollingInterval(monitorWithAnnotations(map[string]string{
		v1alpha1.MonitorPollingIntervalSecondsAnnotation: "5",
	})))
	assert.Equal(t, 30*time.Second, getPollingInterval(monitorWithAnnotations(map[string]string{
		v1alpha1.MonitorPollingIntervalSecondsAnnotation: "not-a-number",
	})))
}

func TestGetSLIExchangeErrorThreshold(t *testing.T) {
	t.Setenv(platform.SLIExchangeErrorPercentage, "7")

	assert.Equal(t, 7, getSLIExchangeErrorThreshold(monitorWithAnnotations(nil)))
	assert.Equal(t, 7, getSLIExchangeErrorThreshold(monitorWithAnnotations(map[string]string{
		v1alpha1.MonitorSLIExchangeErrorPercentageAnnotation: "",
	})))
	assert.Equal(t, 20, getSLIExchangeErrorThreshold(monitorWithAnnotations(map[string]string{
		v1alpha1.MonitorSLIExchangeErrorPercentageAnnotation: "20",
	})))
	assert.Equal(t, 7, getSLIExchangeErrorThreshold(monitorWithAnnotations(map[string]string{
		v1alpha1.MonitorSLIExchangeErrorPercentageAnnotation: "not-a-number",
	})))
}

func TestGetSLIExchangeWarningThreshold(t *testing.T) {
	t.Setenv(platform.SLIExchangeWarningPercentage, "15")

	assert.Equal(t, 15, getSLIExchangeWarningThreshold(monitorWithAnnotations(nil)))
	assert.Equal(t, 15, getSLIExchangeWarningThreshold(monitorWithAnnotations(map[string]string{
		v1alpha1.MonitorSLIExchangeWarningPercentageAnnotation: "",
	})))
	assert.Equal(t, 40, getSLIExchangeWarningThreshold(monitorWithAnnotations(map[string]string{
		v1alpha1.MonitorSLIExchangeWarningPercentageAnnotation: "40",
	})))
	assert.Equal(t, 15, getSLIExchangeWarningThreshold(monitorWithAnnotations(map[string]string{
		v1alpha1.MonitorSLIExchangeWarningPercentageAnnotation: "not-a-number",
	})))
}
