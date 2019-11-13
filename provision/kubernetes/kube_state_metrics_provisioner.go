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
 */

package kubernetes

import (
	"deployment-engine/model"
	"deployment-engine/utils"

	"github.com/sirupsen/logrus"
)

type KSMProvisioner struct {
	scriptsFolder string
}

func (p KSMProvisioner) Provision(config *KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {
	result := make(model.Parameters)
	logger := logrus.WithFields(logrus.Fields{
		"product": "kube-state-metrics",
		"infra":   infra.ID,
	})

	kubeClient, err := NewClient(config.ConfigurationFile)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error getting kubernetes client", err)
	}

	logger.Info("Creating kube-state-metrics monitoring")
	err = kubeClient.ExecuteDeployScript(logger, p.scriptsFolder+"/kube-state-metrics/deploy.yml")
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error deploying kube-state-metrics monitoring", err)
	}

	return result, nil
}
