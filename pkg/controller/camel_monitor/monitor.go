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

package monitor

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/controller/synthetic"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewMonitorAction returns an action that monitors the App.
func NewMonitorAction(hasPrometheusCRDs, hasGrafanaCRDs bool) Action {
	return &monitorAction{
		hasPrometheusCRDs: hasPrometheusCRDs,
		hasGrafanaCRDs:    hasGrafanaCRDs,
	}
}

type monitorAction struct {
	baseAction

	// We cache the discovery call for performance reasons
	hasPrometheusCRDs bool
	hasGrafanaCRDs    bool
}

func (action *monitorAction) Name() string {
	return "monitor"
}

func (action *monitorAction) CanHandle(cmon *v1alpha1.CamelMonitor) bool {
	return true
}

func (action *monitorAction) Handle(ctx context.Context, cmon *v1alpha1.CamelMonitor) (*v1alpha1.CamelMonitor, error) {
	action.L.Infof("Monitoring App %s/%s with status %s", cmon.Namespace, cmon.Name, cmon.Status.Phase)

	objOwner, err := lookupObject(ctx, action.client,
		cmon.Annotations[v1alpha1.MonitorImportedKindLabel], cmon.Namespace, cmon.Annotations[v1alpha1.MonitorImportedNameLabel])
	if err != nil {
		return nil, err
	}

	if objOwner == nil {
		return nil, fmt.Errorf("baking deployment does not exist for App %s/%s", cmon.Namespace, cmon.Name)
	}

	nonManagedApp, err := synthetic.NonManagedCamelMonitorlicationFactory(*objOwner)
	if err != nil {
		return nil, err
	}

	targetApp := cmon.DeepCopy()
	// Important: we keep any previous existing info, so that, in case of scaling to 0
	// we maintain the latest scraped information available.
	targetApp.Status = v1alpha1.CamelMonitorStatus{
		Info:        cmon.Status.Info,
		SuccessRate: cmon.Status.SuccessRate,
	}
	targetApp.ImportCamelAnnotations(nonManagedApp.GetAnnotations())

	deployImage := nonManagedApp.GetAppImage()
	appPhase := nonManagedApp.GetAppPhase(ctx, action.client)
	targetApp.Status.Phase = appPhase
	targetApp.Status.Image = deployImage

	pods, err := nonManagedApp.GetPods(ctx, action.client)
	if err != nil {
		return targetApp, err
	}

	targetApp.Status.Pods = pods
	targetApp.Status.Replicas = nonManagedApp.GetReplicas()

	if pods != nil {
		// Only if any pod is ready
		targetRuntimeInfo := getInfo(pods)
		if targetRuntimeInfo != nil {
			targetApp.Status.Info = formatRuntimeInfo(targetRuntimeInfo)
		}

		appRuntimeInfo := getInfo(cmon.Status.Pods)
		if appRuntimeInfo != nil && targetRuntimeInfo != nil {
			pollingInterval := getPollingInterval(targetApp)
			sliErrPerc := getSLIExchangeErrorThreshold(targetApp)
			sliWarnPerc := getSLIExchangeWarningThreshold(targetApp)
			targetApp.Status.SuccessRate = getSLIExchangeSuccessRate(*appRuntimeInfo, *targetRuntimeInfo, &pollingInterval, sliErrPerc, sliWarnPerc)
		}

		if action.hasPrometheusCRDs && platform.GetCreatePrometheusPodMonitor() == "true" {
			err := addPrometheusPodMonitor(ctx, action.client, targetApp, nonManagedApp.GetMatchLabelsSelector())
			if err != nil {
				return targetApp, err
			}
		}

		if action.hasGrafanaCRDs && platform.GetCreateGrafanaDashboard() == "true" {
			err := addGrafanaDashboard(ctx, action.client, targetApp, nonManagedApp.GetResourcesLimits())
			if err != nil {
				return targetApp, err
			}
		}
	}

	nonManagedApp.SetMonitoringCondition(cmon, targetApp, pods)

	return targetApp, nil
}

