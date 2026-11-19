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
	"testing"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func monitorWithLastPoll(lastPoll *time.Time, interval string) *v1alpha1.CamelMonitor {
	cmon := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "app",
			Annotations: map[string]string{
				v1alpha1.MonitorPollingIntervalSecondsAnnotation: interval,
			},
		},
	}
	if lastPoll != nil {
		cmon.Status.AddCondition(metav1.Condition{
			Type:               v1alpha1.ConditionTypeMonitored,
			LastTransitionTime: metav1.NewTime(*lastPoll),
		})
	}

	return cmon
}

// assertSpread checks that the delays produced by fn are all within [min, max) and are not constant.
func assertSpread(t *testing.T, fn func() time.Duration, minDelay, maxDelay time.Duration) {
	t.Helper()

	seen := map[time.Duration]struct{}{}

	for range 50 {
		d := fn()
		assert.GreaterOrEqual(t, d, minDelay)
		assert.Less(t, d, maxDelay)

		seen[d] = struct{}{}
	}

	assert.Greater(t, len(seen), 1, "jitter should produce different delays")
}

func TestStartupReconcileDelay(t *testing.T) {
	now := time.Now()

	t.Run("never monitored is spread within one polling interval", func(t *testing.T) {
		cmon := monitorWithLastPoll(nil, "60")
		assertSpread(t, func() time.Duration { return startupReconcileDelay(cmon, now) }, 0, 60*time.Second)
	})

	t.Run("not due yet waits for the remaining time plus jitter", func(t *testing.T) {
		lastPoll := now.Add(-20 * time.Second)
		cmon := monitorWithLastPoll(&lastPoll, "60")
		// 40s left, plus up to one polling interval.
		assertSpread(t, func() time.Duration { return startupReconcileDelay(cmon, now) }, 40*time.Second, 100*time.Second)
	})

	t.Run("overdue is spread within one polling interval", func(t *testing.T) {
		lastPoll := now.Add(-10 * time.Minute)
		cmon := monitorWithLastPoll(&lastPoll, "60")
		assertSpread(t, func() time.Duration { return startupReconcileDelay(cmon, now) }, 0, 60*time.Second)
	})

	t.Run("non positive polling interval does not panic", func(t *testing.T) {
		assert.Equal(t, time.Duration(0), startupReconcileDelay(monitorWithLastPoll(nil, "0"), now))
		assert.Equal(t, time.Duration(0), newResourceReconcileDelay(monitorWithLastPoll(nil, "-5")))
	})
}

func TestNewResourceReconcileDelay(t *testing.T) {
	t.Run("is spread within the resource polling interval", func(t *testing.T) {
		cmon := monitorWithLastPoll(nil, "30")
		assertSpread(t, func() time.Duration { return newResourceReconcileDelay(cmon) }, 0, 30*time.Second)
	})
}

func TestStartupSpreadHandlerCreate(t *testing.T) {
	h := &startupSpreadHandler{}
	req := reconcile.Request{NamespacedName: client.ObjectKey{Namespace: "ns", Name: "app"}}

	newQueue := func() workqueue.TypedRateLimitingInterface[reconcile.Request] {
		return workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	}

	for name, initialList := range map[string]bool{"initial list event": true, "new resource": false} {
		t.Run(name+" is delayed", func(t *testing.T) {
			q := newQueue()
			defer q.ShutDown()

			lastPoll := time.Now()
			h.Create(context.TODO(), event.TypedCreateEvent[client.Object]{
				Object:          monitorWithLastPoll(&lastPoll, "3600"),
				IsInInitialList: initialList,
			}, q)

			assert.Equal(t, 0, q.Len())
		})

		t.Run(name+" is eventually enqueued", func(t *testing.T) {
			q := newQueue()
			defer q.ShutDown()

			// Short interval so that the (random) delay is at most one second.
			h.Create(context.TODO(), event.TypedCreateEvent[client.Object]{
				Object:          monitorWithLastPoll(nil, "1"),
				IsInInitialList: initialList,
			}, q)

			require.Eventually(t, func() bool { return q.Len() == 1 }, 3*time.Second, 20*time.Millisecond)

			item, _ := q.Get()
			assert.Equal(t, req, item)
		})
	}
}
