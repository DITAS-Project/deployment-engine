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

const (
	ElasticSearchUrlVarName      = "elasticsearch_url"
	ElasticSearchUsernameVarName = "elasticsearch_user"
	ElasticSearchPasswordVarName = "elasticsearch_password"
)

type VDCInformation struct {
	ID           string `bson:"_id"`
	DeploymentID string `json:"deployment_id" bson:"deployment_id"`
	NumVDCs      int
	VDCs         map[string][]string
}

type Registry struct {
	Name     string
	URL      string
	Username string
	Password string
	Email    string
}
