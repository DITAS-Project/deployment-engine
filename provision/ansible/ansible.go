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
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	InventoryFolderProperty = "ansible.folders.inventory"
	ScriptsFolderProperty   = "ansible.folders.scripts"
	KubesprayFolderProperty = "ansible.folders.kubespray"

	InventoryFolderDefaultValue = "/tmp/ansible_inventories"
	ScriptsFolderDefaultValue   = "provision/ansible"

	AnsibleWaitForSSHReadyProperty = "wait_for_ssh_ready"
	KubesprayFolderDefaultValue    = "provision/ansible/kubespray"
)

type Provisioner struct {
	InventoryFolder string
	ScriptsFolder   string
	Provisioners    map[string]ProductProvisioner
}

type InventoryHost struct {
	Name string
	Vars map[string]string
}

type InventoryGroup struct {
	Name      string
	Hosts     []string
	GroupVars map[string]string
}

type Inventory struct {
	Hosts  []InventoryHost
	Groups []InventoryGroup
}

type ProductProvisioner interface {
	BuildInventory(infra *model.InfrastructureDeploymentInfo, args model.Parameters) (Inventory, error)
	DeployProduct(inventory string, infra *model.InfrastructureDeploymentInfo, args model.Parameters) (model.Parameters, error)
}

func New() (*Provisioner, error) {
	viper.SetDefault(InventoryFolderProperty, InventoryFolderDefaultValue)
	viper.SetDefault(ScriptsFolderProperty, ScriptsFolderDefaultValue)
	viper.SetDefault(KubesprayFolderProperty, KubesprayFolderDefaultValue)

	inventoryFolder := viper.GetString(InventoryFolderProperty)
	scriptsFolder := viper.GetString(ScriptsFolderProperty)
	kubesprayFolder := viper.GetString(KubesprayFolderProperty)

	err := os.MkdirAll(inventoryFolder, os.ModePerm)
	if err != nil {
		log.WithError(err).Errorf("Error creating base inventory folder %s", inventoryFolder)
		return nil, err
	}

	result := Provisioner{
		InventoryFolder: inventoryFolder,
		ScriptsFolder:   scriptsFolder,
	}

	result.Provisioners = map[string]ProductProvisioner{
		"docker":             NewDockerProvisioner(&result),
		"kubernetes":         NewKubernetesProvisioner(&result),
		"kubeadm":            NewKubeadmProvisioner(&result),
		"gluster-kubernetes": NewGlusterfsProvisioner(&result),
		"k3s":                NewK3sProvisioner(&result),
		"private_registries": NewRegistryProvisioner(&result),
		"kubespray":          NewKubesprayProvisioner(&result, kubesprayFolder),
		"helm":               NewHelmProvisioner(&result),
		"fluentd":            NewFluentdProvisioner(&result),
	}

	return &result, nil
}

func (p *Provisioner) AddProvisioner(name string, provisioner ProductProvisioner) {
	p.Provisioners[name] = provisioner
}

func (p *Provisioner) WaitForSSHPortReady(infra *model.InfrastructureDeploymentInfo, args model.Parameters) error {
	logger := log.WithField("infrastructure", infra.ID)
	logger.Info("Waiting for port 22 to be ready")

	inventory, err := p.Provisioners["docker"].BuildInventory(infra, args)
	if err != nil {
		return err
	}

	inventoryPath, err := p.WriteInventory(infra.ID, "common", inventory)
	if err != nil {
		return err
	}

	return ExecutePlaybook(logger, p.ScriptsFolder+"/common/wait_ssh_ready.yml", inventoryPath, nil)
}

