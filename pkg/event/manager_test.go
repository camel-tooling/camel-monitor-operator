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
	"strings"
	"testing"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
)

// helper to read a single event or fail
func requireEvent(t *testing.T, recorder *events.FakeRecorder) string {
	t.Helper()

	select {
	case evt := <-recorder.Events:
		return evt
	default:
		t.Fatalf("expected event, but none was recorded")
		return ""
	}
}

// helper to assert no events were recorded
func requireNoEvent(t *testing.T, recorder *events.FakeRecorder) {
	t.Helper()

	select {
	case evt := <-recorder.Events:
		t.Fatalf("did not expect event, but got: %s", evt)
	default:
		// ok
	}
}

func TestNotifyAppError(t *testing.T) {
	tests := []struct {
		name        string
		old         *v1alpha1.CamelMonitor
		newResource *v1alpha1.CamelMonitor
		expectEvent bool
	}{
		{
			name: "uses new resource when present",
			newResource: &v1alpha1.CamelMonitor{
				ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
			},
			expectEvent: true,
		},
		{
			name: "falls back to old resource",
			old: &v1alpha1.CamelMonitor{
				ObjectMeta: metav1.ObjectMeta{Name: "old-app"},
			},
			expectEvent: true,
		},
		{
			name:        "no resources means no event",
			expectEvent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := events.NewFakeRecorder(1)

			NotifyAppError(
				recorder,
				tt.old,
				tt.newResource,
				assert.AnError,
			)

			if tt.expectEvent {
				evt := requireEvent(t, recorder)

				if !strings.Contains(evt, corev1.EventTypeWarning) {
					t.Errorf("expected Warning event, got: %s", evt)
				}
				if !strings.Contains(evt, "AppError") {
					t.Errorf("expected reason AppError, got: %s", evt)
				}
				if !strings.Contains(evt, "Cannot reconcile App") {
					t.Errorf("unexpected message: %s", evt)
				}
			} else {
				requireNoEvent(t, recorder)
			}
		})
	}
}

func TestNotifyAppUpdated(t *testing.T) {
	tests := []struct {
		name        string
		oldPhase    string
		newPhase    string
		expectEvent bool
	}{
		{
			name:        "phase changed",
			oldPhase:    "Ready",
			newPhase:    "Error",
			expectEvent: true,
		},
		{
			name:        "same phase",
			oldPhase:    "Ready",
			newPhase:    "Ready",
			expectEvent: false,
		},
		{
			name:        "old nil, new has phase",
			oldPhase:    "",
			newPhase:    "Ready",
			expectEvent: true,
		},
		{
			name:        "new resource nil",
			expectEvent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := events.NewFakeRecorder(1)

			var oldApp *v1alpha1.CamelMonitor
			if tt.oldPhase != "" {
				oldApp = &v1alpha1.CamelMonitor{
					Status: v1alpha1.CamelMonitorStatus{
						Phase: v1alpha1.CamelMonitorPhase(tt.oldPhase),
					},
				}
			}

			var newApp *v1alpha1.CamelMonitor
			if tt.newPhase != "" {
				newApp = &v1alpha1.CamelMonitor{
					ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
					Status: v1alpha1.CamelMonitorStatus{
						Phase: v1alpha1.CamelMonitorPhase(tt.newPhase),
					},
				}
			}

			NotifyAppUpdated(recorder, oldApp, newApp)

			if tt.expectEvent {
				evt := requireEvent(t, recorder)

				if !strings.Contains(evt, corev1.EventTypeNormal) {
					t.Errorf("expected Normal event, got: %s", evt)
				}
				if !strings.Contains(evt, "AppUpdated") {
					t.Errorf("expected reason AppUpdated, got: %s", evt)
				}
				if !strings.Contains(evt, tt.newPhase) {
					t.Errorf("expected phase %q in message, got: %s", tt.newPhase, evt)
				}
			} else {
				requireNoEvent(t, recorder)
			}
		})
	}
}

func TestNotifyIfPhaseUpdated_EmptyPhase(t *testing.T) {
	recorder := events.NewFakeRecorder(1)

	app := &v1alpha1.CamelMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
	}

	notifyIfPhaseUpdated(
		recorder,
		app,
		"Ready",
		"",
		"App",
		"my-app",
		"AppUpdated",
		"",
	)

	evt := requireEvent(t, recorder)
	if !strings.Contains(evt, "[none]") {
		t.Errorf("expected empty phase to be rendered as [none], got: %s", evt)
	}
}
