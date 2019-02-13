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

package mongorepo

import (
	"context"
	"deployment-engine/model"
	"errors"

	"github.com/google/uuid"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type MongoRepositoryNative struct {
	database *mongo.Database
}

func CreateRepositoryNative() (*MongoRepositoryNative, error) {
	viper.SetDefault(MongoDBURLName, MongoDBURLDefault)
	mongoConnectionURL := viper.GetString(MongoDBURLName)
	client, err := mongo.Connect(context.Background(), mongoConnectionURL, nil)
	if err == nil {
		db := client.Database("deployment_engine")
		return &MongoRepositoryNative{
			database: db,
		}, nil
	}

	log.WithError(err).Errorf("Error connecting to MongoDB server %s", mongoConnectionURL)

	return nil, err
}

func (m *MongoRepositoryNative) insert(collection *mongo.Collection, object interface{}) error {
	if collection == nil {
		return errors.New("Can't find collection")
	}

	_, err := collection.InsertOne(context.Background(), object)
	if err != nil {
		return err
	}

	return nil
}

func (m *MongoRepositoryNative) update(collection *mongo.Collection, id string, object interface{}, result interface{}) error {
	if collection == nil {
		return errors.New("Can't find collection")
	}

	return collection.FindOneAndReplace(context.Background(), bson.M{"_id": id}, object).Decode(result)
}

func (m *MongoRepositoryNative) get(collection *mongo.Collection, id string, result interface{}) error {
	return collection.FindOne(context.Background(), bson.M{"_id": id}).Decode(result)
}

func (m *MongoRepositoryNative) list(collection *mongo.Collection, appender func(interface{}), current interface{}) error {
	cursor, err := collection.Find(context.Background(), bson.M{})

	defer cursor.Close(context.Background())

	if err != nil {
		return err
	}

	for cursor.Next(context.Background()) {
		err = cursor.Decode(current)

		if err != nil {
			log.WithError(err).Error("Error decoding object")
		} else {
			appender(current)
		}

	}

	return nil
}

func (m *MongoRepositoryNative) Save(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	deployment.ID = uuid.New().String()
	err := m.insert(m.database.Collection("deployment"), deployment)
	return deployment, err
}

//Update a deployment replacing its old contents
func (m *MongoRepositoryNative) Update(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	var dep model.DeploymentInfo
	dep.ID = deployment.ID
	err := m.update(m.database.Collection("deployment"), deployment.ID, deployment, &dep)
	return dep, err
}

//Get the deployment information given its ID
func (m *MongoRepositoryNative) Get(deploymentID string) (model.DeploymentInfo, error) {
	var deployment model.DeploymentInfo
	err := m.get(m.database.Collection("deployment"), deploymentID, &deployment)
	return deployment, err
}

//List all available deployments
func (m *MongoRepositoryNative) List() ([]model.DeploymentInfo, error) {
	result := make([]model.DeploymentInfo, 0)
	var current model.DeploymentInfo

	err := m.list(m.database.Collection("deployment"), func(val interface{}) {
		result = append(result, *val.(*model.DeploymentInfo))
	}, &current)

	return result, err
}
