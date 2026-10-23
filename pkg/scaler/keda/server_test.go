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
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	"github.com/kedacore/keda/v2/pkg/scalers/externalscaler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func TestServerLifecycleAndGRPC(t *testing.T) {
	port, err := getFreePort()
	require.NoError(t, err)

	cmon := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-camel-app",
			Namespace: "default",
		},
		Status: v1alpha1.CamelMonitorStatus{
			Phase: v1alpha1.CamelMonitorPhaseRunning,
			SuccessRate: &v1alpha1.SLIExchangeSuccessRate{
				SamplingIntervalTotal: 88,
			},
		},
	}
	fakeClient, err := internal.NewFakeClient(cmon)
	require.NoError(t, err)

	server := NewServer(fakeClient, port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start(ctx)
	}()

	// Wait for server to start
	connAddr := fmt.Sprintf("127.0.0.1:%d", port)
	var conn *grpc.ClientConn
	for range 20 {
		conn, err = grpc.NewClient(connAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			client := externalscaler.NewExternalScalerClient(conn)
			_, testErr := client.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
				Name:      "test-camel-app",
				Namespace: "default",
			})
			if testErr == nil {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, err)
	defer conn.Close()

	client := externalscaler.NewExternalScalerClient(conn)

	// Test IsActive
	isActiveResp, err := client.IsActive(context.Background(), &externalscaler.ScaledObjectRef{
		Name:      "test-camel-app",
		Namespace: "default",
	})
	require.NoError(t, err)
	assert.True(t, isActiveResp.Result)

	// Test GetMetricSpec
	metricSpecResp, err := client.GetMetricSpec(context.Background(), &externalscaler.ScaledObjectRef{
		Name:      "test-camel-app",
		Namespace: "default",
		ScalerMetadata: map[string]string{
			MetadataKeyThreshold: "50",
		},
	})
	require.NoError(t, err)
	require.Len(t, metricSpecResp.MetricSpecs, 1)
	assert.Equal(t, int64(50), metricSpecResp.MetricSpecs[0].TargetSize)

	// Test GetMetrics
	metricsResp, err := client.GetMetrics(context.Background(), &externalscaler.GetMetricsRequest{
		ScaledObjectRef: &externalscaler.ScaledObjectRef{
			Name:      "test-camel-app",
			Namespace: "default",
		},
	})
	require.NoError(t, err)
	require.Len(t, metricsResp.MetricValues, 1)
	assert.Equal(t, int64(88), metricsResp.MetricValues[0].MetricValue)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errChan:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop in time")
	}
}
