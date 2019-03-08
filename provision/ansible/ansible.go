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

package ansible

import (
	"deployment-engine/model"
	"deployment-engine/utils"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	InventoryFolderProperty = "ansible.folders.inventory"
	ScriptsFolderProperty   = "ansible.folders.scripts"

	InventoryFolderDefaultValue = "/tmp/ansible_inventories"
	ScriptsFolderDefaultValue   = "provision/ansible"
)

type Provisioner struct {
	InventoryFolder string
	ScriptsFolder   string
}

func New() (*Provisioner, error) {
	viper.SetDefault(InventoryFolderProperty, InventoryFolderDefaultValue)
	viper.SetDefault(ScriptsFolderProperty, ScriptsFolderDefaultValue)

	inventoryFolder := viper.GetString(InventoryFolderProperty)
	scriptsFolder := viper.GetString(ScriptsFolderProperty)

	err := os.MkdirAll(inventoryFolder, os.ModePerm)
	if err != nil {
		log.WithError(err).Errorf("Error creating base inventory folder %s", inventoryFolder)
		return nil, err
	}

	return &Provisioner{
		InventoryFolder: inventoryFolder,
		ScriptsFolder:   scriptsFolder,
	}, nil
}

func (p *Provisioner) WaitForSSHPortReady(logger *log.Entry, inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo) error {
	logger.Info("Waiting for port 22 to be ready")
	return ExecutePlaybook(logger, p.ScriptsFolder+"/common/wait_ssh_ready.yml", inventoryPath, nil)
}

func (p *Provisioner) WriteHost(node model.NodeInfo, file *os.File) (int, error) {
	var role string
	if node.Role == "master" {
		role = "master"
	} else {
		role = "node"
	}

	devices := ""
	if node.DataDrives != nil {
		for i := 0; i < len(node.DataDrives); i++ {
			device := fmt.Sprintf("\"/dev/vd%s\"", string(rune('b'+i)))
			devices = devices + device
			if i < (len(node.DataDrives) - 1) {
				devices = devices + ","
			}
		}
	}

	line := fmt.Sprintf("%s ansible_host=%s ansible_user=%s kubernetes_role=%s devices='[%s]'\n", node.Hostname, node.IP, node.Username, role, devices)
	return file.WriteString(line)
}

func (p *Provisioner) defaultProvisionerInventoryWriter(logger *log.Entry, inventory *os.File, infra model.InfrastructureDeploymentInfo) error {
	_, err := inventory.WriteString("[master]\n")
	if err != nil {
		logger.WithError(err).Error("Error writing master header to inventory")
		return err
	}

	_, err = p.WriteHost(infra.Master, inventory)
	if err != nil {
		logger.WithError(err).Error("Error writing master information to inventory")
		return err
	}

	_, err = inventory.WriteString("[slaves]\n")
	if err != nil {
		logger.WithError(err).Error("Error writing slaves header to inventory")
		return err
	}
	for _, slave := range infra.Slaves {
		_, err = p.WriteHost(slave, inventory)
		if err != nil {
			logger.WithError(err).Errorf("Error writing slave %s header to inventory", slave.Hostname)
			return err
		}
	}

	return nil
}

func (p *Provisioner) GetCustomInventory(logger *log.Entry, deploymentID string, infra model.InfrastructureDeploymentInfo, writer func(logger *log.Entry, inventory *os.File, infra model.InfrastructureDeploymentInfo) error) (string, error) {
	path := p.GetInventoryFolder(deploymentID, infra.ID)
	filePath := p.GetInventoryPath(deploymentID, infra.ID)

	if _, err := os.Stat(filePath); err == nil {

		return filePath, nil

	} else if os.IsNotExist(err) {

		err := os.MkdirAll(path, os.ModePerm)
		if err != nil {
			logger.WithError(err).Errorf("Error creating inventory folder %s", path)
			return path, err
		}

		logger.Infof("Creating inventory at %s", filePath)
		inventory, err := os.Create(filePath)
		defer inventory.Close()

		if err != nil {
			logger.WithError(err).Errorf("Error creating inventory file %s", filePath)
			return path, err
		}

		err = writer(logger, inventory, infra)
		if err != nil {
			return path, err
		}

		logger.Info("Inventory correctly created")

		return path, nil
	} else {
		return "", err
	}
}

func (p *Provisioner) GetInventory(logger *log.Entry, deploymentID string, infra model.InfrastructureDeploymentInfo) (string, error) {
	return p.GetCustomInventory(logger, deploymentID, infra, p.defaultProvisionerInventoryWriter)
}

func (p *Provisioner) deployK8s(deploymentID string, infra model.InfrastructureDeploymentInfo) error {
	logger := log.WithField("infrastructure", infra.ID)

	inventoryPath, err := p.GetInventory(logger, deploymentID, infra)
	if err != nil {
		return err
	}

	err = p.WaitForSSHPortReady(logger, inventoryPath, deploymentID, infra)
	if err != nil {
		return err
	}

	logger.Info("Calling Ansible for initial k8s deployment")
	logger.Info("Getting required roles")
	err = utils.ExecuteCommand(logger, "ansible-galaxy", "install", "geerlingguy.docker", "geerlingguy.kubernetes")

	if err != nil {
		logger.WithError(err).Error("Error installing kubernetes roles")
		return err
	}
	//time.Sleep(180 * time.Second)
	err = ExecutePlaybook(logger, p.ScriptsFolder+"/kubernetes/main.yml", inventoryPath, nil)

	if err != nil {
		logger.WithError(err).Error("Error executing ansible deployment for k8s deployment")
		return err
	}

	logger.Info("K8s cluster created")
	return nil
}

func (p Provisioner) Provision(deploymentId string, infra model.InfrastructureDeploymentInfo, product string) error {

	if product == "kubernetes" {
		return p.deployK8s(deploymentId, infra)
	}

	return fmt.Errorf("Product %s not supported by this deployer", product)
}

func (p *Provisioner) GetInventoryFolder(deploymentID, infraID string) string {
	return fmt.Sprintf("%s/%s/%s", p.InventoryFolder, deploymentID, infraID)
}

func (p *Provisioner) GetInventoryPath(deploymentId, infraId string) string {
	return fmt.Sprintf("%s/%s", p.GetInventoryFolder(deploymentId, infraId), "inventory")
}