func (p Provisioner) WriteGroup(inventoryFile *os.File, group InventoryGroup) error {
	_, err := inventoryFile.WriteString(fmt.Sprintf("[%s]\n", group.Name))
	if err != nil {
		return err
	}

	if group.Hosts != nil {
		for _, host := range group.Hosts {
			_, err := inventoryFile.WriteString(fmt.Sprintf("%s\n", host))
			if err != nil {
				return err
			}
		}
	}

	if group.GroupVars != nil {
		_, err := inventoryFile.WriteString(fmt.Sprintf("[%s:vars]\n", group.Name))
		if err != nil {
			return err
		}

		for k, v := range group.GroupVars {
			_, err := inventoryFile.WriteString(fmt.Sprintf("%s=%s\n", k, v))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (p Provisioner) WriteHost(inventoryFile *os.File, host InventoryHost) error {
	_, err := inventoryFile.WriteString(host.Name)
	if err != nil {
		return err
	}

	if host.Vars != nil {
		for k, v := range host.Vars {
			strVar := fmt.Sprintf(" %s=%s", k, v)
			_, err = inventoryFile.WriteString(strVar)
			if err != nil {
				return err
			}
		}
	}

	_, err = inventoryFile.WriteString("\n")
	if err != nil {
		return err
	}

	return err
}

func (p Provisioner) WriteInventory(infraID, product string, inventory Inventory) (string, error) {
	path := p.GetInventoryFolder(infraID)
	filePath := fmt.Sprintf("%s_%s", p.GetInventoryPath(infraID), product)

	if _, err := os.Stat(filePath); err == nil {

		return filePath, nil

	} else if os.IsNotExist(err) {

		err := os.MkdirAll(path, os.ModePerm)
		if err != nil {
			log.WithError(err).Errorf("Error creating inventory folder %s", path)
			return path, err
		}

		log.Infof("Creating inventory at %s", filePath)
		inventoryFile, err := os.Create(filePath)
		defer inventoryFile.Close()

		if err != nil {
			log.WithError(err).Errorf("Error creating inventory file %s", filePath)
			return path, err
		}

		for _, host := range inventory.Hosts {
			err = p.WriteHost(inventoryFile, host)
			if err != nil {
				return filePath, err
			}
		}

		if inventory.Groups != nil {
			for _, group := range inventory.Groups {
				err = p.WriteGroup(inventoryFile, group)
				if err != nil {
					return filePath, err
				}
			}
		}

	}

	return filePath, nil
}

func (p Provisioner) Provision(infra *model.InfrastructureDeploymentInfo, product string, args model.Parameters) (model.Parameters, error) {

	if args == nil {
		args = make(model.Parameters)
	}
	result := make(model.Parameters)

	provisioner := p.Provisioners[product]
	if provisioner == nil {
		return result, fmt.Errorf("Product %s not supported by this deployer", product)
	}

	/*wait, ok := args.GetBool(AnsibleWaitForSSHReadyProperty)
	if ok && wait {
		err := p.WaitForSSHPortReady(infra, args)
		if err != nil {
			log.WithError(err).Error("Error waiting for infrastructure to be ready")
			return err
		}
	}*/
	/*err := utils.WaitForSSHReady(*infra, true)
	if err != nil {
		return fmt.Errorf("Error waiting for ssh port to be ready: %w", err)
	}*/

	inventory, err := provisioner.BuildInventory(infra, args)
	if err != nil {
		log.WithError(err).Errorf("Error getting inventory for product %s", product)
		return result, err
	}

	inventoryPath, err := p.WriteInventory(infra.ID, product, inventory)
	if err != nil {
		log.WithError(err).Errorf("Error creating inventory file for product %s", product)
		return result, err
	}

	res, err := provisioner.DeployProduct(inventoryPath, infra, args)
	if res == nil {
		return result, err
	}
	return res, err
}

func (p Provisioner) GetProducts() []string {
	result := make([]string, 0, len(p.Provisioners))
	for k := range p.Provisioners {
		result = append(result, k)
	}
	return result
}

func (p *Provisioner) GetInventoryFolder(infraID string) string {
	return fmt.Sprintf("%s/%s", p.InventoryFolder, infraID)
}

func (p *Provisioner) GetInventoryPath(infraID string) string {
	return fmt.Sprintf("%s/%s", p.GetInventoryFolder(infraID), "inventory")
}
