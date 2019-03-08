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

const (
	BasicAuthType = "basic"
	OAuth2Type    = "oauth"
	PKIType       = "PKI"
)

type Drive struct {
	Name string `json:"name"` //Name of the image to use. Most of the times, it will be available as /dev/disk/by-id/${name} value in the VM
	Type string `json:"type"` //Type of the drive. It can be "SSD" or "HDD"
	Size int64  `json:"size"` //Size of the disk in Mb
}

type ResourceType struct {
	Name            string            `json:"name"`             //Suffix for the hostname. The real hostname will be formed of the infrastructure name + resource name
	Type            string            `json:"type"`             //Type of the VM to create i.e. n1-small
	CPU             int               `json:"cpu"`              //CPU speed in Mhz. Ignored if type is provided
	Cores           int               `json:"cores"`            //Number of cores. Ignored if type is provided
	RAM             int64             `json:"ram"`              //RAM quantity in Mb. Ignored if type is provided
	Disk            int64             `json:"disk"`             //Boot disk size in Mb
	Role            string            `json:"role"`             //Role that this VM plays. In case of a Kubernetes deployment at least one "master" is needed.
	ImageId         string            `json:"image_id"`         //Boot image ID to use
	IP              string            `json:"ip,omitempty"`     //IP to assign this VM. In case it's not specified, the first available one will be used.
	Drives          []Drive           `json:"drives"`           //List of data drives to attach to this VM
	ExtraProperties map[string]string `json:"extra_properties"` //Extra properties to pass to the provider or the provisioner
}

type CloudProviderInfo struct {
	APIEndpoint string            `json:"api_endpoint"` //Endpoint to use for this infrastructure
	APIType     string            `json:"api_type"`     //Type of the infrastructure. i.e AWS, Cloudsigma, GCP or Edge
	SecretID    string            `json:"secret_id"`    //Secret identifier to use to log in to the infrastructure manager.
	Credentials map[string]string `json:"credentials"`  //Credentials to access the cloud provider. Either this or secret_id is mandatory.
}

type InfrastructureType struct {
	Name            string            `json:"name"`             //Unique name for the infrastructure
	Description     string            `json:"description"`      //Optional description for the infrastructure
	Type            string            `json:"type"`             //Type of the infrastructure: Cloud or Edge
	Provider        CloudProviderInfo `json:"provider"`         //Provider information
	Resources       []ResourceType    `json:"resources"`        //List of resources to deploy
	ExtraProperties map[string]string `json:"extra_properties"` //Extra properties to pass to the provider or the provisioner
}

type Deployment struct {
	Name            string               `json:"name"`            //Name for this deployment
	Description     string               `json:"description"`     //Optional description
	Infrastructures []InfrastructureType `json:"infrastructures"` //List of infrastructures to deploy for this hybrid deployment
}

type DriveInfo struct {
	UUID string `json:"uuid"` //UUID of this data drive in the infrastructure provider
	Name string `json:"name"` //Name of the data drive
}

type NodeInfo struct {
	Hostname        string            `json:"hostname"`                       //Hostname of the node.
	Role            string            `json:"role"`                           //Role of the node. Master or slave.
	IP              string            `json:"ip"`                             //IP assigned to this node.
	Username        string            `json:"username"`                       //Username to use to manage it. If not present, root will be used.
	UUID            string            `json:"uuid"`                           //UUID of this node in the infrastructure provider
	DriveUUID       string            `json:"drive_uuid" bson:"drive_uuid"`   //Boot disk UUID for this node in the infrastructure provider
	DataDrives      []DriveInfo       `json:"data_drives" bson:"data_drives"` //Data drives information
	ExtraProperties map[string]string `json:"extra_properties"`               //Extra properties to pass to the provider or the provisioner
}

type InfrastructureDeploymentInfo struct {
	ID              string            `json:"id"`               //Unique infrastructure ID on the deployment
	Name            string            `json:"name"`             //Name of the infrastructure
	Type            string            `json:"type"`             //Type of the infrastructure: cloud or edge
	Provider        CloudProviderInfo `json:"provider"`         //Provider information
	Slaves          []NodeInfo        `json:"slaves"`           //List of slaves nodes information
	Master          NodeInfo          `json:"master"`           //Master node information
	Status          string            `json:"status"`           //Status of the infrastructure
	Products        []string          `json:"products"`         //List of installed products in this infrastructure
	ExtraProperties map[string]string `json:"extra_properties"` //Extra properties to pass to the provider or the provisioner
}

type DeploymentInfo struct {
	ID              string                         `json:"id" bson:"_id"`   //Unique ID for the deployment
	Name            string                         `json:"name"`            //Name of the deployment
	Infrastructures []InfrastructureDeploymentInfo `json:"infrastructures"` //Lisf of infrastructures.
	Status          string                         `json:"status"`          //Global status of the deployment
}

// Product is a series of scripts that allow to install software in a deployment
type Product struct {
	ID     string `json:"id" bson:"_id"` // Unique ID for the product
	Name   string `json:"name"`          // Unique name of the product
	Folder string `json:"folder"`        // Folder containing the scripts to deploy the product
}

type Secret struct {
	Description string            `json:"description"`
	Format      string            `json:"format"`
	Metadata    map[string]string `json:"metadata"`
	Content     interface{}
}

type BasicAuthSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type OAuth2Secret struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
}

type PKISecret struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

type Deployer interface {
	DeployInfrastructure(deploymentID string, infra InfrastructureType) (InfrastructureDeploymentInfo, error)
	DeleteInfrastructure(deploymentID string, infra InfrastructureDeploymentInfo) map[string]error
}

//Provisioner is the interface that must implement custom provisioners such as ansible, etc
type Provisioner interface {
	Provision(deployment string, infra InfrastructureDeploymentInfo, product string) error
}

// Frontend is the interface that must be implemented for any frontend that will serve an API around the functionality of the deployment engine
type Frontend interface {
	Run(addr string) error
}
