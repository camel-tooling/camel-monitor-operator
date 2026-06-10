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

	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAddPrometheusRule_Success(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	err = addPrometheusRuleAlerts(context.TODO(), fakeClient, "my-ns")
	require.NoError(t, err)

	pr := &monitoringv1.PrometheusRule{}
	err = fakeClient.Get(context.TODO(), ctrl.ObjectKey{
		Name:      "camel-monitor-alerts",
		Namespace: "my-ns",
	}, pr)

	require.NoError(t, err)
	assert.Len(t, pr.Spec.Groups, 1)
	assert.Len(t, pr.Spec.Groups[0].Rules, 1)
	assert.Equal(t, "CamelHighFailureRateCritical", pr.Spec.Groups[0].Rules[0].Alert)
	assert.Contains(t, pr.Spec.Groups[0].Rules[0].Expr.StrVal, "camel_exchanges_failed_total")
	assert.Contains(t, pr.Spec.Groups[0].Rules[0].Expr.StrVal, "camel_exchanges_total")
}