func lookupObject(ctx context.Context, c client.Client, kind, ns string, name string) (*ctrl.Object, error) {
	var obj ctrl.Object

	switch kind {
	case "Deployment":
		obj = &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       kind,
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
		}
	case "CronJob":
		obj = &batchv1.CronJob{
			TypeMeta: metav1.TypeMeta{
				Kind:       kind,
				APIVersion: batchv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
		}
	default:
		return nil, fmt.Errorf("cannot manage Camel application of type %s", kind)
	}

	key := ctrl.ObjectKey{
		Namespace: ns,
		Name:      name,
	}
	err := c.Get(ctx, key, obj)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &obj, nil
}

func getInfo(pods []v1alpha1.PodInfo) *v1alpha1.RuntimeInfo {
	runtimeInfo := v1alpha1.RuntimeInfo{
		Exchange: &v1alpha1.ExchangeInfo{},
	}

	for _, pod := range pods {
		// Collect runtime information only once
		if runtimeInfo.RuntimeProvider == "" && pod.Runtime != nil {
			runtimeInfo.RuntimeProvider = pod.Runtime.RuntimeProvider
			runtimeInfo.RuntimeVersion = pod.Runtime.RuntimeVersion
			runtimeInfo.CamelVersion = pod.Runtime.CamelVersion
		}
		// Sum all the exchanges processed
		if pod.Runtime != nil && pod.Runtime.Exchange != nil {
			runtimeInfo.Exchange.Total += pod.Runtime.Exchange.Total
			runtimeInfo.Exchange.Failed += pod.Runtime.Exchange.Failed
			runtimeInfo.Exchange.Pending += pod.Runtime.Exchange.Pending
			runtimeInfo.Exchange.Succeeded += pod.Runtime.Exchange.Succeeded

			// Set the major timestamp
			if pod.Runtime.Exchange.LastTimestamp != nil {
				if runtimeInfo.Exchange.LastTimestamp == nil || pod.Runtime.Exchange.LastTimestamp.After(runtimeInfo.Exchange.LastTimestamp.Time) {
					runtimeInfo.Exchange.LastTimestamp = pod.Runtime.Exchange.LastTimestamp
				}
			}
		}
	}

	if runtimeInfo.RuntimeProvider == "" && runtimeInfo.Exchange.Total == 0 {
		// Likely there was no available metric at all
		return nil
	}

	return &runtimeInfo
}

func formatRuntimeInfo(runtimeInfo *v1alpha1.RuntimeInfo) string {
	if runtimeInfo.RuntimeProvider != "" {
		return fmt.Sprintf(
			"%s - %s (%s)",
			runtimeInfo.RuntimeProvider, runtimeInfo.RuntimeVersion, runtimeInfo.CamelVersion,
		)
	}

	return ""
}

func getSLIExchangeSuccessRate(app, target v1alpha1.RuntimeInfo, pollingInteval *time.Duration, sliErrPerc, sliWarnPerc int) *v1alpha1.SLIExchangeSuccessRate {
	var failureRate float64

	sliExchangeSuccessRate := v1alpha1.SLIExchangeSuccessRate{
		SamplingIntervalDuration: pollingInteval,
	}

	totalLastInterval := target.Exchange.Total - app.Exchange.Total

	failedLastInterval := target.Exchange.Failed - app.Exchange.Failed
	if totalLastInterval > 0 {
		failureRate = float64(failedLastInterval) / float64(totalLastInterval) * 100
		successRate := 100 - failureRate
		sliExchangeSuccessRate.SuccessPercentage = strconv.FormatFloat(successRate, 'f', 2, 64)
	}

	if totalLastInterval >= 0 {
		sliExchangeSuccessRate.SamplingIntervalTotal = totalLastInterval
	}

	if failedLastInterval >= 0 {
		sliExchangeSuccessRate.SamplingIntervalFailed = failedLastInterval
	}

	if failureRate > float64(sliWarnPerc) {
		sliExchangeSuccessRate.Status = v1alpha1.SLIExchangeStatusError
	} else if failureRate > float64(sliErrPerc) {
		sliExchangeSuccessRate.Status = v1alpha1.SLIExchangeStatusWarning
	} else if totalLastInterval > 0 {
		// We prevent to mark as success when there is no yet exchange
		sliExchangeSuccessRate.Status = v1alpha1.SLIExchangeStatusSuccess
	}

	if target.Exchange.LastTimestamp != nil {
		sliExchangeSuccessRate.LastTimestamp = target.Exchange.LastTimestamp
	}

	return &sliExchangeSuccessRate
}
