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
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// TraefikHTTPPortProperty is the property name in the configuration file that contains the port in which Traefik must listen to http connections
	TraefikHTTPPortProperty = "kubernetes.traefik.service.ports.http"

	// TraefikSslPortProperty is the property name in the configuration file that contains the port in which Traefik must listen to https (ssl) connections
	TraefikSslPortProperty = "kubernetes.traefik.service.ports.ssl"

	// TraefikAdminPortProperty is the property name in the configuration file that contains the port in which Traefik must expose the admin API. In general it must not be exposed to the outside world but it's here just in case.
	TraefikAdminPortProperty = "kubernetes.traefik.service.ports.admin"

	// TraefikServiceTypeProperty is the property name in the configuration file that contains the service type that must be created to expose Traefik. It must be one of TraefikNodePortServiceType, TraefikLoadBalancerServiceType or TraefikClusterIPServiceType. If absent TraefikClusterIPServiceType will be selected.
	TraefikServiceTypeProperty = "kubernetes.traefik.service.type"

	// TraefikNodePortServiceType specifies that Traefik must be exposed using a service of type Node Port
	TraefikNodePortServiceType = "NodePort"

	// TraefikLoadBalancerServiceType specifies that Traefik must be exposed using a service of type Load Balancer
	TraefikLoadBalancerServiceType = "LoadBalancer"

	// TraefikClusterIPServiceType specifies that Traefik must be exposed using a service of type Cluster IP
	TraefikClusterIPServiceType = "ClusterIP"

	// TraefikProvisionMode specifies the mode in which this provisioner works. If not present or if set to "provision", it will deploy Traefik in the target cluster. If set ro "redirect" it will redirect a service in the target cluster through Traefik.
	TraefikProvisionMode = "mode"

	// TraefikRedirectMode sets the provision mode to redirect, which will redirect a service port through Traefik. An instance of Traefik is assumed to exist in the cluster.
	TraefikRedirectMode = "redirect"

	// TraefikRedirectionPrefix is the name of the expected property to contain the URL prefix that must be present to redirect to the service
	TraefikRedirectionPrefix = "prefix"

	// TraefikRedirectionEntryPoint is the name of the expected property to contain the Traefik port name (web or secure) that must serve the redirection
	TraefikRedirectionEntryPoint = "traefik_port"

	// TraefikRedirectionServiceName is the name of the expected property to contain the name of the k8s service to redirect the traffic to
	TraefikRedirectionServiceName = "service"

	// TraefikRedirectionServicePort is the name of the expected property to contain the port of the k8s service to redirect traffic to
	TraefikRedirectionServicePort = "port"

	// TraefikRedirectionServiceNamespace is the name of the expected property to contain the namespace of the k8s service to redirect traffic to
	TraefikRedirectionServiceNamespace = "svc_namespace"

	traefikRedirectFilename = "redirect.yaml"
)

type TraefikProvisioner struct {
	scriptsFolder string
}

func (p TraefikProvisioner) GetPort(name string, port int32, targetPort int, nodePort bool) corev1.ServicePort {
	result := corev1.ServicePort{
		Name:       name,
		Port:       port,
		TargetPort: intstr.FromInt(targetPort),
	}
	if nodePort {
		result.NodePort = port
	}
	return result
}

func (p TraefikProvisioner) ProvisionTraefik(config *KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {
	httpPort := viper.GetInt(TraefikHTTPPortProperty)
	sslPort := viper.GetInt(TraefikSslPortProperty)
	adminPort := viper.GetInt(TraefikAdminPortProperty)
	serviceType := viper.GetString(TraefikServiceTypeProperty)

	k8sServiceType := corev1.ServiceTypeClusterIP
	isNodePort := false

	switch serviceType {
	case TraefikNodePortServiceType:
		k8sServiceType = corev1.ServiceTypeNodePort
		isNodePort = true
	case TraefikLoadBalancerServiceType:
		k8sServiceType = corev1.ServiceTypeLoadBalancer
	}

	result := make(model.Parameters)
	logger := logrus.WithFields(logrus.Fields{
		"product": "traeffik",
		"infra":   infra.ID,
	})

	kubeClient, err := NewClient(config.ConfigurationFile)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error getting kubernetes client", err)
	}

	logger.Info("Creating Traefik ingress controller")
	err = kubeClient.ExecuteDeployScript(logger, p.scriptsFolder+"/traefik/deploy.yaml")
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error creating Traefik ingress controller", err)
	}

	ports := make([]corev1.ServicePort, 0)

	if httpPort != 0 {
		ports = append(ports, p.GetPort("web", int32(httpPort), 8000, isNodePort))
		if isNodePort {
			err := config.ClaimPort(httpPort)
			if err != nil {
				return result, fmt.Errorf("Can't reserve port %d for traefik http: %w", httpPort, err)
			}
		}
	}

	if sslPort != 0 {
		ports = append(ports, p.GetPort("secure", int32(sslPort), 4443, isNodePort))
		if isNodePort {
			err := config.ClaimPort(sslPort)
			if err != nil {
				return result, fmt.Errorf("Can't reserve port %d for traefik ssl: %w", sslPort, err)
			}
		}
	}

	if adminPort != 0 {
		ports = append(ports, p.GetPort("admin", int32(adminPort), 8080, isNodePort))
		if isNodePort {
			err := config.ClaimPort(adminPort)
			if err != nil {
				return result, fmt.Errorf("Can't reserve port %d for traefik admin: %w", adminPort, err)
			}
		}
	}

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "traefik",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "traefik"},
			Ports:    ports,
			Type:     k8sServiceType,
		},
	}

	logger.Info("Creating or updating Traefik service")
	_, err = kubeClient.CreateOrUpdateService(logger, corev1.NamespaceDefault, &service)
	if err != nil {
		return result, err
	}

	return result, err
}
func (p TraefikProvisioner) Redirect(config *KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {
	var result model.Parameters

	logger := logrus.WithFields(logrus.Fields{
		"product": "traeffik",
		"infra":   infra.ID,
		"mode":    "redirect",
	})

	kubeClient, err := NewClient(config.ConfigurationFile)
	if err != nil {
		return result, utils.WrapLogAndReturnError(logger, "Error getting kubernetes client", err)
	}

	redirectFilePath := p.scriptsFolder + "/traefik/" + traefikRedirectFilename

	err = kubeClient.ExecuteDeployTemplate(logger, traefikRedirectFilename, redirectFilePath, args)
	return result, err
}

func (p TraefikProvisioner) Provision(config *KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	mode, ok := args.GetString(TraefikProvisionMode)
	if ok && mode == TraefikRedirectMode {
		return p.Redirect(config, infra, args)
	}
	return p.ProvisionTraefik(config, infra, args)
}
