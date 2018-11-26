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
	"deployment-engine/persistence/mongo"
	"deployment-engine/provision"
	"deployment-engine/provision/ansible"
	"deployment-engine/restfrontend"
	"encoding/json"
	"net/http"

	"github.com/DITAS-Project/blueprint-go"

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
	repository, err := mongo.CreateRepository()
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

func (a *DitasFrontend) createDep(w http.ResponseWriter, r *http.Request) {
	var blueprint blueprint.BlueprintType
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&blueprint); err != nil {
		log.WithError(err).Error("Error deserializing deployment")
		restfrontend.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid payload: %s", err.Error()))
		return
	}
	defer r.Body.Close()

	err := a.VDCManagerInstance.DeployBlueprint(blueprint)	

	if err != nil {
		log.WithError(err).Error("Error deploying blueprint")
		restfrontend.RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deploying blueprint: %s", err.Error()))
		return
	}

}
