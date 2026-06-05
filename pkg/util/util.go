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

package util

import (
	"errors"
	"math"
)

// IToInt8 attempts to convert safely an int to an int8.
func IToInt8(x int) (*int8, error) {
	if x < math.MinInt8 || x > math.MaxInt8 {
		return nil, errors.New("integer overflow casting to int8 type")
	}

	casted := int8(x)

	return &casted, nil
}
