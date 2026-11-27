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
	"github.com/camel-tooling/camel-monitor-operator/pkg/internal"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"

	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcileApp_Reconcile(t *testing.T) {
	app := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.MonitorImportedKindLabel: "Deployment",
				v1alpha1.MonitorImportedNameLabel: "my-deploy",
			},
		},
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "my-image:v1"},
					},
				},
			},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-app"}},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          3,
			AvailableReplicas: 2,
		},
	}
	fakeClient, err := internal.NewFakeClient(app, deploy)
	require.NoError(t, err)
	r := &reconcileApp{
		client:   fakeClient,
		scheme:   fakeClient.Scheme(),
		recorder: events.NewFakeRecorder(10),
	}
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-app",
			Namespace: "default",
		},
	}

	res, err := r.Reconcile(context.TODO(), req)

	require.NoError(t, err)
	require.True(t, res.RequeueAfter >= 0)

	// the monitoring time is stored, so that the next cycle can be calculated after an operator restart
	var got v1alpha1.CamelMonitor
	require.NoError(t, fakeClient.Get(context.TODO(), req.NamespacedName, &got))
	require.NotNil(t, got.Status.LastMonitoredTime)
}

func TestReconcileApp_ConditionTransitionTimeIsKept(t *testing.T) {
	oldTime := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
	app := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.MonitorImportedKindLabel: "Deployment",
				v1alpha1.MonitorImportedNameLabel: "my-deploy",
			},
		},
	}
	app.Status.AddCondition(metav1.Condition{
		Type:               v1alpha1.ConditionTypeMetricsAvailable,
		Status:             metav1.ConditionUnknown,
		LastTransitionTime: oldTime,
		Reason:             "MonitoringComplete",
	})
	app.Status.AddCondition(metav1.Condition{
		Type:               v1alpha1.ConditionTypeHealthy,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: oldTime,
		Reason:             "HealthCheckCompleted",
	})

	// Not evaluated when there is no Pod available: it must not be reported with a stale value
	app.Status.AddCondition(metav1.Condition{
		Type:               v1alpha1.ConditionTypeMemoryPressure,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: oldTime,
	})
	app.Status.AddCondition(metav1.Condition{
		Type:               v1alpha1.ConditionTypePressure,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: oldTime,
	})

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "my-image:v1"}}},
			},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-app"}},
		},
		Status: appsv1.DeploymentStatus{Replicas: 3, AvailableReplicas: 2},
	}
	fakeClient, err := internal.NewFakeClient(app, deploy)
	require.NoError(t, err)

	r := &reconcileApp{client: fakeClient, scheme: fakeClient.Scheme(), recorder: events.NewFakeRecorder(10)}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-app", Namespace: "default"}}
	_, err = r.Reconcile(context.TODO(), req)
	require.NoError(t, err)

	var got v1alpha1.CamelMonitor
	require.NoError(t, fakeClient.Get(context.TODO(), req.NamespacedName, &got))

	// No Pod available: Monitored stays Unknown, same status, same transition time
	monitored := got.Status.GetCondition(v1alpha1.ConditionTypeMetricsAvailable)
	require.NotNil(t, monitored)
	require.Equal(t, metav1.ConditionUnknown, monitored.Status)
	require.True(t, oldTime.Equal(&monitored.LastTransitionTime))
	// Healthy changed from True to Unknown: new transition time
	healthy := got.Status.GetCondition(v1alpha1.ConditionTypeHealthy)
	require.NotNil(t, healthy)
	require.Equal(t, metav1.ConditionUnknown, healthy.Status)
	require.False(t, oldTime.Equal(&healthy.LastTransitionTime))
	// Not applicable in this cycle: removed
	require.Nil(t, got.Status.GetCondition(v1alpha1.ConditionTypeMemoryPressure))
	require.Nil(t, got.Status.GetCondition(v1alpha1.ConditionTypePressure))
}

func TestReconcileApp_NotFound(t *testing.T) {
	fakeClient, err := internal.NewFakeClient()
	require.NoError(t, err)
	r := &reconcileApp{
		client: fakeClient,
		scheme: fakeClient.Scheme(),
	}
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "missing",
			Namespace: "default",
		},
	}

	res, err := r.Reconcile(context.TODO(), req)

	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, res)
}

func monitoredAt(lastPoll *time.Time, interval string) *v1alpha1.CamelMonitor {
	cmon := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        "test-app",
			Annotations: map[string]string{v1alpha1.MonitorPollingIntervalSecondsAnnotation: interval},
		},
	}
	if lastPoll != nil {
		cmon.Status.LastMonitoredTime = ptr.To(metav1.NewTime(*lastPoll))
	}

	return cmon
}

func TestTimeUntilNextPoll(t *testing.T) {
	now := time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)
	at := func(d time.Time) *time.Time { return &d }

	// 1h interval: last monitored at 12:01:02, restart at 13:00:00, next cycle at 13:01:02
	require.Equal(t, 62*time.Second,
		timeUntilNextPoll(monitoredAt(at(time.Date(2026, 1, 1, 12, 1, 2, 0, time.UTC)), "3600"), now))
	require.Equal(t, 40*time.Second, timeUntilNextPoll(monitoredAt(at(now.Add(-20*time.Second)), "60"), now))
	// overdue or never monitored: due now
	require.Equal(t, time.Duration(0), timeUntilNextPoll(monitoredAt(at(now.Add(-10*time.Minute)), "60"), now))
	require.Equal(t, time.Duration(0), timeUntilNextPoll(monitoredAt(nil, "60"), now))
}

