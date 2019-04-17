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

	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	MongoDBURLName    = "mongodb.url"
	MongoDBURLDefault = "mongodb://localhost:27017"

	VaultPassphraseName = "mongodb.vault.passphrase"

	deploymentCollection = "deployments"
)

type MongoRepository struct {
	client                      *mongo.Client
	database                    *mongo.Database
	cipher                      cipher.AEAD
	defaultFindAndUpdateOptions *options.FindOneAndUpdateOptions
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
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoConnectionURL), nil)
	if err != nil {
		log.WithError(err).Errorf("Error connecting to MongoDB server %s", mongoConnectionURL)
		return nil, err
	}

	repo := MongoRepository{
		client:                      client,
		defaultFindAndUpdateOptions: options.FindOneAndUpdate().SetReturnDocument(options.After),
	}
	repo.SetDatabase("deployment_engine")

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

func (m *MongoRepository) SetDatabase(db string) {
	m.database = m.client.Database(db)
}

func (m *MongoRepository) ClearDatabase() error {
	return m.database.Drop(context.Background())
}

func (m *MongoRepository) insert(collection string, object interface{}) error {
	_, err := m.database.Collection(collection).InsertOne(context.Background(), object)
	return err
}

func (m *MongoRepository) replace(collection string, id string, object interface{}, result interface{}) error {
	return m.database.Collection(collection).FindOneAndReplace(context.Background(), bson.M{"_id": id}, object).Decode(result)
}

func (m *MongoRepository) update(collection string, id string, update bson.M, updated interface{}) error {
	result := m.database.Collection(collection).FindOneAndUpdate(context.Background(), bson.M{"_id": id}, update, m.defaultFindAndUpdateOptions)
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
	if deployment.Infrastructures == nil {
		deployment.Infrastructures = make(map[string]model.InfrastructureDeploymentInfo)
	}
	err := m.insert(deploymentCollection, deployment)
	return deployment, err
}

//UpdateDeployment a deployment replacing its old contents
func (m *MongoRepository) UpdateDeployment(deployment model.DeploymentInfo) (model.DeploymentInfo, error) {
	var dep model.DeploymentInfo
	dep.ID = deployment.ID
	if deployment.Infrastructures == nil {
		deployment.Infrastructures = make(map[string]model.InfrastructureDeploymentInfo)
	}
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

//AddInfrastructure adds a new infrastructure to an existing deployment
func (m *MongoRepository) AddInfrastructure(deploymentID string, infra model.InfrastructureDeploymentInfo) (model.DeploymentInfo, error) {
	return m.UpdateInfrastructure(deploymentID, infra)
}

//UpdateInfrastructure updates as a whole an existing infrastructure in a deployment
func (m *MongoRepository) UpdateInfrastructure(deploymentID string, infra model.InfrastructureDeploymentInfo) (model.DeploymentInfo, error) {
	var updated model.DeploymentInfo
	if infra.Products == nil {
		infra.Products = make(map[string]interface{})
	}
	err := m.update(deploymentCollection, deploymentID, bson.M{
		"$set": bson.M{
			fmt.Sprintf("infrastructures.%s", infra.ID): infra,
		},
	}, &updated)
	return updated, err
}

//FindInfrastructure finds an infrastructure in a deployment given their identifiers
func (m *MongoRepository) FindInfrastructure(depoloymentID, infraID string) (model.InfrastructureDeploymentInfo, error) {
	var dep model.DeploymentInfo
	err := m.database.Collection(deploymentCollection).FindOne(
		context.Background(), bson.M{
			"_id": depoloymentID,
		}, &options.FindOneOptions{
			Projection: bson.M{
				fmt.Sprintf("infrastructures.%s", infraID): 1,
			},
		}).Decode(&dep)

	if err != nil {
		return model.InfrastructureDeploymentInfo{}, err
	}

	if dep.Infrastructures == nil {
		return model.InfrastructureDeploymentInfo{}, fmt.Errorf("Can't find infrastructure %s in deployment %s", depoloymentID, infraID)
	}

	infra, ok := dep.Infrastructures[infraID]
	if !ok {
		return infra, fmt.Errorf("Can't find infrastructure %s in deployment %s", depoloymentID, infraID)
	}

	return infra, nil
}

//DeleteInfrastructure will delete an infrastructure from a deployment given their identifiers
func (m *MongoRepository) DeleteInfrastructure(deploymentID, infraID string) (model.DeploymentInfo, error) {
	var dep model.DeploymentInfo
	err := m.update(deploymentCollection, deploymentID, bson.M{
		"$unset": bson.M{
			fmt.Sprintf("infrastructures.%s", infraID): "",
		},
	}, &dep)

	return dep, err
}

// UpdateDeploymentStatus updates the status of a deployment
func (m *MongoRepository) UpdateDeploymentStatus(deploymentID, status string) (model.DeploymentInfo, error) {
	var updated model.DeploymentInfo
	err := m.update(deploymentCollection, deploymentID, bson.M{
		"$set": bson.M{
			"status": status,
		},
	}, &updated)
	return updated, err
}

// UpdateInfrastructureStatus updates the status of a infrastructure in a deployment
func (m *MongoRepository) UpdateInfrastructureStatus(deploymentID, infrastructureID, status string) (model.DeploymentInfo, error) {
	var updated model.DeploymentInfo
	err := m.database.Collection(deploymentCollection).FindOneAndUpdate(
		context.Background(),
		bson.M{
			"_id": deploymentID,
			fmt.Sprintf("infrastructures.%s", infrastructureID): bson.M{"$exists": true},
		},
		bson.M{
			"$set": bson.M{
				fmt.Sprintf("infrastructures.%s.status", infrastructureID): status,
			},
		}).Decode(&updated)
	return updated, err
}

// AddProductToInfrastructure adds a new product to an existing infrastructure
func (m *MongoRepository) AddProductToInfrastructure(deploymentID, infrastructureID, product string, config interface{}) (model.DeploymentInfo, error) {
	var updated model.DeploymentInfo
	err := m.database.Collection(deploymentCollection).FindOneAndUpdate(
		context.Background(),
		bson.M{
			"_id": deploymentID,
			fmt.Sprintf("infrastructures.%s", infrastructureID): bson.M{"$exists": true},
		},
		bson.M{
			"$set": bson.M{
				fmt.Sprintf("infrastructures.%s.products.%s", infrastructureID, product): config,
			},
		}, m.defaultFindAndUpdateOptions).Decode(&updated)
	return updated, err
}
