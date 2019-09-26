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

package ansible

import (
	"deployment-engine/model"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type FluentdProvisioner struct {
	parent *Provisioner
}

func NewFluentdProvisioner(parent *Provisioner) FluentdProvisioner {
	return FluentdProvisioner{
		parent: parent,
	}
}

func (p FluentdProvisioner) BuildInventory(infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(infra, args)
}

func (p FluentdProvisioner) DeployProduct(inventoryPath string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
	})

	extraArgs := make(map[string]interface{})

	for k, v := range args {
		stringVal, ok := v.(string)
		if ok {
			p.addParameter(strings.Split(k, "."), stringVal, extraArgs)
		}
	}

	vals, err := yaml.Marshal(extraArgs)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling elasticsearch parameters: %w", err)
	}

	return nil, ExecutePlaybook(logger, p.parent.ScriptsFolder+"/kubernetes/deploy_fluentd.yml", inventoryPath, map[string]string{
		"values": string(vals),
	})
}

func (p FluentdProvisioner) addParameter(parts []string, value string, values map[string]interface{}) map[string]interface{} {
	if parts == nil || len(parts) <= 0 {
		return nil
	}
	part := parts[0]
	if len(parts) == 1 {
		values[part] = value
	} else {
		val, ok := values[part]
		if !ok {
			val = make(map[string]interface{})
		}
		values[part] = p.addParameter(parts[1:], value, val.(map[string]interface{}))
	}
	return values
}
