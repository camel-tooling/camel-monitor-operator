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

package console

import (
	"context"
	"testing"

	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	consolev1 "github.com/openshift/api/console/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNamespace = "test-ns"
	testImage     = "console-plugin:latest"
)

var testOwnerRef = metav1.OwnerReference{
	APIVersion: "apps/v1",
	Kind:       "Deployment",
	Name:       platform.GetOperatorDeploymentName(),
	UID:        "test-uid-123",
	Controller: new(true),
}

func newTestReconciler(t *testing.T, objs ...ctrl.Object) *reconciler {
	t.Helper()

	fakeClient, err := internal.NewFakeClient(objs...)
	require.NoError(t, err)

	return &reconciler{
		client:    fakeClient,
		reader:    fakeClient,
		namespace: testNamespace,
		image:     testImage,
	}
}

func TestEnsureConfigMap_Create(t *testing.T) {
	r := newTestReconciler(t)

	err := r.ensureConfigMap(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, cm)
	require.NoError(t, err)
	assert.Equal(t, pluginName, cm.Name)
	assert.Equal(t, commonLabels(), cm.Labels)
	assert.Contains(t, cm.Data, "nginx.conf")
}

func TestEnsureConfigMap_Update(t *testing.T) {
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pluginName,
			Namespace: testNamespace,
			Labels:    map[string]string{"old": "label"},
		},
		Data: map[string]string{"nginx.conf": "old-config"},
	}
	r := newTestReconciler(t, existing)

	err := r.ensureConfigMap(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, cm)
	require.NoError(t, err)
	assert.Equal(t, commonLabels(), cm.Labels)
	assert.Contains(t, cm.Data["nginx.conf"], "9443")
}

func TestEnsureDeployment_Create(t *testing.T) {
	r := newTestReconciler(t)

	err := r.ensureDeployment(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	deploy := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, deploy)
	require.NoError(t, err)
	assert.Equal(t, pluginName, deploy.Name)
	assert.Equal(t, commonLabels(), deploy.Labels)
	require.Len(t, deploy.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, testImage, deploy.Spec.Template.Spec.Containers[0].Image)
}

func TestEnsureDeployment_Update(t *testing.T) {
	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pluginName,
			Namespace: testNamespace,
			Labels:    map[string]string{"old": "label"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": pluginName},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: pluginName, Image: "old-image:v0"},
					},
				},
			},
		},
	}
	r := newTestReconciler(t, existing)

	err := r.ensureDeployment(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	deploy := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, deploy)
	require.NoError(t, err)
	assert.Equal(t, commonLabels(), deploy.Labels)
	assert.Equal(t, testImage, deploy.Spec.Template.Spec.Containers[0].Image)
}

func TestEnsureService_Create(t *testing.T) {
	r := newTestReconciler(t)

	err := r.ensureService(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	svc := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, svc)
	require.NoError(t, err)
	assert.Equal(t, pluginName, svc.Name)
	assert.Equal(t, commonLabels(), svc.Labels)
	assert.Equal(t, certSecretName, svc.Annotations["service.alpha.openshift.io/serving-cert-secret-name"])
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(pluginPort), svc.Spec.Ports[0].Port)
}

func TestEnsureService_Update(t *testing.T) {
	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pluginName,
			Namespace: testNamespace,
			Labels:    map[string]string{"old": "label"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Port: 8080},
			},
		},
	}
	r := newTestReconciler(t, existing)

	err := r.ensureService(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	svc := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, svc)
	require.NoError(t, err)
	assert.Equal(t, commonLabels(), svc.Labels)
	assert.Equal(t, certSecretName, svc.Annotations["service.alpha.openshift.io/serving-cert-secret-name"])
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(pluginPort), svc.Spec.Ports[0].Port)
}

func TestEnsureConsolePlugin_Create(t *testing.T) {
	r := newTestReconciler(t)

	err := r.ensureConsolePlugin(context.TODO())
	require.NoError(t, err)

	cp := &consolev1.ConsolePlugin{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName}, cp)
	require.NoError(t, err)
	assert.Equal(t, pluginName, cp.Name)
	assert.Equal(t, commonLabels(), cp.Labels)
	assert.Equal(t, consolev1.Service, cp.Spec.Backend.Type)
	require.NotNil(t, cp.Spec.Backend.Service)
	assert.Equal(t, testNamespace, cp.Spec.Backend.Service.Namespace)
}

