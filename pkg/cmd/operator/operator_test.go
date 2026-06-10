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
