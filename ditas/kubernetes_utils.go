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
	"os/exec"
	"time"

	"text/template"

	"deployment-engine/utils"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type SecretData struct {
	EnvVars  map[string]string
	SecretId string
	Data     map[string]string
}

type VolumeData struct {
	Name         string
	MountPoint   string
	StorageClass string
	Size         string
}

func GetKubernetesConfigPath(provisioner *ansible.Provisioner, deploymentID, infraID string) string {
	return provisioner.GetInventoryFolder(deploymentID, infraID) + "/config"
}

func GetKubernetesConfigFile(provisioner *ansible.Provisioner, deploymentID, infraID string) (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", GetKubernetesConfigPath(provisioner, deploymentID, infraID))
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

func GetSecretDescription(secret SecretData) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secret.SecretId,
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: secret.Data,
	}
}

func GetContainersDescription(images blueprint.ImageSet, secrets []SecretData, volumes []VolumeData) []corev1.Container {

	containers := make([]corev1.Container, 0, len(images))

	for containerId, containerInfo := range images {

		env := make([]corev1.EnvVar, 0, len(containerInfo.Environment)+len(secrets))
		for k, v := range containerInfo.Environment {
			env = append(env, corev1.EnvVar{
				Name:  k,
				Value: v,
			})
		}

		for _, secret := range secrets {
			for envVar, key := range secret.EnvVars {
				env = append(env, corev1.EnvVar{
					Name: envVar,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secret.SecretId,
							},
							Key: key,
						},
					},
				})
			}
		}

		volumeMounts := make([]corev1.VolumeMount, len(volumes))
		for i, volume := range volumes {
			volumeMounts[i] = corev1.VolumeMount{
				Name:      volume.Name,
				MountPath: volume.MountPoint,
			}
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
			VolumeMounts: volumeMounts,
		})
	}

	return containers
}

func GetPodSpecDescrition(labels map[string]string, terminationPeriod int64, images blueprint.ImageSet, secrets []SecretData, volumes []VolumeData) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &terminationPeriod,
			Containers:                    GetContainersDescription(images, secrets, volumes),
		},
	}
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

	podTemplate := GetPodSpecDescrition(labels, terminationPeriod, images, nil, []VolumeData{VolumeData{Name: "config", MountPoint: "/etc/ditas"}})
	podTemplate.Spec.Volumes = []corev1.Volume{configVolume}

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
			Template: podTemplate,
		},
	}

}

func GetDatasourceDescription(name string, replicas int32, terminationPeriod int64, labels map[string]string, image blueprint.ImageInfo, secrets []SecretData, volumes []VolumeData) appsv1.StatefulSet {

	volumesClaims := make([]corev1.PersistentVolumeClaim, 0)
	for _, volume := range volumes {
		if volume.StorageClass != "" && volume.Size != "" {
			quantitySize, err := resource.ParseQuantity(volume.Size)
			if err == nil {
				volumesClaims = append(volumesClaims, corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: volume.Name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: &volume.StorageClass,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: quantitySize,
							},
						},
					},
				})
			} else {
				logrus.WithError(err).Errorf("Invalid size %s of volume %s", volume.Size, volume.Name)
			}
		}
	}

	return appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template:             GetPodSpecDescrition(labels, terminationPeriod, blueprint.ImageSet{name: image}, secrets, volumes),
			VolumeClaimTemplates: volumesClaims,
		},
	}
}

func CreateOrUpdateResource(logger *logrus.Entry, name string, getter func() (interface{}, error), deleter func(string, *metav1.DeleteOptions) error, creater func() (interface{}, error)) (interface{}, error) {
	log := logger
	existing, err := getter()
	if err != nil && !k8serrors.IsNotFound(err) {
		logger.WithError(err).Error("Error getting resource information")
		return existing, err
	}
	if existing != nil {
		log.Info("Resource exists. Deleting")
		deleter(name, &metav1.DeleteOptions{})
		log.Info("Waiting for resource to be deleted")
		_, timeout, err := utils.WaitForStatusChange("Deleting", 2*time.Minute, func() (string, error) {
			exist, err := getter()
			if err != nil && k8serrors.IsNotFound(err) {
				return "Deleted", nil
			}
			if exist != nil {
				return "Deleting", err
			}
			return "Deleted", err
		})

		if err != nil {
			log.WithError(err).Error("Error deleting resource")
			return existing, err
		}

		if timeout {
			log.Error("Timeout waiting for resource to be deleted")
			return existing, errors.New("Timeout waiting for resource to be deleted")
		}

	}

	result, err := creater()
	if err != nil {
		log.WithError(err).Error("Error creating resource")
		return result, err
	}
	if result == nil {
		log.Error("Empty resource created")
		return result, errors.New("No resource created on server")
	}

	return result, nil
}

