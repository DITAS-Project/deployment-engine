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

import "deployment-engine/provision/kubernetes"

const (
	ElasticSearchUrlVarName      = "elasticsearch_url"
	ElasticSearchUsernameVarName = "elasticsearch_user"
	ElasticSearchPasswordVarName = "elasticsearch_password"
)

// DataSourceInformation has information about a datasource running in a cluster
// swagger:model
type DataSourceInformation struct {
	// Type is the type of datasource. e.g. mysql, minio, etc
	Type string
	// Vars are the environment variable used when running the datasource
	Vars map[string]string
	// Secrets is a set of environment variables used by the datasource whose content is in a Kubernetes secres
	Secrets map[string]kubernetes.EnvSecret
}

// InfrastructureInformation contains information about the software running in an infrastructure to help the VDC
// swagger:model
type InfrastructureInformation struct {
	// IP is the IP of the infrastructure that can be targeted for requests
	IP string
	// TombstonePort is the port exposed in this cluster for tombstone
	TombstonePort int
	// CAFPort is the port in which the VDC is listening for requests in this cluster
	CAFPort int
	// Datasources has information about the datasources running in this cluster due to this VDC
	Datasources map[string]DataSourceInformation
	// DALInformation is the ports used by the DALs in this infrastructure, indexed by DAL identifier and then by image identifier
	DALInformation map[string]map[string]int
}

// VDCConfiguration has information about a VDC which might be running in several infrastructures
// swagger:model
type VDCConfiguration struct {
	// Blueprint is the concrete blueprint of this VDC
	Blueprint string
	// AppDeveloperDeployment is the list of infrastructure identifiers which are provided by the Application Developer for this VDC
	AppDeveloperDeployment []string `json:"app_developer_deployment" bson:"app_developer_deployment"`
	// DALsInUse sets the IP to use for every DAL referenced in the VDC if it's been moved
	DALsInUse map[string]string
	// Infrastructures has information about the software running in the different infrastructures in which this VDC is running
	Infrastructures map[string]InfrastructureInformation
}

type VDCInformation struct {
	ID                  string `bson:"_id"`
	VDMIP               string
	VDMInfraID          string
	DataOwnerDeployment []string `json:"data_owner_deployment" bson:"data_owner_deployment"`
	NumVDCs             int
	VDCs                map[string]VDCConfiguration
}

type Registry struct {
	Name     string
	URL      string
	Username string
	Password string
	Email    string
}
