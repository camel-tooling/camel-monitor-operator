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
	"strconv"
	"sync"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/event"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/log"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/monitoring"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

// Actions are cached here for performance reasons.
var actions = []Action{}

func Add(ctx context.Context, mgr manager.Manager, c client.Client) error {
	hasPrometheusAPI, err := prometheusCRDExists(c)
	if err != nil {
		return err
	}

	hasGrafanaDashboardAPI, err := grafanaCRDExists(c)
	if err != nil {
		return err
	}

	return add(mgr, newReconciler(mgr, c, hasPrometheusAPI, hasGrafanaDashboardAPI))
}

func newReconciler(mgr manager.Manager, c client.Client, hasPrometheusCRDs, hasGrafanaCRDs bool) reconcile.Reconciler {
	if hasPrometheusCRDs {
		log.Log.WithName("reconciler").Info("Detected the presence of Prometheus PodMonitor custom resource. If enabled, the operator" +
			" will create Prometheus resources automatically!")
	} else {
		log.Log.WithName("reconciler").Info("No presence of Prometheus PodMonitor custom resource. The operator" +
			" won't be able to create Prometheus resources automatically!")
	}

	if hasGrafanaCRDs {
		log.Log.WithName("reconciler").Info("Detected the presence of GrafanaDashboard custom resource. If enabled, the operator" +
			" will create Grafana dashboards resources automatically! NOTE: this is an experimental feature.")
	} else {
		log.Log.WithName("reconciler").Info("No presence of GrafanaDashboard custom resource. The operator" +
			" won't be able to create Grafana dashboards resources automatically!")
	}

	return monitoring.NewInstrumentedReconciler(
		&reconcileApp{
			client:            c,
			reader:            mgr.GetAPIReader(),
			scheme:            mgr.GetScheme(),
			recorder:          mgr.GetEventRecorder("camel-monitor-controller"),
			hasPrometheusCRDs: hasPrometheusCRDs,
			hasGrafanaCRDs:    hasGrafanaCRDs,
			startedAt:         time.Now(),
		},
		schema.GroupVersionKind{
			Group:   v1alpha1.SchemeGroupVersion.Group,
			Version: v1alpha1.SchemeGroupVersion.Version,
			Kind:    v1alpha1.AppKind,
		},
	)
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	return builder.ControllerManagedBy(mgr).
		Named("monitor-controller").
		For(&v1alpha1.CamelMonitor{}, builder.WithPredicates(UpdateFalsePredicate{})).
		WithOptions(controller.Options{MaxConcurrentReconciles: platform.GetMaxConcurrentReconciles()}).
		Complete(r)
}

// reconcileApp reconciles an App object.
type reconcileApp struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the API server
	client   client.Client
	reader   ctrl.Reader
	scheme   *runtime.Scheme
	recorder events.EventRecorder
	// We cache the discovery call for performance reasons
	hasPrometheusCRDs bool
	hasGrafanaCRDs    bool
	// startedAt is the time the operator started: the shuffle is limited to the operator start-up.
	startedAt time.Time
	// shuffled holds the CamelMonitors (types.NamespacedName) already shuffled by this operator instance.
	shuffled sync.Map
}

func (r *reconcileApp) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	rlog := Log.WithValues("request-namespace", request.Namespace, "request-name", request.Name)

	var instance v1alpha1.CamelMonitor

	err := r.client.Get(ctx, request.NamespacedName, &instance)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.shuffled.Delete(request.NamespacedName)

			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	// Right after the operator starts all the CamelMonitors without status are reconciled at once: we spread them.
	if delay := r.startupShuffleDelay(&instance, request.NamespacedName, time.Now()); delay > 0 {
		return reconcile.Result{RequeueAfter: delay}, nil
	}

	// The polling loop is driven by an in-memory requeue, lost when the operator restarts: all the resources are
	// reconciled at once. We only monitor the ones due, the others wait for their next cycle.
	if remaining := timeUntilNextPoll(&instance, time.Now()); remaining > 0 {
		return reconcile.Result{RequeueAfter: remaining}, nil
	}

	if len(actions) == 0 {
		actions = []Action{
			NewMonitorAction(r.hasPrometheusCRDs, r.hasGrafanaCRDs),
		}
		// when we initialize, we create the PrometheusRule alert as well.
		if r.hasPrometheusCRDs && platform.GetCreatePrometheusRuleAlerts() == "true" {
			err := addPrometheusRuleAlerts(ctx, r.client, request.Namespace)
			if err != nil {
				return reconcile.Result{}, err
			}

			log.Infof("Added a generic alert PrometheusRule in %s namespace", request.Namespace)
		}
	}

	target := instance.DeepCopy()
	targetLog := rlog.ForCamelMonitor(target)

	for _, a := range actions {
		a.InjectClient(r.client)
		a.InjectLogger(targetLog)

		if !a.CanHandle(target) {
			continue
		}

		targetLog.Debugf("Invoking action %s", a.Name())

		target, err = a.Handle(ctx, target)
		if err != nil {
			event.NotifyAppError(r.recorder, &instance, target, err)

			if target != nil {
				_ = r.update(ctx, &instance, target, &targetLog)
			}

			return reconcile.Result{}, err
		}

		if target != nil {
			err := r.update(ctx, &instance, target, &targetLog)
			if err != nil {
				event.NotifyAppError(r.recorder, &instance, target, err)

				return reconcile.Result{}, err
			}
		}

		event.NotifyAppUpdated(r.recorder, &instance, target)
	}

	return reconcile.Result{RequeueAfter: getPollingInterval(target)}, nil
}

