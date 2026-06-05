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

package event

import (
	"context"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"

	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

// NotifyAppError automatically generates error events when the app reconcile cycle phase has an error.
func NotifyAppError(ctx context.Context, c client.Client, recorder events.EventRecorder, old, newResource *v1alpha1.CamelMonitor, err error) {
	app := old
	if newResource != nil {
		app = newResource
	}

	if app == nil {
		return
	}

	recorder.Eventf(app, nil, corev1.EventTypeWarning, "AppError", "ReconcileFailed", "Cannot reconcile App %s: %v", app.Name, err)
}

// NotifyAppUpdated automatically generates events when the app changes.
func NotifyAppUpdated(ctx context.Context, c client.Client, recorder events.EventRecorder, old, newResource *v1alpha1.CamelMonitor) {
	if newResource == nil {
		return
	}

	oldPhase := ""
	if old != nil {
		oldPhase = string(old.Status.Phase)
	}

	notifyIfPhaseUpdated(ctx, c, recorder, newResource, oldPhase, string(newResource.Status.Phase), "App", newResource.Name,
		"AppUpdated", "")
}

func notifyIfPhaseUpdated(ctx context.Context, c client.Client, recorder events.EventRecorder, newResource ctrl.Object,
	oldPhase, newPhase string, resourceType, name, reason, info string) {
	if oldPhase == newPhase {
		return
	}
	// Update information about phase changes
	phase := newPhase
	if phase == "" {
		phase = "[none]"
	}

	recorder.Eventf(newResource, nil, corev1.EventTypeNormal, reason, "Reconciled", "%s %q in phase %q%s", resourceType, name, phase, info)
}
