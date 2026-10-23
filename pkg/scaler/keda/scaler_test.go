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

package keda

import (
	"context"
	"testing"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	"github.com/kedacore/keda/v2/pkg/scalers/externalscaler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsActive(t *testing.T) {
	t.Run("CamelMonitor not found returns active to prevent premature scale-down", func(t *testing.T) {
		fakeClient, err := internal.NewFakeClient()
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)
		resp, err := scaler.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "test-app",
			Namespace: "default",
		})
		require.NoError(t, err)
		assert.True(t, resp.Result)
	})

	t.Run("CamelMonitor uninitialized status returns active", func(t *testing.T) {
		cmon := &v1alpha1.CamelMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "default",
			},
			Status: v1alpha1.CamelMonitorStatus{},
		}
		fakeClient, err := internal.NewFakeClient(cmon)
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)
		resp, err := scaler.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "test-app",
			Namespace: "default",
		})
		require.NoError(t, err)
		assert.True(t, resp.Result)
	})

	t.Run("CamelMonitor with zero activity returns inactive", func(t *testing.T) {
		cmon := &v1alpha1.CamelMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "default",
			},
			Status: v1alpha1.CamelMonitorStatus{
				Phase: v1alpha1.CamelMonitorPhaseRunning,
				SuccessRate: &v1alpha1.SLIExchangeSuccessRate{
					SamplingIntervalTotal: 0,
				},
			},
		}
		fakeClient, err := internal.NewFakeClient(cmon)
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)
		resp, err := scaler.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "test-app",
			Namespace: "default",
		})
		require.NoError(t, err)
		assert.False(t, resp.Result)
	})

	t.Run("CamelMonitor with activity returns active", func(t *testing.T) {
		cmon := &v1alpha1.CamelMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cmon",
				Namespace: "prod",
			},
			Status: v1alpha1.CamelMonitorStatus{
				Phase: v1alpha1.CamelMonitorPhaseRunning,
				SuccessRate: &v1alpha1.SLIExchangeSuccessRate{
					SamplingIntervalTotal: 50,
				},
			},
		}
		fakeClient, err := internal.NewFakeClient(cmon)
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)
		resp, err := scaler.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "scaledobject-name",
			Namespace: "prod",
			ScalerMetadata: map[string]string{
				MetadataKeyCmonName:      "my-cmon",
				MetadataKeyCmonNamespace: "prod",
			},
		})
		require.NoError(t, err)
		assert.True(t, resp.Result)
	})

	t.Run("CamelMonitor with custom activation threshold", func(t *testing.T) {
		cmon := &v1alpha1.CamelMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "default",
			},
			Status: v1alpha1.CamelMonitorStatus{
				Phase: v1alpha1.CamelMonitorPhaseRunning,
				SuccessRate: &v1alpha1.SLIExchangeSuccessRate{
					SamplingIntervalTotal: 10,
				},
			},
		}
		fakeClient, err := internal.NewFakeClient(cmon)
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)

		// Above threshold 5 -> active
		resp, err := scaler.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "test-app",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				MetadataKeyActivationThreshold: "5",
			},
		})
		require.NoError(t, err)
		assert.True(t, resp.Result)

		// Below threshold 15 -> inactive
		resp, err = scaler.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "test-app",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				MetadataKeyActivationThreshold: "15",
			},
		})
		require.NoError(t, err)
		assert.False(t, resp.Result)
	})

	t.Run("CamelMonitor with rate metric and interval duration", func(t *testing.T) {
		duration := 10 * time.Second
		cmon := &v1alpha1.CamelMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "default",
			},
			Status: v1alpha1.CamelMonitorStatus{
				Phase: v1alpha1.CamelMonitorPhaseRunning,
				SuccessRate: &v1alpha1.SLIExchangeSuccessRate{
					SamplingIntervalTotal:    100,
					SamplingIntervalDuration: &duration,
				},
			},
		}
		fakeClient, err := internal.NewFakeClient(cmon)
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)

		// 100 exchanges / 10s = 10 rps. Threshold 5 -> active
		resp, err := scaler.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "test-app",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				MetadataKeyMetricName:          "rate",
				MetadataKeyActivationThreshold: "5",
			},
		})
		require.NoError(t, err)
		assert.True(t, resp.Result)

		// Threshold 15 -> inactive
		resp, err = scaler.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "test-app",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				MetadataKeyMetricName:          "rate",
				MetadataKeyActivationThreshold: "15",
			},
		})
		require.NoError(t, err)
		assert.False(t, resp.Result)
	})
}

