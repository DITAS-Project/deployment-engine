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

type InstanceConfig struct {
	Port         int
	RootSecretID string
	UserSecretID string
}

type MySQLConfig struct {
	NumInstances int                       `json:"num_instances"`
	Instances    map[string]InstanceConfig `json:"instances"`
}

type MySQLProvisioner struct {
}

func (p MySQLProvisioner) Provision(config *KubernetesConfiguration, deploymentID string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
		"product":        "MySQL",
	})

	var err error
	size, ok := args.GetString("size")

	var mySqlConfig MySQLConfig
	rawConfig, ok := config.DeploymentsConfiguration["mysql"]
	if !ok {
		mySqlConfig = MySQLConfig{
			NumInstances: 0,
			Instances:    make(map[string]InstanceConfig),
		}
	} else {
		err := utils.TransformObject(rawConfig, &mySqlConfig)
		if err != nil {
			return fmt.Errorf("Error reading MySQL configuration: %s", err.Error())
		}
	}

	var instanceConfig InstanceConfig

	dsId := fmt.Sprintf("mysql%d", mySqlConfig.NumInstances)
	instanceConfig.RootSecretID = dsId + "root-pw"
	volumeId := dsId + "data"

	rootPassword, err := password.Generate(10, 3, 2, false, false)
	var userPassword string
	var databaseName string
	username, ok := args.GetString("username")
	if ok {
		databaseName, ok = args.GetString("database")
		if !ok {
			return errors.New("Database query parameter is mandatory when username is specified")
		}

		userPassword, ok = args.GetString("user_password")
		if !ok {
			userPassword, err = password.Generate(10, 3, 2, false, false)
			if err != nil {
				return fmt.Errorf("No password specified for user %s and an error occured when trying to generate a new random one: %s", username, err.Error())
			}
		}
	}

	if err != nil {
		return err
	}

	storageclass, ok := args.GetString("storage_class")
	if !ok {
		return errors.New("No storage class specified for persistence")
	}

	secrets := []SecretData{
		SecretData{
			SecretID: instanceConfig.RootSecretID,
			EnvVars: map[string]string{
				"MYSQL_ROOT_PASSWORD": "password",
			},
			Data: map[string]string{
				"password": rootPassword,
			},
		},
	}

	if userPassword != "" {
		secrets = append(secrets, SecretData{
			SecretID: fmt.Sprintf("%s-%s-pw", dsId, username),
			EnvVars: map[string]string{
				"MYSQL_PASSWORD": "password",
			},
			Data: map[string]string{
				"password": userPassword,
			},
		})
	}

	volume := VolumeData{
		Name:         volumeId,
		MountPoint:   "/var/lib/mysql",
		StorageClass: storageclass,
		Size:         size,
	}

	imageEnv := make(map[string]string)
	if username != "" {
		imageEnv["MYSQL_USER"] = username
		imageEnv["MYSQL_DATABASE"] = databaseName
	}

	image := ImageInfo{
		InternalPort: 3306,
		Image:        "mysql/mysql-server",
		Environment:  imageEnv,
	}

	labels := map[string]string{"component": dsId}

	kubernetesClient, err := NewClient(config.ConfigurationFile)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return err
	}

	kubeSecrets := make([]*corev1.Secret, len(secrets))
	for i, secretData := range secrets {

		secret := GetSecretDescription(secretData)

		logger.Infof("Creating Secret %s", secretData.SecretID)
		secretOut, err := kubernetesClient.CreateOrUpdateSecret(logger, apiv1.NamespaceDefault, &secret)

		if err != nil {
			return err
		}
		kubeSecrets[i] = secretOut
		if i == 0 {
			instanceConfig.RootSecretID = secretData.SecretID
		} else {
			instanceConfig.UserSecretID = secretData.SecretID
		}
		logger.Infof("Secret %s successfully created", secretData.SecretID)

	}

	podDescription := GetStatefulSetDescription(dsId, 1, 30, labels, ImageSet{"mysql": image}, secrets, []VolumeData{volume}, nil)

	logger.Info("Creating MySQL pod")
	podOut, err := kubernetesClient.CreateOrUpdateStatefulSet(logger, apiv1.NamespaceDefault, &podDescription)

	defaultDeleteOptions := metav1.DeleteOptions{}

	if err != nil {
		for _, secretOut := range kubeSecrets {
			kubernetesClient.Client.CoreV1().Secrets(secretOut.GetNamespace()).Delete(secretOut.GetName(), &defaultDeleteOptions)
		}
		return err
	}
	logger.Info("MySQL successfully created")

	servicePort := config.GetNewFreePort()
	dsService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: dsId,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name: dsId,
					Port: int32(servicePort),
					TargetPort: intstr.IntOrString{
						IntVal: int32(3306),
					},
				},
			},
		},
	}

	logger.Info("Creating MySQL service")
	_, err = kubernetesClient.CreateOrUpdateService(logger, apiv1.NamespaceDefault, &dsService)

	if err != nil {
		kubernetesClient.Client.AppsV1().StatefulSets(podOut.GetNamespace()).Delete(podOut.GetName(), &defaultDeleteOptions)
		for _, secretOut := range kubeSecrets {
			kubernetesClient.Client.CoreV1().Secrets(secretOut.GetNamespace()).Delete(secretOut.GetName(), &defaultDeleteOptions)
		}
		config.LiberatePort(servicePort)
		return err
	}
	logger.Info("MySQL service successfully created")

	mySqlConfig.NumInstances++
	instanceConfig.Port = servicePort
	mySqlConfig.Instances[dsId] = instanceConfig

	config.DeploymentsConfiguration["mysql"] = mySqlConfig

	return nil
}
