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

package keda

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/camel-tooling/camel-monitor-operator/pkg/client"
	"github.com/kedacore/keda/v2/pkg/scalers/externalscaler"
	"google.golang.org/grpc"
)

// Server wraps the gRPC external scaler server and implements manager.Runnable.
type Server struct {
	client client.Client
	port   int
}

// NewServer creates a new KEDA external scaler Server.
func NewServer(client client.Client, port int) *Server {
	return &Server{
		client: client,
		port:   port,
	}
}

// Start starts the gRPC server and blocks until the context is canceled.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.port)

	var lc net.ListenConfig

	lis, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s for KEDA external scaler: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	scaler := NewCamelMonitorScaler(s.client)
	externalscaler.RegisterExternalScalerServer(grpcServer, scaler)

	errChan := make(chan error, 1)

	go func() {
		log.Infof("Starting KEDA external scaler gRPC server on %s", addr)

		err := grpcServer.Serve(lis)
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("Stopping KEDA external scaler gRPC server...")
		grpcServer.GracefulStop()

		return nil
	case err := <-errChan:
		return err
	}
}
