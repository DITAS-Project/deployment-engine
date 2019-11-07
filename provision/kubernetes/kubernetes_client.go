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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"time"

	"text/template"

	"deployment-engine/utils"

	"deployment-engine/model"

	"github.com/sirupsen/logrus"
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
	SecretID string
	Data     map[string]string
}

type EnvSecret struct {
	EnvName  string
	SecretID string
	Key      string
}

type VolumeData struct {
	Name         string
	MountPoint   string
	PVCName      string
	StorageClass string
	Size         string
}

type RegistrySecretData struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Auth     string `json:"auth"`
}

type RegistryAuthSecretData struct {
	Auths map[string]RegistrySecretData `json:"auths"`
}

// ImageInfo is the information about an image that will be deployed by the deployment engine
// swagger:model
type ImageInfo struct {
	// Port in which the docker image is listening internally. Two images inside the same ImageSet can't have the same internal port.
	InternalPort int `json:"internal_port"`

	// Port in which this image must be exposed. It must be unique across all images in all the ImageSets defined in this blueprint. Due to limitations in k8s, the port range must be bewteen 30000 and 32767
	ExternalPort int `json:"external_port"`

	// Image is the image name in the standard format [group]/<image_name>:[release]
	// required: true
	Image string `json:"image"`

	// Args is a list of arguments to pass to the container when it starts
	Args []string `json:"args"`

	// Environment is a map of environment variables whose key is the variable name and value is the variable value
	Environment map[string]string `json:"environment"`

	// Secrets that must be passed as environmet variables
	Secrets []EnvSecret `json:"secrets"`
}

// ImageSet represents a set of docker images whose key is an identifier and value is a the docker image information such as image name and listening ports
// swagger:model
type ImageSet map[string]ImageInfo

type KubernetesClient struct {
	ConfigPath string
	Config     *rest.Config
	Client     *kubernetes.Clientset
}

func NewClient(configFilePath string) (*KubernetesClient, error) {

	var result KubernetesClient
	result.ConfigPath = configFilePath

	var err error
	result.Config, err = clientcmd.BuildConfigFromFlags("", result.ConfigPath)
	if err != nil {
		return &result, err
	}

	result.Client, err = kubernetes.NewForConfig(result.Config)
	return &result, err
}

func GetConfigMapDataFromFolder(configFolder string, vars map[string]interface{}) (map[string]string, error) {
	result := make(map[string]string)
	files, err := ioutil.ReadDir(configFolder)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if !file.IsDir() {
			fileName := configFolder + "/" + file.Name()
			fileTemplate, err := template.New(file.Name()).ParseFiles(fileName)
			if err != nil {
				return result, utils.WrapLogAndReturnError(logrus.NewEntry(logrus.New()), fmt.Sprintf("Error reading configuration file %s", fileName), err)
			}

			var fileContent bytes.Buffer
			err = fileTemplate.Execute(&fileContent, vars)
			if err != nil {
				return result, utils.WrapLogAndReturnError(logrus.NewEntry(logrus.New()), fmt.Sprintf("Error executing template %s", fileName), err)
			}

			result[file.Name()] = fileContent.String()
		}
	}
	return result, nil
}

func GetConfigMapFromFolder(configFolder, name string, vars map[string]interface{}) (corev1.ConfigMap, error) {
	configMapData, err := GetConfigMapDataFromFolder(configFolder, vars)
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
			Name: secret.SecretID,
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: secret.Data,
	}
}

func GetDockerRegistrySecret(repos map[string]model.DockerRegistry, name string) (corev1.Secret, error) {

	auths := RegistryAuthSecretData{
		Auths: make(map[string]RegistrySecretData, len(repos)),
	}

	for name, repo := range repos {
		auth := fmt.Sprintf("%s:%s", repo.Username, repo.Password)
		auths.Auths[name] = RegistrySecretData{
			Username: repo.Username,
			Password: repo.Password,
			Email:    repo.Email,
			Auth:     base64.StdEncoding.EncodeToString([]byte(auth)),
		}
	}

	jsonAuths, err := json.Marshal(auths)
	if err != nil {
		return corev1.Secret{}, fmt.Errorf("Error marshaling docker registry secret data: %s", err.Error())
	}
	encoded := base64.StdEncoding.EncodeToString(jsonAuths)
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			".dockerconfigjson": []byte(encoded),
		},
	}, nil
}

