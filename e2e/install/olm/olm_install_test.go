//go:build integration
// +build integration

// To enable compilation of this file in Goland, go to "Settings -> Go -> Vendoring & Build Tags -> Custom Tags" and add "integration"

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

package olm

import (
	"context"
	"fmt"
	"os"
	"testing"

	. "github.com/camel-tooling/camel-monitor-operator/e2e/support"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func TestOLMInstallation(t *testing.T) {
	WithNewTestNamespace(t, func(ctx context.Context, g *WithT, ns string) {
		bundleImageName, ok := os.LookupEnv("BUNDLE_IMAGE_NAME")
		g.Expect(ok).To(BeTrue(), "Missing bundle image: you need to build and push to a container registry and set BUNDLE_IMAGE_NAME env var")
		os.Setenv("CAMEL_MONITOR_OPERATOR_TEST_MAKE_DIR", "../../../")
		// Install staged bundle (it must be available by building it before running the test)
		// You can build it locally via `make bundle-push` action
		ExpectExecSucceedWithTimeout(t, g,
			Make(t,
				"bundle-test",
				fmt.Sprintf("BUNDLE_IMAGE_NAME=%s", bundleImageName),
				fmt.Sprintf("NAMESPACE=%s", ns),
			),
			"300s",
		)

		// Check the operator pod is running
		g.Eventually(PodStatusPhase(t, ctx, ns, "camel.apache.org/component=operator"), TestTimeoutMedium).Should(Equal(corev1.PodRunning))
	})
}