// timeUntilNextPoll returns the time left until the next monitoring cycle of the CamelMonitor is due, which
// is the last monitoring time plus the polling interval. It returns 0 if the resource is due or it was never monitored.
func timeUntilNextPoll(cmon *v1alpha1.CamelMonitor, now time.Time) time.Duration {
	if cmon.Status.LastMonitoredTime == nil || cmon.Status.LastMonitoredTime.IsZero() {
		return 0
	}

	return max(cmon.Status.LastMonitoredTime.Add(getPollingInterval(cmon)).Sub(now), 0)
}

// startupShuffleDelay returns how long the first monitoring of the CamelMonitor must be postponed, in order to spread the
// load when the operator starts (restart or reinstall, when the CamelMonitors have no status and would all be monitored
// at once). The delay is applied only if:
//   - the shuffle is enabled (POLLING_SHUFFLE_PERCENT)
//   - the CamelMonitor has no status
//   - the operator started less than a polling interval ago (it does not apply to the CamelMonitors created later)
//   - the CamelMonitor was not shuffled yet (otherwise it would be postponed forever as it has no status until monitored)
//
// It returns 0 when the CamelMonitor must be monitored right away.
func (r *reconcileApp) startupShuffleDelay(cmon *v1alpha1.CamelMonitor, key types.NamespacedName, now time.Time) time.Duration {
	percent := platform.GetPollingShufflePercent()
	if percent <= 0 || cmon.Status.Phase != "" || cmon.Status.LastMonitoredTime != nil {
		return 0
	}

	interval := getPollingInterval(cmon)
	if now.Sub(r.startedAt) > interval {
		return 0
	}

	if _, alreadyShuffled := r.shuffled.LoadOrStore(key, struct{}{}); alreadyShuffled {
		return 0
	}

	return shuffleDelay(interval, percent)
}

// shuffleDelay returns a random duration in the range [0, percent of the interval). It returns 0 if the range is empty.
func shuffleDelay(interval time.Duration, percent int) time.Duration {
	window := interval * time.Duration(percent) / 100
	if window <= 0 {
		return 0
	}

	// The shuffle is only meant to spread the load, it does not need to be cryptographically secure.
	return rand.N(window) //nolint:gosec
}

func (r *reconcileApp) update(ctx context.Context, base *v1alpha1.CamelMonitor, target *v1alpha1.CamelMonitor, log *log.Logger) error {
	err := r.client.Status().Patch(ctx, target, ctrl.MergeFrom(base))
	if err != nil {
		event.NotifyAppError(r.recorder, base, target, err)

		return err
	}

	if target.Status.Phase != base.Status.Phase {
		log.Info(
			"State transition",
			"phase-from", base.Status.Phase,
			"phase-to", target.Status.Phase,
		)
	}

	return nil
}

func getPollingInterval(target *v1alpha1.CamelMonitor) time.Duration {
	defaultPolling := platform.GetPollingInterval()
	if target.Annotations == nil || target.Annotations[v1alpha1.MonitorPollingIntervalSecondsAnnotation] == "" {
		return defaultPolling
	}

	interval, err := strconv.Atoi(target.Annotations[v1alpha1.MonitorPollingIntervalSecondsAnnotation])
	if err == nil {
		return time.Duration(interval) * time.Second
	} else {
		log.Error(err, "could not properly parse polling interval annotation, fallback to default operator value")
	}

	return defaultPolling
}

func getSLIExchangeErrorThreshold(target *v1alpha1.CamelMonitor) int {
	defaultValue := platform.GetSLIExchangeErrorThreshold()
	if target.Annotations == nil || target.Annotations[v1alpha1.MonitorSLIExchangeErrorPercentageAnnotation] == "" {
		return defaultValue
	}

	val, err := strconv.Atoi(target.Annotations[v1alpha1.MonitorSLIExchangeErrorPercentageAnnotation])
	if err == nil {
		return val
	} else {
		log.Error(err, "could not properly parse SLI error percentage, fallback to default operator value")
	}

	return defaultValue
}

func getSLIExchangeWarningThreshold(target *v1alpha1.CamelMonitor) int {
	defaultValue := platform.GetSLIExchangeWarningThreshold()
	if target.Annotations == nil || target.Annotations[v1alpha1.MonitorSLIExchangeWarningPercentageAnnotation] == "" {
		return defaultValue
	}

	val, err := strconv.Atoi(target.Annotations[v1alpha1.MonitorSLIExchangeWarningPercentageAnnotation])
	if err == nil {
		return val
	} else {
		log.Error(err, "could not properly parse SLI warning percentage, fallback to default operator value")
	}

	return defaultValue
}
