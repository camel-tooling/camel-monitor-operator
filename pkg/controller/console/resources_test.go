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
	"testing"

	consolev1 "github.com/openshift/api/console/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestCommonLabels(t *testing.T) {
	labels := commonLabels()

	assert.Equal(t, pluginName, labels["app"])
	assert.Equal(t, pluginName, labels["app.kubernetes.io/name"])
	assert.Equal(t, pluginName, labels["app.kubernetes.io/part-of"])
	assert.Equal(t, managedBy, labels["app.kubernetes.io/managed-by"])
}

func TestNginxConfig(t *testing.T) {
	cfg := nginxConfig()

	assert.Contains(t, cfg, "listen              9443 ssl")
	assert.Contains(t, cfg, "ssl_certificate     /var/cert/tls.crt")
	assert.Contains(t, cfg, "ssl_certificate_key /var/cert/tls.key")
	assert.Contains(t, cfg, "server_tokens       off")
	assert.Contains(t, cfg, `X-Frame-Options "DENY"`)
	assert.Contains(t, cfg, `X-Content-Type-Options "nosniff"`)
	assert.Contains(t, cfg, `Strict-Transport-Security "max-age=31536000; includeSubDomains"`)
	assert.Contains(t, cfg, `Content-Security-Policy "default-src 'none'`)
	assert.Contains(t, cfg, "try_files $uri /locales/en/")
}

func TestDeployment(t *testing.T) {
	deploy := deployment("test-ns", "my-image:v1")

	assert.Equal(t, pluginName, deploy.Name)
	assert.Equal(t, "test-ns", deploy.Namespace)
	assert.Equal(t, commonLabels(), deploy.Labels)

	require.Len(t, deploy.Spec.Template.Spec.Containers, 1)
	container := deploy.Spec.Template.Spec.Containers[0]
	assert.Equal(t, pluginName, container.Name)
	assert.Equal(t, "my-image:v1", container.Image)
	require.Len(t, container.Ports, 1)
	assert.Equal(t, int32(pluginPort), container.Ports[0].ContainerPort)
	assert.Equal(t, corev1.ProtocolTCP, container.Ports[0].Protocol)

	require.Len(t, container.VolumeMounts, 2)
	assert.Equal(t, "plugin-serving-cert", container.VolumeMounts[0].Name)
	assert.Equal(t, "nginx-conf", container.VolumeMounts[1].Name)

	require.Len(t, deploy.Spec.Template.Spec.Volumes, 2)
	assert.Equal(t, certSecretName, deploy.Spec.Template.Spec.Volumes[0].Secret.SecretName)
	assert.Equal(t, pluginName, deploy.Spec.Template.Spec.Volumes[1].ConfigMap.Name)

	require.NotNil(t, deploy.Spec.Template.Spec.SecurityContext)
	assert.True(t, *deploy.Spec.Template.Spec.SecurityContext.RunAsNonRoot)

	require.NotNil(t, container.SecurityContext)
	assert.False(t, *container.SecurityContext.AllowPrivilegeEscalation)
}

func TestDesiredService(t *testing.T) {
	svc := service("test-ns")

	assert.Equal(t, pluginName, svc.Name)
	assert.Equal(t, "test-ns", svc.Namespace)
	assert.Equal(t, commonLabels(), svc.Labels)
	assert.Equal(t, certSecretName, svc.Annotations["service.alpha.openshift.io/serving-cert-secret-name"])
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)

	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(pluginPort), svc.Spec.Ports[0].Port)
	assert.Equal(t, corev1.ProtocolTCP, svc.Spec.Ports[0].Protocol)

	assert.Equal(t, pluginName, svc.Spec.Selector["app"])
}

func TestDesiredConsolePlugin(t *testing.T) {
	cp := consolePlugin("test-ns")

	assert.Equal(t, pluginName, cp.Name)
	assert.Equal(t, consolev1.SchemeGroupVersion.String(), cp.APIVersion)
	assert.Equal(t, "ConsolePlugin", cp.Kind)
	assert.Equal(t, commonLabels(), cp.Labels)

	assert.Equal(t, "Camel Dashboard Console", cp.Spec.DisplayName)
	assert.Equal(t, consolev1.Preload, cp.Spec.I18n.LoadType)
	assert.Equal(t, consolev1.Service, cp.Spec.Backend.Type)

	require.NotNil(t, cp.Spec.Backend.Service)
	assert.Equal(t, pluginName, cp.Spec.Backend.Service.Name)
	assert.Equal(t, "test-ns", cp.Spec.Backend.Service.Namespace)
	assert.Equal(t, int32(pluginPort), cp.Spec.Backend.Service.Port)
	assert.Equal(t, "/", cp.Spec.Backend.Service.BasePath)
}
