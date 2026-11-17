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

package log

import (
	"errors"
	"testing"

	"github.com/camel-tooling/camel-monitor-operator/pkg/apis/camel/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestPackageLevelFunctions(t *testing.T) {
	assert.NotPanics(t, func() {
		Debugf("debug %s", "msg")
		Infof("info %s", "msg")
		Errorf(errors.New("boom"), "error %s", "msg")
		Debug("debug", "key", "value")
		Info("info", "key", "value")
		Error(errors.New("boom"), "error", "key", "value")
	})
}

func TestLoggerMethods(t *testing.T) {
	logger := Log

	assert.NotPanics(t, func() {
		logger.Debugf("debug %s", "msg")
		logger.Infof("info %s", "msg")
		logger.Errorf(errors.New("boom"), "error %s", "msg")
		logger.Debug("debug", "key", "value")
		logger.Info("info", "key", "value")
		logger.Error(errors.New("boom"), "error", "key", "value")
	})
}

func TestLoggerDerivation(t *testing.T) {
	named := Log.WithName("test")
	withValues := named.WithValues("key", "value")

	monitor := v1alpha1.NewCamelMonitor("default", "my-app")
	forMonitor := withValues.ForCamelMonitor(&monitor)

	assert.NotNil(t, forMonitor.AsLogger())
	assert.NotPanics(t, func() {
		forMonitor.Info("derived logger works")
	})
}
