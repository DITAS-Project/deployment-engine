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
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
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

	VaultPassphraseName = "vault.passphrase"

	deploymentCollection = "deployments"
	productCollection    = "products"
)

type MongoRepository struct {
	database *mongo.Database
	cipher   cipher.AEAD
}

func initializeCipher(passphrase string) (cipher.AEAD, error) {
	h := sha256.New()
	h.Write([]byte(passphrase))

	hash := h.Sum(nil)

	block, _ := aes.NewCipher(hash)

	return cipher.NewGCM(block)
}

func CreateRepositoryNative() (*MongoRepository, error) {
	viper.SetDefault(MongoDBURLName, MongoDBURLDefault)
	mongoConnectionURL := viper.GetString(MongoDBURLName)
	client, err := mongo.Connect(context.Background(), mongoConnectionURL, nil)
	if err != nil {
		log.WithError(err).Errorf("Error connecting to MongoDB server %s", mongoConnectionURL)
		return nil, err
	}

	db := client.Database("deployment_engine")
	repo := MongoRepository{
		database: db,
	}

	vaultPassphrase := viper.GetString(VaultPassphraseName)
	if vaultPassphrase != "" {
		cipher, err := initializeCipher(vaultPassphrase)
		if err != nil {
			log.WithError(err).Error("Passphrase defined for vault but an error was found initializing the cipher")
			return nil, err
		}
		repo.cipher = cipher
	}

	return &repo, err
}

func (m *MongoRepository) insert(collection string, object interface{}) error {
	_, err := m.database.Collection(collection).InsertOne(context.Background(), object)
	return err
}

func (m *MongoRepository) replace(collection string, id string, object interface{}, result interface{}) error {
	return m.database.Collection(collection).FindOneAndReplace(context.Background(), bson.M{"_id": id}, object).Decode(result)
}

func (m *MongoRepository) update(collection string, id string, update bson.M, updated interface{}) error {
	result := m.database.Collection(collection).FindOneAndUpdate(context.Background(), bson.M{"_id": id}, update)
	return result.Decode(updated)
}

func (m *MongoRepository) get(collection string, id string, result interface{}) error {
	return m.database.Collection(collection).FindOne(context.Background(), bson.M{"_id": id}).Decode(result)
}

func (m *MongoRepository) list(collection string, appender func(interface{}), current interface{}) error {
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

func (m *MongoRepository) delete(collection, ID string) error {
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
func (m *MongoRepository) SaveDeployment(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	deployment.ID = uuid.New().String()
	err := m.insert(deploymentCollection, deployment)
	return deployment, err
}

//UpdateDeployment a deployment replacing its old contents
func (m *MongoRepository) UpdateDeployment(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	var dep model.DeploymentInfo
	dep.ID = deployment.ID
	err := m.replace(deploymentCollection, deployment.ID, deployment, &dep)
	return dep, err
}

//GetDeployment the deployment information given its ID
func (m *MongoRepository) GetDeployment(deploymentID string) (model.DeploymentInfo, error) {
	var deployment model.DeploymentInfo
	err := m.get(deploymentCollection, deploymentID, &deployment)
	return deployment, err
}

//ListDeployment all available deployments
func (m *MongoRepository) ListDeployment() ([]model.DeploymentInfo, error) {
	result := make([]model.DeploymentInfo, 0)
	var current model.DeploymentInfo

	err := m.list(deploymentCollection, func(val interface{}) {
		result = append(result, *val.(*model.DeploymentInfo))
	}, &current)

	return result, err
}

//DeleteDeployment a deployment given its ID
func (m *MongoRepository) DeleteDeployment(deploymentID string) error {
	return m.delete(deploymentCollection, deploymentID)
}

// UpdateDeploymentStatus updates the status of a deployment
func (m *MongoRepository) UpdateDeploymentStatus(deploymentID, status string) error {
	var updated model.DeploymentInfo
	return m.update(deploymentCollection, deploymentID, bson.M{
		"$set": bson.M{
			"status": status,
		},
	}, &updated)
}

// UpdateInfrastructureStatus updates the status of a infrastructure in a deployment
func (m *MongoRepository) UpdateInfrastructureStatus(deploymentID, infrastructureID, status string) error {
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
func (m *MongoRepository) SaveProduct(product model.Product) (model.Product, error) {
	product.ID = uuid.New().String()
	return product, m.insert(productCollection, product)
}

//GetProduct the product information given its ID
func (m *MongoRepository) GetProduct(productID string) (model.Product, error) {
	var result model.Product
	err := m.get(productCollection, productID, &result)
	return result, err
}

//ListProducts all available products
func (m *MongoRepository) ListProducts() ([]model.Product, error) {
	products := make([]model.Product, 0)
	var product model.Product
	err := m.list(productCollection, func(prod interface{}) {
		products = append(products, *prod.(*model.Product))
	}, &product)
	return products, err
}

//UpdateProduct a product replacing its old contents
func (m *MongoRepository) UpdateProduct(product model.Product) (model.Product, error) {
	var result model.Product
	err := m.replace(productCollection, product.ID, product, &result)
	return result, err
}

//DeleteProduct a product given its ID
func (m *MongoRepository) DeleteProduct(productID string) error {
	return m.delete(productCollection, productID)
}
