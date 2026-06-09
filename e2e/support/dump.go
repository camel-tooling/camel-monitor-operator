//go:build integration

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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"go.yaml.in/yaml/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Dump prints all information about the given namespace to debug errors
func Dump(ctx context.Context, c *kubernetes.Clientset, ns string, t *testing.T) error {
	t.Logf("-------------------- start dumping namespace %s --------------------\n", ns)

	// CamelMonitors
	cli := *CamelMonitorClient(t)
	camelMonitors := &v1alpha1.CamelMonitorList{}
	err := cli.List(
		ctx,
		camelMonitors,
		client.InNamespace(ns),
	)
	if err != nil {
		return err
	}
	t.Logf("Found %d Camel Monitors:\n", len(camelMonitors.Items))
	for _, cmon := range camelMonitors.Items {
		ref := cmon
		data, err := toYAMLNoManagedFields(&ref)
		if err != nil {
			return err
		}
		t.Logf("---\n%s\n---\n", string(data))
	}

	// Deployments
	deployments, err := c.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	t.Logf("Found %d deployments:\n", len(deployments.Items))
	for _, deployment := range deployments.Items {
		ref := deployment
		data, err := toYAMLNoManagedFields(&ref)
		if err != nil {
			return err
		}
		t.Logf("---\n%s\n---\n", string(data))
	}

	// Cronjobs
	cronjobs, err := c.BatchV1().CronJobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	t.Logf("\nFound %d cronjobs:\n", len(cronjobs.Items))
	for _, cronjobs := range cronjobs.Items {
		ref := cronjobs
		data, err := toYAMLNoManagedFields(&ref)
		if err != nil {
			return err
		}
		t.Logf("---\n%s\n---\n", string(data))
	}

	// Pods
	lst, err := c.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	t.Logf("\nFound %d pods in %q:\n", len(lst.Items), ns)
	for _, pod := range lst.Items {
		t.Logf("name=%s\n", pod.Name)

		ref := pod
		data, err := toYAMLNoManagedFields(&ref)
		if err != nil {
			return err
		}
		t.Logf("---\n%s\n---\n", string(data))

		dumpConditions("  ", pod.Status.Conditions, t)
		t.Logf("  logs:\n")
		var allContainers []corev1.Container
		allContainers = append(allContainers, pod.Spec.InitContainers...)
		allContainers = append(allContainers, pod.Spec.Containers...)
		for _, container := range allContainers {
			pad := "    "
			t.Logf("%s%s\n", pad, container.Name)
			err := dumpLogs(ctx, c, fmt.Sprintf("%s> ", pad), ns, pod.Name, container.Name, t)
			if err != nil {
				t.Logf("%sERROR while reading the logs: %v\n", pad, err)
			}
		}
	}

	t.Logf("-------------------- end dumping namespace %s --------------------\n", ns)
	return nil
}

func dumpConditions(prefix string, conditions []corev1.PodCondition, t *testing.T) {
	for _, cond := range conditions {
		t.Logf("%scondition type=%s, status=%s, reason=%s, message=%q\n", prefix, cond.Type, cond.Status, cond.Reason, cond.Message)
	}
}

func dumpLogs(ctx context.Context, c *kubernetes.Clientset, prefix string, ns string, name string, container string, t *testing.T) error {
	logOptions := &corev1.PodLogOptions{
		Container: container,
	}

	lines := int64(150)
	logOptions.TailLines = &lines

	stream, err := c.CoreV1().Pods(ns).GetLogs(name, logOptions).Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	scanner := bufio.NewScanner(stream)
	printed := false
	for scanner.Scan() {
		printed = true
		t.Logf("%s%s\n", prefix, scanner.Text())
	}
	if !printed {
		t.Logf("%s[no logs available]\n", prefix)
	}
	return nil
}

func toYAMLNoManagedFields(value runtime.Object) ([]byte, error) {
	jsondata, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	mapdata, err := jSONToMap(jsondata)
	if err != nil {
		return nil, err
	}

	if m, ok := mapdata["metadata"].(map[string]any); ok {
		delete(m, "managedFields")
	}

	return mapToYAML(mapdata)
}

func jSONToMap(src []byte) (map[string]any, error) {
	jsondata := map[string]any{}
	err := json.Unmarshal(src, &jsondata)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling json: %w", err)
	}

	return jsondata, nil
}

func mapToYAML(src map[string]any) ([]byte, error) {
	yamldata, err := yaml.Marshal(&src)
	if err != nil {
		return nil, fmt.Errorf("error marshalling to yaml: %w", err)
	}

	return yamldata, nil
}
