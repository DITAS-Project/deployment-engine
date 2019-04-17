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

package model

import (
	"fmt"
	"strconv"
)

const (
	BasicAuthType = "basic"
	OAuth2Type    = "oauth"
	PKIType       = "PKI"
)

// ExtraPropertiesType represents extra properties to define for resources, infrastructures or deployments. This properties are provisioner or deployment specific and they should document them when they expect any.
// swagger:model
type ExtraPropertiesType map[string]string

// Drive holds information about a data drive attached to a node
// swagger:model
type Drive struct {
	// Unique name for the drive
	// required:true
	Name string `json:"name"`
	// Type of the drive. It can be "SSD" or "HDD"
	// pattern: SSD|HDD
	// example: SSD
	Type string `json:"type"`
	// Size of the disk in Mb
	// required:true
	Size int64 `json:"size"`
}

// ResourceType has information about a node that needs to be created by a deployer.
// swagger:model
type ResourceType struct {
	// Suffix for the hostname. The real hostname will be formed of the infrastructure name + resource name
	// required:true
	// unique:true
	Name string `json:"name"`
	// Type of the VM to create i.e. n1-small
	// example: n1-small
	Type string `json:"type"`
	// CPU speed in Mhz. Ignored if type is provided
	CPU int `json:"cpu"`
	// Number of cores. Ignored if type is provided
	Cores int `json:"cores"`
	// RAM quantity in Mb. Ignored if type is provided
	RAM int64 `json:"ram"`
	// Boot disk size in Mb
	// required:true
	Disk int64 `json:"disk"`
	// Role that this VM plays. In case of a Kubernetes deployment at least one "master" is needed.
	Role string `json:"role"`
	// Boot image ID to use
	// required:true
	ImageId string `json:"image_id"`
	// IP to assign this VM. In case it's not specified, the first available one will be used.
	IP string `json:"ip,omitempty"`
	// List of data drives to attach to this VM
	Drives []Drive `json:"drives"`
	// Extra properties to pass to the provider or the provisioner
	ExtraProperties ExtraPropertiesType `json:"extra_properties"`
}

// CloudProviderInfo contains information about a cloud provider
// swagger:model
type CloudProviderInfo struct {
	// Endpoint to use for this infrastructure
	// required:true
	APIEndpoint string `json:"api_endpoint"`
	// Type of the infrastructure. i.e AWS, Cloudsigma, GCP or Edge
	APIType string `json:"api_type"`
	// Secret identifier to use to log in to the infrastructure manager.
	SecretID string `json:"secret_id"`
	// Credentials to access the cloud provider. Either this or secret_id is mandatory. Each cloud provider should define the format of this element.
	Credentials map[string]string `json:"credentials"`
}

// InfrastructureType is a set of resources that need to be created or configured to form a cluster
// swagger:model
type InfrastructureType struct {
	// Unique name for the infrastructure
	// required:true
	// unique:true
	Name string `json:"name"`
	// Optional description for the infrastructure
	Description string `json:"description"`
	// Type of the infrastructure: Cloud or Edge: Cloud infrastructures mean that the resources will be VMs that need to be instantiated. Edge means that the infrastructure is already in place and its information will be added to the database but no further work will be done by a deployer.
	Type string `json:"type"`
	// Provider information. Required in case of Cloud type
	Provider CloudProviderInfo `json:"provider"`
	// List of resources to deploy
	// required:true
	Resources []ResourceType `json:"resources"`
	// Extra properties to pass to the provider or the provisioner
	ExtraProperties ExtraPropertiesType `json:"extra_properties"`
}

// Deployment is a set of infrastructures that need to be instantiated or configurated to form clusters
// swagger:model
type Deployment struct {
	// Name for this deployment
	// required:true
	// unique:true
	Name string `json:"name"`
	// Optional description
	Description string `json:"description"`
	// List of infrastructures to deploy for this hybrid deployment
	// required:true
	Infrastructures []InfrastructureType `json:"infrastructures"`
}

// DriveInfo is the information of a drive that has been instantiated
// swagger:model
type DriveInfo struct {
	// UUID of this data drive in the infrastructure provider
	// unique:true
	// required:true
	UUID string `json:"uuid"`
	// Name of the data drive
	// unique:true
	// required:true
	Name string `json:"name"`
	// Size of the disk in bytes
	// required:true
	Size int64 `json:"size"`
}

// NodeInfo is the information of a virtual machine that has been instantiated or a physical one that was pre-existing
// swagger:model
type NodeInfo struct {
	// Hostname of the node.
	// requiered:true
	// unique:true
	Hostname string `json:"hostname"`
	// Role of the node. Master or slave in case of Kubernetes.
	// example:master
	Role string `json:"role"`
	// IP assigned to this node.
	// required:true
	// unique:true
	IP string `json:"ip"`
	// Username to use to manage it. If not present, root will be used.
	Username string `json:"username"`
	// UUID of this node in the infrastructure provider
	// required:true
	// unique:true
	UUID string `json:"uuid"`
	// Boot disk UUID for this node in the infrastructure provider
	// required:true
	// unique:true
	DriveUUID string `json:"drive_uuid" bson:"drive_uuid"`
	// Size of the boot disk in bytes
	// required:true
	// unique:true
	DriveSize int64 `json:"drive_size" bson:"drive_size"`
	// Data drives information
	DataDrives []DriveInfo `json:"data_drives" bson:"data_drives"`
	// Extra properties to pass to the provider or the provisioner
	ExtraProperties ExtraPropertiesType `json:"extra_properties"`
}

