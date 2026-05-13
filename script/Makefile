# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#
# Use bash explicitly in this Makefile to avoid unexpected platform
# incompatibilities among Linux distros.
#
SHELL := /bin/bash

MKFILE_PATH := $(abspath $(lastword $(MAKEFILE_LIST)))
PROJECT_PATH := $(patsubst %/,%,$(dir $(MKFILE_PATH)))
LOCALBIN := $(PROJECT_PATH)/bin

VERSIONFILE := pkg/util/defaults/defaults.go
VERSION ?= 0.3.0-SNAPSHOT
LAST_RELEASED_IMAGE_NAME := camel-monitor-operator
LAST_RELEASED_VERSION ?= 0.2.0
# Kubernetes tooling
CONTROLLER_GEN_VERSION := v0.17.2
CONTROLLER_GEN=$(GOBIN)/controller-gen
CODEGEN_VERSION := v0.35.0
OPERATOR_SDK_VERSION := v1.30.0
KUSTOMIZE_VERSION := v4.5.4
OPM_VERSION := v1.24.0
# Golang tooling
LINTER ?= $(LOCALBIN)/golangci-lint
DEADCODE ?= $(LOCALBIN)/deadcode
GOVULNCHECK ?= $(LOCALBIN)/govulncheck
GOIMPORT ?= $(LOCALBIN)/goimports
LINTER_VERSION ?= v1.64.5
GOVULNCHECK_VERSION ?= latest
LINT_GOGC ?= 20
LINT_DEADLINE ?= 10m

# Operator building configuration
BASE_IMAGE := registry.access.redhat.com/ubi8/ubi-minimal:8.10
IMAGE_NAME ?= quay.io/camel-tooling/camel-monitor-operator
# Test for arm64, fall back to amd64
IMAGE_ARCH ?= $(if $(filter arm64 aarch64,$(shell uname -m)),arm64,amd64)
OFFICIAL_IMAGE_ARCH := amd64
CUSTOM_IMAGE ?= $(IMAGE_NAME)
CUSTOM_VERSION ?= $(VERSION)
OPERATOR_VERSION := $(subst -SNAPSHOT,,$(CUSTOM_VERSION))

RELEASE_GIT_REMOTE := origin
GIT_COMMIT := $(shell if [ -d .git ]; then git rev-list -1 HEAD; else echo "$(CUSTOM_VERSION)"; fi)

# Used to push pre-release artifacts
STAGING_IMAGE := quay.io/camel-tooling/camel-monitor-operator

# Tests may run in parallel to each other. This count sets the amount of tests run in parallel.
# (default value would be otherwise GOMAXPROCS)
TEST_COMMON_PARALLEL_COUNT ?= 2
TEST_ADVANCED_PARALLEL_COUNT ?= 2

# OLM (Operator Lifecycle Manager and Operator Hub): uncomment to override operator settings at build time
#GOLDFLAGS += -X github.com/camel-tooling/camel-monitor-operator/pkg/util/olm.DefaultOperatorName=camel-monitor-operator
#GOLDFLAGS += -X github.com/camel-tooling/camel-monitor-operator/pkg/util/olm.DefaultPackage=camel-dashboard
#GOLDFLAGS += -X github.com/camel-tooling/camel-monitor-operator/pkg/util/olm.DefaultChannel=alpha
#GOLDFLAGS += -X github.com/camel-tooling/camel-monitor-operator/pkg/util/olm.DefaultSource=community-operators
#GOLDFLAGS += -X github.com/camel-tooling/camel-monitor-operator/pkg/util/olm.DefaultSourceNamespace=openshift-marketplace
#GOLDFLAGS += -X github.com/camel-tooling/camel-monitor-operator/pkg/util/olm.DefaultStartingCSV=
#GOLDFLAGS += -X github.com/camel-tooling/camel-monitor-operator/pkg/util/olm.DefaultGlobalNamespace=openshift-operators

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Build
ifdef GIT_COMMIT
GOLDFLAGS += -X github.com/camel-tooling/camel-monitor-operator/pkg/util/defaults.GitCommit=$(GIT_COMMIT)
else
$(warning Could not retrieve a valid Git Commit)
endif

GOFLAGS = -ldflags "$(GOLDFLAGS)" -trimpath

define LICENSE_HEADER
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
endef

export LICENSE_HEADER
default: build

bump-replace:
	@# Bump version and replace with the variables provided by the user
	@sed -i 's/^VERSION ?= .*$//VERSION ?= $(VERSION)/' ./script/Makefile
	@sed -i 's/^LAST_RELEASED_VERSION ?= .*$//LAST_RELEASED_VERSION ?= $(LAST_RELEASED_VERSION)/' ./script/Makefile

