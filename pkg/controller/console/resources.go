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
	"fmt"

	consolev1 "github.com/openshift/api/console/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	pluginName     = "camel-dashboard-console"
	pluginPort     = 9443
	certSecretName = "camel-dashboard-console-cert"
	managedBy      = "camel-monitor-operator"
)

func commonLabels() map[string]string {
	return map[string]string{
		"app":                          pluginName,
		"app.kubernetes.io/name":       pluginName,
		"app.kubernetes.io/part-of":    pluginName,
		"app.kubernetes.io/managed-by": managedBy,
	}
}

const nginxConfigTemplate = `error_log /dev/stdout info;
events {}
http {
  access_log         /dev/stdout;
  include            /etc/nginx/mime.types;
  default_type       application/octet-stream;
  keepalive_timeout  65;
  server {
    listen              %d ssl;
    listen              [::]:%d ssl;
    ssl_certificate     /var/cert/tls.crt;
    ssl_certificate_key /var/cert/tls.key;
    root                /usr/share/nginx/html;
  }
}
`

func nginxConfig() string {
	return fmt.Sprintf(nginxConfigTemplate, pluginPort, pluginPort)
}

func deployment(namespace, image string) *appsv1.Deployment {
	labels := commonLabels()
	defaultMode := int32(420)

	return &appsv1.Deployment{
		Name:      pluginName,
		Namespace: namespace,
		Labels:    labels,
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": pluginName,
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
					MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					DNSPolicy:     corev1.DNSClusterFirst,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: new(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            pluginName,
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: pluginPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: new(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "plugin-serving-cert",
									MountPath: "/var/cert",
									ReadOnly:  true,
								},
								{
									Name:      "nginx-conf",
									MountPath: "/etc/nginx/nginx.conf",
									SubPath:   "nginx.conf",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "plugin-serving-cert",
							Secret: &corev1.SecretVolumeSource{
								SecretName:  certSecretName,
								DefaultMode: &defaultMode,
							},
						},
						{
							Name: "nginx-conf",
							ConfigMap: &corev1.ConfigMapVolumeSource{
								Name:        pluginName,
								DefaultMode: &defaultMode,
							},
						},
					},
				},
			},
		},
	}
}

func service(namespace string) *corev1.Service {
	return &corev1.Service{
		Name:      pluginName,
		Namespace: namespace,
		Labels:    commonLabels(),
		Annotations: map[string]string{
			"service.alpha.openshift.io/serving-cert-secret-name": certSecretName,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       fmt.Sprintf("%d-tcp", pluginPort),
					Protocol:   corev1.ProtocolTCP,
					Port:       pluginPort,
					TargetPort: intstr.FromInt32(pluginPort),
				},
			},
			Selector: map[string]string{
				"app": pluginName,
			},
		},
	}
}

func consolePlugin(namespace string) *consolev1.ConsolePlugin {
	return &consolev1.ConsolePlugin{
		APIVersion: consolev1.SchemeGroupVersion.String(),
		Kind:       "ConsolePlugin",
		Name:       pluginName,
		Labels:     commonLabels(),
		Spec: consolev1.ConsolePluginSpec{
			DisplayName: "Camel Dashboard Console",
			I18n: consolev1.ConsolePluginI18n{
				LoadType: consolev1.Preload,
			},
			Backend: consolev1.ConsolePluginBackend{
				Type: consolev1.Service,
				Service: &consolev1.ConsolePluginService{
					Name:      pluginName,
					Namespace: namespace,
					Port:      pluginPort,
					BasePath:  "/",
				},
			},
		},
	}
}
