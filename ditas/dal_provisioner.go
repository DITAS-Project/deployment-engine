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
	"deployment-engine/provision/kubernetes"
	"deployment-engine/utils"
	"errors"
	"fmt"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	DALIdentifierProperty = "DALID"
	SecretsProperty       = "secrets"
)

type DALProvisioner struct {
}

func NewDALProvisioner() *DALProvisioner {
	return &DALProvisioner{}
}

func (p DALProvisioner) Provision(config *kubernetes.KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {
	result := make(model.Parameters)

	var err error
	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
	})

	blueprintRaw, ok := args[BlueprintProperty]
	if !ok {
		return result, errors.New("Can't find blueprint in parameters")
	}

	bp, ok := blueprintRaw.(blueprint.Blueprint)
	if !ok {
		return result, errors.New("Invalid type for blueprint parameter. Expected blueprint.Blueprint")
	}

	dalID, ok := args.GetString(DALIdentifierProperty)
	if !ok {
		return result, errors.New("DAL identifier is mandatory")
	}
	logger = logger.WithField("DAL", dalID)

	varsRaw, ok := args[VariablesProperty]
	if !ok {
		return result, errors.New("Can't find the substitution variables parameter")
	}

	vars, ok := varsRaw.(map[string]interface{})
	if !ok {
		return result, errors.New("Invalid type for substitution variables parameter. Expected map[string]interface{}")
	}

	secretsRaw, ok := args[SecretsProperty]
	if !ok {
		return result, errors.New("Can't find secrets information to deploy this DAL")
	}

	dsSecrets, ok := secretsRaw.(map[string]kubernetes.EnvSecret)
	if !ok {
		return result, errors.New("Invalid type found for secrets associated to the infrastructure")
	}

	dalInfo, ok := bp.InternalStructure.DALImages[dalID]
	if !ok {
		return result, fmt.Errorf("Can't find DAL %s in blueprint %s", dalID, bp.ID)
	}

	var dalImages kubernetes.ImageSet
	err = utils.TransformObject(dalInfo.Images, &dalImages)
	logger.Info("Replacing environment variables")
	kubernetes.ReplaceEnvVars(dalImages, vars)

	for imageName, imageInfo := range dalImages {
		toRemove := make([]string, 0)
		dalSecrets := make([]kubernetes.EnvSecret, 0)
		for envName, envValue := range imageInfo.Environment {
			if envSecret, ok := dsSecrets[envValue]; ok {
				envSecret.EnvName = envName
				dalSecrets = append(dalSecrets, envSecret)
				toRemove = append(toRemove, envName)
			}
		}

		for _, envName := range toRemove {
			delete(imageInfo.Environment, envName)
		}

		imageInfo.Secrets = dalSecrets
		dalImages[imageName] = imageInfo
	}

	kubeClient, err := kubernetes.NewClient(config.ConfigurationFile)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return result, err
	}

	labels := map[string]string{
		"dal": dalID,
	}

	var repoSecrets []string
	if config.RegistriesSecret != "" {
		repoSecrets = []string{config.RegistriesSecret}
	}

	deployment := kubernetes.GetDeploymentDescription(dalID, int32(1), int64(30), labels, dalImages, "", "", repoSecrets, nil)

	logger.Info("Creating DAL deployment")
	_, err = kubeClient.CreateOrUpdateDeployment(logger, DitasNamespace, &deployment)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, fmt.Sprintf("Error deploying DAL %s", dalID), err)
	}

	ports := make([]corev1.ServicePort, 0, len(dalImages))
	for _, image := range dalImages {
		if image.ExternalPort != 0 {
			err := config.ClaimPort(image.ExternalPort)
			if err != nil {
				for _, port := range ports {
					config.LiberatePort(port.TargetPort.IntValue())
				}
				//kubeClient.Client.AppsV1().Deployments(DitasNamespace).Delete(dalID, metav1.NewDeleteOptions(int64(10)))
				return result, utils.WrapLogAndReturnError(logger, fmt.Sprintf("Error reserving port %d", image.ExternalPort), err)
			}
			ports = append(ports, corev1.ServicePort{
				Port:       int32(image.ExternalPort),
				NodePort:   int32(image.ExternalPort),
				TargetPort: intstr.FromInt(image.InternalPort),
			})
		}
	}

	vdcService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: dalID,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labels,
			Ports:    ports,
		},
	}

	logger.Info("Creating or updating DAL service")
	_, err = kubeClient.CreateOrUpdateService(logger, DitasNamespace, &vdcService)
	if err != nil {
		for _, port := range ports {
			config.LiberatePort(port.TargetPort.IntValue())
		}
		//kubeClient.Client.AppsV1().Deployments(DitasNamespace).Delete(dalID, metav1.NewDeleteOptions(int64(10)))
		return result, utils.WrapLogAndReturnError(logger, "Error creating DAL service", err)
	}

	logger.Info("DAL successfully deployed")

	return result, nil
}
