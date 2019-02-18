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
	"deployment-engine/infrastructure"
	"deployment-engine/persistence/mongorepo"
	"deployment-engine/provision"
	"deployment-engine/provision/ansible"
	"deployment-engine/restfrontend"
	"encoding/json"
	"net/http"

	"fmt"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type DitasFrontend struct {
	DefaultFrontend       *restfrontend.App
	Router                *mux.Router
	DeploymentController  *infrastructure.Deployer
	ProvisionerController *provision.ProvisionerController
	VDCManagerInstance    *VDCManager
}

func NewDitasFrontend() (*DitasFrontend, error) {
	repository, err := mongorepo.CreateRepositoryNative()
	if err != nil {
		return nil, err
	}

	provisioner, err := ansible.New()
	if err != nil {
		return nil, err
	}

	deployer := &infrastructure.Deployer{
		Repository: repository,
	}

	controller := &provision.ProvisionerController{
		Repository:  repository,
		Provisioner: provisioner,
	}

	vdcManager, err := NewVDCManager(provisioner, deployer, controller)
	if err != nil {
		return nil, err
	}

	result := DitasFrontend{
		Router:                mux.NewRouter(),
		DeploymentController:  deployer,
		ProvisionerController: controller,
		DefaultFrontend: &restfrontend.App{
			DeploymentController:  deployer,
			ProvisionerController: controller,
		},
		VDCManagerInstance: vdcManager,
	}

	result.initializeRoutes()
	return &result, nil
}

func (a DitasFrontend) Run(addr string) error {
	return http.ListenAndServe(addr, a.Router)
}

func (a *DitasFrontend) initializeRoutes() {
	a.Router.HandleFunc("/deployment", a.createDep).Methods("POST")
	//a.Router.HandleFunc("/deployment/{depId}/{infraId}", a.DefaultFrontend.deleteInfra).Methods("DELETE")
}

// swagger:operation POST /deployment deployment createDeployment
//
// Creates a DITAS deployment with the infrastructures passed as parameter.
//
// Creates a Kubernetes installation on each infrastructure and then deploys a VDC on the first one
// based on the blueprint passed as parameter.
//
// ---
// consumes:
// - application/json
//
// produces:
// - application/json
// - text/plain
//
// parameters:
// - name: request
//   in: body
//   description: The request object is composed of an abstract blueprint and a list of resources to use to deploy VDCs
//   required: true
//   schema:
//     $ref: "#/definitions/CreateDeploymentRequest"
//
// responses:
//   200:
//     description: OK
//   400:
//     description: Bad request
//   500:
//     description: Internal error
func (a *DitasFrontend) createDep(w http.ResponseWriter, r *http.Request) {
	var request CreateDeploymentRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		log.WithError(err).Error("Error deserializing deployment")
		restfrontend.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid payload: %s", err.Error()))
		return
	}
	defer r.Body.Close()

	err := a.VDCManagerInstance.DeployBlueprint(request)

	if err != nil {
		log.WithError(err).Error("Error deploying blueprint")
		restfrontend.RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deploying blueprint: %s", err.Error()))
		return
	}

}
