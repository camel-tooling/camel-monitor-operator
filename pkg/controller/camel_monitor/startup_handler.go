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
	"math/rand/v2"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// startupSpreadHandler is the event handler of the CamelMonitor controller.
//
// Why it exists: the polling loop is driven by an in-memory requeue (RequeueAfter), which is lost when
// the operator Pod restarts. Each resource reschedules itself "polling interval" after its own last
// reconciliation, so if many resources are reconciled at the same instant they stay in lockstep forever
// and produce a load peak on every polling cycle. This happens in two situations:
//   - Operator restart: the informer replays every existing CamelMonitor as a Create event
//     ("initial list") and the default handler would enqueue all of them at once.
//   - Fresh (re)install: many CamelMonitors are created together (for example the workloads are
//     applied/recreated at once, or the CamelMonitors are deleted and created again from scratch).
//
// To avoid that, no Create event is enqueued immediately. A random delay (jitter) of up to one polling
// interval is added, so the resources are spread over the interval. As every cycle is then rescheduled
// from the actual reconciliation time, the spread is kept in the following cycles.
// The jitter window is the polling interval of the resource (operator configuration, possibly overridden
// by the resource annotation), see getPollingInterval.
//
//   - Initial list events: see startupReconcileDelay (also takes into account when the resource is due).
//   - New resources: see newResourceReconcileDelay.
//
// Everything else keeps the default behavior:
//   - Delete events are enqueued immediately (the reconciler ignores NotFound).
//   - Update/Generic events are discarded by UpdateFalsePredicate before reaching this handler.
//   - Regular polling cycles never go through here: they are scheduled by Reconcile's RequeueAfter.
type startupSpreadHandler struct {
	handler.TypedEnqueueRequestForObject[client.Object]
}

// Create delays the reconciliation of the CamelMonitors, either replayed on operator start-up or newly created.
func (h *startupSpreadHandler) Create(
	ctx context.Context,
	e event.TypedCreateEvent[client.Object],
	q workqueue.TypedRateLimitingInterface[reconcile.Request],
) {
	cmon, ok := e.Object.(*v1alpha1.CamelMonitor)
	if !ok {
		// Unexpected type, nothing we can compute: fallback to the default behavior.
		h.TypedEnqueueRequestForObject.Create(ctx, e, q)

		return
	}

	delay := newResourceReconcileDelay(cmon)
	if e.IsInInitialList {
		delay = startupReconcileDelay(cmon, time.Now())
	}

	q.AddAfter(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cmon)}, delay)
}

// newResourceReconcileDelay returns the delay before the first reconciliation of a CamelMonitor created
// while the operator is running: a random value within one polling interval.
//
// NOTE: the status conditions of a new CamelMonitor are therefore populated up to one polling interval
// after its creation.
func newResourceReconcileDelay(cmon *v1alpha1.CamelMonitor) time.Duration {
	return jitter(getPollingInterval(cmon))
}

// startupReconcileDelay returns how long to wait before the first reconciliation of an existing
// CamelMonitor after an operator restart. The next poll is due at:
//
//	Monitored.LastTransitionTime + pollingInterval
//
// (the Monitored condition is rewritten with the current time on every monitoring cycle).
//
// The delay is the time left until the resource is due (0 if it is already overdue, for instance because
// the operator was down longer than the interval) plus a random jitter within one polling interval:
//   - Resources that are not due yet keep their original order, but are shifted by a random amount. Resources
//     that were created together (and so polled in lockstep) are spread, instead of being preserved as a group.
//   - Resources that are overdue would all be due at the same instant, which is precisely the peak we want to
//     avoid: the jitter spreads them.
//   - Resources never monitored (no condition) only get the jitter.
func startupReconcileDelay(cmon *v1alpha1.CamelMonitor, now time.Time) time.Duration {
	pollingInterval := getPollingInterval(cmon)

	var remaining time.Duration

	condition := cmon.Status.GetCondition(v1alpha1.ConditionTypeMonitored)
	if condition != nil && !condition.LastTransitionTime.IsZero() {
		remaining = max(condition.LastTransitionTime.Add(pollingInterval).Sub(now), 0)
	}

	return remaining + jitter(pollingInterval)
}

// jitter returns a random duration in the range [0, window). It returns 0 if the window is not positive.
func jitter(window time.Duration) time.Duration {
	if window <= 0 {
		return 0
	}

	// Jitter is only meant to spread the load, it does not need to be cryptographically secure.
	return rand.N(window) //nolint:gosec
}
