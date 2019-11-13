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
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DatasourceIDProperty       = "id"
	DatasourcePortProperty     = "port"
	DatasourceSecretIDProperty = "secret"
)

type DatasourceInstanceConfig struct {
	Port     int32
	SecretID string
	Extra    model.Parameters
}

type DatasourceConfig struct {
	NumInstances int
	Instances    map[string]DatasourceInstanceConfig
}

type DatasourceConfigurer interface {
	GetSecret(dsID string, args model.Parameters) (secretData SecretData, extraParams model.Parameters, err error)
	GetDeploymentConfiguration(dsID string, args model.Parameters, secret SecretData) (image ImageSet, extraParams model.Parameters, err error)
}

type DatasourceProvisioner struct {
	Configurer      DatasourceConfigurer
	DatabaseType    string
	VolumeMountPath string
	InternalPort    int32
}

func (p DatasourceProvisioner) Provision(config *KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	result := make(model.Parameters)
	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
		"product":        p.DatabaseType,
	})

	size, ok := args.GetString("size")
	if !ok {
		return result, errors.New("Persistent volume size is mandatory")
	}

	storageclass, ok := args.GetString("storage_class")
	if !ok {
		return result, errors.New("No storage class specified for persistence")
	}

	namespace, ok := args.GetString("namespace")
	if !ok {
		namespace = apiv1.NamespaceDefault
	}

	var dsConfig DatasourceConfig
	rawConfig, ok := config.DeploymentsConfiguration[p.DatabaseType]
	if !ok {
		dsConfig = DatasourceConfig{
			NumInstances: 0,
			Instances:    make(map[string]DatasourceInstanceConfig),
		}
	} else {
		err := utils.TransformObject(rawConfig, &dsConfig)
		if err != nil {
			return result, fmt.Errorf("Error reading MySQL configuration: %w", err)
		}
	}

	dsID := fmt.Sprintf("%s-%d", p.DatabaseType, dsConfig.NumInstances)
	instanceConfig := DatasourceInstanceConfig{
		Port:  p.InternalPort,
		Extra: make(model.Parameters),
	}

	kubernetesClient, err := NewClient(config.ConfigurationFile)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return result, err
	}

	secretData, extraParams, err := p.Configurer.GetSecret(dsID, args)
	if err != nil {
		return result, fmt.Errorf("Error getting secret for datasource: %w", err)
	}

	if extraParams != nil {
		for param, value := range extraParams {
			result[param] = value
			instanceConfig.Extra[param] = value
		}
	}

	secret := GetSecretDescription(secretData)

	logger.Infof("Creating Secret %s", secretData.SecretID)
	secretOut, err := kubernetesClient.CreateOrUpdateSecret(logger, namespace, &secret)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error creating secret", err)
	}

	instanceConfig.SecretID = secretData.SecretID

	imageSet, extraParams, err := p.Configurer.GetDeploymentConfiguration(dsID, args, secretData)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error getting image configuration for datasource", err)
	}

	if extraParams != nil {
		for param, value := range extraParams {
			result[param] = value
			instanceConfig.Extra[param] = value
		}
	}

	labels := map[string]string{
		"datasource": dsID,
	}

	volume := VolumeData{
		Name:         fmt.Sprintf("%s-volume", dsID),
		MountPoint:   p.VolumeMountPath,
		StorageClass: storageclass,
		Size:         size,
	}

	defaultDeleteOptions := metav1.DeleteOptions{}
	podDescription, err := GetStatefulSetDescription(dsID, 1, 30, labels, imageSet, []VolumeData{volume}, nil)
	if err != nil {
		kubernetesClient.Client.CoreV1().Secrets(secretOut.GetNamespace()).Delete(secretOut.GetName(), &defaultDeleteOptions)
		return result, utils.WrapLogAndReturnError(logger, "Error getting datasource deployment descriptor", err)
	}

	logger.Info("Creating datasource pod")
	podOut, err := kubernetesClient.CreateOrUpdateStatefulSet(logger, namespace, &podDescription)
	if err != nil {

		kubernetesClient.Client.CoreV1().Secrets(secretOut.GetNamespace()).Delete(secretOut.GetName(), &defaultDeleteOptions)
		return result, utils.WrapLogAndReturnError(logger, "Error deploying datasource", err)
	}
	logger.Info("Datasource successfully created")

	dsConfig.NumInstances++

	dsService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: dsID,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name: dsID,
					Port: p.InternalPort,
				},
			},
		},
	}

	logger.Info("Creating datasource service")
	_, err = kubernetesClient.CreateOrUpdateService(logger, namespace, &dsService)

	if err != nil {
		kubernetesClient.Client.AppsV1().StatefulSets(podOut.GetNamespace()).Delete(podOut.GetName(), &defaultDeleteOptions)
		kubernetesClient.Client.CoreV1().Secrets(secretOut.GetNamespace()).Delete(secretOut.GetName(), &defaultDeleteOptions)
		return result, utils.WrapLogAndReturnError(logger, "Error deploying datasource service", err)
	}
	logger.Info("Datasource service successfully created")

	dsConfig.Instances[dsID] = instanceConfig
	config.DeploymentsConfiguration[p.DatabaseType] = dsConfig

	result[DatasourceIDProperty] = dsID
	result[DatasourcePortProperty] = p.InternalPort
	result[DatasourceSecretIDProperty] = secretData.SecretID

	return result, err
}
