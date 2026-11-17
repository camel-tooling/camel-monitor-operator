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

package kubernetes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func newLogClient(t *testing.T, handler http.HandlerFunc) kubernetes.Interface {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)
	return client
}

func TestDumpLog(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "my-ns"}}

	t.Run("returns the full log", func(t *testing.T) {
		client := newLogClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("line1\nline2\n"))
		})

		got, err := DumpLog(context.Background(), client, pod, corev1.PodLogOptions{})
		require.NoError(t, err)
		assert.Equal(t, "line1\nline2\n", got)
	})

	t.Run("returns empty string for empty log", func(t *testing.T) {
		client := newLogClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		got, err := DumpLog(context.Background(), client, pod, corev1.PodLogOptions{})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("requests the log of the given pod with the given options", func(t *testing.T) {
		var gotPath, gotContainer, gotTail string
		client := newLogClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotContainer = r.URL.Query().Get("container")
			gotTail = r.URL.Query().Get("tailLines")
			_, _ = w.Write([]byte("ok"))
		})

		tail := int64(10)
		_, err := DumpLog(context.Background(), client, pod, corev1.PodLogOptions{Container: "app", TailLines: &tail})
		require.NoError(t, err)
		assert.Equal(t, "/api/v1/namespaces/my-ns/pods/my-pod/log", gotPath)
		assert.Equal(t, "app", gotContainer)
		assert.Equal(t, "10", gotTail)
	})

	t.Run("returns error when the API call fails", func(t *testing.T) {
		client := newLogClient(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "pod not found", http.StatusNotFound)
		})

		got, err := DumpLog(context.Background(), client, pod, corev1.PodLogOptions{})
		require.Error(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error when the context is cancelled", func(t *testing.T) {
		client := newLogClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("never read"))
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		got, err := DumpLog(ctx, client, pod, corev1.PodLogOptions{})
		require.Error(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error when reading the body fails", func(t *testing.T) {
		client := newLogClient(t, func(w http.ResponseWriter, r *http.Request) {
			// Announce more bytes than are sent, then drop the connection.
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("partial"))
			if h, ok := w.(http.Hijacker); ok {
				conn, _, err := h.Hijack()
				if err == nil {
					_ = conn.Close()
				}
			}
		})

		got, err := DumpLog(context.Background(), client, pod, corev1.PodLogOptions{})
		require.Error(t, err)
		assert.Empty(t, got)
	})
}
