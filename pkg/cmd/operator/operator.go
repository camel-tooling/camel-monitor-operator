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

package operator

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	integreatlyv1beta1 "github.com/grafana/grafana-operator/v5/api/v1beta1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	zapctrl "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis"
	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/camel-tooling/camel-monitor-operator/pkg/controller"
	"github.com/camel-tooling/camel-monitor-operator/pkg/controller/synthetic"
	"github.com/camel-tooling/camel-monitor-operator/pkg/platform"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util"
	"github.com/camel-tooling/camel-monitor-operator/pkg/util/defaults"
	logutil "github.com/camel-tooling/camel-monitor-operator/pkg/util/log"
)

var log = logutil.Log.WithName("cmd")

func printVersion() {
	log.Infof("Go Version: %s", runtime.Version())
	log.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	log.Infof("Camel Monitor Operator Version: %v", defaults.Version)
	log.Infof("Camel Monitor Operator Git Commit: %v", defaults.GitCommit)
	log.Infof("Camel Monitor Operator ID: %v", defaults.OperatorID())

	// Will only appear if DEBUG level has been enabled using the env var LOG_LEVEL
	log.Debug("*** DEBUG level messages will be logged ***")
}

// Run starts the Camel Monitor operator.
func Run(healthPort, monitoringPort int, leaderElection bool, leaderElectionID string) {
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.

	// The constants specified here are zap specific
	var logLevel zapcore.Level

	logLevelVal, ok := os.LookupEnv("LOG_LEVEL")
	if ok {
		switch strings.ToLower(logLevelVal) {
		case "error":
			logLevel = zapcore.ErrorLevel
		case "info":
			logLevel = zapcore.InfoLevel
		case "debug":
			logLevel = zapcore.DebugLevel
		default:
			customLevel, err := strconv.Atoi(strings.ToLower(logLevelVal))
			exitOnError(err, "Invalid log-level")
			int8Lev, err := util.IToInt8(customLevel)
			exitOnError(err, "Invalid log-level")
			// Need to multiply by -1 to turn logr expected level into zap level
			logLevel = zapcore.Level(*int8Lev * -1)
		}
	} else {
		logLevel = zapcore.InfoLevel
	}

	// Use and set atomic level that all following log events are compared with
	// in order to evaluate if a given log level on the event is enabled.
	logf.SetLogger(zapctrl.New(func(o *zapctrl.Options) {
		o.Development = false
		o.Level = zap.NewAtomicLevelAt(logLevel)
	}))
	klog.SetLogger(log.AsLogger())

	log.Infof("Starting the operator with leaderElection parameters %t: %s", leaderElection, leaderElectionID)

	_, err := maxprocs.Set(maxprocs.Logger(func(f string, a ...any) { log.Info(fmt.Sprintf(f, a)) }))
	if err != nil {
		log.Error(err, "failed to set GOMAXPROCS from cgroups")
	}

	printVersion()

	watchNamespace, err := getWatchNamespace()
	exitOnError(err, "failed to get watch namespace")

	ctx := signals.SetupSignalHandler()

	cfg, err := config.GetConfig()
	exitOnError(err, "cannot get client config")

	operatorNamespace := platform.GetOperatorNamespace()
	if operatorNamespace == "" {
		// Fallback to using the watch namespace when the operator is not in-cluster.
		// It does not support local (off-cluster) operator watching resources globally,
		// in which case it's not possible to determine a namespace.
		operatorNamespace = watchNamespace
		if operatorNamespace == "" {
			leaderElection = false

			log.Info("unable to determine namespace for leader election")
		}
	}

	if !leaderElection {
		log.Info("Leader election is disabled!")
	}

	labelsSelector := getLabelSelector()

	selector := cache.ByObject{
		Label: labelsSelector,
	}

	cacheWatchNamespace := getNamespacesSelector(watchNamespace)
	if !platform.IsCurrentOperatorGlobal() {
		log.Infof("This operator will watch only %s namespace", watchNamespace)

		selector = cache.ByObject{
			Label:      labelsSelector,
			Namespaces: cacheWatchNamespace,
		}
	} else {
		log.Info("This operator is marked as global and will watch all namespaces!")
	}

	selectors := map[ctrl.Object]cache.ByObject{
		&appsv1.Deployment{}: selector,
		&batchv1.CronJob{}:   selector,
	}

	options := cache.Options{
		ByObject: selectors,
	}
	if !platform.IsCurrentOperatorGlobal() {
		options.DefaultNamespaces = cacheWatchNamespace
	}

	mgr, err := manager.New(cfg, manager.Options{
		LeaderElection:                leaderElection,
		LeaderElectionNamespace:       operatorNamespace,
		LeaderElectionID:              leaderElectionID,
		LeaderElectionResourceLock:    resourcelock.LeasesResourceLock,
		LeaderElectionReleaseOnCancel: true,
		HealthProbeBindAddress:        ":" + strconv.Itoa(healthPort),
		Metrics:                       metricsserver.Options{BindAddress: ":" + strconv.Itoa(monitoringPort)},
		Cache:                         options,
	})
	exitOnError(err, "Some error happened while creating a new manager")

	log.Info("Configuring manager")
	exitOnError(mgr.AddHealthzCheck("health-probe", healthz.Ping), "Unable add liveness check")
	exitOnError(apis.AddToScheme(mgr.GetScheme()), "Could not add Camel Monitor API to scheme")
	exitOnError(monitoringv1.AddToScheme(mgr.GetScheme()), "Could not add Prometheus API to scheme")
	exitOnError(integreatlyv1beta1.AddToScheme(mgr.GetScheme()), "Could not add Grafana API to scheme")
	ctrlClient, err := client.FromManager(mgr)
	exitOnError(err, "")
	exitOnError(controller.AddToManager(ctx, mgr, ctrlClient), "")
	exitOnError(synthetic.ManageSyntheticCamelMonitors(ctx, ctrlClient, mgr.GetCache()), "Camel App Synthetic manager error")
	log.Info("Starting the manager")
	exitOnError(mgr.Start(ctx), "manager exited non-zero")
}

func getLabelSelector() labels.Selector {
	labelSelector := platform.GetMonitorLabelSelector()
	log.Infof("Using (%s) label selector to identify Camel applications on the cluster.", labelSelector)

	if labelSelector == v1alpha1.MonitorLabel {
		log.Infof("NOTE: You can change this setting by adding a variable named %s", platform.CamelMonitorLabelSelector)
	}

	hasAppLabel, err := labels.NewRequirement(labelSelector, selection.Exists, []string{})
	exitOnError(err, "cannot create App label selector")

	labelsSelector := labels.NewSelector().Add(*hasAppLabel)

	return labelsSelector
}

func getNamespacesSelector(watchNamespace string) map[string]cache.Config {
	namespacesSelector := map[string]cache.Config{
		watchNamespace: {},
	}

	return namespacesSelector
}

// getWatchNamespace returns the Namespace the operator should be watching for changes.
func getWatchNamespace() (string, error) {
	ns, found := os.LookupEnv(platform.OperatorWatchNamespaceEnvVariable)
	if !found {
		return "", fmt.Errorf("%s must be set", platform.OperatorWatchNamespaceEnvVariable)
	}

	return ns, nil
}

func exitOnError(err error, msg string) {
	if err != nil {
		log.Error(err, msg)
		os.Exit(1)
	}
}
