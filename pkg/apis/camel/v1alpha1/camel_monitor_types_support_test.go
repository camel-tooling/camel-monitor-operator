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

package v1alpha1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddCondition(t *testing.T) {
	old := metav1.NewTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))

	status := CamelMonitorStatus{}
	status.AddCondition(metav1.Condition{Type: ConditionTypeMetricsAvailable, Status: metav1.ConditionTrue, LastTransitionTime: old, Message: "old"})
	status.AddCondition(metav1.Condition{Type: ConditionTypeHealthy, Status: metav1.ConditionTrue, LastTransitionTime: old})

	// same status: transition time is kept, the rest of the condition is the new one
	status.AddCondition(metav1.Condition{Type: ConditionTypeMetricsAvailable, Status: metav1.ConditionTrue, Message: "new"})
	// status changed: transition time is updated
	status.AddCondition(metav1.Condition{Type: ConditionTypeHealthy, Status: metav1.ConditionFalse})
	// new condition: transition time is set
	status.AddCondition(metav1.Condition{Type: ConditionTypeCPUPressure, Status: metav1.ConditionFalse})

	assert.Len(t, status.Conditions, 3, "conditions of the same type must be replaced, not appended")

	monitored := status.GetCondition(ConditionTypeMetricsAvailable)
	assert.Equal(t, old, monitored.LastTransitionTime)
	assert.Equal(t, "new", monitored.Message)
	assert.True(t, status.GetCondition(ConditionTypeHealthy).LastTransitionTime.After(old.Time))
	assert.False(t, status.GetCondition(ConditionTypeCPUPressure).LastTransitionTime.IsZero())
}

func TestRemoveCondition(t *testing.T) {
	status := CamelMonitorStatus{}
	status.AddCondition(metav1.Condition{Type: ConditionTypeMetricsAvailable, Status: metav1.ConditionTrue})
	status.AddCondition(metav1.Condition{Type: ConditionTypeCPUPressure, Status: metav1.ConditionFalse})
	status.AddCondition(metav1.Condition{Type: ConditionTypeMemoryPressure, Status: metav1.ConditionFalse})

	status.RemoveCondition(ConditionTypeCPUPressure, ConditionTypeMemoryPressure, ConditionTypeGrafanaDashboard)

	assert.Len(t, status.Conditions, 1)
	assert.NotNil(t, status.GetCondition(ConditionTypeMetricsAvailable))
	assert.Nil(t, status.GetCondition(ConditionTypeCPUPressure))
	assert.Nil(t, status.GetCondition(ConditionTypeMemoryPressure))
}
