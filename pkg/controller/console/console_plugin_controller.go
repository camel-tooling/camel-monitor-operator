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
	"os"
	"strings"

	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/kubernetes"
	logutil "github.com/camel-tooling/camel-monitor-operator/pkg/util/log"
	consolev1 "github.com/openshift/api/console/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	consoleImageEnv        = "OPENSHIFT_CONSOLE_PLUGIN_IMAGE"
	operatorDeploymentName = "camel-monitor-operator"
	operatorCSVPrefix      = "camel-monitor-operator."
)

var log = logutil.Log.WithName("console")

func Add(ctx context.Context, mgr manager.Manager, c client.Client) error {
	if !platform.IsCurrentOperatorGlobal() {
		log.Info("Operator is not global, skipping console deployment (ConsolePlugin is cluster-scoped)")

		return nil
	}

	hasConsoleAPI, err := kubernetes.IsAPIResourceInstalled(c, "console.openshift.io/v1", "ConsolePlugin")
	if err != nil {
		return err
	}

	if !hasConsoleAPI {
		log.Info("ConsolePlugin CRD not found, skipping console deployment")

		return nil
	}

	image := os.Getenv(consoleImageEnv)
	if image == "" {
		log.Info("OPENSHIFT_CONSOLE_PLUGIN_IMAGE not set, skipping console deployment")

		return nil
	}

	namespace := platform.GetOperatorNamespace()
	if namespace == "" {
		log.Info("Operator namespace not determined, skipping console deployment")

		return nil
	}

	r := &reconciler{
		client:    mgr.GetClient(),
		reader:    mgr.GetAPIReader(),
		namespace: namespace,
		image:     image,
	}

	nameFilter := predicate.NewPredicateFuncs(func(obj ctrl.Object) bool {
		return obj.GetName() == pluginName
	})

	mapToConsole := handler.EnqueueRequestsFromMapFunc(
		func(_ context.Context, _ ctrl.Object) []reconcile.Request {
			return []reconcile.Request{{
				Name:      pluginName,
				Namespace: namespace}}
		},
	)

	csvFilter := predicate.NewPredicateFuncs(func(obj ctrl.Object) bool {
		return strings.HasPrefix(obj.GetName(), operatorCSVPrefix)
	})

	bootstrapCh := make(chan event.GenericEvent, 1)

	err = builder.ControllerManagedBy(mgr).
		Named("console-controller").
		Watches(&appsv1.Deployment{}, mapToConsole, builder.WithPredicates(nameFilter)).
		Watches(&consolev1.ConsolePlugin{}, mapToConsole, builder.WithPredicates(nameFilter)).
		Watches(&operatorsv1alpha1.ClusterServiceVersion{}, mapToConsole, builder.WithPredicates(csvFilter)).
		WatchesRawSource(source.Channel(bootstrapCh, mapToConsole)).
		Complete(r)
	if err != nil {
		return err
	}

	return mgr.Add(manager.RunnableFunc(func(_ context.Context) error {
		log.Info("Triggering initial console reconciliation")

		bootstrapCh <- event.GenericEvent{
			Object: &corev1.ConfigMap{
				Name:      pluginName,
				Namespace: namespace},
		}

		return nil
	}))
}

type reconciler struct {
	client    ctrl.Client
	reader    ctrl.Reader
	namespace string
	image     string
}

func (r *reconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	log.Info("Reconciling console resources")

	if r.isUninstalling(ctx) {
		return reconcile.Result{}, r.handleUninstall(ctx)
	}

	operatorDeploy := &appsv1.Deployment{}

	err := r.reader.Get(ctx, types.NamespacedName{
		Name:      operatorDeploymentName,
		Namespace: r.namespace,
	}, operatorDeploy)
	if err != nil {
		return reconcile.Result{}, err
	}

	ownerRef := metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       operatorDeploy.Name,
		UID:        operatorDeploy.UID,
		Controller: new(true),
	}

	err = r.ensureAllResources(ctx, ownerRef)
	if err != nil {
		log.Error(err, "Failed console reconciliation")

		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *reconciler) isUninstalling(ctx context.Context) bool {
	csvList := &operatorsv1alpha1.ClusterServiceVersionList{}

	err := r.reader.List(ctx, csvList, ctrl.InNamespace(r.namespace))
	if err != nil {
		return false
	}

	found := false

	for i := range csvList.Items {
		if strings.HasPrefix(csvList.Items[i].Name, operatorCSVPrefix) {
			found = true

			if csvList.Items[i].DeletionTimestamp == nil {
				return false
			}
		}
	}

	return found
}

