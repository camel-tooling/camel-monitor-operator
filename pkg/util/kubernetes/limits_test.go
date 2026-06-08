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

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetResourcesLimitInMillis(t *testing.T) {
	tests := []struct {
		name     string
		limits   corev1.ResourceList
		resource corev1.ResourceName
		expected float64
	}{
		{
			name:     "nil limits",
			limits:   nil,
			resource: corev1.ResourceCPU,
			expected: -1,
		},
		{
			name: "resource missing",
			limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			resource: corev1.ResourceCPU,
			expected: -1,
		},
		{
			name: "cpu limit present",
			limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("500m"),
			},
			resource: corev1.ResourceCPU,
			expected: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected,
				GetResourcesLimitInMillis(tt.limits, tt.resource))
		})
	}
}
