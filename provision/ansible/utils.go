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
	"encoding/json"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	ansibleHostProperty    = "ansible_host"
	ansibleUserProperty    = "ansible_user"
	kuberneterRoleProperty = "kubernetes_role"
)

func DefaultInventoryHost(node model.NodeInfo) InventoryHost {
	return InventoryHost{
		Name: node.Hostname,
		Vars: map[string]string{
			ansibleHostProperty: node.IP,
			ansibleUserProperty: node.Username,
		},
	}
}

func DefaultKubernetesInventoryHost(node model.NodeInfo) InventoryHost {
	var role string
	if strings.ToLower(node.Role) == "master" {
		role = "master"
	} else {
		role = "node"
	}

	result := DefaultInventoryHost(node)
	result.Vars[kuberneterRoleProperty] = role
	return result
}

func BuildInventory(infra model.InfrastructureDeploymentInfo, nodeTransformer func(node model.NodeInfo) InventoryHost) Inventory {
	result := Inventory{
		Hosts: make([]InventoryHost, 0, infra.NumNodes()),
	}
	infra.ForEachNode(func(node model.NodeInfo) {
		result.Hosts = append(result.Hosts, nodeTransformer(node))
	})
	return result
}

func DefaultAllInventory(infra model.InfrastructureDeploymentInfo) Inventory {
	return BuildInventory(infra, DefaultInventoryHost)
}

func DefaultKubernetesInventory(infra model.InfrastructureDeploymentInfo) Inventory {
	result := BuildInventory(infra, DefaultKubernetesInventoryHost)
	result.Groups = make([]InventoryGroup, 0, len(infra.Nodes))
	for role, hosts := range infra.Nodes {
		group := InventoryGroup{
			Name:  strings.ToLower(role),
			Hosts: make([]string, len(hosts)),
		}
		for i, host := range hosts {
			group.Hosts[i] = host.Hostname
		}
		result.Groups = append(result.Groups, group)
	}
	return result
}

func ExecutePlaybook(logger *log.Entry, script string, inventory string, extravars map[string]string) error {
	args := make([]string, 1)
	args[0] = script

	if inventory != "" {
		args = append(args, fmt.Sprintf("--inventory=%s", inventory))
	}

	if extravars != nil && len(extravars) > 0 {
		args = append(args, "--extra-vars")
		vars, err := json.Marshal(extravars)
		if err != nil {
			return fmt.Errorf("Error marshaling ansible variables: %w", err)
		}
		args = append(args, string(vars))
	}

	return utils.ExecuteCommand(logger, "ansible-playbook", args...)
}