func GetContainersDescription(images ImageSet, volumes []VolumeData) []corev1.Container {

	containers := make([]corev1.Container, 0, len(images))

	for containerId, containerInfo := range images {

		container := corev1.Container{
			Name:  containerId,
			Image: containerInfo.Image,
		}

		env := make([]corev1.EnvVar, 0, len(containerInfo.Environment)+len(containerInfo.Secrets))
		for k, v := range containerInfo.Environment {
			env = append(env, corev1.EnvVar{
				Name:  k,
				Value: v,
			})
		}

		for _, secret := range containerInfo.Secrets {
			env = append(env, corev1.EnvVar{
				Name: secret.EnvName,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret.SecretID,
						},
						Key: secret.Key,
					},
				},
			})
		}

		if len(env) > 0 {
			container.Env = env
		}

		volumeMounts := make([]corev1.VolumeMount, len(volumes))
		for i, volume := range volumes {
			volumeMounts[i] = corev1.VolumeMount{
				Name:      volume.Name,
				MountPath: volume.MountPoint,
			}
		}

		if len(volumeMounts) > 0 {
			container.VolumeMounts = volumeMounts
		}

		if containerInfo.InternalPort != 0 {
			container.Ports = []corev1.ContainerPort{
				corev1.ContainerPort{
					ContainerPort: int32(containerInfo.InternalPort),
				},
			}
		}
		container.Args = containerInfo.Args

		containers = append(containers, container)
	}

	return containers
}

func GetPodSpecDescrition(labels map[string]string, terminationPeriod int64, images ImageSet, volumes []VolumeData, repositorySecrets []string) corev1.PodTemplateSpec {
	result := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			TerminationGracePeriodSeconds: &terminationPeriod,
			Containers:                    GetContainersDescription(images, volumes),
		},
	}

	if repositorySecrets != nil && len(repositorySecrets) > 0 {
		secrets := make([]corev1.LocalObjectReference, len(repositorySecrets))
		for i := range repositorySecrets {
			secrets[i] = corev1.LocalObjectReference{
				Name: repositorySecrets[i],
			}
		}
		result.Spec.ImagePullSecrets = secrets
	}

	if volumes != nil {
		podVolumes := make([]corev1.Volume, 0)
		for _, volume := range volumes {
			if volume.PVCName != "" {
				podVolumes = append(podVolumes, corev1.Volume{
					Name: volume.Name,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: volume.PVCName,
						},
					},
				})
			}
		}
		if len(podVolumes) > 0 {
			result.Spec.Volumes = podVolumes
		}
	}
	return result
}

func GetDeploymentDescription(name string, replicas int32, terminationPeriod int64, labels map[string]string, images ImageSet, configMap, configMountPoint string, repositorySecrets []string, volumes []VolumeData) appsv1.Deployment {

	var volumeData []VolumeData
	if volumes != nil {
		volumeData = volumes
	} else {
		volumeData = make([]VolumeData, 0)
	}

	hasConfig := false
	if configMap != "" && configMountPoint != "" {
		hasConfig = true
		volumeConfig := VolumeData{Name: "config", MountPoint: configMountPoint}
		volumeData = append(volumeData, volumeConfig)
	}

	podTemplate := GetPodSpecDescrition(labels, terminationPeriod, images, volumeData, repositorySecrets)

	if hasConfig {
		if podTemplate.Spec.Volumes == nil {
			podTemplate.Spec.Volumes = make([]corev1.Volume, 0, 1)
		}
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
		podTemplate.Spec.Volumes = append(podTemplate.Spec.Volumes, configVolume)
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
			Template: podTemplate,
		},
	}

}

func GetPersistentVolumeClaim(volume VolumeData) (corev1.PersistentVolumeClaim, error) {
	var result corev1.PersistentVolumeClaim
	if volume.StorageClass == "" || volume.Size == "" {
		return result, fmt.Errorf("Storage class and volume size are mandatory for volume %s", volume.Name)
	}

	quantitySize, err := resource.ParseQuantity(volume.Size)
	if err != nil {
		return result, fmt.Errorf("Invalid size %s of volume %s", volume.Size, volume.Name)
	}

	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: volume.Name,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			StorageClassName: &volume.StorageClass,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: quantitySize,
				},
			},
		},
	}, nil
}

func GetStatefulSetDescription(name string, replicas int32, terminationPeriod int64, labels map[string]string, images ImageSet, volumes []VolumeData, repositorySecrets []string) (appsv1.StatefulSet, error) {

	volumesClaims := make([]corev1.PersistentVolumeClaim, 0)
	for _, volume := range volumes {
		claim, err := GetPersistentVolumeClaim(volume)
		if err != nil {
			return appsv1.StatefulSet{}, utils.WrapLogAndReturnError(logrus.WithField("volume", volume.Name), "Error generating claim for volume", err)
		}
		volumesClaims = append(volumesClaims, claim)
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
			Template:             GetPodSpecDescrition(labels, terminationPeriod, images, volumes, repositorySecrets),
			VolumeClaimTemplates: volumesClaims,
		},
	}, nil
}

