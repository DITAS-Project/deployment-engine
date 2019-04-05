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
	"bytes"
	"deployment-engine/provision/ansible"
	"errors"
	"io/ioutil"
	"time"

	"text/template"

	"deployment-engine/utils"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetKubernetesConfigFile(provisioner *ansible.Provisioner, deploymentID, infraID string) (*rest.Config, error) {
	configPath := provisioner.GetInventoryFolder(deploymentID, infraID) + "/config"
	return clientcmd.BuildConfigFromFlags("", configPath)
}

func GetKubernetesClient(provisioner *ansible.Provisioner, deploymentID, infraID string) (*kubernetes.Clientset, error) {
	kubeConfig, err := GetKubernetesConfigFile(provisioner, deploymentID, infraID)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(kubeConfig)
}

func GetConfigMapDataFromFolder(configFolder string) (map[string]string, error) {
	result := make(map[string]string)
	files, err := ioutil.ReadDir(configFolder)
	if err != nil {
		return nil, err
	}

	generalConfigFolder, err := utils.ConfigurationFolder()
	if err != nil {
		logrus.WithError(err).Errorf("Error getting DITAS configuration folder")
		return result, err
	}
	reader := viper.New()
	reader.SetConfigName("vars")
	reader.AddConfigPath(generalConfigFolder)
	reader.ReadInConfig()
	vars := reader.AllSettings()

	for _, file := range files {
		if !file.IsDir() {
			fileName := configFolder + "/" + file.Name()
			fileTemplate, err := template.New(file.Name()).ParseFiles(fileName)
			if err != nil {
				logrus.WithError(err).Errorf("Error reading configuration file %s", fileName)
			} else {
				var fileContent bytes.Buffer
				err = fileTemplate.Execute(&fileContent, vars)
				if err != nil {
					logrus.WithError(err).Errorf("Error executing template %s", fileName)
				} else {
					result[file.Name()] = fileContent.String()
				}
			}
		}
	}
	return result, nil
}

func GetConfigMapFromFolder(configFolder, name string) (corev1.ConfigMap, error) {
	configMapData, err := GetConfigMapDataFromFolder(configFolder)
	if err != nil {
		return corev1.ConfigMap{}, err
	}

	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: configMapData,
	}, nil
}

func GetContainersDescription(images blueprint.ImageSet, configName string) []corev1.Container {

	containers := make([]corev1.Container, 0, len(images))

	configMount := corev1.VolumeMount{
		Name:      configName,
		MountPath: "/etc/ditas",
	}

	for containerId, containerInfo := range images {

		env := make([]corev1.EnvVar, 0, len(containerInfo.Environment))
		for k, v := range containerInfo.Environment {
			env = append(env, corev1.EnvVar{
				Name:  k,
				Value: v,
			})
		}

		containers = append(containers, corev1.Container{
			Name:  containerId,
			Image: containerInfo.Image,
			Ports: []corev1.ContainerPort{
				corev1.ContainerPort{
					ContainerPort: int32(containerInfo.InternalPort),
				},
			},
			Env:          env,
			VolumeMounts: []corev1.VolumeMount{configMount},
		})
	}

	return containers
}

func GetPodDescription(name string, replicas int32, terminationPeriod int64, labels map[string]string, images blueprint.ImageSet, configMap string) appsv1.Deployment {

	configVolume := corev1.Volume{
		Name: "config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMap,
				},
			},
		},
	}

	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &terminationPeriod,
					Containers:                    GetContainersDescription(images, "config"),
					Volumes:                       []corev1.Volume{configVolume},
				},
			},
		},
	}

}

func CreateOrUpdateResource(logger *logrus.Entry, name string, getter func(string) (bool, error), deleter func(string, *metav1.DeleteOptions) error, creater func(string) error) error {
	log := logger
	existing, err := getter(name)
	if err != nil && !k8serrors.IsNotFound(err) {
		logger.WithError(err).Error("Error getting resource information")
		return err
	}
	if existing {
		log.Info("Resource exists. Deleting")
		deleter(name, &metav1.DeleteOptions{})
		log.Info("Waiting for resource to be deleted")
		_, timeout, err := utils.WaitForStatusChange("Deleting", 2*time.Minute, func() (string, error) {
			exist, err := getter(name)
			if err != nil && k8serrors.IsNotFound(err) {
				return "Deleted", nil
			}
			if exist {
				return "Deleting", err
			}
			return "Deleted", err
		})

		if err != nil {
			log.WithError(err).Error("Error deleting resource")
			return err
		}

		if timeout {
			log.Error("Timeout waiting for resource to be deleted")
			return errors.New("Timeout waiting for resource to be deleted")
		}

	}

	return creater(name)
}
