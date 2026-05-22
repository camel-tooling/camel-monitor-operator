#!/bin/bash

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

set -e

if [ "$#" -lt 2 ]; then
    echo "usage: $0 prepare-operators <release-version> <github-user>"
    exit 1
fi

location=$(dirname $0)
version=$1
gh_user=$2

cd bundle/

mkdir -p k8s-operatorhub/$1/manifests/
mkdir -p k8s-operatorhub/$1/metadata/
mkdir -p k8s-operatorhub/$1/tests/scorecard/
mkdir -p openshift-ecosystem/$1/manifests/
mkdir -p openshift-ecosystem/$1/metadata/
mkdir -p openshift-ecosystem/$1/tests/scorecard/

cp ./manifests/camel.apache.org_camelmonitors.yaml k8s-operatorhub/$1/manifests/camelmonitors.camel.apache.org.crd.yaml
cp ./manifests/camel-monitor-operator.clusterserviceversion.yaml k8s-operatorhub/$1/manifests/camel-monitor-operator.v$1.clusterserviceversion.yaml
cp ./metadata/annotations.yaml k8s-operatorhub/$1/metadata/annotations.yaml
cp ./tests/scorecard/config.yaml k8s-operatorhub/$1/tests/scorecard/config.yaml

cp ./manifests/camel.apache.org_camelmonitors.yaml openshift-ecosystem/$1/manifests/camelmonitors.camel.apache.org.crd.yaml
cp ./manifests/camel-monitor-operator.clusterserviceversion.yaml openshift-ecosystem/$1/manifests/camel-monitor-operator.v$1.clusterserviceversion.yaml
cp ./metadata/annotations.yaml openshift-ecosystem/$1/metadata/annotations.yaml
cp ./tests/scorecard/config.yaml openshift-ecosystem/$1/tests/scorecard/config.yaml

# Clone projects
git clone https://github.com/$gh_user/community-operators.git /tmp/operators/community-operators
mkdir -p /tmp/operators/community-operators/operators/camel-monitor-operator
cp -r k8s-operatorhub/$version /tmp/operators/community-operators/operators/camel-monitor-operator/.
git clone https://github.com/$gh_user/community-operators-prod.git /tmp/operators/community-operators-prod
mkdir -p /tmp/operators/community-operators-prod/operators/camel-monitor-operator
cp -r openshift-ecosystem/$version /tmp/operators/community-operators-prod/operators/camel-monitor-operator/.

# Community operators
cd /tmp/operators/community-operators
git checkout -b feat/v$version
git add operators/camel-monitor-operator/$version
git commit -s -m "operator camel-monitor ($version)"
git remote add upstream https://github.com/k8s-operatorhub/community-operators -f
git pull --rebase upstream main
git push --set-upstream origin feat/v$version

# Community operators PROD
cd /tmp/operators/community-operators-prod
git checkout -b feat/v$version
git add operators/camel-monitor-operator/$version
git commit -s -m "operator camel-monitor ($version)"
git remote add upstream https://github.com/redhat-openshift-ecosystem/community-operators-prod -f
git pull --rebase upstream main
git push --set-upstream origin feat/v$version

echo "### You need to create PRs manually:"
echo "--> https://github.com/$gh_user/community-operators/pull/new/feat/v$version"
echo "--> https://github.com/$gh_user/community-operators-prod/pull/new/feat/v$version"