// InfrastructureDeploymentInfo contains information about a cluster of nodes that has been instantiated or were already existing.
// swagger:model
type InfrastructureDeploymentInfo struct {
	// Unique infrastructure ID on the deployment
	// required:true
	// unique:true
	ID string `json:"id"`
	// Name of the infrastructure
	Name string `json:"name"`
	// Type of the infrastructure: cloud or edge
	// pattern:cloud|edge
	// required:true
	Type string `json:"type"`
	// Provider information
	// required:true
	Provider CloudProviderInfo `json:"provider"`
	// Set of nodes in the infrastructure indexed by role
	// required:true
	Nodes map[string][]NodeInfo
	// Status of the infrastructure
	Status string `json:"status"`
	// Configuration of installed products, indexed by product name, in this infrastructure.
	Products map[string]interface{} `json:"products"`
	// Extra properties to pass to the provider or the provisioner
	ExtraProperties ExtraPropertiesType `json:"extra_properties"`
}

// DeploymentInfo contains information of a deployment than may compromise several clusters
// swagger:model
type DeploymentInfo struct {
	// Unique ID for the deployment
	// required:true
	// unique:true
	ID string `json:"id" bson:"_id"`
	// Name of the deployment
	Name string `json:"name"`
	// Lisf of infrastructures, each one representing a different cluster.
	Infrastructures map[string]InfrastructureDeploymentInfo `json:"infrastructures"`
	// Extra properties bound to the deployment
	ExtraProperties ExtraPropertiesType `json:"extra_properties"`
	// Global status of the deployment
	Status string `json:"status"`
}

// Secret is a structure that will be saved as cyphered data in the database. Once saved it will receive an identifier and deployments, infrastructures, providers and provisioners can make reference to it by ID.
// swagger:model
type Secret struct {
	// Description of the content in natural language
	Description string `json:"description"`
	// Format of the secret if it applies
	// example:oauth2
	Format string `json:"format"`
	// Metadata associated to the secret. It will be saved in plain text and it can be queried to find required secrets when the ID is unknown.
	Metadata map[string]string `json:"metadata"`
	// Content of the secret that will be saved in cyphered format.
	Content interface{}
}

// BasicAuthSecret is a standard representation of HTTP Basic Authorization credetials
// swagger:model
type BasicAuthSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// OAuth2Secret is a standard representation of OAuth2 credetials
// swagger:model
type OAuth2Secret struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
}

// PKISecret is a standard representation of PKI credetials
// swagger:model
type PKISecret struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

// Deployer is the interface that a module that can deploy virtual resources in a cloud provider must implement.
type Deployer interface {
	DeployInfrastructure(deploymentID string, infra InfrastructureType) (InfrastructureDeploymentInfo, error)
	DeleteInfrastructure(deploymentID string, infra InfrastructureDeploymentInfo) map[string]error
}

//Provisioner is the interface that must implement custom provisioners such as ansible, etc. If some configuration needs to be passed to other provisioners or saved in the database, it should be done by setting them in the Products field of the passed infrastructure
type Provisioner interface {
	Provision(deployment string, infra *InfrastructureDeploymentInfo, product string, args map[string][]string) error
}

// Frontend is the interface that must be implemented for any frontend that will serve an API around the functionality of the deployment engine
type Frontend interface {
	Run(addr string) error
}

// GetBool is an utility function to extract a boolean value from an extra property
func (p ExtraPropertiesType) GetBool(property string) bool {
	if p == nil {
		return false
	}

	strVal, ok := p[property]
	if !ok {
		return false
	}

	boolVal, err := strconv.ParseBool(strVal)
	if err != nil {
		return false
	}

	return boolVal
}

// ForEachNode executes the function passed as parameter for each node in the infrastructure
func (i InfrastructureDeploymentInfo) ForEachNode(apply func(NodeInfo)) {
	for _, nodes := range i.Nodes {
		for _, node := range nodes {
			apply(node)
		}
	}
}

// NumNodes returns the number of total nodes present in the infrastructure
func (i InfrastructureDeploymentInfo) NumNodes() int {
	n := 0
	i.ForEachNode(func(node NodeInfo) {
		n++
	})
	return n
}

// GetFirstNodeOfRole is an utility function that returns the first node of a given role. Used mostly to get the master of a kubernetes cluster.
func (i InfrastructureDeploymentInfo) GetFirstNodeOfRole(role string) (NodeInfo, error) {
	nodes, ok := i.Nodes[role]
	if !ok || nodes == nil || len(nodes) == 0 {
		return NodeInfo{}, fmt.Errorf("Can't find any node with role %s", role)
	}

	return nodes[0], nil
}
