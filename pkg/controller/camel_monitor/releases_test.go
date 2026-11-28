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
	"io"
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

	conf := metadataConf{
		metadataUrl: server.URL,
		checkBom:    false,
	}

	cached, err := fetchMavenMetadata(context.Background(), conf)
	require.NoError(t, err)

	assert.Equal(t, "org.apache.camel", cached.GroupID)
	assert.Equal(t, "camel-core", cached.ArtifactID)
	assert.Equal(t, "4.14.0", cached.Versioning.Latest)
	assert.Equal(t, "4.14.0", cached.Versioning.Release)

	target, err := getMavenMetadata(context.Background(), &cached, conf)
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

	conf := metadataConf{
		metadataUrl: server.URL,
		checkBom:    false,
	}

	_, err := fetchMavenMetadata(context.Background(), conf)
	require.Error(t, err)
	_, err = getMavenMetadata(context.Background(), nil, conf)
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

	conf := metadataConf{
		metadataUrl: server.URL,
		checkBom:    false,
	}

	target, err := getMavenMetadata(context.Background(), &cached, conf)
	require.NoError(t, err)

	assert.Equal(t, "4.14.0", target.Versioning.Latest)
	assert.Equal(t, "4.14.0", target.Versioning.Release)
}

func TestCamelMainMavenMetadata(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/org/apache/camel/camel-core/maven-metadata.xml" {
			http.NotFound(w, r)

			return
		}

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

	t.Setenv("MAVEN_BASE_REPO_URL", server.URL)

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

		switch r.URL.Path {
		case "/io/quarkus/platform/quarkus-camel-bom/maven-metadata.xml":
			fmt.Fprint(w, `
			<metadata>
				<groupId>io.quarkus.platform</groupId>
				<artifactId>quarkus-camel-bom</artifactId>
				<versioning>
					<latest>2.40.1</latest>
					<release>2.40.1</release>
				</versioning>
			</metadata>
		`)
		case "/io/quarkus/platform/quarkus-camel-bom/2.40.1/quarkus-camel-bom-2.40.1.pom":
			fmt.Fprint(w, `
		<project xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
			<dependencyManagement>
				<dependencies>
					<dependency>
						<groupId>org.apache.camel.quarkus</groupId>
						<artifactId>camel-quarkus-core</artifactId>
						<version>2.40.0</version>
					</dependency>
				</dependencies>
			</dependencyManagement>
		</project>
		`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("MAVEN_BASE_REPO_URL", server.URL)

	cached, err := GetCamelQuarkusMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "org.apache.camel.quarkus", cached.GroupID)
	assert.Equal(t, "camel-quarkus-core", cached.ArtifactID)
	assert.Equal(t, "2.40.0", cached.Versioning.Latest)
	assert.Equal(t, "2.40.0", cached.Versioning.Release)

	target, err := GetCamelQuarkusMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, cached.Versioning.Latest, target.Versioning.Latest)
	assert.Equal(t, cached.Versioning.Release, target.Versioning.Release)
	assert.Equal(t, cached.FetchedAt, target.FetchedAt)
}

func TestCamelSpringBootMavenMetadata(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/org/apache/camel/springboot/camel-spring-boot-bom/maven-metadata.xml" {
			http.NotFound(w, r)

			return
		}

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

	t.Setenv("MAVEN_BASE_REPO_URL", server.URL)

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

func TestCamelQuarkusMavenMetadata404(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `
			<error>
				not found
			</error>
		`)
	}))
	defer server.Close()

	t.Setenv("MAVEN_BASE_REPO_URL", server.URL)

	_, err := GetCamelQuarkusMetadata(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch Maven metadata: unexpected HTTP status 404 Not Found from url")
}

func TestMetadataURLBuilder(t *testing.T) {
	assert.Equal(t, "https://repo1.maven.org/maven2/path/to/metadata", buildMetadataURL("path/to/metadata"))
}

func newXMLServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)

	return server
}

func TestFetchResponseBodySuccess(t *testing.T) {
	server := newXMLServer(t, http.StatusOK, "<ok/>")

	body, err := fetchResponseBody(context.Background(), server.URL)
	require.NoError(t, err)
	defer body.Close()

	// the body must still be readable by the caller (ownership is transferred to the caller)
	content, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "<ok/>", string(content))
}

func TestFetchResponseBodyInvalidURL(t *testing.T) {
	_, err := fetchResponseBody(context.Background(), "http://[::1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create Maven metadata request")
}

func TestFetchResponseBodyUnreachable(t *testing.T) {
	server := newXMLServer(t, http.StatusOK, "")
	url := server.URL
	server.Close()

	_, err := fetchResponseBody(context.Background(), url)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch Maven metadata")
}

