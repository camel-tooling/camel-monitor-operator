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

package synthetic

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgocache "k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"

	v1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
)

var errBoom = errors.New("boom")

// failingClient wraps a client letting each test inject failures and/or a custom REST mapper.
type failingClient struct {
	client.Client

	getErr     error
	createErr  error
	restMapper meta.RESTMapper
}

func (c *failingClient) Get(ctx context.Context, key ctrl.ObjectKey, obj ctrl.Object, opts ...ctrl.GetOption) error {
	if c.getErr != nil {
		return c.getErr
	}

	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *failingClient) Create(ctx context.Context, obj ctrl.Object, opts ...ctrl.CreateOption) error {
	if c.createErr != nil {
		return c.createErr
	}

	return c.Client.Create(ctx, obj, opts...)
}

func (c *failingClient) RESTMapper() meta.RESTMapper {
	if c.restMapper != nil {
		return c.restMapper
	}

	return c.Client.RESTMapper()
}

// restMapperWithCronJob returns a REST mapper which knows the batch/v1 CronJob kind.
func restMapperWithCronJob() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{batchv1.SchemeGroupVersion})
	mapper.Add(batchv1.SchemeGroupVersion.WithKind("CronJob"), meta.RESTScopeNamespace)

	return mapper
}

// failingCache wraps the fake informers failing the informer retrieval for a given kind
// and/or decorating the informers so that registering the event handler fails.
type failingCache struct {
	*informertest.FakeInformers

	failOnCronJob       bool
	failAddEventHandler bool
}

func (c *failingCache) GetInformer(ctx context.Context, obj ctrl.Object, opts ...cache.InformerGetOption) (cache.Informer, error) {
	if _, isCron := obj.(*batchv1.CronJob); isCron && c.failOnCronJob {
		return nil, errBoom
	}

	informer, err := c.FakeInformers.GetInformer(ctx, obj, opts...)
	if err != nil {
		return nil, err
	}

	if c.failAddEventHandler {
		return &failingInformer{Informer: informer}, nil
	}

	return informer, nil
}

type failingInformer struct {
	cache.Informer
}

func (i *failingInformer) AddEventHandler(clientgocache.ResourceEventHandler) (clientgocache.ResourceEventHandlerRegistration, error) {
	return nil, errBoom
}

// notCtrlObject is a metav1.Object which is not a runtime.Object, hence not a ctrl.Object.
type notCtrlObject struct {
	metav1.ObjectMeta
}

func newMonitoredDeployment(name, monitorName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       "1234",
			Labels:    map[string]string{v1alpha1.MonitorLabel: monitorName},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "my-image:v1"}},
				},
			},
		},
	}
}

func newMonitoredCronJob(name, monitorName string) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       "5678",
			Labels:    map[string]string{v1alpha1.MonitorLabel: monitorName},
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "* * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "main", Image: "my-image:v1"}},
						},
					},
				},
			},
		},
	}
}

func fakeInformerFor(t *testing.T, c *informertest.FakeInformers, obj ctrl.Object) *controllertest.FakeInformer {
	t.Helper()

	informer, err := c.FakeInformerFor(context.TODO(), obj)
	require.NoError(t, err)

	return informer
}

func TestGetInformersDeploymentOnly(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	// The default fake REST mapper does not know the CronJob kind
	informers, err := getInformers(context.TODO(), fakeClient, &informertest.FakeInformers{})
	require.NoError(t, err)
	assert.Len(t, informers, 1)
}

func TestGetInformersWithCronJob(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)
	c := &failingClient{Client: fakeClient, restMapper: restMapperWithCronJob()}

	informers, err := getInformers(context.TODO(), c, &informertest.FakeInformers{})
	require.NoError(t, err)
	assert.Len(t, informers, 2)
}

func TestGetInformersDeploymentInformerError(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	informers, err := getInformers(context.TODO(), fakeClient, &informertest.FakeInformers{Error: errBoom})
	require.ErrorIs(t, err, errBoom)
	assert.Nil(t, informers)
}

func TestGetInformersCronJobInformerError(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)
	c := &failingClient{Client: fakeClient, restMapper: restMapperWithCronJob()}

	informers, err := getInformers(context.TODO(), c, &failingCache{FakeInformers: &informertest.FakeInformers{}, failOnCronJob: true})
	require.ErrorIs(t, err, errBoom)
	assert.Nil(t, informers)
}

func TestManageSyntheticCamelMonitorsInformerError(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	err = ManageSyntheticCamelMonitors(context.TODO(), fakeClient, &informertest.FakeInformers{Error: errBoom})
	require.ErrorIs(t, err, errBoom)
}

func TestManageSyntheticCamelMonitorsAddEventHandlerError(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	err = ManageSyntheticCamelMonitors(context.TODO(), fakeClient,
		&failingCache{FakeInformers: &informertest.FakeInformers{}, failAddEventHandler: true})
	require.ErrorIs(t, err, errBoom)
}

