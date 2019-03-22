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
package restfrontend

import (
	"deployment-engine/infrastructure"
	"deployment-engine/model"
	"deployment-engine/persistence"
	"deployment-engine/provision"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type App struct {
	Router                *mux.Router
	DeploymentController  *infrastructure.Deployer
	ProvisionerController *provision.ProvisionerController
	Vault                 persistence.Vault
}

func New(repository persistence.DeploymentRepository, vault persistence.Vault, provisioner model.Provisioner) *App {
	result := App{
		Router: mux.NewRouter(),
		DeploymentController: &infrastructure.Deployer{
			Repository: repository,
			Vault:      vault,
		},
		ProvisionerController: &provision.ProvisionerController{
			Repository:  repository,
			Provisioner: provisioner,
		},
		Vault: vault,
	}
	result.initializeRoutes()
	return &result
}

func (a App) Run(addr string) error {
	return http.ListenAndServe(addr, a.Router)
}

func (a *App) initializeRoutes() {
	a.Router.HandleFunc("/deployment", a.CreateDep).Methods("POST")
	a.Router.HandleFunc("/deployment/{depId}", a.DeleteDeployment).Methods("DELETE")
	a.Router.HandleFunc("/deployment/{depId}/{infraId}", a.DeleteInfra).Methods("DELETE")
	a.Router.HandleFunc("/deployment/{depId}/{infraId}/{product}", a.DeployProduct).Methods("PUT")
	a.Router.HandleFunc("/secrets", a.CreateSecret).Methods("POST")
}

func (a *App) GetQueryParam(key string, r *http.Request) (string, bool) {
	values := mux.Vars(r)
	if values != nil {
		value, ok := values[key]
		return value, ok
	}

	return "", false
}

func (a *App) ReadBody(r *http.Request, result interface{}) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(result); err != nil {
		log.WithError(err).Error("Error deserializing deployment")
		return fmt.Errorf("Invalid payload: %s", err.Error())
	}
	return nil
}

func (a *App) CreateDep(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var deployment model.Deployment
	if err := a.ReadBody(r, &deployment); err != nil {
		RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := a.DeploymentController.CreateDeployment(deployment)

	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	RespondWithJSON(w, http.StatusCreated, result)

	//RespondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func (a *App) DeleteDeployment(w http.ResponseWriter, r *http.Request) {
	depId, ok := a.GetQueryParam("depId", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find deployment ID parameter")
		return
	}

	err := a.DeploymentController.DeleteDeployment(depId)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deleting deployment: %s", err.Error()))
		return
	}

	Respond(w, http.StatusNoContent, []byte{}, "plain/text")
	return
}

func (a *App) DeleteInfra(w http.ResponseWriter, r *http.Request) {
	depId, ok := a.GetQueryParam("depId", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find deployment ID parameter")
		return
	}

	infraId, ok := a.GetQueryParam("infraId", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find infrastructure ID parameter")
		return
	}

	dep, err := a.DeploymentController.DeleteInfrastructure(depId, infraId)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deleting infrastructure: %s", err.Error()))
		return
	}

	RespondWithJSON(w, http.StatusOK, dep)
	return
}

func (a *App) DeployProduct(w http.ResponseWriter, r *http.Request) {
	depId, ok := a.GetQueryParam("depId", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find deployment ID parameter")
		return
	}

	infraId, ok := a.GetQueryParam("infraId", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find infrastructure ID parameter")
		return
	}

	product, ok := a.GetQueryParam("product", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find product parameter")
		return
	}

	params := r.URL.Query()

	deployment, err := a.ProvisionerController.Provision(depId, infraId, product, params)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deploying product: %s", err.Error()))
		return
	}

	RespondWithJSON(w, http.StatusOK, deployment)
	return
}

func (a *App) CreateSecret(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var secret model.Secret
	if err := a.ReadBody(r, &secret); err != nil {
		RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	secretID, err := a.Vault.AddSecret(secret)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	Respond(w, http.StatusCreated, []byte(secretID), "plain/text")
	return

}

func RespondWithError(w http.ResponseWriter, code int, message string) {
	RespondWithJSON(w, code, map[string]string{"error": message})
}

func Respond(w http.ResponseWriter, code int, payload []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(code)
	w.Write(payload)
}

func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	Respond(w, code, response, "application/json")
}