func TestFetchResponseBodyErrorBodyUnreadable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// declare more bytes than we send so the client fails reading the body
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "partial")
	}))
	defer server.Close()

	_, err := fetchResponseBody(context.Background(), server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected HTTP status 500 Internal Server Error (failed to read response body")
}

func TestFetchMavenMetadataWithBomErrors(t *testing.T) {
	const quarkusMetadata = `
		<metadata>
			<groupId>io.quarkus.platform</groupId>
			<artifactId>quarkus-camel-bom</artifactId>
			<versioning>
				<latest>2.40.1</latest>
				<release>2.40.1</release>
			</versioning>
		</metadata>`

	tests := []struct {
		name        string
		bomStatus   int
		bomBody     string
		errContains string
	}{
		{
			name:        "bom fetch fails",
			bomStatus:   http.StatusNotFound,
			bomBody:     "not found",
			errContains: "unexpected HTTP status 404 Not Found",
		},
		{
			name:        "bom is not a valid xml",
			bomStatus:   http.StatusOK,
			bomBody:     "",
			errContains: "EOF",
		},
		{
			name:      "dependency not found in bom",
			bomStatus: http.StatusOK,
			bomBody: `
				<project>
					<dependencyManagement>
						<dependencies>
							<dependency>
								<groupId>org.apache.camel.quarkus</groupId>
								<artifactId>camel-quarkus-other</artifactId>
								<version>2.40.0</version>
							</dependency>
						</dependencies>
					</dependencyManagement>
				</project>`,
			errContains: "could not fetch properly BOM from",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadataServer := newXMLServer(t, http.StatusOK, quarkusMetadata)
			bomServer := newXMLServer(t, tt.bomStatus, tt.bomBody)
			t.Setenv("MAVEN_BASE_REPO_URL", bomServer.URL)

			_, err := fetchMavenMetadata(context.Background(), metadataConf{
				metadataUrl: metadataServer.URL,
				checkBom:    true,
				groupId:     "org.apache.camel.quarkus",
				artifactId:  "camel-quarkus-core",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestAdaptBomToMavenMetadata(t *testing.T) {
	b := &bom{Dependencies: []dependency{
		{GroupID: "org.apache.camel.quarkus", ArtifactID: "camel-quarkus-other", Version: "1.0.0"},
		{GroupID: "other.group", ArtifactID: "camel-quarkus-core", Version: "2.0.0"},
		{GroupID: "org.apache.camel.quarkus", ArtifactID: "camel-quarkus-core", Version: "3.0.0"},
	}}

	found := adaptBomToMavenMetadata("org.apache.camel.quarkus", "camel-quarkus-core", b)
	require.NotNil(t, found)
	assert.Equal(t, "3.0.0", found.Versioning.Latest)
	assert.Equal(t, "3.0.0", found.Versioning.Release)

	assert.Nil(t, adaptBomToMavenMetadata("org.apache.camel.quarkus", "missing", b))
	assert.Nil(t, adaptBomToMavenMetadata("missing", "camel-quarkus-core", &bom{}))
}

func TestMavenMetadataPath(t *testing.T) {
	path, err := mavenMetadataPath("org.apache.camel:camel-core")
	require.NoError(t, err)
	assert.Equal(t, "org/apache/camel/camel-core/maven-metadata.xml", path)

	path, err = mavenMetadataPath(" io.quarkus.platform:quarkus-camel-bom ")
	require.NoError(t, err)
	assert.Equal(t, "io/quarkus/platform/quarkus-camel-bom/maven-metadata.xml", path)

	for _, coordinates := range []string{
		"",
		"org.apache.camel",
		"org.apache.camel:",
		":camel-core",
		"org.apache.camel:camel-core:4.14.0",
		"org/apache/camel:camel-core",
		"org.apache.camel:camel core",
	} {
		_, err := mavenMetadataPath(coordinates)
		require.Error(t, err, coordinates)
		assert.Contains(t, err.Error(), "expected groupId:artifactId")
	}
}

func TestCamelMainMavenMetadataCustomCoordinates(t *testing.T) {
	cleanCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/com/example/my-camel-core/maven-metadata.xml" {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `
			<metadata>
				<groupId>com.example</groupId>
				<artifactId>my-camel-core</artifactId>
				<versioning>
					<latest>4.14.0-custom</latest>
					<release>4.14.0-custom</release>
				</versioning>
			</metadata>
		`)
	}))
	defer server.Close()

	t.Setenv("MAVEN_BASE_REPO_URL", server.URL)
	t.Setenv("CAMEL_MAIN_MAVEN_COORDINATES", "com.example:my-camel-core")

	meta, err := GetCamelMainMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "4.14.0-custom", meta.Versioning.Release)
}

func TestCamelMainMavenMetadataInvalidCoordinates(t *testing.T) {
	cleanCache(t)
	t.Setenv("CAMEL_MAIN_MAVEN_COORDINATES", "org/apache/camel/camel-core/maven-metadata.xml")

	_, err := GetCamelMainMetadata(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CAMEL_MAIN_MAVEN_COORDINATES: invalid Maven coordinates")
}
