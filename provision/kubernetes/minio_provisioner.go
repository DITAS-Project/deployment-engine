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

	"github.com/sethvargo/go-password/password"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	MinioAccessKeySecret = "minio-access-key-secret"
	MinioSecretKeySecret = "minio-secret-key-secret"
)

type MinioInstanceConfig struct {
	Port              int
	AccessKeySecretID string
	SecretKeySecretID string
}

type MinioConfig struct {
	NumInstances int                            `json:"num_instances"`
	Instances    map[string]MinioInstanceConfig `json:"instances"`
}

type MinioProvisioner struct {
}

func (p MinioProvisioner) Provision(config *KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	result := make(model.Parameters)
	logger := logrus.WithFields(logrus.Fields{
		"infrastructure": infra.ID,
		"product":        "minio",
	})

	var err error
	size, ok := args.GetString("size")
	if !ok {
		return result, errors.New("Persistent volume size is mandatory")
	}

	var minioConfig MinioConfig
	rawConfig, ok := config.DeploymentsConfiguration["minio"]
	if !ok {
		minioConfig = MinioConfig{
			NumInstances: 0,
			Instances:    make(map[string]MinioInstanceConfig),
		}
	} else {
		err := utils.TransformObject(rawConfig, &minioConfig)
		if err != nil {
			return result, fmt.Errorf("Error reading MySQL configuration: %w", err)
		}
	}

	var instanceConfig MinioInstanceConfig

	dsID := fmt.Sprintf("minio%d", minioConfig.NumInstances)
	instanceConfig.AccessKeySecretID = dsID + "-access-key"
	instanceConfig.SecretKeySecretID = dsID + "-secret-key"
	volumeID := dsID + "-data"

	storageclass, ok := args.GetString("storage_class")
	if !ok {
		return result, errors.New("No storage class specified for persistence")
	}

	accessKey, err := password.Generate(10, 4, 0, false, false)
	if err != nil {
		return result, fmt.Errorf("Error generating access key: %w", err)
	}

	secretKey, err := password.Generate(10, 4, 0, false, false)
	if err != nil {
		return result, fmt.Errorf("Error generating secret key: %w", err)
	}

	secrets := []SecretData{
		SecretData{
			SecretID: instanceConfig.AccessKeySecretID,
			EnvVars: map[string]string{
				"MINIO_ACCESS_KEY": "password",
			},
			Data: map[string]string{
				"password": accessKey,
			},
		},
		SecretData{
			SecretID: instanceConfig.SecretKeySecretID,
			EnvVars: map[string]string{
				"MINIO_SECRET_KEY": "password",
			},
			Data: map[string]string{
				"password": secretKey,
			},
		},
	}
	result[MinioAccessKeySecret] = instanceConfig.AccessKeySecretID
	result[MinioSecretKeySecret] = instanceConfig.SecretKeySecretID

	volume := VolumeData{
		Name:         volumeID,
		MountPoint:   "/data",
		StorageClass: storageclass,
		Size:         size,
	}

	image := ImageInfo{
		InternalPort: 9000,
		Image:        "minio/minio",
		Args:         []string{"server", "/data"},
	}

	labels := map[string]string{"component": dsID}

	kubernetesClient, err := NewClient(config.ConfigurationFile)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return result, err
	}

	kubeSecrets := make([]*corev1.Secret, len(secrets))
	for i, secretData := range secrets {

		secret := GetSecretDescription(secretData)

		logger.Infof("Creating Secret %s", secretData.SecretID)
		secretOut, err := kubernetesClient.CreateOrUpdateSecret(logger, apiv1.NamespaceDefault, &secret)

		if err != nil {
			return result, err
		}
		kubeSecrets[i] = secretOut
		logger.Infof("Secret %s successfully created", secretData.SecretID)

	}

	defaultDeleteOptions := metav1.DeleteOptions{}
	podDescription, err := GetStatefulSetDescription(dsID, 1, 30, labels, ImageSet{"minio": image}, secrets, []VolumeData{volume}, nil)
	if err != nil {
		for _, secretOut := range kubeSecrets {
			kubernetesClient.Client.CoreV1().Secrets(secretOut.GetNamespace()).Delete(secretOut.GetName(), &defaultDeleteOptions)
		}
		return result, err
	}

	logger.Info("Creating Minio pod")
	podOut, err := kubernetesClient.CreateOrUpdateStatefulSet(logger, apiv1.NamespaceDefault, &podDescription)

	if err != nil {
		for _, secretOut := range kubeSecrets {
			kubernetesClient.Client.CoreV1().Secrets(secretOut.GetNamespace()).Delete(secretOut.GetName(), &defaultDeleteOptions)
		}
		return result, err
	}
	logger.Info("Minio successfully created")

	if val, _ := args.GetBool("expose"); val {
		servicePort := config.GetNewFreePort()
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
						Port: int32(servicePort),
						TargetPort: intstr.IntOrString{
							IntVal: int32(3306),
						},
					},
				},
			},
		}

		logger.Info("Creating Minio service")
		_, err = kubernetesClient.CreateOrUpdateService(logger, apiv1.NamespaceDefault, &dsService)

		if err != nil {
			kubernetesClient.Client.AppsV1().StatefulSets(podOut.GetNamespace()).Delete(podOut.GetName(), &defaultDeleteOptions)
			for _, secretOut := range kubeSecrets {
				kubernetesClient.Client.CoreV1().Secrets(secretOut.GetNamespace()).Delete(secretOut.GetName(), &defaultDeleteOptions)
			}
			config.LiberatePort(servicePort)
			return result, err
		}
		logger.Info("Minio service successfully created")
		instanceConfig.Port = servicePort
	}

	minioConfig.NumInstances++
	minioConfig.Instances[dsID] = instanceConfig
	config.DeploymentsConfiguration["minio"] = minioConfig

	return result, nil
}
