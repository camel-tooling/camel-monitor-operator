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
	consolev1 "github.com/openshift/api/console/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
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
	log.Infof("Camel Monitor Operator Build: %v", defaults.BuildProvider)
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
	// Increase maximum burst and QPS used by client-side throttling,
	// to prevent API requests from being throttled during startup or high reconciliation loads.
	cfg.QPS = platform.GetClientQPS()
	cfg.Burst = platform.GetClientBurst()

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

	labelsSelector, err := getLabelSelector()
	exitOnError(err, "cannot create App label selector")

	selector := cache.ByObject{
		Label: labelsSelector,
	}

	cacheWatchNamespaces := getNamespacesSelector(watchNamespace)
	if !platform.IsCurrentOperatorGlobal() {
		log.Infof("This operator will watch only %s namespace(s)", watchNamespace)

		selector = cache.ByObject{
			Label:      labelsSelector,
			Namespaces: cacheWatchNamespaces,
		}
	} else {
		log.Info("This operator is marked as global and will watch all namespaces!")
	}

	selectors := map[ctrl.Object]cache.ByObject{
		&appsv1.Deployment{}: selector,
		&batchv1.CronJob{}:   selector,
		// The operator only needs a few Pod fields: keep the informer memory footprint small.
		&corev1.Pod{}: {Transform: slimPod},
	}

	options := cache.Options{
		ByObject:         selectors,
		DefaultTransform: cache.TransformStripManagedFields(),
	}
	if !platform.IsCurrentOperatorGlobal() {
		options.DefaultNamespaces = cacheWatchNamespaces
	}

	// The scheme must be complete before creating the manager, as the client resolves
	// the types excluded from the cache (see below) when it is built.
	scheme := clientgoscheme.Scheme
	exitOnError(apis.AddToScheme(scheme), "Could not add Camel Monitor API to scheme")
	exitOnError(monitoringv1.AddToScheme(scheme), "Could not add Prometheus API to scheme")
	exitOnError(integreatlyv1beta1.AddToScheme(scheme), "Could not add Grafana API to scheme")
	exitOnError(consolev1.AddToScheme(scheme), "Could not add ConsolePlugin API to scheme")
	exitOnError(operatorsv1alpha1.AddToScheme(scheme), "Could not add OLM API to scheme")

	mgr, err := manager.New(cfg, manager.Options{
		Scheme:                        scheme,
		LeaderElection:                leaderElection,
		LeaderElectionNamespace:       operatorNamespace,
		LeaderElectionID:              leaderElectionID,
		LeaderElectionResourceLock:    resourcelock.LeasesResourceLock,
		LeaderElectionReleaseOnCancel: true,
		HealthProbeBindAddress:        ":" + strconv.Itoa(healthPort),
		Metrics:                       metricsserver.Options{BindAddress: ":" + strconv.Itoa(monitoringPort)},
		Cache:                         options,
		Client: ctrl.Options{
			Cache: &ctrl.CacheOptions{
				// These resources are only read once per reconciliation to create or update them:
				// read them straight from the API server instead of keeping an informer
				// (and a full copy of every object of that kind) in memory.
				DisableFor: []ctrl.Object{
					&monitoringv1.PodMonitor{},
					&monitoringv1.PrometheusRule{},
					&integreatlyv1beta1.GrafanaDashboard{},
				},
			},
		},
	})
	exitOnError(err, "Some error happened while creating a new manager")

	log.Info("Configuring manager")
	exitOnError(mgr.AddHealthzCheck("health-probe", healthz.Ping), "Unable add liveness check")
	ctrlClient, err := client.FromManager(mgr)
	exitOnError(err, "")
	exitOnError(controller.AddToManager(ctx, mgr, ctrlClient), "")
	exitOnError(synthetic.ManageSyntheticCamelMonitors(ctx, ctrlClient, mgr.GetCache()), "Camel App Synthetic manager error")

	if platform.GetCheckVersionUpgrade() == "true" {
		log.Infof("The operator is configured to check any new version released on %s repository", defaults.DefaultMavenBaseRepo)
	}

	log.Info("Starting the manager")
	exitOnError(mgr.Start(ctx), "manager exited non-zero")
}

// slimPod reduces a Pod to the only fields used by the operator (identity, labels, phase, IP, conditions and
// container ports) before it is stored in the informer cache.
func slimPod(obj any) (any, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		// i.e., a DeletedFinalStateUnknown tombstone
		return obj, nil
	}

	containers := make([]corev1.Container, len(pod.Spec.Containers))
	for i, c := range pod.Spec.Containers {
		containers[i] = corev1.Container{Name: c.Name, Ports: c.Ports}
	}

	return &corev1.Pod{
		TypeMeta:          pod.TypeMeta,
		Name:              pod.Name,
		Namespace:         pod.Namespace,
		UID:               pod.UID,
		ResourceVersion:   pod.ResourceVersion,
		Labels:            pod.Labels,
		DeletionTimestamp: pod.DeletionTimestamp,
		Spec:              corev1.PodSpec{Containers: containers},
		Status: corev1.PodStatus{
			Phase:      pod.Status.Phase,
			PodIP:      pod.Status.PodIP,
			Conditions: pod.Status.Conditions,
		},
	}, nil
}

func getLabelSelector() (labels.Selector, error) {
	labelSelector := platform.GetMonitorLabelSelector()
	log.Infof("Using (%s) label selector to identify Camel applications on the cluster.", labelSelector)

	if labelSelector == v1alpha1.MonitorLabel {
		log.Infof("NOTE: You can change this setting by adding a variable named %s", platform.CamelMonitorLabelSelector)
	}

	hasAppLabel, err := labels.NewRequirement(labelSelector, selection.Exists, []string{})
	if err != nil {
		return nil, err
	}

	labelsSelector := labels.NewSelector().Add(*hasAppLabel)

	return labelsSelector, nil
}

func getNamespacesSelector(watchNamespaces string) map[string]cache.Config {
	namespacesSelector := make(map[string]cache.Config)

	for ns := range strings.SplitSeq(watchNamespaces, ",") {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			continue
		}

		namespacesSelector[ns] = cache.Config{}
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
