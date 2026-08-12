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
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/util/log"
)

type MavenMetadata struct {
	GroupID    string          `xml:"groupId"`
	ArtifactID string          `xml:"artifactId"`
	Versioning MavenVersioning `xml:"versioning"`
	FetchedAt  time.Time       `xml:"-"`
}

type MavenVersioning struct {
	Latest  string `xml:"latest"`
	Release string `xml:"release"`
}

var (
	mavenMetadataMu sync.Mutex

	camelMainMetadataCache       *MavenMetadata
	camelQuarkusMetadataCache    *MavenMetadata
	camelSpringBootMetadataCache *MavenMetadata

	releaseUpdateHttpClient = &http.Client{
		Timeout: 10 * time.Second,
	}
)

const (
	defaultCamelMainMavenMetadata       = "https://repo1.maven.org/maven2/org/apache/camel/camel-core/maven-metadata.xml"
	defaultCamelQuarkusMavenMetadata    = "https://repo1.maven.org/maven2/io/quarkus/platform/quarkus-camel-bom/maven-metadata.xml"
	defaultCamelSpringBootMavenMetadata = "https://repo1.maven.org/maven2/org/apache/camel/springboot/camel-spring-boot-bom/maven-metadata.xml"

	cacheMetadataTTL = 24 * time.Hour
)

// GetCamelMainMetadata is in charge to recover the Camel Main maven metadata.
func GetCamelMainMetadata(ctx context.Context) (MavenMetadata, error) {
	camelMainMavenMetadataURL := os.Getenv("CAMEL_MAIN_MAVEN_META_URL")
	if camelMainMavenMetadataURL == "" {
		camelMainMavenMetadataURL = defaultCamelMainMavenMetadata
	}

	meta, err := getMavenMetadata(ctx, camelMainMetadataCache, camelMainMavenMetadataURL)
	if err != nil {
		return MavenMetadata{}, err
	}

	camelMainMetadataCache = &meta

	return *camelMainMetadataCache, nil
}

// GetCamelQuarkusMetadata is in charge to recover the Camel Quarkus maven metadata.
func GetCamelQuarkusMetadata(ctx context.Context) (MavenMetadata, error) {
	camelQuarkusMavenMetadataURL := os.Getenv("CAMEL_QUARKUS_MAVEN_META_URL")
	if camelQuarkusMavenMetadataURL == "" {
		camelQuarkusMavenMetadataURL = defaultCamelQuarkusMavenMetadata
	}

	meta, err := getMavenMetadata(ctx, camelQuarkusMetadataCache, camelQuarkusMavenMetadataURL)
	if err != nil {
		return MavenMetadata{}, err
	}

	camelQuarkusMetadataCache = &meta

	return *camelQuarkusMetadataCache, nil
}

// GetCamelSpringBootMetadata is in charge to recover the Camel Spring Boot metadata.
func GetCamelSpringBootMetadata(ctx context.Context) (MavenMetadata, error) {
	camelSpringBootMavenMetadataURL := os.Getenv("CAMEL_SPRING_BOOT_MAVEN_META_URL")
	if camelSpringBootMavenMetadataURL == "" {
		camelSpringBootMavenMetadataURL = defaultCamelSpringBootMavenMetadata
	}

	meta, err := getMavenMetadata(ctx, camelSpringBootMetadataCache, camelSpringBootMavenMetadataURL)
	if err != nil {
		return MavenMetadata{}, err
	}

	camelSpringBootMetadataCache = &meta

	return *camelSpringBootMetadataCache, nil
}

// getMavenMetadata recover the metadata as expected in maven-metadata.xml file
// (eg, https://repo1.maven.org/maven2/org/apache/camel/camel-core/maven-metadata.xml). It fetches the url if
// the cache TTL has expired.
func getMavenMetadata(ctx context.Context, cached *MavenMetadata, url string) (MavenMetadata, error) {
	mavenMetadataMu.Lock()
	defer mavenMetadataMu.Unlock()

	if cached != nil &&
		time.Now().Before(cached.FetchedAt.Add(cacheMetadataTTL)) {
		return *cached, nil
	}

	metadata, err := fetchMavenMetadata(ctx, url)
	if err != nil {
		return MavenMetadata{}, err
	}

	return metadata, nil
}

func fetchMavenMetadata(ctx context.Context, url string) (MavenMetadata, error) {
	// #nosec G704 -- URL comes from trusted administrator configuration.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return MavenMetadata{}, fmt.Errorf("create Maven metadata request: %w", err)
	}

	// #nosec G704 -- URL comes from trusted administrator configuration.
	resp, err := releaseUpdateHttpClient.Do(req)
	if err != nil {
		return MavenMetadata{}, fmt.Errorf("fetch Maven metadata: %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Error(err, "failed to close response body")
		}
	}()

	var metadata MavenMetadata

	err = xml.NewDecoder(resp.Body).Decode(&metadata)
	if err != nil {
		return MavenMetadata{}, fmt.Errorf("decode Maven metadata: %w", err)
	}

	metadata.FetchedAt = time.Now()

	return metadata, nil
}
