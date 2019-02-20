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
	"errors"
	"fmt"
	"os"
	"time"

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

func (p *Provisioner) clearKnownHost(logger *log.Entry, ip string) error {
	return ExecutePlaybook(logger, p.ScriptsFolder+"/common/clear_known_hosts.yml", "", map[string]string{
		"host_ip": ip,
	})
}

func (p *Provisioner) addHostToHostFile(log *log.Entry, hostInfo model.NodeInfo) error {
	logger := log.WithField("host", hostInfo.Hostname)

	err := p.clearKnownHost(logger, hostInfo.IP)
	if err != nil {
		logger.WithError(err).Error("Error clearing known hosts")
		return err
	}

	host := fmt.Sprintf("%s@%s", hostInfo.Username, hostInfo.IP)
	command := fmt.Sprintf("echo %s %s | sudo tee -a /etc/hosts > /dev/null 2>&1", hostInfo.IP, hostInfo.Hostname)
	timeout := 30 * time.Second
	logger.Info("Waiting for ssh service to be ready")
	_, timedOut, _ := utils.WaitForStatusChange("starting", timeout, func() (string, error) {
		err := utils.ExecuteCommand(logger, "ssh", "-o", "StrictHostKeyChecking=no", host, command)
		if err != nil {
			return "starting", nil
		}
		return "started", nil
	})
	if timedOut {
		msg := "Timeout waiting for ssh service to start"
		logger.Errorf(msg)
		return errors.New(msg)
	}
	logger.Info("Ssh service ready")

	return nil
}

func (p *Provisioner) addToHostFile(logger *log.Entry, infra model.InfrastructureDeploymentInfo) error {

	logger.Info("Adding master to hosts")
	err := p.addHostToHostFile(logger, infra.Master)

	if err != nil {
		logger.WithError(err).Error("Error adding master to hosts")
		return err
	}

	logger.Info("Master added. Adding slaves to hosts")

	for _, slave := range infra.Slaves {
		err = p.addHostToHostFile(logger, slave)
		if err != nil {
			logger.WithError(err).Errorf("Error adding slave %s to hosts", slave.Hostname)
			return err
		}
	}

	logger.Info("Slaves added")

	return nil
}

func (p *Provisioner) writeHost(node model.NodeInfo, file *os.File) (int, error) {
	line := fmt.Sprintf("%s ansible_host=%s ansible_user=%s\n", node.Hostname, node.IP, node.Username)
	return file.WriteString(line)
}

func (p *Provisioner) createInventory(logger *log.Entry, deploymentID string, infra model.InfrastructureDeploymentInfo) (string, error) {
	path := p.GetInventoryFolder(deploymentID, infra.ID)

	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		logger.WithError(err).Errorf("Error creating inventory folder %s", path)
		return path, err
	}

	filePath := p.GetInventoryPath(deploymentID, infra.ID)
	logger.Infof("Creating inventory at %s", filePath)
	inventory, err := os.Create(filePath)
	defer inventory.Close()

	if err != nil {
		logger.WithError(err).Errorf("Error creating inventory file %s", filePath)
		return path, err
	}

	_, err = inventory.WriteString("[master]\n")
	if err != nil {
		logger.WithError(err).Error("Error writing master header to inventory")
		return path, err
	}

	_, err = p.writeHost(infra.Master, inventory)
	if err != nil {
		logger.WithError(err).Error("Error writing master information to inventory")
		return path, err
	}

	_, err = inventory.WriteString("[slaves]\n")
	if err != nil {
		logger.WithError(err).Error("Error writing slaves header to inventory")
		return path, err
	}
	for _, slave := range infra.Slaves {
		_, err = p.writeHost(slave, inventory)
		if err != nil {
			logger.WithError(err).Errorf("Error writing slave %s header to inventory", slave.Hostname)
			return path, err
		}
	}

	logger.Info("Inventory correctly created")

	return path, nil
}

func (p *Provisioner) deployK8s(deploymentID string, infra model.InfrastructureDeploymentInfo) error {
	logger := log.WithField("infrastructure", infra.ID)
	inventoryPath, err := p.createInventory(logger, deploymentID, infra)
	if err != nil {
		return err
	}

	logger.Info("Calling Ansible for initial k8s deployment")
	//time.Sleep(180 * time.Second)
	err = ExecutePlaybook(logger, p.ScriptsFolder+"/kubernetes/ansible_deploy.yml", inventoryPath, map[string]string{
		"masterUsername": infra.Master.Username,
	})

	if err != nil {
		logger.WithError(err).Error("Error executing ansible deployment for k8s deployment")
		return err
	}

	logger.Info("K8s cluster created")
	return nil
}

func (p Provisioner) Provision(deploymentId string, infra model.InfrastructureDeploymentInfo, product string) error {

	if len(infra.Products) == 0 {
		err := p.addToHostFile(log.WithField("infrastructure", infra.ID), infra)
		if err != nil {
			log.WithError(err).Error("Error setting known_hosts")
			return err
		}
	}

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
