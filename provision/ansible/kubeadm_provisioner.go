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
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	KubeadmPreinstalledProperty = "kubeadm_preinstalled_image"
)

type KubeadmProvisioner struct {
	parent        *Provisioner
	scriptsFolder string
}

func NewKubeadmProvisioner(parent *Provisioner) KubeadmProvisioner {
	return KubeadmProvisioner{
		parent:        parent,
		scriptsFolder: parent.ScriptsFolder,
	}
}

func (p KubeadmProvisioner) BuildInventory(deploymentID string, infra *model.InfrastructureDeploymentInfo, args map[string][]string) (Inventory, error) {
	inventory, err := p.parent.Provisioners["kubernetes"].BuildInventory(deploymentID, infra, args)
	if err != nil {
		return inventory, err
	}

	masters := infra.Nodes["master"]
	if len(masters) == 0 {
		return inventory, fmt.Errorf("At least a node in the infrastructure %s needs to have role 'master' to be able to deploy kubernetes", infra.ID)
	}

	if len(masters) > 1 {
		return inventory, fmt.Errorf("More than one master found in infrastructure %s. High Availability is not supported by this provisioner", infra.ID)
	}

	master := masters[0]

	slavesGroup := make([]string, 0)
	infra.ForEachNode(func(node model.NodeInfo) {
		if strings.ToLower(node.Role) != "master" {
			slavesGroup = append(slavesGroup, node.Hostname)
		}
	})

	inventory.Groups = []InventoryGroup{
		InventoryGroup{
			Name:  "master",
			Hosts: []string{master.Hostname},
		},
		InventoryGroup{
			Name:  "slaves",
			Hosts: slavesGroup,
		},
	}

	return inventory, err
}

func (p KubeadmProvisioner) DeployProduct(inventoryPath, deploymentID string, infra *model.InfrastructureDeploymentInfo, args map[string][]string) error {

	logger := logrus.WithFields(map[string]interface{}{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	inventoryFolder := p.parent.GetInventoryFolder(deploymentID, infra.ID)

	if infra.ExtraProperties.GetBool(KubeadmPreinstalledProperty) {
		err := ExecutePlaybook(logger, p.scriptsFolder+"/kubernetes/kubeadm.yml", inventoryPath, map[string]string{
			"inventory_folder": inventoryFolder,
		})
		if err != nil {
			return err
		}

		infra.Products["kubernetes"] = KubernetesConfiguration{
			ConfigurationFile: inventoryFolder + "/config",
		}
		return nil
	}

	return p.parent.Provisioners["kubernetes"].DeployProduct(inventoryPath, deploymentID, infra, args)
}
