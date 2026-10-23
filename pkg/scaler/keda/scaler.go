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
	"math"
	"strconv"
	"strings"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	logutil "github.com/camel-tooling/camel-monitor-operator/pkg/util/log"
	"github.com/kedacore/keda/v2/pkg/scalers/externalscaler"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var log = logutil.Log.WithName("keda-scaler")

const (
	MetricNameSamplingIntervalTotal = "samplingIntervalTotal"
	MetricNameRate                  = "rate"
	MetricNameRatePerSecond         = "ratePerSecond"
	MetricNameRatePerMinute         = "ratePerMinute"
	DefaultMetricName               = MetricNameSamplingIntervalTotal
	DefaultTargetSize               = int64(100)
	DefaultActivationValue          = int64(0)

	MetadataKeyCmonName              = "cmonName"
	MetadataKeyCamelMonitorName      = "camelMonitorName"
	MetadataKeyName                  = "name"
	MetadataKeyCmonNamespace         = "cmonNamespace"
	MetadataKeyCamelMonitorNamespace = "camelMonitorNamespace"
	MetadataKeyNamespace             = "namespace"
	MetadataKeyMetricName            = "metricName"
	MetadataKeyMetric                = "metric"
	MetadataKeyThreshold             = "threshold"
	MetadataKeyTargetValue           = "targetValue"
	MetadataKeyTargetSize            = "targetSize"
	MetadataKeyTarget                = "target"
	MetadataKeyActivationThreshold   = "activationThreshold"
	MetadataKeyActivationTargetValue = "activationTargetValue"
)

// scalerMetadata holds configuration extracted from the ScaledObject reference.
type scalerMetadata struct {
	cmonName            string
	cmonNamespace       string
	metricName          string
	targetSize          int64
	activationThreshold int64
}

// CamelMonitorScaler implements the KEDA ExternalScalerServer gRPC interface.
type CamelMonitorScaler struct {
	externalscaler.UnimplementedExternalScalerServer

	client client.Client
}

// NewCamelMonitorScaler creates a new CamelMonitorScaler instance.
func NewCamelMonitorScaler(client client.Client) *CamelMonitorScaler {
	return &CamelMonitorScaler{
		client: client,
	}
}

// IsActive returns whether the target resource is currently active and should not scale to zero.
func (s *CamelMonitorScaler) IsActive(ctx context.Context, ref *externalscaler.ScaledObjectRef) (*externalscaler.IsActiveResponse, error) {
	meta := extractMetadata(ref)

	var cmon v1alpha1.CamelMonitor

	err := s.client.Get(ctx, types.NamespacedName{Name: meta.cmonName, Namespace: meta.cmonNamespace}, &cmon)
	if err != nil {
		if errors.IsNotFound(err) {
			// CR does not exist yet. Return active to prevent premature scale-down to 0 while the resource is being provisioned.
			log.Debugf("CamelMonitor %s/%s not found, returning IsActive=true to prevent premature scale-down", meta.cmonNamespace, meta.cmonName)

			return &externalscaler.IsActiveResponse{Result: true}, nil
		}

		log.Errorf(err, "Failed to get CamelMonitor %s/%s", meta.cmonNamespace, meta.cmonName)

		return nil, status.Errorf(codes.Internal, "failed to get CamelMonitor %s/%s: %v", meta.cmonNamespace, meta.cmonName, err)
	}

	// If status or successRate is not populated yet, return active during initialization phase
	if cmon.Status.SuccessRate == nil {
		log.Debugf("CamelMonitor %s/%s status not ready yet, returning IsActive=true", meta.cmonNamespace, meta.cmonName)

		return &externalscaler.IsActiveResponse{Result: true}, nil
	}

	val, valFloat := resolveSamplingMetric(&cmon, meta.metricName)

	var isActive bool
	if meta.activationThreshold > 0 {
		isActive = val > meta.activationThreshold || valFloat > float64(meta.activationThreshold)
	} else {
		isActive = val > 0 || valFloat > 0
	}

	return &externalscaler.IsActiveResponse{Result: isActive}, nil
}

// StreamIsActive is not implemented; KEDA falls back to polling IsActive.
func (s *CamelMonitorScaler) StreamIsActive(_ *externalscaler.ScaledObjectRef, _ externalscaler.ExternalScaler_StreamIsActiveServer) error {
	return status.Errorf(codes.Unimplemented, "method StreamIsActive not implemented")
}

// GetMetricSpec returns the metric specification used by KEDA/HPA for scaling calculations.
func (s *CamelMonitorScaler) GetMetricSpec(_ context.Context, ref *externalscaler.ScaledObjectRef) (*externalscaler.GetMetricSpecResponse, error) {
	meta := extractMetadata(ref)

	return &externalscaler.GetMetricSpecResponse{
		MetricSpecs: []*externalscaler.MetricSpec{
			{
				MetricName:      meta.metricName,
				TargetSize:      meta.targetSize,
				TargetSizeFloat: float64(meta.targetSize),
			},
		},
	}, nil
}