bump: bump-replace codegen

# Generates the version file
codegen:
	$(info Regenerating $(VERSIONFILE))
	@echo "/*" > $(VERSIONFILE)
	@echo "$$LICENSE_HEADER" >> $(VERSIONFILE)
	@echo "*/" >> $(VERSIONFILE)
	@echo "" >> $(VERSIONFILE)
	@echo "package defaults" >> $(VERSIONFILE)
	@echo "" >> $(VERSIONFILE)
	@echo "// ***********************" >> $(VERSIONFILE)
	@echo "//  DO NOT EDIT THIS FILE"  >> $(VERSIONFILE)
	@echo "// ***********************" >> $(VERSIONFILE)
	@echo "" >> $(VERSIONFILE)
	@echo "const (" >> $(VERSIONFILE)
	@echo "  // Version -- " >> $(VERSIONFILE)
	@echo "  Version = \"$(CUSTOM_VERSION)\"" >> $(VERSIONFILE)
	@echo "" >> $(VERSIONFILE)
	@echo ")" >> $(VERSIONFILE)
	@echo "" >> $(VERSIONFILE)
	@echo "// GitCommit must be provided during application build" >> $(VERSIONFILE)
	@echo "var GitCommit string" >> $(VERSIONFILE)
	@echo "" >> $(VERSIONFILE)

	gofmt -w pkg/util/defaults/defaults.go

generate: codegen-tools-install
	@# We better force the developer to generate all the resources everytime as
	@# it happens we tend to forget some code generation in the past
	./script/gen_client.sh
	./script/gen_crd.sh
	cd pkg/apis && $(CONTROLLER_GEN) paths="./..." object
	./script/gen_helm.sh

codegen-tools-install:
	@# We must force the installation to make sure we are using the correct version
	@# Note: as there is no --version in the tools, we cannot rely on cached local versions
	@echo "Installing k8s.io/code-generator tools with version $(CODEGEN_VERSION)"
	go install k8s.io/code-generator/cmd/applyconfiguration-gen@$(CODEGEN_VERSION)
	go install k8s.io/code-generator/cmd/client-gen@$(CODEGEN_VERSION)
	go install k8s.io/code-generator/cmd/lister-gen@$(CODEGEN_VERSION)
	go install k8s.io/code-generator/cmd/informer-gen@$(CODEGEN_VERSION)
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

build: codegen test build-camel-monitor build-compile-integration-tests

build-platform: build build-camel-monitor-platform

ci-build: clean codegen set-version check-licenses dir-licenses build-camel-monitor cross-compile

do-build: gotestfmt-install

gotestfmt-install:
ifeq (, $(shell command -v gotestfmt 2> /dev/null))
	go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@latest
endif

# Build images without running unit tests
# NOTEST=1 make images
test: do-build
ifndef NOTEST
	@echo "####### Running unit test..."
	go test ./... $(COVERAGE_OPTS)
else
	@echo "####### Skipping unit test..."
endif

test-common:
	FAILED=0; \
	go test -p $(TEST_COMMON_PARALLEL_COUNT) -timeout 30m -v ./e2e/common/... --parallel 1 -tags=integration $(GOTESTFMT) || ((FAILED++)); \
	exit $${FAILED}

test-knative:
	FAILED=0; \
	go test -p $(TEST_COMMON_PARALLEL_COUNT) -timeout 30m -v ./e2e/knative/... --parallel 1 -tags=integration $(GOTESTFMT) || ((FAILED++)); \
	exit $${FAILED}

test-prometheus:
	FAILED=0; \
	go test -p $(TEST_COMMON_PARALLEL_COUNT) -timeout 30m -v ./e2e/prometheus/... --parallel 1 -tags=integration $(GOTESTFMT) || ((FAILED++)); \
	exit $${FAILED}

