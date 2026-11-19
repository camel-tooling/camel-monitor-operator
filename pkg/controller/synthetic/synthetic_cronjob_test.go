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
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/stretchr/testify/require"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCamelMonitor(t *testing.T) {
	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cron",
			Namespace: "default",
			UID:       "12345",
			Labels: map[string]string{
				v1alpha1.MonitorLabel: "my-app",
			},
		},
	}

	adapter := &nonManagedCamelCronjob{
		cron: cron,
	}

	app := adapter.CamelMonitor(context.TODO(), nil)

	require.Equal(t, "default", app.Namespace)
	require.Equal(t, "my-app", app.Name)

	require.Equal(t, "my-cron", app.Annotations[v1alpha1.MonitorImportedNameLabel])
	require.Equal(t, "CronJob", app.Annotations[v1alpha1.MonitorImportedKindLabel])

	require.Len(t, app.OwnerReferences, 1)
	require.Equal(t, "CronJob", app.OwnerReferences[0].Kind)
}

func TestGetAppPhase(t *testing.T) {
	tests := []struct {
		name     string
		active   []corev1.ObjectReference
		expected v1alpha1.CamelMonitorPhase
	}{
		{
			name:     "running when active jobs exist",
			active:   []corev1.ObjectReference{{Name: "job1"}},
			expected: v1alpha1.CamelMonitorPhaseRunning,
		},
		{
			name:     "paused when no active jobs",
			active:   []corev1.ObjectReference{},
			expected: v1alpha1.CamelMonitorPhasePaused,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cron := &batchv1.CronJob{
				Status: batchv1.CronJobStatus{
					Active: tt.active,
				},
			}

			adapter := &nonManagedCamelCronjob{cron: cron}
			phase := adapter.GetAppPhase(context.TODO(), nil)

			require.Equal(t, tt.expected, phase)
		})
	}
}

func TestGetReplicas(t *testing.T) {
	cron := &batchv1.CronJob{
		Status: batchv1.CronJobStatus{
			Active: []corev1.ObjectReference{
				{Name: "job1"},
				{Name: "job2"},
			},
		},
	}

	adapter := &nonManagedCamelCronjob{cron: cron}

	replicas := adapter.GetReplicas()
	require.NotNil(t, replicas)
	require.Equal(t, int32(2), *replicas)
}

func TestGetAppImage(t *testing.T) {
	cron := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Image: "my-image:latest"},
							},
						},
					},
				},
			},
		},
	}

	adapter := &nonManagedCamelCronjob{cron: cron}
	image := adapter.GetAppImage()

	require.Equal(t, "my-image:latest", image)
}

func TestSetMonitoringCondition_NoPods(t *testing.T) {
	cron := &batchv1.CronJob{}
	adapter := &nonManagedCamelCronjob{cron: cron}

	target := &v1alpha1.CamelMonitor{}

	adapter.SetMonitoringCondition(nil, target, nil)

	require.Len(t, target.Status.Conditions, 1)
	require.Equal(t, v1alpha1.ConditionTypeMonitored, target.Status.Conditions[0].Type)
	require.Equal(t, metav1.ConditionFalse, target.Status.Conditions[0].Status)
}

func TestSetMonitoringCondition_AllSucceeded(t *testing.T) {
	now := metav1.NewTime(time.Now())

	cron := &batchv1.CronJob{
		Status: batchv1.CronJobStatus{
			LastScheduleTime:   &now,
			LastSuccessfulTime: &now,
		},
	}

	adapter := &nonManagedCamelCronjob{cron: cron}

	target := &v1alpha1.CamelMonitor{}

	pods := []v1alpha1.PodInfo{
		{Status: "Succeeded"},
		{Status: "Succeeded"},
	}

	adapter.SetMonitoringCondition(nil, target, pods)

	require.Len(t, target.Status.Conditions, 3)

	var healthy *metav1.Condition
	for i := range target.Status.Conditions {
		if target.Status.Conditions[i].Type == v1alpha1.ConditionTypeHealthy {
			healthy = &target.Status.Conditions[i]
		}
	}

	require.NotNil(t, healthy)
	require.Equal(t, metav1.ConditionTrue, healthy.Status)
	require.Contains(t, healthy.Message, "2 out of last 2")
}
