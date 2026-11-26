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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
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

// dependency holds only the fields we care about.
type dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

// bom maps only the path project > dependencyManagement > dependencies > dependency.
// Everything else in the POM (build, plugins, properties, top-level <dependencies>, ...) is skipped.
type bom struct {
	Dependencies []dependency `xml:"dependencyManagement>dependencies>dependency"`
}

type metadataConf struct {
	metadataUrl string
	checkBom    bool
	groupId     string
	artifactId  string
}

var (
	mavenMetadataMu sync.Mutex

	camelMainMetadataCache       *MavenMetadata
	camelQuarkusMetadataCache    *MavenMetadata
	camelSpringBootMetadataCache *MavenMetadata

	releaseUpdateHttpClient = &http.Client{
		Timeout: 10 * time.Second,
	}

	defaultCamelMainMavenMetadata       = "org/apache/camel/camel-core/maven-metadata.xml"
	defaultCamelQuarkusMavenMetadata    = "io/quarkus/platform/quarkus-camel-bom/maven-metadata.xml"
	defaultCamelSpringBootMavenMetadata = "org/apache/camel/springboot/camel-spring-boot-bom/maven-metadata.xml"
)

const (
	cacheMetadataTTL = 24 * time.Hour
	maxErrorBodySize = 16 * 1024
)

// GetCamelMainMetadata is in charge to recover the Camel Main maven metadata.
func GetCamelMainMetadata(ctx context.Context) (MavenMetadata, error) {
	camelMainMavenMetadataURL := os.Getenv("CAMEL_MAIN_MAVEN_META_URL")
	if camelMainMavenMetadataURL == "" {
		camelMainMavenMetadataURL = buildMetadataURL(defaultCamelMainMavenMetadata)
	}

	conf := metadataConf{
		metadataUrl: camelMainMavenMetadataURL,
		checkBom:    false,
	}

	meta, err := getMavenMetadata(ctx, camelMainMetadataCache, conf)
	if err != nil {
		return MavenMetadata{}, err
	}

	camelMainMetadataCache = &meta

	return *camelMainMetadataCache, nil
}

// GetCamelQuarkusMetadata is in charge to recover the Camel Quarkus maven metadata. We must make two request:
// 1. to get the quarkus platform version
// 2. to get the camel quarkus core version used in such a platform.
func GetCamelQuarkusMetadata(ctx context.Context) (MavenMetadata, error) {
	camelQuarkusMavenMetadataURL := os.Getenv("CAMEL_QUARKUS_MAVEN_META_URL")
	if camelQuarkusMavenMetadataURL == "" {
		camelQuarkusMavenMetadataURL = buildMetadataURL(defaultCamelQuarkusMavenMetadata)
	}

	conf := metadataConf{
		metadataUrl: camelQuarkusMavenMetadataURL,
		checkBom:    true,
		groupId:     "org.apache.camel.quarkus",
		artifactId:  "camel-quarkus-core",
	}

	meta, err := getMavenMetadata(ctx, camelQuarkusMetadataCache, conf)
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
		camelSpringBootMavenMetadataURL = buildMetadataURL(defaultCamelSpringBootMavenMetadata)
	}

	conf := metadataConf{
		metadataUrl: camelSpringBootMavenMetadataURL,
		checkBom:    false,
	}

	meta, err := getMavenMetadata(ctx, camelSpringBootMetadataCache, conf)
	if err != nil {
		return MavenMetadata{}, err
	}

	camelSpringBootMetadataCache = &meta

	return *camelSpringBootMetadataCache, nil
}

func buildMetadataURL(meta string) string {
	return platform.GetMavenBaseRepoUrl() + "/" + meta
}

// getMavenMetadata recover the metadata as expected in maven-metadata.xml file
// (eg, https://repo1.maven.org/maven2/org/apache/camel/camel-core/maven-metadata.xml). It fetches the url if
// the cache TTL has expired.
func getMavenMetadata(ctx context.Context, cached *MavenMetadata, conf metadataConf) (MavenMetadata, error) {
	mavenMetadataMu.Lock()
	defer mavenMetadataMu.Unlock()

	if cached != nil &&
		time.Now().Before(cached.FetchedAt.Add(cacheMetadataTTL)) {
		return *cached, nil
	}

	metadata, err := fetchMavenMetadata(ctx, conf)
	if err != nil {
		return MavenMetadata{}, err
	}

	return metadata, nil
}