build-camel-monitor:
	@echo "####### Building camel-monitor CLI for linux/$(IMAGE_ARCH) architecture..."
	CGO_ENABLED=0 GOOS=linux GOARCH=$(IMAGE_ARCH) go build $(GOFLAGS) -o build/_output/bin/camel-monitor-$(IMAGE_ARCH) ./cmd/camel-monitor/*.go
	# Symbolic link to a local CLI
	ln -sf build/_output/bin/camel-monitor-$(IMAGE_ARCH) ./camel-monitor

build-camel-monitor-platform:
# Perform only when running on OS other than linux
ifneq ($(shell uname -s 2>/dev/null || echo Unknown),Linux)
	@echo "####### Building platform specific camel-monitor CLI for $(IMAGE_ARCH) architecture..."
	CGO_ENABLED=0 GOARCH=$(IMAGE_ARCH) go build $(GOFLAGS) -o build/_output/bin/camel-monitor-$(IMAGE_ARCH) ./cmd/camel-monitor/*.go
	# Symbolic link to a local CLI
	ln -sf build/_output/bin/camel-monitor-$(IMAGE_ARCH) ./camel-monitor
endif

build-compile-integration-tests:
ifndef NOTEST
	@echo "####### Compiling integration tests..."
	export CAMEL_K_E2E_JUST_COMPILE="true"; \
	go test -run nope -tags="integration" ./e2e/...
else
	@echo "####### Skipping integration tests compilation..."
endif

clean:
	# disable gomodules when executing go clean:
	#
	#    https://github.com/golang/go/issues/31002
	#
	GO111MODULE=off go clean
	rm -f camel-monitor
	rm -f *.test
	rm -rf $(LOCALBIN)
	rm -rf build/_maven_output
	rm -rf build/_output
	rm -rf camel-monitor-client-*.tar.gz
	rm -rf camel-monitor-examples-*.tar.gz
	rm -rf $(INDEX_DIR) $(INDEX_DIR).Dockerfile

dep:
	@echo "Cleaning dependencies..."
	go mod tidy

detect-os:
ifeq '$(findstring ;,$(PATH))' ';'
OS := Windows
OS_LOWER := windows
else
OS := $(shell echo $$(uname 2>/dev/null) || echo Unknown)
OS := $(patsubst CYGWIN%,Cygwin,$(OS))
OS := $(patsubst MSYS%,MSYS,$(OS))
OS := $(patsubst MINGW%,MSYS,$(OS))
OS_LOWER := $(shell echo $(OS) | tr '[:upper:]' '[:lower:]')
endif

check: lint vuln

.PHONY: lint
lint: golangci-lint
	GOGC=$(LINT_GOGC) $(LINTER) run --config .golangci.yml --out-format colored-tab --timeout $(LINT_DEADLINE) --verbose

.PHONY: lint-fix
lint-fix: golangci-lint
	GOGC=$(LINT_GOGC) $(LINTER) run --config .golangci.yml --out-format colored-tab --timeout $(LINT_DEADLINE) --fix

.PHONY: vuln
vuln: govulncheck
	@$(GOVULNCHECK) ./...

.PHONY: fmt
fmt: goimport
	$(GOIMPORT) -l -w .
	go fmt ./...

dead-code:
	GOBIN=$(LOCALBIN) go install golang.org/x/tools/cmd/deadcode@latest
	GOGC=$(LINT_GOGC) $(DEADCODE) ./...


dir-licenses:
	./script/vendor-license-directory.sh

check-licenses:
	./script/check_licenses.sh

DOCKER_TAG := $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-$(IMAGE_ARCH)

images: build image-build build-camel-monitor-platform

image-build:
	@echo "####### Building Camel K operator arch $(IMAGE_ARCH) container image..."
	docker buildx build \
		--platform=linux/$(IMAGE_ARCH) \
		--build-arg IMAGE_ARCH=$(IMAGE_ARCH) \
		--load -t $(DOCKER_TAG) -f build/Dockerfile .
ifeq ($(IMAGE_ARCH),$(OFFICIAL_IMAGE_ARCH))
	docker tag $(DOCKER_TAG) $(CUSTOM_IMAGE):$(CUSTOM_VERSION)
endif
	docker tag $(DOCKER_TAG) $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-$(IMAGE_ARCH)

# Mainly used for internal CI purposes
image-push:
	docker push $(CUSTOM_IMAGE):$(CUSTOM_VERSION)

# Make sure the current docker builder must supports the wanted platform list, which may not be the case for the default builder
#
# docker buildx inspect
# ...
# Platforms: linux/amd64*, linux/arm64*
#
#
# docker buildx create --name mybuilder --platform linux/amd64,linux/arm64
# docker buildx use mybuilder

ARCH_VERSIONS = amd64 arm64
images-all:
	for j in $(ARCH_VERSIONS); do \
		make IMAGE_ARCH=$$j images; \
	done

IMAGE_PUSH = $(CUSTOM_IMAGE):$(CUSTOM_VERSION)
images-push-all:
	make IMAGE_PUSH=$(CUSTOM_IMAGE):$(CUSTOM_VERSION) images-push

images-push-staging-all:
	make IMAGE_PUSH=$(STAGING_IMAGE):$(CUSTOM_VERSION) images-push

images-push:
	docker push $(IMAGE_PUSH)
	docker manifest create $(IMAGE_PUSH) $(IMAGE_PUSH)
	for i in $(ARCH_VERSIONS); do \
		docker push $(IMAGE_PUSH)-$$i ; \
		docker manifest create $(IMAGE_PUSH) -a $(IMAGE_PUSH)-$$i; \
	done
	docker manifest push $(IMAGE_PUSH) --purge

images-tag-staging-all:
	docker tag $(CUSTOM_IMAGE):$(CUSTOM_VERSION) $(STAGING_IMAGE):$(CUSTOM_VERSION)
	for j in $(ARCH_VERSIONS); do \
		docker tag $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-$$j $(STAGING_IMAGE):$(CUSTOM_VERSION)-$$j; \
	done; \
	docker tag $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-$$i $(STAGING_IMAGE):$(CUSTOM_VERSION)-$$i; \
	for j in $(ARCH_VERSIONS); do \
		docker tag $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-$$i-$$j $(STAGING_IMAGE):$(CUSTOM_VERSION)-$$i-$$j; \
	done

images-push-staging:
	docker tag $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-amd64 $(STAGING_IMAGE):$(CUSTOM_VERSION)-amd64
	docker tag $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-amd64 $(STAGING_IMAGE):$(CUSTOM_VERSION)
	docker push $(STAGING_IMAGE):$(CUSTOM_VERSION)-amd64
	docker push $(STAGING_IMAGE):$(CUSTOM_VERSION)
	@if docker inspect $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-arm64 &> /dev/null; then \
		echo "Image $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-arm64 exists, building the multiarch manifest"; \
		docker tag $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-arm64 $(STAGING_IMAGE):$(CUSTOM_VERSION)-arm64; \
		docker push $(STAGING_IMAGE):$(CUSTOM_VERSION)-arm64; \
		docker manifest create $(STAGING_IMAGE):$(CUSTOM_VERSION) --amend $(STAGING_IMAGE):$(CUSTOM_VERSION)-amd64 --amend $(STAGING_IMAGE):$(CUSTOM_VERSION)-arm64; \
		docker manifest push --purge $(STAGING_IMAGE):$(CUSTOM_VERSION); \
	else \
		echo "Image $(CUSTOM_IMAGE):$(CUSTOM_VERSION)-arm64 does not exist"; \
	fi

get-image:
	@echo $(CUSTOM_IMAGE)

get-version:
	@echo $(CUSTOM_VERSION)

get-last-released-img-name:
	@echo $(LAST_RELEASED_IMAGE_NAME)

get-last-released-version:
	@echo $(LAST_RELEASED_VERSION)

set-version:
	./script/set_version.sh $(CUSTOM_VERSION) $(CUSTOM_IMAGE)

git-tag:
	./script/git_tag.sh $(CUSTOM_VERSION) $(RELEASE_GIT_REMOTE)

cross-compile:
	# we must wrap the goldflags parameters with quotes as they will need to
	# be processed as a single argument by the cross compile script
	./script/cross_compile.sh $(CUSTOM_VERSION) $(subst ","\",$(GOFLAGS))

release-staging: clean codegen set-version check-licenses images-all images-tag-staging-all images-push-staging-all cross-compile release-helm git-tag sbomgen

sbomgen:
	cyclonedx-gomod mod -licenses -json -output sbom.json

# Build images
images-nightly: clean codegen set-version images-all

# Release artifacts
release-nightly: cross-compile images-push-all

release-helm:
	./script/set_version.sh $(CUSTOM_VERSION) $(CUSTOM_IMAGE)
	./script/release_helm.sh $(CUSTOM_VERSION)

.PHONY: do-build build build-camel-monitor build-camel-monitor-platform dep codegen images images-push image-build test check clean release cross-compile package-examples set-version git-tag check-licenses release-helm release-staging release-nightly get-staging-repo get-version
.PHONY: kubectl kustomize operator-sdk opm

kubectl:
ifeq (, $(shell command -v kubectl 2> /dev/null))
	$(error "No kubectl found in PATH. Please install and re-run")
endif

bundle: set-version kustomize

kustomize:
	go install sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION)
KUSTOMIZE=$(GOBIN)/kustomize

operator-sdk: detect-os
	@echo "####### Installing operator-sdk version $(OPERATOR_SDK_VERSION)..."
	curl \
		-s -L https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$(OS_LOWER)_amd64 \
		-o operator-sdk ; \
	chmod +x operator-sdk ;\
	mkdir -p $(GOBIN) ;\
	mv operator-sdk $(GOBIN)/ ;
OPERATOR_SDK=$(GOBIN)/operator-sdk

opm: detect-os
ifeq (, $(shell command -v opm 2> /dev/null))
	@{ \
	set -e ;\
	curl \
		-L https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$(OS_LOWER)-amd64-opm \
		-o opm; \
	chmod +x opm;\
	mkdir -p $(GOBIN) ;\
	mv opm $(GOBIN)/ ;\
	}
OPM=$(GOBIN)/opm
else
	@{ \
	echo -n "opm already installed: "; \
  opm version | sed -n 's/.*"v\([^"]*\)".*/\1/p'; \
	echo " If this is less than $(OPM_VERSION) then please consider moving it aside and allowing the approved version to be downloaded."; \
	}
