//go:build integration
// +build integration

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

package support

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis"
	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	camel "github.com/camel-tooling/camel-monitor-operator/pkg/client/camel/clientset/versioned"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client/camel/clientset/versioned/scheme"
	integreatlyv1beta1 "github.com/grafana/grafana-operator/v5/api/v1beta1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewClient() (*kubernetes.Clientset, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		kubeconfig := filepath.Join(
			os.Getenv("HOME"),
			".kube",
			"config",
		)

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create kube config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

// NewClientWithConfig creates a new k8s client that can be used from outside the cluster (ie, for testing purposes).
func NewClientWithConfig(cfg *rest.Config) (client.Client, error) {
	var err error
	clientScheme := scheme.Scheme
	if !clientScheme.IsVersionRegistered(v1alpha1.SchemeGroupVersion) {
		// Setup Scheme for all Camel CRs
		err = apis.AddToScheme(clientScheme)
		if err != nil {
			return nil, err
		}
	}
	if !clientScheme.IsVersionRegistered(monitoringv1.SchemeGroupVersion) {
		// Setup Scheme for Prometheus CRs
		err = monitoringv1.AddToScheme(clientScheme)
		if err != nil {
			return nil, err
		}
	}
	if !clientScheme.IsVersionRegistered(integreatlyv1beta1.SchemeGroupVersion) {
		// Setup Scheme for Grafana CRs
		err = integreatlyv1beta1.AddToScheme(clientScheme)
		if err != nil {
			return nil, err
		}
	}

	var clientset kubernetes.Interface
	if clientset, err = kubernetes.NewForConfig(cfg); err != nil {
		return nil, err
	}

	var camelClientset camel.Interface
	if camelClientset, err = camel.NewForConfig(cfg); err != nil {
		return nil, err
	}

	// Create a new client to avoid using cache (enabled by default with controller-runtime client)
	clientOptions := ctrl.Options{
		Scheme: clientScheme,
	}
	dynClient, err := ctrl.New(cfg, clientOptions)
	if err != nil {
		return nil, err
	}

	return &client.DefaultClient{
		Client:    dynClient,
		Interface: clientset,
		Camel:     camelClientset,
	}, nil
}
