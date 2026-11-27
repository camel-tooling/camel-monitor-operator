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

package synthetic

import (
	"testing"

	v1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestPodPressureHelpers(t *testing.T) {
	assert.False(t, podMemoryPressure(nil))
	assert.False(t, podCpuPressure(nil))

	pods := []v1alpha1.PodInfo{{}, {}}
	assert.False(t, podMemoryPressure(pods))
	assert.False(t, podCpuPressure(pods))

	pods[1].HasMemoryPressure = true
	assert.True(t, podMemoryPressure(pods))
	assert.False(t, podCpuPressure(pods))

	pods[1].HasCPUPressure = true
	assert.True(t, podCpuPressure(pods))
}

func TestSetMonitoringConditionHelperNoPods(t *testing.T) {
	target := &v1alpha1.CamelMonitor{}

	setMonitoringCondition(&v1alpha1.CamelMonitor{}, target, nil)

	monitored := target.Status.GetCondition(v1alpha1.ConditionTypeMetricsAvailable)
	require.NotNil(t, monitored)
	assert.Equal(t, metav1.ConditionUnknown, monitored.Status)
	assert.Equal(t, "No active Pod available", monitored.Message)

	healthy := target.Status.GetCondition(v1alpha1.ConditionTypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionUnknown, healthy.Status)

	assert.Nil(t, target.Status.GetCondition(v1alpha1.ConditionTypePressure))
}

func TestSetMonitoringConditionHelperAllHealthy(t *testing.T) {
	src := &v1alpha1.CamelMonitor{}
	src.Status.Replicas = ptr.To(int32(1))
	target := &v1alpha1.CamelMonitor{}
	pods := []v1alpha1.PodInfo{
		{Ready: true, Runtime: &v1alpha1.RuntimeInfo{Status: v1alpha1.PodStatusUP}},
	}

	setMonitoringCondition(src, target, pods)

	assert.Equal(t, metav1.ConditionTrue, target.Status.GetCondition(v1alpha1.ConditionTypeMetricsAvailable).Status)
	assert.Equal(t, "Success", target.Status.GetCondition(v1alpha1.ConditionTypeMetricsAvailable).Message)
	assert.Equal(t, metav1.ConditionTrue, target.Status.GetCondition(v1alpha1.ConditionTypeHealthy).Status)
	assert.Equal(t, metav1.ConditionFalse, target.Status.GetCondition(v1alpha1.ConditionTypeMemoryPressure).Status)
	assert.Equal(t, metav1.ConditionFalse, target.Status.GetCondition(v1alpha1.ConditionTypeCPUPressure).Status)
	assert.Equal(t, metav1.ConditionFalse, target.Status.GetCondition(v1alpha1.ConditionTypePressure).Status)
}

func TestSetMonitoringConditionHelperMissingReplicasAndPressure(t *testing.T) {
	src := &v1alpha1.CamelMonitor{}
	src.Status.Replicas = ptr.To(int32(3))
	target := &v1alpha1.CamelMonitor{}
	pods := []v1alpha1.PodInfo{
		{
			Ready:             true,
			HasMemoryPressure: true,
			HasCPUPressure:    true,
			Runtime:           &v1alpha1.RuntimeInfo{Status: v1alpha1.PodStatusUP},
		},
	}

	setMonitoringCondition(src, target, pods)

	assert.Equal(t, "1 out of 3 pods available", target.Status.GetCondition(v1alpha1.ConditionTypeMetricsAvailable).Message)
	assert.Equal(t, metav1.ConditionTrue, target.Status.GetCondition(v1alpha1.ConditionTypeMemoryPressure).Status)
	assert.Equal(t, metav1.ConditionTrue, target.Status.GetCondition(v1alpha1.ConditionTypeCPUPressure).Status)
	assert.Equal(t, "At least one Pod has CPU pressure", target.Status.GetCondition(v1alpha1.ConditionTypeCPUPressure).Message)
	assert.Equal(t, metav1.ConditionTrue, target.Status.GetCondition(v1alpha1.ConditionTypePressure).Status)
}

func TestSetMonitoringConditionHelperPodNotReady(t *testing.T) {
	target := &v1alpha1.CamelMonitor{}
	pods := []v1alpha1.PodInfo{{Ready: false}}

	setMonitoringCondition(&v1alpha1.CamelMonitor{}, target, pods)

	assert.Equal(t, metav1.ConditionFalse, target.Status.GetCondition(v1alpha1.ConditionTypeMetricsAvailable).Status)
	assert.Equal(t, metav1.ConditionFalse, target.Status.GetCondition(v1alpha1.ConditionTypeHealthy).Status)
}
