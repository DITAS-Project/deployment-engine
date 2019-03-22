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
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/sirupsen/logrus"
)

const (
	DitasGlusterFSClientInstalled = "ditas_glusterfs_client_installed"
)

type glusterFSHostnamesType struct {
	Manage  []string `json:"manage"`
	Storage []string `json:"storage"`
}

type glusterFSNodeInfoType struct {
	Hostnames glusterFSHostnamesType `json:"hostnames"`
	Zone      int                    `json:"zone"`
}

type glusterFSNodeType struct {
	Node    glusterFSNodeInfoType `json:"node"`
	Devices []string              `json:"devices"`
}

type glusterFSClusterType struct {
	Nodes []glusterFSNodeType `json:"nodes"`
}

type glusterFSTopology struct {
	Clusters []glusterFSClusterType `json:"clusters"`
}

type GlusterfsProvisioner struct {
	parent        *ansible.Provisioner
	scriptsFolder string
}

func NewGlusterfsProvisioner(parent *ansible.Provisioner, scriptsFolder string) GlusterfsProvisioner {
	return GlusterfsProvisioner{
		parent:        parent,
		scriptsFolder: scriptsFolder,
	}
}

func (p GlusterfsProvisioner) BuildInventory(deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) (ansible.Inventory, error) {
	return p.parent.Provisioners["kubeadm"].BuildInventory(deploymentID, infra, args)
}

func (p GlusterfsProvisioner) toGlusterFSDevices(devices []model.DriveInfo) []string {
	result := make([]string, len(devices))
	for i := 0; i < len(devices); i++ {
		result[i] = fmt.Sprintf("/dev/vd%s", string(rune('b'+i)))
	}
	return result
}

func (p GlusterfsProvisioner) toGlusterFSNode(node model.NodeInfo) glusterFSNodeType {
	return glusterFSNodeType{
		Node: glusterFSNodeInfoType{
			Hostnames: glusterFSHostnamesType{
				Manage:  []string{node.Hostname},
				Storage: []string{node.IP},
			},
			Zone: 1,
		},
		Devices: p.toGlusterFSDevices(node.DataDrives),
	}
}

func (p GlusterfsProvisioner) generateGlusterFSTopology(infra model.InfrastructureDeploymentInfo) (string, error) {
	nodes := make([]glusterFSNodeType, len(infra.Slaves)+1)
	nodes[0] = p.toGlusterFSNode(infra.Master)
	for i, node := range infra.Slaves {
		nodes[i+1] = p.toGlusterFSNode(node)
	}
	result, err := json.Marshal(glusterFSTopology{
		Clusters: []glusterFSClusterType{
			glusterFSClusterType{
				Nodes: nodes,
			},
		},
	})
	if err != nil {
		return "", err
	}

	return string(result), nil
}

func (p GlusterfsProvisioner) DeployProduct(inventoryPath, deploymentID string, infra model.InfrastructureDeploymentInfo, args map[string][]string) error {

	logger := logrus.WithFields(logrus.Fields{
		"deployment":     deploymentID,
		"infrastructure": infra.ID,
	})

	installClient := !infra.ExtraProperties.GetBool(DitasGlusterFSClientInstalled)

	topology, err := p.generateGlusterFSTopology(infra)
	if err != nil {
		return err
	}

	singleNode := ""
	if len(infra.Slaves) < 2 {
		singleNode = "--single-node"
	}

	return ansible.ExecutePlaybook(logger, p.scriptsFolder+"/deploy_glusterfs.yml", inventoryPath, map[string]string{
		"topology":       topology,
		"single_node":    singleNode,
		"install_client": string(strconv.AppendBool([]byte{}, installClient)),
	})
}