func CreateOrUpdateDeployment(logger *logrus.Entry, client *kubernetes.Clientset, namespace string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	depClient := client.AppsV1().Deployments(namespace)
	name := deployment.ObjectMeta.Name
	result, err := CreateOrUpdateResource(logger.WithField("resource", "Deployment").WithField("name", name), name,
		func() (interface{}, error) {
			return depClient.Get(name, metav1.GetOptions{})
		},
		depClient.Delete,
		func() (interface{}, error) {
			return depClient.Create(deployment)
		})
	return result.(*appsv1.Deployment), err
}

func CreateOrUpdateConfigMap(logger *logrus.Entry, client *kubernetes.Clientset, namespace string, configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	depClient := client.CoreV1().ConfigMaps(namespace)
	name := configMap.ObjectMeta.Name
	result, err := CreateOrUpdateResource(logger.WithField("resource", "ConfigMap").WithField("name", name), name,
		func() (interface{}, error) {
			return depClient.Get(name, metav1.GetOptions{})
		},
		depClient.Delete,
		func() (interface{}, error) {
			return depClient.Create(configMap)
		})
	return result.(*corev1.ConfigMap), err
}

func CreateOrUpdateService(logger *logrus.Entry, client *kubernetes.Clientset, namespace string, service *corev1.Service) (*corev1.Service, error) {
	depClient := client.CoreV1().Services(namespace)
	name := service.ObjectMeta.Name
	result, err := CreateOrUpdateResource(logger.WithField("resource", "Service").WithField("name", name), name,
		func() (interface{}, error) {
			return depClient.Get(name, metav1.GetOptions{})
		},
		depClient.Delete,
		func() (interface{}, error) {
			return depClient.Create(service)
		})
	return result.(*corev1.Service), err
}

func CreateOrUpdateSecret(logger *logrus.Entry, client *kubernetes.Clientset, namespace string, secret *corev1.Secret) (*corev1.Secret, error) {
	depClient := client.CoreV1().Secrets(namespace)
	name := secret.ObjectMeta.Name
	result, err := CreateOrUpdateResource(logger.WithField("resource", "Secret").WithField("name", name), name,
		func() (interface{}, error) {
			return depClient.Get(name, metav1.GetOptions{})
		},
		depClient.Delete,
		func() (interface{}, error) {
			return depClient.Create(secret)
		})
	return result.(*corev1.Secret), err
}

func CreateOrUpdateStatefulSet(logger *logrus.Entry, client *kubernetes.Clientset, namespace string, set *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	depClient := client.AppsV1().StatefulSets(namespace)
	name := set.ObjectMeta.Name
	result, err := CreateOrUpdateResource(logger.WithField("resource", "StatefulSet").WithField("name", name), name,
		func() (interface{}, error) {
			return depClient.Get(name, metav1.GetOptions{})
		},
		depClient.Delete,
		func() (interface{}, error) {
			return depClient.Create(set)
		})
	return result.(*appsv1.StatefulSet), err
}

func CreateKubectlCommand(logger *logrus.Entry, configFile, action string, args ...string) *exec.Cmd {
	finalArgs := append([]string{action}, args...)
	return utils.CreateCommand(logger, map[string]string{
		"KUBECONFIG": configFile,
	}, true, "kubectl", finalArgs...)
}

func ExecuteKubectlCommand(logger *logrus.Entry, configFile, action string, args ...string) error {
	return CreateKubectlCommand(logger, configFile, action, args...).Run()
}

func ExecuteDeployScript(logger *logrus.Entry, configFile, script string) error {
	return ExecuteKubectlCommand(logger, configFile, "create", "-f", script)
}