func CreateOrUpdateResource(logger *logrus.Entry, name string, getter func() (interface{}, error), deleter func(string, *metav1.DeleteOptions) error, creater func() (interface{}, error)) (interface{}, error) {
	log := logger
	existing, err := getter()
	if err != nil && !k8serrors.IsNotFound(err) {
		logger.WithError(err).Error("Error getting resource information")
		return existing, err
	}
	if existing != nil && err == nil {
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

func (c KubernetesClient) CreateOrUpdateDeployment(logger *logrus.Entry, namespace string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	depClient := c.Client.AppsV1().Deployments(namespace)
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

func (c KubernetesClient) CreateOrUpdateConfigMap(logger *logrus.Entry, namespace string, configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	depClient := c.Client.CoreV1().ConfigMaps(namespace)
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

func (c KubernetesClient) CreateOrUpdateService(logger *logrus.Entry, namespace string, service *corev1.Service) (*corev1.Service, error) {
	depClient := c.Client.CoreV1().Services(namespace)
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

func (c KubernetesClient) CreateOrUpdateSecret(logger *logrus.Entry, namespace string, secret *corev1.Secret) (*corev1.Secret, error) {
	depClient := c.Client.CoreV1().Secrets(namespace)
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

func (c KubernetesClient) CreateOrUpdateStatefulSet(logger *logrus.Entry, namespace string, set *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	depClient := c.Client.AppsV1().StatefulSets(namespace)
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

func (c KubernetesClient) CreateOrUpdatePVC(logger *logrus.Entry, namespace string, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	depClient := c.Client.CoreV1().PersistentVolumeClaims(namespace)
	name := pvc.ObjectMeta.Name
	result, err := CreateOrUpdateResource(logger.WithField("resource", "PVC").WithField("name", name), name,
		func() (interface{}, error) {
			return depClient.Get(name, metav1.GetOptions{})
		},
		depClient.Delete,
		func() (interface{}, error) {
			return depClient.Create(pvc)
		})

	return result.(*corev1.PersistentVolumeClaim), err
}

func (c KubernetesClient) CreateKubectlCommand(logger *logrus.Entry, action string, args ...string) *exec.Cmd {
	finalArgs := append([]string{action}, args...)
	return utils.CreateCommand(logger, map[string]string{
		"KUBECONFIG": c.ConfigPath,
	}, true, "kubectl", finalArgs...)
}

func (c KubernetesClient) ListServices() (map[string]*corev1.ServiceList, error) {
	nsList, err := c.Client.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("Error getting k8s namespace list: %w", err)
	}

	result := make(map[string]*corev1.ServiceList)

	for _, namespace := range nsList.Items {
		result[namespace.Name], err = c.Client.CoreV1().Services(namespace.Name).List(metav1.ListOptions{})
		if err != nil {
			return result, fmt.Errorf("Error getting services of namespace %s: %w", namespace.Name, err)
		}
	}
	return result, nil
}

func (c KubernetesClient) GetUsedNodePorts() ([]int, error) {
	svcs, err := c.ListServices()
	if err != nil {
		return nil, fmt.Errorf("Error getting list of services: %w", err)
	}
	ports := make([]int, 0)
	for _, svcList := range svcs {
		for _, svc := range svcList.Items {
			if svc.Spec.Type == corev1.ServiceTypeNodePort {
				for _, currentPorts := range svc.Spec.Ports {
					ports = append(ports, int(currentPorts.NodePort))
				}
			}
		}
	}
	return ports, nil
}

func (c KubernetesClient) ExecuteKubectlCommand(logger *logrus.Entry, action string, args ...string) error {
	return c.CreateKubectlCommand(logger, action, args...).Run()
}

func (c KubernetesClient) ExecuteDeployScript(logger *logrus.Entry, script string) error {
	return c.ExecuteKubectlCommand(logger, "create", "-f", script)
}

func (c KubernetesClient) ExecuteDeployTemplate(logger *logrus.Entry, name, templateFile string, vars map[string]interface{}) error {
	clusterDefinition, err := template.New(name).ParseFiles(templateFile)
	if err != nil {
		return utils.WrapLogAndReturnError(logger, fmt.Sprintf("Error reading template file %s", templateFile), err)
	}

	cmd := c.CreateKubectlCommand(logger, "create", "-f", "-")
	writer, err := cmd.StdinPipe()
	if err != nil {
		return utils.WrapLogAndReturnError(logger, "Error getting input pipe for command", err)
	}

	go func() {
		defer writer.Close()
		err = clusterDefinition.Execute(writer, vars)
		if err != nil {
			logger.WithError(err).Error("Error executing deployment template")
		}
	}()

	err = cmd.Run()
	if err != nil {
		return utils.WrapLogAndReturnError(logger, fmt.Sprintf("Error executing template file %s", templateFile), err)
	}

	return nil
}
