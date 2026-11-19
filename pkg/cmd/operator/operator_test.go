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

package operator

import (
	"reflect"
	"testing"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestGetNamespacesSelector(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]cache.Config
	}{
		{
			name:  "single namespace",
			input: "default",
			expected: map[string]cache.Config{
				"default": {},
			},
		},
		{
			name:  "multiple namespaces",
			input: "default,kube-system,monitoring",
			expected: map[string]cache.Config{
				"default":     {},
				"kube-system": {},
				"monitoring":  {},
			},
		},
		{
			name:  "handles spaces around names",
			input: " default , kube-system , monitoring ",
			expected: map[string]cache.Config{
				"default":     {},
				"kube-system": {},
				"monitoring":  {},
			},
		},
		{
			name:  "ignores empty entries",
			input: "default,,kube-system,",
			expected: map[string]cache.Config{
				"default":     {},
				"kube-system": {},
			},
		},
		{
			name:     "all empty input",
			input:    ", , ,",
			expected: map[string]cache.Config{},
		},
		{
			name:     "empty string input",
			input:    "",
			expected: map[string]cache.Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getNamespacesSelector(tt.input)

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("expected %#v, got %#v", tt.expected, got)
			}
		})
	}
}

func TestSlimPod(t *testing.T) {
	now := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-pod",
			Namespace:         "ns",
			UID:               "uid",
			ResourceVersion:   "42",
			Labels:            map[string]string{"app": "foo"},
			Annotations:       map[string]string{"big": "annotation"},
			DeletionTimestamp: &now,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "main",
				Image: "my-image",
				Env:   []corev1.EnvVar{{Name: "A", Value: "B"}},
				Ports: []corev1.ContainerPort{{Name: "jolokia", ContainerPort: 8778}},
			}},
			Volumes: []corev1.Volume{{Name: "vol"}},
		},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			PodIP:             "10.0.0.1",
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: "main"}},
		},
	}

	res, err := slimPod(pod)
	require.NoError(t, err)

	slim, ok := res.(*corev1.Pod)
	require.True(t, ok)

	assert.Equal(t, "my-pod", slim.Name)
	assert.Equal(t, "ns", slim.Namespace)
	assert.Equal(t, "42", slim.ResourceVersion)
	assert.Equal(t, pod.Labels, slim.Labels)
	assert.NotNil(t, slim.DeletionTimestamp)
	assert.Empty(t, slim.Annotations)
	assert.Equal(t, corev1.PodRunning, slim.Status.Phase)
	assert.Equal(t, "10.0.0.1", slim.Status.PodIP)
	assert.Equal(t, pod.Status.Conditions, slim.Status.Conditions)
	assert.Empty(t, slim.Status.ContainerStatuses)
	assert.Empty(t, slim.Spec.Volumes)
	require.Len(t, slim.Spec.Containers, 1)
	assert.Equal(t, pod.Spec.Containers[0].Ports, slim.Spec.Containers[0].Ports)
	assert.Empty(t, slim.Spec.Containers[0].Env)
	assert.Empty(t, slim.Spec.Containers[0].Image)

	// the original object must not be mutated
	assert.Len(t, pod.Spec.Containers[0].Env, 1)
}

func TestSlimPod_NotAPod(t *testing.T) {
	res, err := slimPod("tombstone")
	require.NoError(t, err)
	assert.Equal(t, "tombstone", res)
}

func TestGetLabelSelector(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		label string
	}{
		{
			name:  "uses env label selector",
			env:   "app",
			label: "app",
		},
		{
			name:  "uses default monitor label",
			env:   "",
			label: v1alpha1.MonitorLabel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(platform.CamelMonitorLabelSelector, tt.env)

			sel, err := getLabelSelector()

			require.NoError(t, err)
			require.NotNil(t, sel)

			reqs, exists := sel.Requirements()
			assert.True(t, exists)
			require.Len(t, reqs, 1)

			req := reqs[0]

			// validate selector logic
			assert.Equal(t, tt.label, req.Key())
			assert.Equal(t, selection.Exists, req.Operator())
			assert.Empty(t, req.Values())
		})
	}
}

func TestGetLabelSelector_Error(t *testing.T) {
	t.Setenv(platform.CamelMonitorLabelSelector, "invalid label with spaces")

	sel, err := getLabelSelector()

	require.Error(t, err)
	require.Nil(t, sel)
}

func TestGetWatchNamespace(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		expectError bool
		expected    string
	}{
		{
			name:        "returns namespace when env is set",
			envValue:    "default",
			expectError: false,
			expected:    "default",
		},
		{
			name:        "returns error when env is empty",
			envValue:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.envValue != "" {
				t.Setenv(platform.OperatorWatchNamespaceEnvVariable, tt.envValue)
			}

			ns, err := getWatchNamespace()

			if tt.expectError {
				require.Error(t, err)
				require.EqualError(t, err, platform.OperatorWatchNamespaceEnvVariable+" must be set")
				require.Empty(t, ns)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, ns)
		})
	}
}
