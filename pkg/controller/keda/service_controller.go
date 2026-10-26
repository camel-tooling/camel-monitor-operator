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

	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// ScalerServiceName is the name of the Service exposing the KEDA external scaler gRPC endpoint.
	ScalerServiceName = "camel-monitor-keda-scaler"
)

// ServiceController reconciles the KEDA external scaler Service once on startup,
// setting the operator Deployment as owner so the Service is garbage-collected with it.
// It also registers and starts the gRPC Server with the manager.
type ServiceController struct {
	client    ctrlclient.Client
	reader    ctrlclient.Reader
	mgr       manager.Manager
	namespace string
	port      int32
	// scalerClient is the Kubernetes client used by the gRPC scaler (may differ from the
	// controller-runtime client when injected in tests).
	scalerClient client.Client
}

// NewServiceController creates a new ServiceController that owns both the gRPC Server
// and the Kubernetes Service resource. Callers only need to register this one runnable.
func NewServiceController(mgr manager.Manager, scalerClient client.Client, port int32) *ServiceController {
	return &ServiceController{
		client:       mgr.GetClient(),
		reader:       mgr.GetAPIReader(),
		mgr:          mgr,
		namespace:    platform.GetOperatorNamespace(),
		port:         port,
		scalerClient: scalerClient,
	}
}

// Start implements manager.Runnable. It registers the gRPC Server with the manager,
// then reconciles the Kubernetes Service once before returning.
func (s *ServiceController) Start(ctx context.Context) error {
	// Register the gRPC server so the manager starts it alongside other runnables.
	if s.mgr != nil {
		err := s.mgr.Add(NewServer(s.scalerClient, s.port))
		if err != nil {
			return err
		}
	}

	if s.namespace == "" {
		log.Info("Operator namespace not determined, skipping KEDA scaler Service reconciliation")

		return nil
	}

	operatorDeploy := &appsv1.Deployment{}

	err := s.reader.Get(ctx, types.NamespacedName{
		Name:      platform.DefaultOperatorName,
		Namespace: s.namespace,
	}, operatorDeploy)
	if err != nil {
		return err
	}

	ownerRef := metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       operatorDeploy.Name,
		UID:        operatorDeploy.UID,
		Controller: new(true),
	}

	return s.ensureService(ctx, ownerRef)
}

// ensureService creates or updates the ClusterIP Service with the given owner reference.
func (s *ServiceController) ensureService(ctx context.Context, ownerRef metav1.OwnerReference) error {
	svc := &corev1.Service{}
	key := ctrlclient.ObjectKey{Name: ScalerServiceName, Namespace: s.namespace}

	err := s.reader.Get(ctx, key, svc)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	desired := buildScalerService(s.namespace, s.port)
	desired.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	if k8serrors.IsNotFound(err) {
		log.Infof("Creating KEDA external scaler Service %s/%s on port %d", s.namespace, ScalerServiceName, s.port)

		return s.client.Create(ctx, desired)
	}

	// Update only the mutable fields.
	svc.Spec.Ports = desired.Spec.Ports
	svc.Spec.Selector = desired.Spec.Selector
	svc.Labels = desired.Labels
	svc.SetOwnerReferences(desired.GetOwnerReferences())

	return s.client.Update(ctx, svc)
}

func buildScalerService(namespace string, port int32) *corev1.Service {
	return &corev1.Service{
		Name:      ScalerServiceName,
		Namespace: namespace,
		Labels: map[string]string{
			"app":                        "camel-monitor",
			"camel.apache.org/component": "keda-scaler",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"name": platform.DefaultOperatorName,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "grpc",
					Port:       port,
					TargetPort: intstr.FromInt32(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}
