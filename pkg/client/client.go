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

package client

import (
	"k8s.io/client-go/kubernetes"

	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	camel "github.com/camel-tooling/camel-monitor-operator/pkg/client/camel/clientset/versioned"
	camelv1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/client/camel/clientset/versioned/typed/camel/v1alpha1"
)

// Client is an abstraction for a k8s client.
type Client interface {
	ctrl.Client
	kubernetes.Interface
	CamelV1alpha1() camelv1alpha1.CamelV1alpha1Interface
}

// Injectable identifies objects that can receive a Client.
type Injectable interface {
	InjectClient(client Client)
}

type DefaultClient struct {
	ctrl.Client
	kubernetes.Interface

	Camel camel.Interface
}

func (c *DefaultClient) CamelV1alpha1() camelv1alpha1.CamelV1alpha1Interface {
	return c.Camel.CamelV1alpha1()
}

// FromManager creates a new k8s client from a manager object.
func FromManager(manager manager.Manager) (Client, error) {
	var (
		err       error
		clientset kubernetes.Interface
	)
	if clientset, err = kubernetes.NewForConfig(manager.GetConfig()); err != nil {
		return nil, err
	}

	var camelClientset camel.Interface
	if camelClientset, err = camel.NewForConfig(manager.GetConfig()); err != nil {
		return nil, err
	}

	return &DefaultClient{
		Client:    manager.GetClient(),
		Interface: clientset,
		Camel:     camelClientset,
	}, nil
}