OPM=$(shell command -v opm 2> /dev/null)
endif

yq:
ifeq (, $(shell command -v yq 2> /dev/null))
	@GO111MODULE=on go install github.com/mikefarah/yq/v3
YQ=$(GOBIN)/yq
else
YQ=$(shell command -v yq 2> /dev/null)
endif

## Location to install dependencies to
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: golangci-lint
golangci-lint:
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(LINTER_VERSION)

.PHONY: govulncheck
govulncheck:
	GOBIN=$(LOCALBIN) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

.PHONY: goimport
goimport:
	GOBIN=$(LOCALBIN) go install golang.org/x/tools/cmd/goimports@latest

#####
# START Local installation procedure. Handy for development purpose
#####

KUSTOMIZE_DIR = "install/overlays/kubernetes/descoped"
DEFAULT_NS = "camel-monitor"

.PHONY: install install-k8s-global install-k8s-ns install-openshift-global install-openshift-ns

clone-kustomize-dir:
	$(eval KUST_TMP=$(shell mktemp -d))
	@echo "... cloning installation files to $(KUST_TMP)"
	cp -Lr install $(KUST_TMP)/.

# the set-operator targets are mostly thought as an utility for the e2e which require certain operator settings
set-operator-id:
ifdef OPERATOR_ID
	@echo "INFO: adding a suffix for ClusterRoleBindings $(OPERATOR_ID)"
	cd $(KUST_TMP)/install/base/config/rbac/descoped && kustomize edit set namesuffix -- -$(OPERATOR_ID)
