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
	"time"

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
	return m.database.Collection(collection).FindOneAndReplace(context.Background(), bson.M{"_id": id}, object, options.FindOneAndReplace().SetReturnDocument(options.After)).Decode(result)
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

//AddInfrastructure adds a new infrastructure to an existing deployment
func (m *MongoRepository) AddInfrastructure(infra model.InfrastructureDeploymentInfo) (model.InfrastructureDeploymentInfo, error) {
	if infra.ID == "" {
		infra.ID = uuid.New().String()
	}
	if infra.Products == nil {
		infra.Products = make(map[string]interface{})
	}
	infra.CreationTime = time.Now()
	infra.UpdateTime = time.Now()
	return infra, m.insert(deploymentCollection, infra)
}

//UpdateInfrastructure updates as a whole an existing infrastructure in a deployment
func (m *MongoRepository) UpdateInfrastructure(infra model.InfrastructureDeploymentInfo) (model.InfrastructureDeploymentInfo, error) {
	var updated model.InfrastructureDeploymentInfo
	infra.UpdateTime = time.Now()
	err := m.replace(deploymentCollection, infra.ID, infra, &updated)
	return updated, err
}

//FindInfrastructure finds an infrastructure in a deployment given their identifiers
func (m *MongoRepository) FindInfrastructure(infraID string) (model.InfrastructureDeploymentInfo, error) {
	var result model.InfrastructureDeploymentInfo
	err := m.get(deploymentCollection, infraID, &result)
	return result, err
}

//DeleteInfrastructure will delete an infrastructure from a deployment given their identifiers
func (m *MongoRepository) DeleteInfrastructure(infraID string) (model.InfrastructureDeploymentInfo, error) {
	result, err := m.FindInfrastructure(infraID)
	if err != nil {
		return result, err
	}
	err = m.delete(deploymentCollection, infraID)
	return result, err
}

// UpdateInfrastructureStatus updates the status of a infrastructure in a deployment
func (m *MongoRepository) UpdateInfrastructureStatus(infrastructureID, status string) (model.InfrastructureDeploymentInfo, error) {
	var result model.InfrastructureDeploymentInfo
	err := m.update(deploymentCollection, infrastructureID, bson.M{
		"$set": bson.M{
			"status":     status,
			"updatetime": time.Now(),
		},
	}, &result)
	return result, err
}

// AddProductToInfrastructure adds a new product to an existing infrastructure
func (m *MongoRepository) AddProductToInfrastructure(infrastructureID, product string, config interface{}) (model.InfrastructureDeploymentInfo, error) {
	var updated model.InfrastructureDeploymentInfo
	err := m.update(deploymentCollection, infrastructureID, bson.M{
		"$set": bson.M{
			fmt.Sprintf("products.%s", product): config,
			"updatetime":                        time.Now(),
		},
	}, &updated)
	return updated, err
}
