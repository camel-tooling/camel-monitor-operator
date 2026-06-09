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

package internal

import (
	camelv1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	integreatlyv1beta1 "github.com/grafana/grafana-operator/v5/api/v1beta1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type TestClient struct {
	ctrl.Client
}

func (c *TestClient) Status() ctrl.SubResourceWriter {
	return &FakeStatusWriter{client: c}
}

// NewFakeClient creates a client to use simulating Kubernetes objects in unit test. Mind that
// you need to provide CRD objects (camelObjs) separately from core objects.
func NewFakeClient(objs ...ctrl.Object) (client.Client, error) {
	scheme := runtime.NewScheme()

	err := corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = appsv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = batchv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = camelv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = monitoringv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = integreatlyv1beta1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	// NOTE: register any more type required by the unit tests

	ctrlClient := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()

	return &TestClient{
		Client: ctrlClient,
	}, nil
}