func TestManageSyntheticCamelMonitorsDeploymentLifecycle(t *testing.T) {
	ctx := context.TODO()
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)
	informers := &informertest.FakeInformers{}

	require.NoError(t, ManageSyntheticCamelMonitors(ctx, fakeClient, informers))
	deployInformer := fakeInformerFor(t, informers, &appsv1.Deployment{})

	// Add event: the label is set on a Deployment, a CamelMonitor is created
	deploy := newMonitoredDeployment("my-deploy", "app-a")
	deployInformer.Add(deploy)
	created, err := getSyntheticCamelMonitor(ctx, fakeClient, "default", "app-a")
	require.NoError(t, err)
	assert.Equal(t, "my-deploy", created.GetAnnotations()[v1alpha1.MonitorImportedNameLabel])
	assert.Equal(t, "Deployment", created.GetAnnotations()[v1alpha1.MonitorImportedKindLabel])

	// Update event: no label drift, nothing changes
	deployInformer.Update(deploy, deploy)
	_, err = getSyntheticCamelMonitor(ctx, fakeClient, "default", "app-a")
	require.NoError(t, err)

	// Update event: label value drift, the old CamelMonitor is replaced by a new one
	drifted := newMonitoredDeployment("my-deploy", "app-b")
	deployInformer.Update(deploy, drifted)
	_, err = getSyntheticCamelMonitor(ctx, fakeClient, "default", "app-a")
	require.Error(t, err)
	_, err = getSyntheticCamelMonitor(ctx, fakeClient, "default", "app-b")
	require.NoError(t, err)

	// Delete event: the label is removed, the CamelMonitor is deleted
	deployInformer.Delete(drifted)
	_, err = getSyntheticCamelMonitor(ctx, fakeClient, "default", "app-b")
	require.Error(t, err)
}

func TestManageSyntheticCamelMonitorsCronJobAdd(t *testing.T) {
	ctx := context.TODO()
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)
	c := &failingClient{Client: fakeClient, restMapper: restMapperWithCronJob()}
	informers := &informertest.FakeInformers{}

	require.NoError(t, ManageSyntheticCamelMonitors(ctx, c, informers))
	// Both Deployment and CronJob informers must be registered
	assert.Len(t, informers.InformersByGVK, 2)

	fakeInformerFor(t, informers, &batchv1.CronJob{}).Add(newMonitoredCronJob("my-cron", "cron-app"))
	created, err := getSyntheticCamelMonitor(ctx, fakeClient, "default", "cron-app")
	require.NoError(t, err)
	assert.Equal(t, "my-cron", created.GetAnnotations()[v1alpha1.MonitorImportedNameLabel])
	assert.Equal(t, "CronJob", created.GetAnnotations()[v1alpha1.MonitorImportedKindLabel])
}

func TestManageSyntheticCamelMonitorsIgnoresNonCtrlObjects(t *testing.T) {
	ctx := context.TODO()
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)
	informers := &informertest.FakeInformers{}

	require.NoError(t, ManageSyntheticCamelMonitors(ctx, fakeClient, informers))
	deployInformer := fakeInformerFor(t, informers, &appsv1.Deployment{})

	invalid := &notCtrlObject{ObjectMeta: metav1.ObjectMeta{
		Name:      "invalid",
		Namespace: "default",
		Labels:    map[string]string{v1alpha1.MonitorLabel: "invalid-app"},
	}}
	valid := newMonitoredDeployment("my-deploy", "app-a")

	// None of the handlers must act on an object which is not a ctrl.Object
	deployInformer.Add(invalid)
	deployInformer.Update(invalid, valid)
	deployInformer.Update(valid, invalid)
	deployInformer.Delete(invalid)

	cmons := &v1alpha1.CamelMonitorList{}
	require.NoError(t, fakeClient.List(ctx, cmons, ctrl.InNamespace("default")))
	assert.Empty(t, cmons.Items)
}

func TestOnAddGetError(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)
	c := &failingClient{Client: fakeClient, getErr: errBoom}

	// The lookup of the existing CamelMonitor fails with an error other than NotFound: nothing is created
	onAdd(context.TODO(), c, newMonitoredDeployment("my-deploy", "app-a"))

	cmons := &v1alpha1.CamelMonitorList{}
	require.NoError(t, fakeClient.List(context.TODO(), cmons, ctrl.InNamespace("default")))
	assert.Empty(t, cmons.Items)
}

func TestCreateMonitorCreateError(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)
	c := &failingClient{Client: fakeClient, createErr: errBoom}

	createMonitor(context.TODO(), c, newMonitoredDeployment("my-deploy", "app-a"), "app-a", "")

	cmons := &v1alpha1.CamelMonitorList{}
	require.NoError(t, fakeClient.List(context.TODO(), cmons, ctrl.InNamespace("default")))
	assert.Empty(t, cmons.Items)
}

func TestCreateSyntheticCamelMonitorSuffix(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)
	app := v1alpha1.NewCamelMonitor("default", "my-app")

	require.NoError(t, createSyntheticCamelMonitor(context.TODO(), fakeClient, &app, "-suffix"))
	assert.Equal(t, "my-app-suffix", app.GetName())
	_, err = getSyntheticCamelMonitor(context.TODO(), fakeClient, "default", "my-app-suffix")
	require.NoError(t, err)
}

func TestOnDeleteNotFound(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)

	// The CamelMonitor does not exist: the error is logged and nothing else happens
	onDelete(context.TODO(), fakeClient, newMonitoredDeployment("my-deploy", "missing-app"))

	_, err = getSyntheticCamelMonitor(context.TODO(), fakeClient, "default", "missing-app")
	require.Error(t, err)
}
