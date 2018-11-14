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
	blueprint "github.com/DITAS-Project/blueprint-go"
)

type NodeInfo struct {
	Name          string `json:"name"`
	Role          string `json:"role"`
	IP            string `json:"ip"`
	Username      string `json:"username"`
	UUID          string `json:"uuid"`
	DriveUUID     string `json:"drive_uuid" bson:"drive_uuid"`
	DataDriveUUID string `json:"data_drive_uuid" bson:"data_drive_uuid"`
}

type VDCInfo struct {
	Blueprint blueprint.BlueprintType
	Port      int
}

type InfrastructureDeployment struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Slaves   []NodeInfo         `json:"slaves"`
	Master   NodeInfo           `json:"master"`
	NumVDCs  int                `json:"num_vdcs" bson:"num_vdcs"`
	LastPort int                `json:"last_port" bson:"last_port"`
	Status   string             `json:"status"`
	VDCs     map[string]VDCInfo `json:"vdcs"`
}

type Deployment struct {
	ID              string                     `json:"id" bson:"_id"`
	Infrastructures []InfrastructureDeployment `json:"infrastructures"`
	Status          string                     `json:"status"`
}

type Deployer interface {
	DeployInfrastructure(infrastructure blueprint.InfrastructureType, namePrefix string) (InfrastructureDeployment, error)
}