func fetchMavenMetadata(ctx context.Context, conf metadataConf) (MavenMetadata, error) {
	emptyMetadata := MavenMetadata{}

	responseBody, err := fetchResponseBody(ctx, conf.metadataUrl)
	if err != nil {
		return emptyMetadata, err
	}

	defer func() {
		err := responseBody.Close()
		if err != nil {
			log.Error(err, "failed to close response body")
		}
	}()

	// Read from metadata
	metadata, err := readMavenMetadata(responseBody)
	if err != nil {
		return emptyMetadata, fmt.Errorf("decode Maven metadata: %w", err)
	}

	// Read from BOM
	if conf.checkBom {
		bomUrl := buildMetadataURL(buildBomUrl(metadata))

		responseBody, err := fetchResponseBody(ctx, bomUrl)
		if err != nil {
			return emptyMetadata, err
		}

		defer func() {
			err := responseBody.Close()
			if err != nil {
				log.Error(err, "failed to close response body")
			}
		}()

		bom, err := readBom(responseBody)
		if err != nil {
			return emptyMetadata, err
		}

		metadata = adaptBomToMavenMetadata(conf.groupId, conf.artifactId, bom)
		if metadata == nil {
			return emptyMetadata, errors.New("could not fetch properly BOM from " + bomUrl)
		}
	}

	metadata.FetchedAt = time.Now()

	return *metadata, nil
}

func fetchResponseBody(ctx context.Context, url string) (io.ReadCloser, error) {
	// #nosec G704 -- URL comes from trusted administrator configuration.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create Maven metadata request: %w", err)
	}

	// #nosec G704 -- URL comes from trusted administrator configuration.
	resp, err := releaseUpdateHttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Maven metadata: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() {
			err := resp.Body.Close()
			if err != nil {
				log.Error(err, "failed to close response body")
			}
		}()

		body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		if err != nil {
			return nil, fmt.Errorf(
				"fetch Maven metadata: unexpected HTTP status %s (failed to read response body: %w)",
				resp.Status,
				err,
			)
		}

		// We log the content of the error but we better not report as it ends up in the
		// failing condition (could be a big chunk of code). This is likely useful info
		// for admin that can access to operator log.
		log.Infof("http call to %s errored with status code %d: %s",
			url, resp.StatusCode, strings.TrimSpace(string(body)))

		return nil, fmt.Errorf(
			"fetch Maven metadata: unexpected HTTP status %s from url %s",
			resp.Status,
			url,
		)
	}

	return resp.Body, nil
}

// Scan the content as a Bom type.
func readBom(r io.Reader) (*bom, error) {
	var p bom

	err := xml.NewDecoder(r).Decode(&p)
	if err != nil {
		return nil, err
	}

	return &p, nil
}

// Scan the content as a Maven metadata type.
func readMavenMetadata(r io.Reader) (*MavenMetadata, error) {
	var metadata MavenMetadata

	err := xml.NewDecoder(r).Decode(&metadata)
	if err != nil {
		return nil, err
	}

	return &metadata, nil
}

// turn a bom into a Maven metadata type.
func adaptBomToMavenMetadata(groupId, artifactId string, b *bom) *MavenMetadata {
	for _, d := range b.Dependencies {
		if d.GroupID == groupId && artifactId == d.ArtifactID {
			return &MavenMetadata{
				GroupID:    d.GroupID,
				ArtifactID: d.ArtifactID,
				Versioning: MavenVersioning{
					Latest:  d.Version,
					Release: d.Version,
				},
			}
		}
	}

	return nil
}

// return the url of the bom referenced in the metadata,
// for instance "io/quarkus/platform/quarkus-camel-bom/3.40.1/quarkus-camel-bom-3.40.1.pom".
func buildBomUrl(meta *MavenMetadata) string {
	gid := strings.ReplaceAll(meta.GroupID, ".", "/")
	art := meta.ArtifactID + "/" + meta.Versioning.Latest + "/" + meta.ArtifactID + "-" + meta.Versioning.Latest + ".pom"

	return gid + "/" + art
}
