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
	}
	result.initializeRoutes()
	return &result
}

func (a App) Run(addr string) error {
	return http.ListenAndServe(addr, a.Router)
}

func (a *App) initializeRoutes() {
	a.Router.HandleFunc("/deployment", a.CreateDep).Methods("POST")
	a.Router.HandleFunc("/deployment/{depId}/{infraId}", a.DeleteInfra).Methods("DELETE")
	a.Router.HandleFunc("/deployment/{depId}/{infraId}/{product}", a.DeployProduct).Methods("PUT")
	/*a.Router.HandleFunc("/deployment", a.getAllDeps).Methods("GET")
	a.Router.HandleFunc("/deployment/{id}", a.getDep).Methods("GET")
	a.Router.HandleFunc("/deployment/{id}", a.deleteDep).Methods("DELETE")*/

}

func (a *App) GetQueryParam(key string, r *http.Request) (string, bool) {
	values := mux.Vars(r)
	if values != nil {
		ok, val := values[key]
		return ok, val
	}

	return "", false
}

func (a *App) CreateDep(w http.ResponseWriter, r *http.Request) {
	var deployment model.Deployment
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&deployment); err != nil {
		log.WithError(err).Error("Error deserializing deployment")
		RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid payload: %s", err.Error()))
		return
	}
	defer r.Body.Close()

	result, err := a.DeploymentController.CreateDeployment(deployment)

	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	RespondWithJSON(w, http.StatusCreated, result)

	//RespondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func (a *App) DeleteInfra(w http.ResponseWriter, r *http.Request) {
	depId, ok := a.GetQueryParam("depId", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find deployment ID parameter")
	}

	infraId, ok := a.GetQueryParam("infraId", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find infrastructure ID parameter")
	}

	dep, err := a.DeploymentController.DeleteInfrastructure(depId, infraId)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deleting infrastructure: %s", err.Error()))
	}

	RespondWithJSON(w, http.StatusOK, dep)
}

func (a *App) DeployProduct(w http.ResponseWriter, r *http.Request) {
	depId, ok := a.GetQueryParam("depId", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find deployment ID parameter")
	}

	infraId, ok := a.GetQueryParam("infraId", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find infrastructure ID parameter")
	}

	product, ok := a.GetQueryParam("product", r)
	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Can't find product parameter")
	}

	deployment, err := a.ProvisionerController.Provision(depId, infraId, product)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error deploying product: %s", err.Error()))
	}

	RespondWithJSON(w, http.StatusOK, deployment)
}

/*func (a *App) deployKubernetes(w http.ResponseWriter, r *http.Request) {

}

func (a *App) getAllDeps(w http.ResponseWriter, r *http.Request) {
	deps, err := a.Controller.GetAllDeps()
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondWithJSON(w, http.StatusOK, deps)
}

func (a *App) getDep(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	dep, err := a.Controller.GetDep(id)
	if err != nil {
		switch err {
		case mgo.ErrNotFound:
			RespondWithError(w, http.StatusNotFound, err.Error())
		default:
			RespondWithError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	RespondWithJSON(w, http.StatusOK, dep)
}

func (a *App) deleteDep(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	vars := mux.Vars(r)
	id, ok := vars["id"]

	if !ok {
		RespondWithError(w, http.StatusBadRequest, "Deployment id is mandatory")
		return
	}

	vdcID := getQueryParam("vdc", values)
	deleteDeployment, err := strconv.ParseBool(getQueryParam("deleteDeployment", values))
	if err != nil {
		fmt.Printf("deleteDeployment parameter not found or invalid. Assuming false")
		deleteDeployment = false
	}

	if err := a.Controller.DeleteVDC(id, vdcID, deleteDeployment); err != nil {
		RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	RespondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}*/

func RespondWithError(w http.ResponseWriter, code int, message string) {
	RespondWithJSON(w, code, map[string]string{"error": message})
}

func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