func (r *reconciler) handleUninstall(ctx context.Context) error {
	log.Info("Operator CSV being deleted, cleaning up console resources")

	cp := &consolev1.ConsolePlugin{Name: pluginName}

	err := r.client.Delete(ctx, cp)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	return nil
}

func setOwnerRef(obj ctrl.Object, ownerRef metav1.OwnerReference) {
	obj.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
}

// createOrUpdate uses the direct API reader to avoid the manager cache, which would
// create cluster-scoped informers for types like ConfigMap and Service that the
// operator lacks cluster-level RBAC for.
func (r *reconciler) createOrUpdate(ctx context.Context, obj ctrl.Object, f controllerutil.MutateFn) error {
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}

	err := r.reader.Get(ctx, key, obj)
	if k8serrors.IsNotFound(err) {
		err = f()
		if err != nil {
			return err
		}

		return r.client.Create(ctx, obj)
	}

	if err != nil {
		return err
	}

	err = f()
	if err != nil {
		return err
	}

	return r.client.Update(ctx, obj)
}

func (r *reconciler) ensureAllResources(ctx context.Context, ownerRef metav1.OwnerReference) error {
	err := r.ensureConfigMap(ctx, ownerRef)
	if err != nil {
		return err
	}

	err = r.ensureDeployment(ctx, ownerRef)
	if err != nil {
		return err
	}

	err = r.ensureService(ctx, ownerRef)
	if err != nil {
		return err
	}

	return r.ensureConsolePlugin(ctx)
}

func (r *reconciler) ensureConfigMap(ctx context.Context, ownerRef metav1.OwnerReference) error {
	cm := &corev1.ConfigMap{
		Name:      pluginName,
		Namespace: r.namespace}

	return r.createOrUpdate(ctx, cm, func() error {
		cm.Labels = commonLabels()
		cm.Data = map[string]string{"nginx.conf": nginxConfig()}
		setOwnerRef(cm, ownerRef)

		return nil
	})
}

func (r *reconciler) ensureDeployment(ctx context.Context, ownerRef metav1.OwnerReference) error {
	deploy := &appsv1.Deployment{
		Name:      pluginName,
		Namespace: r.namespace}

	return r.createOrUpdate(ctx, deploy, func() error {
		d := deployment(r.namespace, r.image)
		deploy.Labels = d.Labels
		deploy.Spec = d.Spec
		setOwnerRef(deploy, ownerRef)

		return nil
	})
}

func (r *reconciler) ensureService(ctx context.Context, ownerRef metav1.OwnerReference) error {
	svc := &corev1.Service{
		Name:      pluginName,
		Namespace: r.namespace}

	return r.createOrUpdate(ctx, svc, func() error {
		s := service(r.namespace)
		svc.Labels = s.Labels
		svc.Annotations = s.Annotations
		svc.Spec.Ports = s.Spec.Ports
		svc.Spec.Selector = s.Spec.Selector
		svc.Spec.Type = s.Spec.Type
		setOwnerRef(svc, ownerRef)

		return nil
	})
}

func (r *reconciler) ensureConsolePlugin(ctx context.Context) error {
	cp := &consolev1.ConsolePlugin{
		Name: pluginName}

	return r.createOrUpdate(ctx, cp, func() error {
		p := consolePlugin(r.namespace)
		cp.Labels = p.Labels
		cp.Spec = p.Spec

		return nil
	})
}
