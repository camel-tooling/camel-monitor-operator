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
	"fmt"
	"net/http"
	"reflect"
	"time"

	v1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/kubernetes"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/log"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	clientgocache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	controller = true
)

// ManageSyntheticCamelMonitors is the controller for synthetic Camel Applications. Consider that the lifecycle of the objects are driven
// by the way we are monitoring them. Since we're filtering by some label in the cached client, you must consider an add, update or delete
// accordingly, ie, when the user label the resource, then it is considered as an add, when it removes the label, it is considered as a delete.
func ManageSyntheticCamelMonitors(ctx context.Context, c client.Client, cache cache.Cache) error {
	informers, err := getInformers(ctx, c, cache)
	if err != nil {
		return err
	}

	for _, informer := range informers {
		_, err := informer.AddEventHandler(clientgocache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				ctrlObj, ok := obj.(ctrl.Object)
				if !ok {
					log.Error(fmt.Errorf("type assertion failed: %v", obj), "Failed to retrieve Object on add event")

					return
				}

				onAdd(ctx, c, ctrlObj)
			},
			DeleteFunc: func(obj any) {
				ctrlObj, ok := obj.(ctrl.Object)
				if !ok {
					log.Errorf(fmt.Errorf("type assertion failed: %v", obj), "Failed to retrieve Object on delete event")

					return
				}

				onDelete(ctx, c, ctrlObj)
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func onAdd(ctx context.Context, c client.Client, ctrlObj ctrl.Object) {
	log.Infof("Detected a new resource named %s in namespace %s", ctrlObj.GetName(), ctrlObj.GetNamespace())
	appName := ctrlObj.GetLabels()[platform.GetMonitorLabelSelector()]

	existingApp, err := getSyntheticCamelMonitor(ctx, c, ctrlObj.GetNamespace(), appName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			createMonitor(ctx, c, ctrlObj, appName, "")
		} else {
			log.Errorf(err, "Some error happened while loading a Camel Monitor %s", appName)
		}
	} else if existingApp.Annotations[v1alpha1.MonitorImportedNameLabel] != ctrlObj.GetName() {
		log.Infof("A Camel Monitor named %s was already created. Creating a new revision.", appName)
		createMonitor(ctx, c, ctrlObj, appName, "-"+ctrlObj.GetName())
	} else {
		// Do nothing, the app was already imported
		log.Infof("Resource %s has already a CamelMonitor associated.", ctrlObj.GetName())
	}
}

func createMonitor(ctx context.Context, c client.Client, ctrlObj ctrl.Object, appName, suffix string) {
	adapter, err := NonManagedCamelMonitorlicationFactory(ctrlObj)
	if err != nil {
		log.Errorf(err, "Some error happened while creating a Camel application adapter for %s", appName)

		return
	}

	app := adapter.CamelMonitor(ctx, c)
	if err = createSyntheticCamelMonitor(ctx, c, app, suffix); err != nil {
		log.Errorf(err, "Some error happened while creating a Camel Monitor %s", appName)

		return
	}

	log.Infof("Created a Camel Monitor %s after %s resource object named %s", app.GetName(),
		app.Annotations[v1alpha1.MonitorImportedKindLabel], ctrlObj.GetName())
}

func onDelete(ctx context.Context, c client.Client, ctrlObj ctrl.Object) {
	appName := ctrlObj.GetLabels()[platform.GetMonitorLabelSelector()]
	// Importing label removed
	err := deleteSyntheticCamelMonitor(ctx, c, ctrlObj.GetNamespace(), appName)
	if err != nil {
		log.Errorf(err, "Some error happened while deleting a Camel Monitor %s", appName)

		return
	}

	log.Infof("Deleted Camel Monitor %s", appName)
}

func getInformers(ctx context.Context, cl client.Client, c cache.Cache) ([]cache.Informer, error) {
	deploy, err := c.GetInformer(ctx, &appsv1.Deployment{})
	if err != nil {
		return nil, err
	}

	informers := []cache.Informer{deploy}
	// Watch for the CronJob conditionally
	if ok, err := kubernetes.IsAPIResourceInstalled(cl, batchv1.SchemeGroupVersion.String(), reflect.TypeFor[batchv1.CronJob]().Name()); ok && err == nil {
		cron, err := c.GetInformer(ctx, &batchv1.CronJob{})
		if err != nil {
			return nil, err
		}

		informers = append(informers, cron)
	}

	return informers, nil
}

func getSyntheticCamelMonitor(ctx context.Context, c client.Client, namespace, name string) (*v1alpha1.CamelMonitor, error) {
	app := v1alpha1.NewCamelMonitor(namespace, name)
	err := c.Get(ctx, ctrl.ObjectKeyFromObject(&app), &app)

	return &app, err
}

// createSyntheticCamelMonitor creates a new CamelMonitor, with the possibility to add a suffix it.
func createSyntheticCamelMonitor(ctx context.Context, c client.Client, app *v1alpha1.CamelMonitor, suffix string) error {
	if suffix != "" {
		app.Name += suffix
	}

	return c.Create(ctx, app, ctrl.FieldOwner("camel-monitor-operator"))
}

func deleteSyntheticCamelMonitor(ctx context.Context, c client.Client, namespace, name string) error {
	// As the Integration label was removed, we don't know which is the Synthetic Camel Application to remove
	app := v1alpha1.NewCamelMonitor(namespace, name)

	return c.Delete(ctx, &app)
}

// NonManagedCamelMonitorlicationAdapter represents a Camel application built and deployed outside the operator lifecycle.
type NonManagedCamelMonitorlicationAdapter interface {
	// CamelMonitor returns a CamelMonitor resource fed by the Camel application adapter.
	CamelMonitor(ctx context.Context, c client.Client) *v1alpha1.CamelMonitor
	// GetAppPhase returns the phase of the backing Camel application.
	GetAppPhase(ctx context.Context, c client.Client) v1alpha1.CamelMonitorPhase
	// GetAppImage returns the container image of the backing Camel application.
	GetAppImage() string
	// GetReplicas returns the number of desired replicas for the backing Camel application.
	GetReplicas() *int32
	// GetPods returns the actual Pods backing the Camel application.
	GetPods(ctx context.Context, c client.Client) ([]v1alpha1.PodInfo, error)
	// GetAnnotations returns the backing deployment object annotations.
	GetAnnotations() map[string]string
	// GetMatchLabelsSelector returns the labels selector used to select Pods belonging to the backing application.
	GetMatchLabelsSelector() map[string]string
	// SetMonitoringCondition sets the health and monitoring conditions on the target app.
	SetMonitoringCondition(app, targetApp *v1alpha1.CamelMonitor, pods []v1alpha1.PodInfo)
	// GetResourcesLimits returns the resource limits of the backing Camel application.
	GetResourcesLimits() corev1.ResourceList
}

func NonManagedCamelMonitorlicationFactory(obj ctrl.Object) (NonManagedCamelMonitorlicationAdapter, error) {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	deploy, ok := obj.(*appsv1.Deployment)
	if ok {
		return &nonManagedCamelDeployment{deploy: deploy, httpClient: httpClient}, nil
	}

	cronjob, ok := obj.(*batchv1.CronJob)
	if ok {
		return &nonManagedCamelCronjob{cron: cronjob, httpClient: httpClient}, nil
	}

	return nil, fmt.Errorf("unsupported %s object kind", obj.GetObjectKind().GroupVersionKind().Kind)
}