// GetMetrics returns the current metric values from the CamelMonitor CR.
func (s *CamelMonitorScaler) GetMetrics(ctx context.Context, req *externalscaler.GetMetricsRequest) (*externalscaler.GetMetricsResponse, error) {
	meta := extractMetadata(req.GetScaledObjectRef())

	metricName := req.GetMetricName()
	if metricName == "" {
		metricName = meta.metricName
	}

	var cmon v1alpha1.CamelMonitor

	err := s.client.Get(ctx, types.NamespacedName{Name: meta.cmonName, Namespace: meta.cmonNamespace}, &cmon)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Debugf("CamelMonitor %s/%s not found, returning metric value 0", meta.cmonNamespace, meta.cmonName)

			return &externalscaler.GetMetricsResponse{
				MetricValues: []*externalscaler.MetricValue{
					{
						MetricName:       metricName,
						MetricValue:      0,
						MetricValueFloat: 0,
					},
				},
			}, nil
		}

		log.Errorf(err, "Failed to get CamelMonitor %s/%s", meta.cmonNamespace, meta.cmonName)

		return nil, status.Errorf(codes.Internal, "failed to get CamelMonitor %s/%s: %v", meta.cmonNamespace, meta.cmonName, err)
	}

	metricValue, metricValueFloat := resolveSamplingMetric(&cmon, metricName)

	return &externalscaler.GetMetricsResponse{
		MetricValues: []*externalscaler.MetricValue{
			{
				MetricName:       metricName,
				MetricValue:      metricValue,
				MetricValueFloat: metricValueFloat,
			},
		},
	}, nil
}

func extractMetadata(ref *externalscaler.ScaledObjectRef) scalerMetadata {
	result := scalerMetadata{
		metricName:          DefaultMetricName,
		targetSize:          DefaultTargetSize,
		activationThreshold: DefaultActivationValue,
	}

	if ref == nil {
		return result
	}

	result.cmonName = ref.GetName()
	result.cmonNamespace = ref.GetNamespace()

	if ref.ScalerMetadata == nil {
		return result
	}

	meta := ref.GetScalerMetadata()

	if val := getMetadataValue(meta, MetadataKeyCmonName, MetadataKeyCamelMonitorName, MetadataKeyName); val != "" {
		result.cmonName = val
	}

	if val := getMetadataValue(meta, MetadataKeyCmonNamespace, MetadataKeyCamelMonitorNamespace, MetadataKeyNamespace); val != "" {
		result.cmonNamespace = val
	}

	if val := getMetadataValue(meta, MetadataKeyMetricName, MetadataKeyMetric); val != "" {
		result.metricName = val
	}

	if val := getMetadataValue(meta, MetadataKeyThreshold, MetadataKeyTargetValue, MetadataKeyTargetSize, MetadataKeyTarget); val != "" {
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err == nil && parsed > 0 {
			result.targetSize = parsed
		} else {
			parsedFloat, err := strconv.ParseFloat(val, 64)
			if err == nil && parsedFloat > 0 {
				result.targetSize = int64(math.Round(parsedFloat))
			}
		}
	}

	if val := getMetadataValue(meta, MetadataKeyActivationThreshold, MetadataKeyActivationTargetValue); val != "" {
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err == nil {
			result.activationThreshold = parsed
		} else {
			parsedFloat, err := strconv.ParseFloat(val, 64)
			if err == nil {
				result.activationThreshold = int64(math.Round(parsedFloat))
			}
		}
	}

	return result
}

func getMetadataValue(meta map[string]string, keys ...string) string {
	for _, key := range keys {
		if val, ok := meta[key]; ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}

	return ""
}

func resolveSamplingMetric(cmon *v1alpha1.CamelMonitor, metricName string) (int64, float64) {
	if cmon == nil || cmon.Status.SuccessRate == nil {
		return 0, 0
	}

	total := float64(cmon.Status.SuccessRate.SamplingIntervalTotal)

	duration := platform.GetPollingInterval()
	if cmon.Status.SuccessRate.SamplingIntervalDuration != nil && *cmon.Status.SuccessRate.SamplingIntervalDuration > 0 {
		duration = *cmon.Status.SuccessRate.SamplingIntervalDuration
	}

	durationSec := duration.Seconds()
	if durationSec <= 0 {
		durationSec = 1
	}

	normMetric := strings.ToLower(strings.TrimSpace(metricName))
	switch normMetric {
	case "rate", "ratepersecond", "rate_per_second", "rps", "exchangespersecond", "exchanges_per_second":
		rate := total / durationSec

		return int64(math.Round(rate)), rate
	case "rateperminute", "rate_per_minute", "rpm", "exchangesperminute", "exchanges_per_minute":
		ratePerMin := (total / durationSec) * 60

		return int64(math.Round(ratePerMin)), ratePerMin
	default:
		// Default to samplingIntervalTotal
		return int64(cmon.Status.SuccessRate.SamplingIntervalTotal), total
	}
}
