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
	"fmt"

	"github.com/google/uuid"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	MongoDBURLName    = "mongodb.url"
	MongoDBURLDefault = "mongodb://localhost:27017"

	deploymentCollection = "deployments"
	productCollection    = "products"
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

func (m *MongoRepositoryNative) insert(collection string, object interface{}) error {
	_, err := m.database.Collection(collection).InsertOne(context.Background(), object)
	return err
}

func (m *MongoRepositoryNative) replace(collection string, id string, object interface{}, result interface{}) error {
	return m.database.Collection(collection).FindOneAndReplace(context.Background(), bson.M{"_id": id}, object).Decode(result)
}

func (m *MongoRepositoryNative) update(collection string, id string, update bson.M, updated interface{}) error {
	result := m.database.Collection(collection).FindOneAndUpdate(context.Background(), bson.M{"_id": id}, update)
	return result.Decode(updated)
}

func (m *MongoRepositoryNative) get(collection string, id string, result interface{}) error {
	return m.database.Collection(collection).FindOne(context.Background(), bson.M{"_id": id}).Decode(result)
}

func (m *MongoRepositoryNative) list(collection string, appender func(interface{}), current interface{}) error {
	cursor, err := m.database.Collection(collection).Find(context.Background(), bson.M{})

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

func (m *MongoRepositoryNative) delete(collection, ID string) error {
	result, err := m.database.Collection(collection).DeleteOne(context.Background(), bson.M{"_id": ID})
	if err != nil {
		return err
	}

	if result.DeletedCount < 1 {
		return fmt.Errorf("Can't find %s with id %s", collection, ID)
	}

	return nil
}

//SaveDeployment a new deployment information and return the updated deployment from the underlying database
func (m *MongoRepositoryNative) SaveDeployment(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	deployment.ID = uuid.New().String()
	err := m.insert(deploymentCollection, deployment)
	return deployment, err
}

//UpdateDeployment a deployment replacing its old contents
func (m *MongoRepositoryNative) UpdateDeployment(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	var dep model.DeploymentInfo
	dep.ID = deployment.ID
	err := m.replace(deploymentCollection, deployment.ID, deployment, &dep)
	return dep, err
}

//GetDeployment the deployment information given its ID
func (m *MongoRepositoryNative) GetDeployment(deploymentID string) (model.DeploymentInfo, error) {
	var deployment model.DeploymentInfo
	err := m.get(deploymentCollection, deploymentID, &deployment)
	return deployment, err
}

//ListDeployment all available deployments
func (m *MongoRepositoryNative) ListDeployment() ([]model.DeploymentInfo, error) {
	result := make([]model.DeploymentInfo, 0)
	var current model.DeploymentInfo

	err := m.list(deploymentCollection, func(val interface{}) {
		result = append(result, *val.(*model.DeploymentInfo))
	}, &current)

	return result, err
}

//DeleteDeployment a deployment given its ID
func (m *MongoRepositoryNative) DeleteDeployment(deploymentID string) error {
	return m.delete(deploymentCollection, deploymentID)
}

// UpdateDeploymentStatus updates the status of a deployment
func (m *MongoRepositoryNative) UpdateDeploymentStatus(deploymentID, status string) error {
	var updated model.DeploymentInfo
	return m.update(deploymentCollection, deploymentID, bson.M{
		"$set": bson.M{
			"status": status,
		},
	}, &updated)
}

// UpdateInfrastructureStatus updates the status of a infrastructure in a deployment
func (m *MongoRepositoryNative) UpdateInfrastructureStatus(deploymentID, infrastructureID, status string) error {
	var updated model.DeploymentInfo
	return m.database.Collection(deploymentCollection).FindOneAndUpdate(
		context.Background(),
		bson.M{
			"_id":                deploymentID,
			"infrastructures.id": infrastructureID,
		},
		bson.M{
			"$set": bson.M{
				"infrastructures.$.status": status,
			},
		}).Decode(&updated)
}

//SaveProduct a new product information and return the created product from the underlying database
func (m *MongoRepositoryNative) SaveProduct(product model.Product) (model.Product, error) {
	product.ID = uuid.New().String()
	return product, m.insert(productCollection, product)
}

//GetProduct the product information given its ID
func (m *MongoRepositoryNative) GetProduct(productID string) (model.Product, error) {
	var result model.Product
	err := m.get(productCollection, productID, &result)
	return result, err
}

//ListProducts all available products
func (m *MongoRepositoryNative) ListProducts() ([]model.Product, error) {
	products := make([]model.Product, 0)
	var product model.Product
	err := m.list(productCollection, func(prod interface{}) {
		products = append(products, *prod.(*model.Product))
	}, &product)
	return products, err
}

//UpdateProduct a product replacing its old contents
func (m *MongoRepositoryNative) UpdateProduct(product model.Product) (model.Product, error) {
	var result model.Product
	err := m.replace(productCollection, product.ID, product, &result)
	return result, err
}

//DeleteProduct a product given its ID
func (m *MongoRepositoryNative) DeleteProduct(productID string) error {
	return m.delete(productCollection, productID)
}