func TestEnsureConsolePlugin_Update(t *testing.T) {
	existing := &consolev1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name:   pluginName,
			Labels: map[string]string{"old": "label"},
		},
		Spec: consolev1.ConsolePluginSpec{
			DisplayName: "Old Name",
		},
	}
	r := newTestReconciler(t, existing)

	err := r.ensureConsolePlugin(context.TODO())
	require.NoError(t, err)

	cp := &consolev1.ConsolePlugin{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName}, cp)
	require.NoError(t, err)
	assert.Equal(t, commonLabels(), cp.Labels)
	assert.Equal(t, "Camel Dashboard Console", cp.Spec.DisplayName)
	assert.Equal(t, consolev1.Service, cp.Spec.Backend.Type)
}

func TestEnsureNetworkPolicy_Create(t *testing.T) {
	r := newTestReconciler(t)

	err := r.ensureNetworkPolicy(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, np)
	require.NoError(t, err)
	assert.Equal(t, pluginName, np.Name)
	assert.Equal(t, commonLabels(), np.Labels)
	require.Len(t, np.Spec.Ingress, 1)
	require.Len(t, np.Spec.Ingress[0].Ports, 1)
	assert.Equal(t, int32(pluginPort), np.Spec.Ingress[0].Ports[0].Port.IntVal)
}

func TestEnsureNetworkPolicy_Update(t *testing.T) {
	existing := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pluginName,
			Namespace: testNamespace,
			Labels:    map[string]string{"old": "label"},
		},
		Spec: networkingv1.NetworkPolicySpec{},
	}
	r := newTestReconciler(t, existing)

	err := r.ensureNetworkPolicy(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, np)
	require.NoError(t, err)
	assert.Equal(t, commonLabels(), np.Labels)
	assert.Equal(t, int32(pluginPort), np.Spec.Ingress[0].Ports[0].Port.IntVal)
}

func TestEnsureAllResources(t *testing.T) {
	r := newTestReconciler(t)

	err := r.ensureAllResources(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, cm)
	assert.NoError(t, err)

	deploy := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, deploy)
	assert.NoError(t, err)

	svc := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, svc)
	assert.NoError(t, err)

	cp := &consolev1.ConsolePlugin{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName}, cp)
	assert.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, np)
	assert.NoError(t, err)
}

func TestEnsureAllResources_OwnerRefs(t *testing.T) {
	r := newTestReconciler(t)

	err := r.ensureAllResources(context.TODO(), testOwnerRef)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, cm)
	require.NoError(t, err)
	require.Len(t, cm.OwnerReferences, 1)
	assert.Equal(t, platform.GetOperatorDeploymentName(), cm.OwnerReferences[0].Name)
	assert.Equal(t, types.UID("test-uid-123"), cm.OwnerReferences[0].UID)

	deploy := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, deploy)
	require.NoError(t, err)
	require.Len(t, deploy.OwnerReferences, 1)
	assert.Equal(t, platform.GetOperatorDeploymentName(), deploy.OwnerReferences[0].Name)

	svc := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, svc)
	require.NoError(t, err)
	require.Len(t, svc.OwnerReferences, 1)
	assert.Equal(t, platform.GetOperatorDeploymentName(), svc.OwnerReferences[0].Name)

	cp := &consolev1.ConsolePlugin{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName}, cp)
	require.NoError(t, err)
	assert.Empty(t, cp.OwnerReferences)

	np := &networkingv1.NetworkPolicy{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName, Namespace: testNamespace}, np)
	require.NoError(t, err)
	require.Len(t, np.OwnerReferences, 1)
	assert.Equal(t, platform.GetOperatorDeploymentName(), np.OwnerReferences[0].Name)
}

func TestHandleUninstall(t *testing.T) {
	cp := &consolev1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name: pluginName,
		},
	}

	r := newTestReconciler(t, cp)

	err := r.handleUninstall(context.TODO())
	require.NoError(t, err)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pluginName}, &consolev1.ConsolePlugin{})
	assert.True(t, k8serrors.IsNotFound(err))
}
