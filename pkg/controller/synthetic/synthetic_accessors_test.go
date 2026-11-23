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
	"net/http"
	"testing"

	v1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testPod(name string, labels map[string]string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: labels},
		Status:     corev1.PodStatus{Phase: phase, PodIP: "10.0.0.1"},
	}
}

func testContainerWithCPULimit() corev1.Container {
	return corev1.Container{
		Name:  "main",
		Image: "my-image:v1",
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
		},
	}
}

func TestCronjobAccessors(t *testing.T) {
	labels := map[string]string{"app": "my-cron"}
	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cron", Namespace: "default", Annotations: map[string]string{"foo": "bar"}},
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{testContainerWithCPULimit()}},
					},
				},
			},
		},
	}
	app := &nonManagedCamelCronjob{cron: cron}

	assert.Equal(t, "bar", app.GetAnnotations()["foo"])
	assert.Equal(t, labels, app.GetMatchLabelsSelector())
	assert.Equal(t, resource.MustParse("500m"), app.GetResourcesLimits()[corev1.ResourceCPU])
}

func TestCronjobGetPods(t *testing.T) {
	labels := map[string]string{"app": "my-cron"}
	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cron", Namespace: "default"},
		Spec: batchv1.CronJobSpec{
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{testContainerWithCPULimit()}},
					},
				},
			},
		},
	}
	// Running Pod is not inspected for a CronJob.
	fakeClient, err := internal.NewFakeClient(testPod("pod1", labels, corev1.PodRunning),
		testPod("other", map[string]string{"app": "other"}, corev1.PodRunning))
	require.NoError(t, err)

	app := &nonManagedCamelCronjob{cron: cron, httpClient: &http.Client{}}
	pods, err := app.GetPods(t.Context(), fakeClient, appObservabilityConf{})

	require.NoError(t, err)
	require.Len(t, pods, 1)
	assert.Equal(t, "pod1", pods[0].Name)
	assert.Equal(t, "Running", pods[0].Status)
}

func TestDeploymentAccessors(t *testing.T) {
	labels := map[string]string{"app": "my-app"}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{testContainerWithCPULimit()}},
			},
		},
	}
	app := &nonManagedCamelDeployment{deploy: deploy}

	assert.Equal(t, labels, app.GetMatchLabelsSelector())
	assert.Equal(t, resource.MustParse("500m"), app.GetResourcesLimits()[corev1.ResourceCPU])
}

func TestDeploymentGetPods(t *testing.T) {
	labels := map[string]string{"app": "my-app"}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{testContainerWithCPULimit()}},
			},
		},
	}
	// Not ready Pod: no inspection is attempted.
	fakeClient, err := internal.NewFakeClient(testPod("pod1", labels, corev1.PodPending))
	require.NoError(t, err)

	app := &nonManagedCamelDeployment{deploy: deploy, httpClient: &http.Client{}}
	pods, err := app.GetPods(t.Context(), fakeClient, appObservabilityConf{})

	require.NoError(t, err)
	require.Len(t, pods, 1)
	assert.False(t, pods[0].Ready)
}

func TestDeploymentSetMonitoringCondition(t *testing.T) {
	app := &nonManagedCamelDeployment{}
	target := &v1alpha1.CamelMonitor{}

	app.SetMonitoringCondition(&v1alpha1.CamelMonitor{}, target, nil)

	require.NotNil(t, target.Status.GetCondition(v1alpha1.ConditionTypeMetricsAvailable))
	assert.Equal(t, metav1.ConditionUnknown, target.Status.GetCondition(v1alpha1.ConditionTypeMetricsAvailable).Status)
}