func TestShuffleDelay(t *testing.T) {
	seen := map[time.Duration]struct{}{}

	for range 200 {
		d := shuffleDelay(100*time.Second, 20)
		require.GreaterOrEqual(t, d, time.Duration(0))
		require.Less(t, d, 20*time.Second)

		seen[d] = struct{}{}
	}

	require.Greater(t, len(seen), 1)
	require.Equal(t, time.Duration(0), shuffleDelay(0, 20))
	require.Equal(t, time.Duration(0), shuffleDelay(100*time.Second, 0))
}

func shuffleFixtures(interval string) (*v1alpha1.CamelMonitor, *appsv1.Deployment) {
	app := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
			Annotations: map[string]string{
				v1alpha1.MonitorImportedKindLabel:                "Deployment",
				v1alpha1.MonitorImportedNameLabel:                "my-deploy",
				v1alpha1.MonitorPollingIntervalSecondsAnnotation: interval,
			},
		},
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "my-image:v1"}}},
			},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-app"}},
		},
	}

	return app, deploy
}

func TestReconcileApp_StartupShuffle(t *testing.T) {
	key := types.NamespacedName{Name: "test-app", Namespace: "default"}
	req := ctrl.Request{NamespacedName: key}

	newReconciler := func(t *testing.T, startedAt time.Time, app *v1alpha1.CamelMonitor, deploy *appsv1.Deployment) (*reconcileApp, ctrlclient.Client) {
		t.Helper()

		fakeClient, err := internal.NewFakeClient(app, deploy)
		require.NoError(t, err)

		return &reconcileApp{
			client:    fakeClient,
			scheme:    fakeClient.Scheme(),
			recorder:  events.NewFakeRecorder(10),
			startedAt: startedAt,
		}, fakeClient
	}

	monitored := func(t *testing.T, c ctrlclient.Client) bool {
		t.Helper()

		var got v1alpha1.CamelMonitor
		require.NoError(t, c.Get(context.TODO(), key, &got))

		return got.Status.LastMonitoredTime != nil
	}

	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv(platform.PollingShufflePercentEnvVariable, "")

		app, deploy := shuffleFixtures("3600")
		r, c := newReconciler(t, time.Now(), app, deploy)

		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		require.Equal(t, time.Hour, res.RequeueAfter)
		require.True(t, monitored(t, c))
	})

	t.Run("without status is postponed once, then monitored without jitter", func(t *testing.T) {
		t.Setenv(platform.PollingShufflePercentEnvVariable, "100")

		app, deploy := shuffleFixtures("3600")
		r, c := newReconciler(t, time.Now(), app, deploy)

		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		require.Greater(t, res.RequeueAfter, time.Duration(0))
		require.Less(t, res.RequeueAfter, time.Hour)
		require.False(t, monitored(t, c), "nothing must be monitored while shuffling")

		// the requeue after the shuffle goes through
		res, err = r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		require.Equal(t, time.Hour, res.RequeueAfter)
		require.True(t, monitored(t, c))
	})

	t.Run("with status is not postponed", func(t *testing.T) {
		t.Setenv(platform.PollingShufflePercentEnvVariable, "100")

		app, deploy := shuffleFixtures("3600")
		app.Status.Phase = v1alpha1.CamelMonitorPhaseRunning
		r, c := newReconciler(t, time.Now(), app, deploy)

		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		require.Equal(t, time.Hour, res.RequeueAfter)
		require.True(t, monitored(t, c))
	})

	t.Run("after the operator start-up is not postponed", func(t *testing.T) {
		t.Setenv(platform.PollingShufflePercentEnvVariable, "100")

		app, deploy := shuffleFixtures("3600")
		r, c := newReconciler(t, time.Now().Add(-2*time.Hour), app, deploy)

		res, err := r.Reconcile(context.TODO(), req)
		require.NoError(t, err)
		require.Equal(t, time.Hour, res.RequeueAfter)
		require.True(t, monitored(t, c))
	})

	t.Run("deleted CamelMonitor is forgotten", func(t *testing.T) {
		r := &reconcileApp{}
		fakeClient, err := internal.NewFakeClient()
		require.NoError(t, err)
		r.client = fakeClient
		r.scheme = fakeClient.Scheme()
		r.shuffled.Store(key, struct{}{})

		_, err = r.Reconcile(context.TODO(), req)
		require.NoError(t, err)

		_, found := r.shuffled.Load(key)
		require.False(t, found)
	})
}

func TestReconcileApp_NotDueYetIsRequeuedAtNextCycle(t *testing.T) {
	lastPoll := time.Now().Add(-10 * time.Second)
	app := monitoredAt(&lastPoll, "60")
	fakeClient, err := internal.NewFakeClient(app)
	require.NoError(t, err)

	r := &reconcileApp{client: fakeClient, scheme: fakeClient.Scheme(), recorder: events.NewFakeRecorder(10)}
	res, err := r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-app", Namespace: "default"}})

	require.NoError(t, err)
	require.Greater(t, res.RequeueAfter, 45*time.Second)
	require.LessOrEqual(t, res.RequeueAfter, 50*time.Second)

	// nothing was monitored
	var got v1alpha1.CamelMonitor
	require.NoError(t, fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-app", Namespace: "default"}, &got))
	require.Empty(t, got.Status.Phase)
}
