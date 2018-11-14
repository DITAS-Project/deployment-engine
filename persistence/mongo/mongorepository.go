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

package mongo

import (
	"deployment-engine/model"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	MongoDBURLName    = "mongodb.url"
	MongoDBURLDefault = "mongodb://localhost:27017"
)

type MongoRepository struct {
	collection *mgo.Collection
}

func CreateRepository() (*MongoRepository, error) {
	viper.SetDefault(MongoDBURLName, MongoDBURLDefault)
	mongoConnectionURL := viper.GetString(MongoDBURLName)
	client, err := mgo.Dial(mongoConnectionURL)
	if err == nil {
		db := client.DB("deployment_engine")
		if db != nil {
			return &MongoRepository{
				collection: db.C("deployments"),
			}, nil
		}
	} else {
		log.WithError(err).Errorf("Error connecting to MongoDB server %s", mongoConnectionURL)
	}
	return nil, err
}

//Save a new deployment information and return the updated deployment from the underlying database
func (r *MongoRepository) Save(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	err := r.collection.Insert(&deployment)
	return deployment, err
}

//Get the deployment information given its ID
func (r *MongoRepository) Get(deploymentID string) (model.DeploymentInfo, error) {
	var result model.DeploymentInfo
	err := r.collection.FindId(deploymentID).One(&result)
	return result, err
}

//List all available deployments
func (r *MongoRepository) List() ([]model.DeploymentInfo, error) {
	var result []model.DeploymentInfo
	err := r.collection.Find(bson.M{}).All(&result)
	return result, err
}

//Update a deployment replacing its old contents
func (r *MongoRepository) Update(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	err := r.collection.UpdateId(deployment.ID, deployment)
	return deployment, err
}

//Delete a deployment given its ID
func (r *MongoRepository) Delete(deploymentID string) error {
	return r.collection.RemoveId(deploymentID)
}
