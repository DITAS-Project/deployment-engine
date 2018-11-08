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

package main

import (
	"deployment-engine/src/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	//"os"
	"strconv"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/globalsign/mgo"
	"github.com/gorilla/mux"
	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/spf13/viper"
)

type App struct {
	Router     *mux.Router
	Controller *DeploymentEngineController
}

func (a *App) ReadConfig(home string) {
	configFile := fmt.Sprintf("%s/%s", home, utils.ConfigFileName)

	log.Infof("Reading configuration from file %s", configFile)

	viper.SetEnvPrefix(utils.ConfigPrefix)
	viper.AutomaticEnv()
	viper.SetDefault(utils.ElasticSearchURLName, utils.ElasticSearchURLDefault)
	viper.SetDefault(utils.MongoDBURLName, utils.MongoDBURLDefault)

	viper.SetConfigFile(configFile)
}

func (a *App) Initialize() {

	a.Router = mux.NewRouter()
	a.initializeRoutes()

	home, err := homedir.Dir()
	if err == nil {
		a.ReadConfig(home)
		mongoConnectionURL := viper.GetString(utils.MongoDBURLName)
		client, err := mgo.Dial(mongoConnectionURL)
		if err == nil {
			db := client.DB("deployment_engine")
			if db != nil {
				controller := DeploymentEngineController{
					collection: db.C("deployments"),
					homedir:    home,
				}
				a.Controller = &controller
			}
		} else {
			log.WithError(err).Errorf("Error connecting to MongoDB server %s", mongoConnectionURL)
		}
	} else {
		log.Errorf("Error getting home dir")
	}

}

func (a *App) Run(addr string) {
	log.Fatal(http.ListenAndServe(addr, a.Router))
}

func (a *App) initializeRoutes() {
	a.Router.HandleFunc("/deps", a.createDep).Methods("POST")
	a.Router.HandleFunc("/deps", a.getAllDeps).Methods("GET")
	a.Router.HandleFunc("/deps/{id}", a.getDep).Methods("GET")
	a.Router.HandleFunc("/deps/{id}", a.deleteDep).Methods("DELETE")
}

func getQueryParam(key string, values url.Values) string {
	val, ok := values[key]
	if ok && len(val) > 0 {
		return val[0]
	}
	return ""
}

func (a *App) createDep(w http.ResponseWriter, r *http.Request) {
	var bp blueprint.BlueprintType
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&bp); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	if err := a.Controller.CreateDep(bp); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusCreated, bp)
	//respondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func (a *App) getAllDeps(w http.ResponseWriter, r *http.Request) {
	deps, err := a.Controller.GetAllDeps()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, deps)
}

func (a *App) getDep(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	dep, err := a.Controller.GetDep(id)
	if err != nil {
		switch err {
		case mgo.ErrNotFound:
			respondWithError(w, http.StatusNotFound, err.Error())
		default:
			respondWithError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	respondWithJSON(w, http.StatusOK, dep)
}

func (a *App) deleteDep(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	vars := mux.Vars(r)
	id, ok := vars["id"]

	if !ok {
		respondWithError(w, http.StatusBadRequest, "Deployment id is mandatory")
		return
	}

	vdcID := getQueryParam("vdc", values)
	deleteDeployment, err := strconv.ParseBool(getQueryParam("deleteDeployment", values))
	if err != nil {
		fmt.Printf("deleteDeployment parameter not found or invalid. Assuming false")
		deleteDeployment = false
	}

	if err := a.Controller.DeleteVDC(id, vdcID, deleteDeployment); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
