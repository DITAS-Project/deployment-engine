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

type Drive struct {
	Name string `json:"name"` //Name of the image to use. Most of the times, it will be available as /dev/disk/by-id/${name} value in the VM
	Type string `json:"type"` //Type of the drive. It can be "SSD" or "HDD"
	Size int64  `json:"size"` //Size of the disk in Mb
}

type ResourceType struct {
	Name    string  `json:"name"`         //Suffix for the hostname. The real hostname will be formed of the infrastructure name + resource name
	Type    string  `json:"type"`         //Type of the VM to create i.e. n1-small
	CPU     int     `json:"cpu"`          //CPU speed in Mhz. Ignored if type is provided
	Cores   int     `json:"cores"`        //Number of cores. Ignored if type is provided
	RAM     int64   `json:"ram"`          //RAM quantity in Mb. Ignored if type is provided
	Disk    int64   `json:"disk"`         //Boot disk size in Mb
	Role    string  `json:"role"`         //Role that this VM plays. In case of a Kubernetes deployment at least one "master" is needed.
	ImageId string  `json:"image_id"`     //Boot image ID to use
	IP      string  `json:"ip,omitempty"` //IP to assign this VM. In case it's not specified, the first available one will be used.
	Drives  []Drive `json:"drives"`       //List of data drives to attach to this VM
}

type CloudProviderInfo struct {
	APIEndpoint string `json:"api_endpoint"` //Endpoint to use for this infrastructure
	APIType     string `json:"api_type"`     //Type of the infrastructure. i.e AWS, Cloudsigma, GCP or Edge
	KeypairID   string `json:"keypair_id"`   //Keypair to use to log in to the infrastructure manager
}

type InfrastructureType struct {
	Name        string            `json:"name"`        //Unique name for the infrastructure
	Description string            `json:"description"` //Optional description for the infrastructure
	Type        string            `json:"type"`        //Type of the infrastructure: Cloud or Edge
	Provider    CloudProviderInfo `json:"provider"`    //Provider information
	Resources   []ResourceType    `json:"resources"`   //List of resources to deploy
}

type Deployment struct {
	Name           string               `json:"name"`           //Name for this deployment
	Description    string               `json:"description"`    //Optional description
	Infrastructure []InfrastructureType `json:"infrastructure"` //List of infrastructures to deploy for this hybrid deployment
}

type DriveInfo struct {
	UUID string `json:"uuid"` //UUID of this data drive in the infrastructure provider
	Name string `json:"name"` //Name of the data drive
}

type NodeInfo struct {
	Hostname   string      `json:"hostname"`                       //Hostname of the node.
	Role       string      `json:"role"`                           //Role of the node. Master or slave.
	IP         string      `json:"ip"`                             //IP assigned to this node.
	Username   string      `json:"username"`                       //Username to use to manage it. If not present, root will be used.
	UUID       string      `json:"uuid"`                           //UUID of this node in the infrastructure provider
	DriveUUID  string      `json:"drive_uuid" bson:"drive_uuid"`   //Boot disk UUID for this node in the infrastructure provider
	DataDrives []DriveInfo `json:"data_drives" bson:"data_drives"` //Data drives information
}

type InfrastructureDeploymentInfo struct {
	ID       string            `json:"id"`       //Unique infrastructure ID on the deployment
	Type     string            `json:"type"`     //Type of the infrastructure: cloud or edge
	Provider CloudProviderInfo `json:"provider"` //Provider information
	Slaves   []NodeInfo        `json:"slaves"`   //List of slaves nodes information
	Master   NodeInfo          `json:"master"`   //Master node information
	Status   string            `json:"status"`   //Status of the infrastructure
	Products []string          `json:"products"` //List of installed products in this infrastructure
}

type DeploymentInfo struct {
	ID              string                         `json:"id" bson:"_id"`   //Unique ID for the deployment
	Infrastructures []InfrastructureDeploymentInfo `json:"infrastructures"` //Lisf of infrastructures.
	Status          string                         `json:"status"`          //Global status of the deployment
}

type Deployer interface {
	DeployInfrastructure(infra InfrastructureType) (InfrastructureDeploymentInfo, error)
	DeleteInfrastructure(infra InfrastructureDeploymentInfo) map[string]error
}

//Provisioner is the interface that must implement custom provisioners such as ansible, etc
type Provisioner interface {
	Provision(deployment string, infra InfrastructureDeploymentInfo, product string) error
}

type Frontent interface {
	Run(addr string) error
}
