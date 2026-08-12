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
	"context"
	"testing"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestMonitorActionBakingCronjobMissing(t *testing.T) {
	app := &v1alpha1.CamelMonitor{}
	app.Name = "test-app"
	app.Namespace = "default"
	app.Annotations = map[string]string{
		v1alpha1.MonitorImportedKindLabel: "CronJob",
		v1alpha1.MonitorImportedNameLabel: "test-cron",
	}

	fakeClient, err := internal.NewFakeClient(app)
	require.NoError(t, err)

	action := &monitorAction{}
	action.InjectClient(fakeClient)

	_, err = action.Handle(context.TODO(), app)

	require.Error(t, err)
	require.Equal(t, "baking deployment object does not exist for App default/test-app", err.Error())
}

func TestMonitorActionDeploymentCronJobWaiting(t *testing.T) {
	app := &v1alpha1.CamelMonitor{}
	app.Name = "test-app"
	app.Namespace = "default"
	app.Annotations = map[string]string{
		v1alpha1.MonitorImportedKindLabel: "CronJob",
		v1alpha1.MonitorImportedNameLabel: "my-test-cron",
	}

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "my-test-cron", Namespace: "default"},
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{Template: v1.PodTemplateSpec{Spec: v1.PodSpec{
					Containers: []v1.Container{{Image: "my-camel-image"}}}}},
			},
		},
		Status: batchv1.CronJobStatus{
			Active: nil,
		},
	}

	fakeClient, err := internal.NewFakeClient(app, cronjob)
	require.NoError(t, err)

	action := &monitorAction{}
	action.InjectClient(fakeClient)

	target, err := action.Handle(context.TODO(), app)

	require.NoError(t, err)
	require.NotNil(t, target)
	assert.Equal(t, "my-camel-image", target.Status.Image)
	assert.Equal(t, ptr.To(int32(0)), target.Status.Replicas)
	assert.Equal(t, v1alpha1.CamelMonitorPhasePaused, target.Status.Phase)

	monitored := target.Status.GetCondition("Monitored")
	assert.NotNil(t, monitored)
	assert.Equal(t, metav1.ConditionFalse, monitored.Status)
	assert.Equal(t, "No scheduled job has run yet", monitored.Message)
	healthy := target.Status.GetCondition("Healthy")
	assert.Nil(t, healthy)
}

func TestMonitorActionDeploymentCronJobActive(t *testing.T) {
	app := &v1alpha1.CamelMonitor{}
	app.Name = "test-app"
	app.Namespace = "default"
	app.Annotations = map[string]string{
		v1alpha1.MonitorImportedKindLabel: "CronJob",
		v1alpha1.MonitorImportedNameLabel: "my-test-cron",
	}

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "my-test-cron", Namespace: "default"},
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "my-test-cron"},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{{Image: "my-camel-image"}}}}},
			},
		},
		Status: batchv1.CronJobStatus{
			Active: []v1.ObjectReference{{Name: "job"}},
		},
	}
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod-1", Namespace: "default", Labels: map[string]string{"app": "my-test-cron"}},
		Status:     v1.PodStatus{Phase: corev1.PodRunning},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod-2", Namespace: "default", Labels: map[string]string{"app": "my-test-cron"}},
		Status:     v1.PodStatus{Phase: corev1.PodSucceeded},
	}

	fakeClient, err := internal.NewFakeClient(app, cronjob, pod1, pod2)
	require.NoError(t, err)

	action := &monitorAction{}
	action.InjectClient(fakeClient)

	target, err := action.Handle(context.TODO(), app)

	require.NoError(t, err)
	require.NotNil(t, target)
	assert.Equal(t, "my-camel-image", target.Status.Image)
	assert.Equal(t, ptr.To(int32(1)), target.Status.Replicas)
	assert.Equal(t, v1alpha1.CamelMonitorPhaseRunning, target.Status.Phase)
	assert.Len(t, target.Status.Pods, 2)

	monitored := target.Status.GetCondition("Monitored")
	assert.NotNil(t, monitored)
	assert.Equal(t, metav1.ConditionTrue, monitored.Status)
	healthy := target.Status.GetCondition("Healthy")
	assert.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionTrue, healthy.Status)
	assert.Equal(t, "1 out of last 1 job succeeded", target.Status.Info)
}

func TestMonitorActionDeploymentCronJobFailed(t *testing.T) {
	app := &v1alpha1.CamelMonitor{}
	app.Name = "test-app"
	app.Namespace = "default"
	app.Annotations = map[string]string{
		v1alpha1.MonitorImportedKindLabel: "CronJob",
		v1alpha1.MonitorImportedNameLabel: "my-test-cron",
	}

	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "my-test-cron", Namespace: "default"},
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "my-test-cron"},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{{Image: "my-camel-image"}}}}},
			},
		},
		Status: batchv1.CronJobStatus{
			Active: []v1.ObjectReference{{Name: "job"}},
		},
	}
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod-1", Namespace: "default", Labels: map[string]string{"app": "my-test-cron"}},
		Status:     v1.PodStatus{Phase: corev1.PodFailed},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod-2", Namespace: "default", Labels: map[string]string{"app": "my-test-cron"}},
		Status:     v1.PodStatus{Phase: corev1.PodSucceeded},
	}

	fakeClient, err := internal.NewFakeClient(app, cronjob, pod1, pod2)
	require.NoError(t, err)

	action := &monitorAction{}
	action.InjectClient(fakeClient)

	target, err := action.Handle(context.TODO(), app)

	require.NoError(t, err)
	require.NotNil(t, target)
	assert.Equal(t, "my-camel-image", target.Status.Image)
	assert.Equal(t, ptr.To(int32(1)), target.Status.Replicas)
	assert.Equal(t, v1alpha1.CamelMonitorPhaseRunning, target.Status.Phase)
	assert.Len(t, target.Status.Pods, 2)

	monitored := target.Status.GetCondition("Monitored")
	assert.NotNil(t, monitored)
	assert.Equal(t, metav1.ConditionTrue, monitored.Status)
	healthy := target.Status.GetCondition("Healthy")
	assert.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionFalse, healthy.Status)
	assert.Equal(t, "1 out of last 2 job succeeded", target.Status.Info)
}
