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
	"deployment-engine/provision/ansible"
	"deployment-engine/utils"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	DitasGitInstalledProperty = "ditas_git_installed"
)

type RookCluster struct {
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

type RookProvisioner struct {
	parent        *ansible.Provisioner
	scriptsFolder string
}

func NewRookProvisioner(parent *ansible.Provisioner, scriptsFolder string) RookProvisioner {
	return RookProvisioner{
		parent:        parent,
		scriptsFolder: scriptsFolder,
	}
}

func (p RookProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) (ansible.Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(deploymentID, infra, args)
}

func (p RookProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	installGit := !infra.ExtraProperties.GetBool(DitasGitInstalledProperty)
	haAvailable := len(infra.Slaves) > 1
	numMons := 1
	if haAvailable {
		numMons = 3
	}

	err := ansible.ExecutePlaybook(logger, p.scriptsFolder+"/deploy_rook.yml", inventoryPath, map[string]string{
		"ha_available": string(strconv.AppendBool([]byte{}, haAvailable)),
		"install_git":  string(strconv.AppendBool([]byte{}, installGit)),
		"num_mons":     strconv.Itoa(numMons),
	})

	if err != nil {
		return err
	}

	config, err := GetKubernetesConfigFile(p.parent, deploymentID, infra.ID)
	if err != nil {
		return err
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
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
		return errors.New("Timeout waiting for CEPH cluster to be ready")
	}

	if finalStatus != "Created" {
		return fmt.Errorf("Invalid cluster status: %s", finalStatus)
	}

	return err
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
