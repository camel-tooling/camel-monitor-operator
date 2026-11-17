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

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/stretchr/testify/assert"
)

func TestNonManagedCamelDeploymentStatic(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-app",
			Namespace:   "default",
			UID:         "1234",
			Annotations: map[string]string{"foo": "bar"},
			Labels:      map[string]string{v1alpha1.MonitorLabel: "my-app"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "my-image:v1"},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          3,
			AvailableReplicas: 2,
		},
	}

	app := nonManagedCamelDeployment{deploy: deploy}

	// Test GetAppImage
	image := app.GetAppImage()
	assert.Equal(t, "my-image:v1", image)

	// Test GetReplicas
	replicas := app.GetReplicas()
	assert.NotNil(t, replicas)
	assert.Equal(t, int32(3), *replicas)

	// Test GetAnnotations
	ann := app.GetAnnotations()
	assert.Equal(t, "bar", ann["foo"])

	// Test GetAppPhase when not all replicas available
	phase := app.GetAppPhase(context.TODO(), nil)
	assert.Equal(t, v1alpha1.CamelMonitorPhaseError, phase)

	// Test GetAppPhase when all replicas available
	deploy.Status.AvailableReplicas = 3
	phase = app.GetAppPhase(context.TODO(), nil)
	assert.Equal(t, v1alpha1.CamelMonitorPhaseRunning, phase)

	// Test GetAppPhase when replicas = 0
	deploy.Status.Replicas = 0
	deploy.Status.AvailableReplicas = 0
	phase = app.GetAppPhase(context.TODO(), nil)
	assert.Equal(t, v1alpha1.CamelMonitorPhasePaused, phase)

	// Test GetAppPhase when spec replicas = 0 but status replicas > 0 (scale-down in progress)
	deploy.Spec.Replicas = ptr.To(int32(0))
	deploy.Status.Replicas = 2
	deploy.Status.AvailableReplicas = 0
	phase = app.GetAppPhase(context.TODO(), nil)
	assert.Equal(t, v1alpha1.CamelMonitorPhasePaused, phase)
	deploy.Spec.Replicas = ptr.To(int32(3))

	// Test CamelMonitor static fields
	deploy.Status.Replicas = 2
	deploy.Status.AvailableReplicas = 2
	cmon := app.CamelMonitor(t.Context(), nil)
	assert.Equal(t, "my-app", cmon.Name)
	assert.Equal(t, "default", cmon.Namespace)
	assert.Equal(t, "my-app", cmon.Annotations[v1alpha1.MonitorImportedNameLabel])
	assert.Equal(t, "Deployment", cmon.Annotations[v1alpha1.MonitorImportedKindLabel])
	assert.Len(t, cmon.OwnerReferences, 1)
	assert.Equal(t, "1234", string(cmon.OwnerReferences[0].UID))
}
