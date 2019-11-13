/**
 * Copyright 2018 Atos
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 *
 * This is being developed for the DITAS Project: https://www.ditas-project.eu/
 */

package ditas

import (
	"deployment-engine/model"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func GetPortPair(vars model.Parameters, internalPortVar, externalPortVar string) (int, int, error) {
	internalPort, ok := vars.GetInt(internalPortVar)
	if !ok {
		return internalPort, 0, fmt.Errorf("Can't find internal port variable %s in configuration file", internalPortVar)
	}

	externalPort, _ := vars.GetInt(externalPortVar)
	return internalPort, externalPort, nil
}

func AppendDebugPort(ports []corev1.ServicePort, name string, internalPort int, externalPort int) []corev1.ServicePort {
	if externalPort != 0 {
		return append(ports, corev1.ServicePort{
			Name:       name,
			NodePort:   int32(externalPort),
			Port:       int32(externalPort),
			TargetPort: intstr.FromInt(internalPort),
		})
	}
	return ports
}
