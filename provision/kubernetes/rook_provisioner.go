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
	"encoding/json"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type RookCluster struct {
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

type RookProvisioner struct {
	scriptsFolder string
}

func (p RookProvisioner) GetHostCapacity(node model.NodeInfo) (int64, error) {
	capacity := int64(0)
	for _, drive := range node.DataDrives {
		if drive.Size < 5*1024*1024*1024 {
			return 0, fmt.Errorf("Data drive %s of host %s is smaller than 5GB. Installation will fail. Please, remove or resize the drive update the drive size information in the database and try again", drive.UUID, node.Hostname)
		}
		capacity = capacity + drive.Size
	}
	return capacity, nil
}

func (p RookProvisioner) Provision(config *KubernetesConfiguration, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error) {

	result := make(model.Parameters)
	logger := logrus.WithFields(logrus.Fields{
		"config": config.ConfigurationFile,
	})

	var err error
	capacity := int64(0)
	numNodes := 0
	infra.ForEachNode(func(node model.NodeInfo) {
		nodeCapacity, nodeError := p.GetHostCapacity(node)
		if nodeError == nil {
			capacity = capacity + nodeCapacity
		} else {
			err = nodeError
		}
		numNodes++
	})

	if err != nil {
		return result, err
	}

	if capacity == 0 {
		return result, errors.New("Can't find any valid data drives attached to the nodes of the infrastructure. To deploy Rook at least one attached non-formatted data drive of at least 5GB is needed in a node")
	}

	logger.Infof("Total capacity of the cluster: %f Gb", float32(capacity)/float32(1024*1024*1024))

	kubeClient, err := NewClient(config.ConfigurationFile)
	if err != nil {
		logger.WithError(err).Error("Error getting kubernetes client")
		return result, err
	}

	logger.Info("Creating Rook operator")
	err = kubeClient.ExecuteDeployScript(logger, p.scriptsFolder+"/rook/rook_operator.yaml")
	if err != nil {
		logger.WithError(err).Errorf("Error creating Rook operator plane")
		return result, err
	}

	logger.Info("Waiting for Rook operator to be ready")
	err = kubeClient.ExecuteKubectlCommand(logger, "wait", "deployment/rook-ceph-operator", "--for", "condition=available", "--timeout=120s", "--namespace", "rook-ceph-system")
	if err != nil {
		logger.WithError(err).Errorf("Error waiting for Rook operator plane to be ready")
		return result, err
	}

	haAvailable := numNodes > 2
	numMons := 1
	if haAvailable {
		numMons = 3
	}

	logger.Info("Creating Ceph cluster in Rook")
	fileName := "cluster.yaml.tmpl"
	clusterDefinition, err := template.New(fileName).ParseFiles(p.scriptsFolder + "/rook/" + fileName)
	if err != nil {
		logger.WithError(err).Error("Error reading cluster template file")
		return result, err
	}

	cmd := kubeClient.CreateKubectlCommand(logger, "create", "-f", "-")
	writer, err := cmd.StdinPipe()
	if err != nil {
		logger.WithError(err).Error("Error getting input pipe for command")
		return result, err
	}

	go func() {
		defer writer.Close()
		err = clusterDefinition.Execute(writer, map[string]interface{}{
			"num_mons": numMons,
		})
		if err != nil {
			logger.WithError(err).Error("Error executing deployment template")
		}
	}()

	err = cmd.Run()
	if err != nil {
		logger.WithError(err).Error("Error creating cluster")
		return result, err
	}

	logger.Info("Creating Non-High Available storage class")
	err = kubeClient.ExecuteDeployScript(logger, p.scriptsFolder+"/rook/storageclass_rook_single.yml")
	if err != nil {
		logger.WithError(err).Errorf("Error creating Non-High Available storage class")
		return result, err
	}

	if haAvailable {
		logger.Info("Creating High Available storage class")
		err = kubeClient.ExecuteDeployScript(logger, p.scriptsFolder+"/rook/storageclass_rook_ha.yml")
		if err != nil {
			logger.WithError(err).Errorf("Error creating High Available storage class")
			return result, err
		}
	}

	client, err := dynamic.NewForConfig(kubeClient.Config)
	if err != nil {
		return result, err
	}

	resClient := client.Resource(schema.GroupVersionResource{
		Group:    "ceph.rook.io",
		Version:  "v1",
		Resource: "cephclusters",
	}).Namespace("rook-ceph")

	logger.Info("Waiting for cluster to be ready")

	finalStatus, timeout, err := utils.WaitForStatusChange("Creating", 5*time.Minute, func() (string, error) {
		status, err := p.getClusterStatus(logger, resClient)
		if err != nil {
			return "", err
		}
		if status.Status.State == "" {
			return "Creating", nil
		}

		return status.Status.State, nil
	})

	if timeout {
		return result, errors.New("Timeout waiting for CEPH cluster to be ready")
	}

	if finalStatus != "Created" {
		return result, fmt.Errorf("Invalid cluster status: %s", finalStatus)
	}

	logger.Info("Rook cluster successfully created")

	return result, err
}

func (p RookProvisioner) getClusterStatus(logger *logrus.Entry, client dynamic.ResourceInterface) (RookCluster, error) {
	var clusterStatus RookCluster

	cluster, err := client.Get("rook-ceph", metav1.GetOptions{})
	if err != nil {
		logger.WithError(err).Error("Error getting CEPH cluster")
		return clusterStatus, err
	}

	jsonDef, err := cluster.MarshalJSON()
	if err != nil {
		logger.WithError(err).Error("Error marshaling CEPH cluster to JSON")
		return clusterStatus, err
	}

	err = json.Unmarshal(jsonDef, &clusterStatus)
	if err != nil {
		logger.WithError(err).Error("Error unmarshaling JSON cluster definition")
		return clusterStatus, err
	}

	return clusterStatus, err
}