else
	@echo "INFO: no operator ID variable \"OPERATOR_ID\" provided. Keeping default."
endif

# ENV is expected to be in the formal ENV="myvar1=v1 myvar2=v2 ..."
set-operator-env:
ifdef ENV
	@for n in $(ENV); do \
		k=$${n%=*}; \
		v=$${n#*=}; \
		echo "INFO: setting operator env var $${k}=$${v}"; \
		cd $(KUST_TMP)/$(KUSTOMIZE_DIR) && kustomize edit add patch --kind Deployment --patch \
		 	"[{\"op\": \"add\", \"path\": \"/spec/template/spec/containers/0/env/-\", \"value\": {\"name\": \"$${k}\", \"value\": \"$${v}\"}}]"; \
    done
endif

install-operator:
	@echo "####### Installing operator..."
ifndef NAMESPACE
	@echo "WARN: no namespace specified, using default namespace $(DEFAULT_NS)"
	$(eval NAMESPACE=$(DEFAULT_NS))
endif
	cd $(KUST_TMP)/$(KUSTOMIZE_DIR) && kustomize edit set namespace $(NAMESPACE)
	kubectl apply -k $(KUST_TMP)/$(KUSTOMIZE_DIR) --server-side --force-conflicts

install-k8s-global: DEFAULT_NS="camel-monitor"
install-k8s-global: KUSTOMIZE_DIR="install/overlays/kubernetes/descoped"
install-k8s-global: clone-kustomize-dir set-operator-id set-operator-env install-operator

install-k8s-ns: DEFAULT_NS="default"
install-k8s-ns: KUSTOMIZE_DIR="install/overlays/kubernetes/namespaced"
install-k8s-ns: clone-kustomize-dir set-operator-id set-operator-env install-operator

uninstall:
ifdef NAMESPACE
	kubectl delete deploy -l app=camel-monitor -n $(NAMESPACE)
	kubectl delete configmap,secret,sa,rolebindings,clusterrolebindings,roles,clusterroles,prometheusrules -l app=camel-monitor -n $(NAMESPACE)
else
	kubectl delete deploy -l app=camel-monitor
	kubectl delete configmap,secret,sa,rolebindings,clusterrolebindings,roles,clusterroles,prometheusrules -l app=camel-monitor
endif

install-crds:
	kubectl apply -k install/base/config/crd --server-side --force-conflicts

uninstall-crds:
	kubectl delete crd -l app=camel-monitor

uninstall-all: uninstall uninstall-crds

#####
# END Local installation procedure. Handy for development purpose
#####
