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

	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNamespace = "test-ns"
	testPort      = 9666
)

var testOwnerRef = metav1.OwnerReference{
	APIVersion: "apps/v1",
	Kind:       "Deployment",
	Name:       platform.DefaultOperatorName,
	UID:        "test-uid-abc",
	Controller: new(true),
}

// newTestServiceController builds a ServiceController backed by a fake client seeded with objs.
// The mgr field is left nil because the tests call ensureService/Start directly and never
// reach the mgr.Add(NewServer(...)) path (Start is called only in TestStart_* which seeds
// a real Deployment, and the nil-mgr guard is covered by keeping mgr out of the path under test).
func newTestServiceController(t *testing.T, objs ...ctrl.Object) *ServiceController {
	t.Helper()

	fakeClient, err := internal.NewFakeClient(objs...)
	require.NoError(t, err)

	return &ServiceController{
		client:       fakeClient,
		reader:       fakeClient,
		namespace:    testNamespace,
		port:         testPort,
		scalerClient: fakeClient,
	}
}

// --------------------------------------------------------------------------
// buildScalerService
// --------------------------------------------------------------------------

func TestBuildScalerService(t *testing.T) {
	svc := buildScalerService(testNamespace, testPort)

	assert.Equal(t, ScalerServiceName, svc.Name)
	assert.Equal(t, testNamespace, svc.Namespace)
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	assert.Equal(t, map[string]string{"name": platform.DefaultOperatorName}, svc.Spec.Selector)
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, "grpc", svc.Spec.Ports[0].Name)
	assert.Equal(t, int32(testPort), svc.Spec.Ports[0].Port)
	assert.Equal(t, intstr.FromInt32(int32(testPort)), svc.Spec.Ports[0].TargetPort)
	assert.Equal(t, corev1.ProtocolTCP, svc.Spec.Ports[0].Protocol)
}

// --------------------------------------------------------------------------
// ensureService — create path
// --------------------------------------------------------------------------

func TestEnsureService_Create(t *testing.T) {
	sc := newTestServiceController(t)

	err := sc.ensureService(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	svc := &corev1.Service{}
	err = sc.client.Get(context.TODO(), types.NamespacedName{
		Name:      ScalerServiceName,
		Namespace: testNamespace,
	}, svc)
	require.NoError(t, err)

	assert.Equal(t, ScalerServiceName, svc.Name)
	assert.Equal(t, testNamespace, svc.Namespace)
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	assert.Equal(t, int32(testPort), svc.Spec.Ports[0].Port)
}

func TestEnsureService_Create_SetsOwnerReference(t *testing.T) {
	sc := newTestServiceController(t)

	err := sc.ensureService(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	svc := &corev1.Service{}
	err = sc.client.Get(context.TODO(), types.NamespacedName{
		Name:      ScalerServiceName,
		Namespace: testNamespace,
	}, svc)
	require.NoError(t, err)

	require.Len(t, svc.OwnerReferences, 1)
	assert.Equal(t, platform.DefaultOperatorName, svc.OwnerReferences[0].Name)
	assert.Equal(t, types.UID("test-uid-abc"), svc.OwnerReferences[0].UID)
	assert.Equal(t, "Deployment", svc.OwnerReferences[0].Kind)
	require.NotNil(t, svc.OwnerReferences[0].Controller)
	assert.True(t, *svc.OwnerReferences[0].Controller)
}

// --------------------------------------------------------------------------
// ensureService — update path
// --------------------------------------------------------------------------

func TestEnsureService_Update_UpdatesPortAndOwner(t *testing.T) {
	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ScalerServiceName,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{Name: "grpc", Port: 1234, Protocol: corev1.ProtocolTCP},
			},
		},
	}

	sc := newTestServiceController(t, existing)

	updatedOwner := metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       platform.DefaultOperatorName,
		UID:        "new-uid-xyz",
		Controller: new(true),
	}

	err := sc.ensureService(context.TODO(), updatedOwner)
	require.NoError(t, err)

	svc := &corev1.Service{}
	err = sc.client.Get(context.TODO(), types.NamespacedName{
		Name:      ScalerServiceName,
		Namespace: testNamespace,
	}, svc)
	require.NoError(t, err)

	// Port must be updated to the controller's port.
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(testPort), svc.Spec.Ports[0].Port)

	// Owner reference must be updated.
	require.Len(t, svc.OwnerReferences, 1)
	assert.Equal(t, types.UID("new-uid-xyz"), svc.OwnerReferences[0].UID)
}

func TestEnsureService_Update_IsIdempotent(t *testing.T) {
	sc := newTestServiceController(t)

	// First call: creates
	require.NoError(t, sc.ensureService(context.TODO(), testOwnerRef))
	// Second call: updates (same values) — must not error
	require.NoError(t, sc.ensureService(context.TODO(), testOwnerRef))

	svc := &corev1.Service{}
	err := sc.client.Get(context.TODO(), types.NamespacedName{
		Name:      ScalerServiceName,
		Namespace: testNamespace,
	}, svc)
	require.NoError(t, err)
	assert.Equal(t, int32(testPort), svc.Spec.Ports[0].Port)
}

// --------------------------------------------------------------------------
// Start — operator Deployment not found
// --------------------------------------------------------------------------

func TestStart_OperatorDeploymentNotFound(t *testing.T) {
	sc := newTestServiceController(t) // no Deployment pre-seeded

	err := sc.Start(context.TODO())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --------------------------------------------------------------------------
// Start — happy path
// --------------------------------------------------------------------------

func TestStart_CreatesServiceWithOwnerRef(t *testing.T) {
	operatorDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      platform.DefaultOperatorName,
			Namespace: testNamespace,
			UID:       "deploy-uid-999",
		},
	}

	sc := newTestServiceController(t, operatorDeploy)

	err := sc.Start(context.TODO())
	require.NoError(t, err)

	svc := &corev1.Service{}
	err = sc.client.Get(context.TODO(), types.NamespacedName{
		Name:      ScalerServiceName,
		Namespace: testNamespace,
	}, svc)
	require.NoError(t, err)

	assert.Equal(t, int32(testPort), svc.Spec.Ports[0].Port)
	require.Len(t, svc.OwnerReferences, 1)
	assert.Equal(t, types.UID("deploy-uid-999"), svc.OwnerReferences[0].UID)
	assert.Equal(t, platform.DefaultOperatorName, svc.OwnerReferences[0].Name)
}
