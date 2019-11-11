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

type DataSourceInformation struct {
	Type    string
	Vars    map[string]string
	Secrets map[string]kubernetes.EnvSecret
}

type InfrastructureInformation struct {
	IP            string
	TombstonePort int
	CAFPort       int
	Datasources   map[string]DataSourceInformation
	// DALInformation is the ports used by the DALs in this infrastructure, indexed by DAL identifier and then by image identifier
	DALInformation map[string]map[string]int
}

type VDCConfiguration struct {
	Blueprint              string
	AppDeveloperDeployment []string `json:"app_developer_deployment" bson:"app_developer_deployment"`
	// DALsInUse sets the IP to use for every DAL referenced in the VDC if it's been moved
	DALsInUse       map[string]string
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
