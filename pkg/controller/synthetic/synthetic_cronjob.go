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
	"time"

	v1alpha1 "github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nonManagedCamelCronjob represents a cron Camel application built and deployed outside the operator lifecycle.
type nonManagedCamelCronjob struct {
	cron       *batchv1.CronJob
	httpClient *http.Client
}

// CamelMonitor return an CamelMonitor resource fed by the Camel application adapter.
func (app *nonManagedCamelCronjob) CamelMonitor(ctx context.Context, c client.Client) *v1alpha1.CamelMonitor {
	newApp := v1alpha1.NewCamelMonitor(app.cron.Namespace, app.cron.Labels[platform.GetMonitorLabelSelector()])
	newApp.SetAnnotations(map[string]string{
		v1alpha1.MonitorImportedNameLabel: app.cron.Name,
		v1alpha1.MonitorImportedKindLabel: "CronJob",
	})
	references := []metav1.OwnerReference{
		{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
			Name:       app.cron.Name,
			UID:        app.cron.UID,
			Controller: &controller,
		},
	}
	newApp.SetOwnerReferences(references)

	return &newApp
}

// GetAppPhase returns the phase of the backing Camel application.
func (app *nonManagedCamelCronjob) GetAppPhase(ctx context.Context, c client.Client) v1alpha1.CamelMonitorPhase {
	if len(app.cron.Status.Active) > 0 {
		return v1alpha1.CamelMonitorPhaseRunning
	}

	// If none is active, then it means the app is waiting for scheduling execution.
	return v1alpha1.CamelMonitorPhasePaused
}

// GetReplicas returns the number of desired replicas for the backing Camel application.
func (app *nonManagedCamelCronjob) GetReplicas() *int32 {
	// In the case of a CronJob we use the number of active jobs instead.
	//nolint:gosec
	return new(int32(len(app.cron.Status.Active)))
}

// GetAppImage returns the container image of the backing Camel application.
func (app *nonManagedCamelCronjob) GetAppImage() string {
	return app.cron.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image
}

// GetPods returns the pods backing the Camel application.
func (app *nonManagedCamelCronjob) GetPods(ctx context.Context, c client.Client) ([]v1alpha1.PodInfo, error) {
	// In the CronJob case we don't want to inspect the Pod as we are not sure we have the Pod live when
	// the monitoring happens.
	return getPods(*app.httpClient, ctx, c, app.cron.GetNamespace(),
		app.GetMatchLabelsSelector(), getObservabilityPort(app.GetAnnotations()), false, nil)
}

// GetAnnotations returns the backing deployment object annotations.
func (app *nonManagedCamelCronjob) GetAnnotations() map[string]string {
	return app.cron.Annotations
}

// GetMatchLabelsSelector returns the labels selector used to select Pods belonging to the backing application.
func (app *nonManagedCamelCronjob) GetMatchLabelsSelector() map[string]string {
	return app.cron.Spec.JobTemplate.Spec.Template.Labels
}

// SetMonitoringCondition sets the health and monitoring conditions on the target app.
//
//nolint:nestif
func (app *nonManagedCamelCronjob) SetMonitoringCondition(srcApp, targetApp *v1alpha1.CamelMonitor, pods []v1alpha1.PodInfo) {
	monitorCondition := metav1.ConditionFalse
	monitorMessage := "No scheduled job has run yet"
	// We only verify the status of latest executions. If they are all successful, then we consider the workload healthy.
	if len(pods) > 0 {
		monitorCondition = metav1.ConditionTrue
		monitorMessage = "At least one scheduled job has run"

		runningPods := countPodsWithStatus(pods, "Running")
		succeededPods := countPodsWithStatus(pods, "Succeeded")
		info := fmt.Sprintf("%d out of last %d job succeeded", succeededPods, len(pods)-runningPods)

		healthCond := metav1.ConditionFalse
		if len(pods) == runningPods+succeededPods {
			healthCond = metav1.ConditionTrue
		}

		targetApp.Status.AddCondition(metav1.Condition{
			Type:               "Healthy",
			Status:             healthCond,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "HealthCheckCompleted",
			Message:            info,
		})

		schedulingMessage := ""
		// Special condition for cronjob
		if app.cron.Status.LastScheduleTime != nil {
			schedulingMessage += "Last scheduled time: " + app.cron.Status.LastScheduleTime.Format("2006-01-02 15:04:05")
		}

		if app.cron.Status.LastSuccessfulTime != nil {
			if schedulingMessage != "" {
				schedulingMessage += "; "
			}

			schedulingMessage += "Last successful time: " + app.cron.Status.LastSuccessfulTime.Format("2006-01-02 15:04:05")
		}

		targetApp.Status.AddCondition(metav1.Condition{
			Type:               "CronJobExecution",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "Scheduling",
			Message:            schedulingMessage,
		})
		targetApp.Status.Info = info
	}

	targetApp.Status.AddCondition(metav1.Condition{
		Type:               "Monitored",
		Status:             monitorCondition,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "MonitoringComplete",
		Message:            monitorMessage,
	})
}

// GetResourcesLimits returns the resource limits of the backing Camel application.
func (app *nonManagedCamelCronjob) GetResourcesLimits() corev1.ResourceList {
	return app.cron.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Resources.Limits
}