func TestGetMetricSpec(t *testing.T) {
	t.Run("Default MetricSpec", func(t *testing.T) {
		fakeClient, err := internal.NewFakeClient()
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)
		resp, err := scaler.GetMetricSpec(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "test-app",
			Namespace: "default",
		})
		require.NoError(t, err)
		require.Len(t, resp.MetricSpecs, 1)
		assert.Equal(t, MetricNameSamplingIntervalTotal, resp.MetricSpecs[0].MetricName)
		assert.Equal(t, DefaultTargetSize, resp.MetricSpecs[0].TargetSize)
	})

	t.Run("Custom MetricSpec threshold", func(t *testing.T) {
		fakeClient, err := internal.NewFakeClient()
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)
		resp, err := scaler.GetMetricSpec(context.Background(), &externalscaler.ScaledObjectRef{
			Name:      "test-app",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				MetadataKeyThreshold: "25",
			},
		})
		require.NoError(t, err)
		require.Len(t, resp.MetricSpecs, 1)
		assert.Equal(t, MetricNameSamplingIntervalTotal, resp.MetricSpecs[0].MetricName)
		assert.Equal(t, int64(25), resp.MetricSpecs[0].TargetSize)
		assert.Equal(t, float64(25), resp.MetricSpecs[0].TargetSizeFloat)
	})
}

func TestGetMetrics(t *testing.T) {
	t.Run("CamelMonitor not found returns 0", func(t *testing.T) {
		fakeClient, err := internal.NewFakeClient()
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)
		resp, err := scaler.GetMetrics(context.Background(), &externalscaler.GetMetricsRequest{
			ScaledObjectRef: &externalscaler.ScaledObjectRef{
				Name:      "test-app",
				Namespace: "default",
			},
			MetricName: "samplingIntervalTotal",
		})
		require.NoError(t, err)
		require.Len(t, resp.MetricValues, 1)
		assert.Equal(t, "samplingIntervalTotal", resp.MetricValues[0].MetricName)
		assert.Equal(t, int64(0), resp.MetricValues[0].MetricValue)
	})

	t.Run("CamelMonitor returns samplingIntervalTotal", func(t *testing.T) {
		cmon := &v1alpha1.CamelMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "default",
			},
			Status: v1alpha1.CamelMonitorStatus{
				Phase: v1alpha1.CamelMonitorPhaseRunning,
				SuccessRate: &v1alpha1.SLIExchangeSuccessRate{
					SamplingIntervalTotal:  150,
					SamplingIntervalFailed: 10,
					SuccessPercentage:      "93.3",
				},
			},
		}
		fakeClient, err := internal.NewFakeClient(cmon)
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)
		resp, err := scaler.GetMetrics(context.Background(), &externalscaler.GetMetricsRequest{
			ScaledObjectRef: &externalscaler.ScaledObjectRef{
				Name:      "test-app",
				Namespace: "default",
			},
		})
		require.NoError(t, err)
		require.Len(t, resp.MetricValues, 1)
		assert.Equal(t, MetricNameSamplingIntervalTotal, resp.MetricValues[0].MetricName)
		assert.Equal(t, int64(150), resp.MetricValues[0].MetricValue)
	})

	t.Run("CamelMonitor returns rate evaluated with interval duration", func(t *testing.T) {
		duration := 20 * time.Second
		cmon := &v1alpha1.CamelMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "default",
			},
			Status: v1alpha1.CamelMonitorStatus{
				Phase: v1alpha1.CamelMonitorPhaseRunning,
				SuccessRate: &v1alpha1.SLIExchangeSuccessRate{
					SamplingIntervalTotal:    100,
					SamplingIntervalDuration: &duration,
				},
			},
		}
		fakeClient, err := internal.NewFakeClient(cmon)
		require.NoError(t, err)

		scaler := NewCamelMonitorScaler(fakeClient)

		// Test rate (100 / 20 = 5 rps)
		resp, err := scaler.GetMetrics(context.Background(), &externalscaler.GetMetricsRequest{
			ScaledObjectRef: &externalscaler.ScaledObjectRef{
				Name:      "test-app",
				Namespace: "default",
			},
			MetricName: "rate",
		})
		require.NoError(t, err)
		require.Len(t, resp.MetricValues, 1)
		assert.Equal(t, "rate", resp.MetricValues[0].MetricName)
		assert.Equal(t, int64(5), resp.MetricValues[0].MetricValue)
		assert.Equal(t, float64(5), resp.MetricValues[0].MetricValueFloat)

		// Test ratePerMinute (100 / 20 * 60 = 300 rpm)
		resp, err = scaler.GetMetrics(context.Background(), &externalscaler.GetMetricsRequest{
			ScaledObjectRef: &externalscaler.ScaledObjectRef{
				Name:      "test-app",
				Namespace: "default",
			},
			MetricName: "ratePerMinute",
		})
		require.NoError(t, err)
		require.Len(t, resp.MetricValues, 1)
		assert.Equal(t, "ratePerMinute", resp.MetricValues[0].MetricName)
		assert.Equal(t, int64(300), resp.MetricValues[0].MetricValue)
		assert.Equal(t, float64(300), resp.MetricValues[0].MetricValueFloat)
	})
}

func TestStreamIsActive(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	scaler := NewCamelMonitorScaler(fakeClient)
	err = scaler.StreamIsActive(&externalscaler.ScaledObjectRef{}, nil)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}
