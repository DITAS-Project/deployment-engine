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

import blueprint "github.com/DITAS-Project/blueprint-go"

type VDCInformation struct {
	ID           string `bson:"_id"`
	DeploymentID string `json:"deployment_id" bson:"deployment_id"`
	InfraVDCs    map[string]InfraServicesInformation
}

type InfraServicesInformation struct {
	LastPort           int                       `json:"last_port"`
	LastDatasourcePort int                       `json:"last_datasource_port"`
	VdcNumber          int                       `json:"vdc_number"`
	Initialized        bool                      `json:"initalized"`
	VdcPorts           map[string]int            `json:"vdc_ports"`
	Datasources        map[string]map[string]int `json:"datasources"` // DatasourceType -> DatasourceId -> Port
}

// CreateDeploymentRequest is a request to create a deployment of a VDC of a given blueprint in a series of resources
// swagger:model
type CreateDeploymentRequest struct {
	// The abstract blueprint to use to create the VDC
	// required: true
	Blueprint blueprint.BlueprintType `json:"blueprint"`

	// The list of infrastructures to use to deploy the VDC
	// required: true
	Resources []blueprint.InfrastructureType `json:"resources"`
}
