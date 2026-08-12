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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMavenMetadata(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `
			<metadata>
				<groupId>org.apache.camel</groupId>
				<artifactId>camel-core</artifactId>
				<versioning>
					<latest>4.14.0</latest>
					<release>4.14.0</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	cached, err := fetchMavenMetadata(context.Background(), server.URL)
	require.NoError(t, err)

	assert.Equal(t, "org.apache.camel", cached.GroupID)
	assert.Equal(t, "camel-core", cached.ArtifactID)
	assert.Equal(t, "4.14.0", cached.Versioning.Latest)
	assert.Equal(t, "4.14.0", cached.Versioning.Release)

	target, err := getMavenMetadata(context.Background(), &cached, server.URL)
	require.NoError(t, err)

	assert.Equal(t, cached.Versioning.Latest, target.Versioning.Latest)
	assert.Equal(t, cached.Versioning.Release, target.Versioning.Release)
	assert.Equal(t, cached.FetchedAt, target.FetchedAt)
}

func TestMissingMavenMetadata(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, ``)
	}))
	defer server.Close()

	_, err := fetchMavenMetadata(context.Background(), server.URL)
	require.Error(t, err)
	_, err = getMavenMetadata(context.Background(), nil, server.URL)
	require.Error(t, err)
}

func TestRefreshCacheMavenMetadata(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `
			<metadata>
				<groupId>org.apache.camel</groupId>
				<artifactId>camel-core</artifactId>
				<versioning>
					<latest>4.14.0</latest>
					<release>4.14.0</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	cached := MavenMetadata{
		GroupID:    "org.apache.camel",
		ArtifactID: "camel-core",
		FetchedAt:  time.Now().Add(-25 * time.Hour),
		Versioning: MavenVersioning{
			Latest:  "4.0.0",
			Release: "4.0.0",
		},
	}

	target, err := getMavenMetadata(context.Background(), &cached, server.URL)
	require.NoError(t, err)

	assert.Equal(t, "4.14.0", target.Versioning.Latest)
	assert.Equal(t, "4.14.0", target.Versioning.Release)
}

func TestCamelMainMavenMetadata(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `
			<metadata>
				<groupId>org.apache.camel</groupId>
				<artifactId>camel-core</artifactId>
				<versioning>
					<latest>4.14.0</latest>
					<release>4.14.0</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	t.Setenv("CAMEL_MAIN_MAVEN_META_URL", server.URL)

	cached, err := GetCamelMainMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "org.apache.camel", cached.GroupID)
	assert.Equal(t, "camel-core", cached.ArtifactID)
	assert.Equal(t, "4.14.0", cached.Versioning.Latest)
	assert.Equal(t, "4.14.0", cached.Versioning.Release)

	target, err := GetCamelMainMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, cached.Versioning.Latest, target.Versioning.Latest)
	assert.Equal(t, cached.Versioning.Release, target.Versioning.Release)
	assert.Equal(t, cached.FetchedAt, target.FetchedAt)
}

func TestCamelQuarkusMavenMetadata(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `
			<metadata>
				<groupId>org.apache.camel.quarkus</groupId>
				<artifactId>camel-quarkus</artifactId>
				<versioning>
					<latest>4.14.0</latest>
					<release>4.14.0</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	t.Setenv("CAMEL_QUARKUS_MAVEN_META_URL", server.URL)

	cached, err := GetCamelQuarkusMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "org.apache.camel.quarkus", cached.GroupID)
	assert.Equal(t, "camel-quarkus", cached.ArtifactID)
	assert.Equal(t, "4.14.0", cached.Versioning.Latest)
	assert.Equal(t, "4.14.0", cached.Versioning.Release)

	target, err := GetCamelQuarkusMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, cached.Versioning.Latest, target.Versioning.Latest)
	assert.Equal(t, cached.Versioning.Release, target.Versioning.Release)
	assert.Equal(t, cached.FetchedAt, target.FetchedAt)
}

func TestCamelSpringBootMavenMetadata(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `
			<metadata>
				<groupId>org.apache.camel.springboot</groupId>
				<artifactId>camel-sb</artifactId>
				<versioning>
					<latest>4.14.0</latest>
					<release>4.14.0</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	t.Setenv("CAMEL_SPRING_BOOT_MAVEN_META_URL", server.URL)

	cached, err := GetCamelSpringBootMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "org.apache.camel.springboot", cached.GroupID)
	assert.Equal(t, "camel-sb", cached.ArtifactID)
	assert.Equal(t, "4.14.0", cached.Versioning.Latest)
	assert.Equal(t, "4.14.0", cached.Versioning.Release)

	target, err := GetCamelSpringBootMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, cached.Versioning.Latest, target.Versioning.Latest)
	assert.Equal(t, cached.Versioning.Release, target.Versioning.Release)
	assert.Equal(t, cached.FetchedAt, target.FetchedAt)
}

// cleanCache is required to perform a clean state and avoid caching affect other tests execution.
func cleanCache(t *testing.T) {
	t.Helper()

	camelMainMetadataCache = nil
	camelQuarkusMetadataCache = nil
	camelSpringBootMetadataCache = nil
}
