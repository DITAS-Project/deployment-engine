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

package kubernetes

import (
	"deployment-engine/model"
	"errors"
	"fmt"

	"github.com/sethvargo/go-password/password"
)

const (
	MySQLType                  = "mysql"
	MySQLUsernameProperty      = "username"
	MySQLDatabaseProperty      = "database"
	MySQLRootPasswordSecretKey = "mysql-root-pw"
	MySQLUserPasswordSecretKey = "mysql-user-pw"
)

type MySQLConfigurer struct {
}

func (c MySQLConfigurer) GetSecret(dsID string, args model.Parameters) (SecretData, model.Parameters, error) {

	result := make(model.Parameters)

	rootPassword, err := password.Generate(10, 3, 2, false, false)

	secretData := SecretData{
		SecretID: fmt.Sprintf("%s-secret", dsID),
		Data: map[string]string{
			MySQLRootPasswordSecretKey: rootPassword,
		},
	}

	username, ok := args.GetString("username")
	if ok {
		result[MySQLUsernameProperty] = username
		database, ok := args.GetString("database")
		if !ok {
			return secretData, result, errors.New("Database query parameter is mandatory when username is specified")
		}

		result[MySQLDatabaseProperty] = database

		userPassword, ok := args.GetString("user_password")
		if !ok {
			userPassword, err = password.Generate(10, 3, 2, false, false)
			if err != nil {
				return secretData, result, fmt.Errorf("No password specified for user %s and an error occured when trying to generate a new random one: %w", username, err)
			}
		}

		secretData.Data[MySQLUserPasswordSecretKey] = userPassword
	}

	return secretData, result, nil
}

func (c MySQLConfigurer) GetDeploymentConfiguration(dsID string, args model.Parameters, secret SecretData) (ImageSet, model.Parameters, error) {

	imageInfo := ImageInfo{
		InternalPort: 3306,
		Image:        "mysql/mysql-server",
	}

	envSecrets := []EnvSecret{
		EnvSecret{
			EnvName:  "MYSQL_ROOT_PASSWORD",
			SecretID: secret.SecretID,
			Key:      MySQLRootPasswordSecretKey,
		},
	}

	imageEnv := make(map[string]string)
	username, ok := args.GetString("username")
	if ok {
		imageEnv["MYSQL_USER"] = username
		databaseName, ok := args.GetString("database")
		if !ok {
			return nil, nil, errors.New("Database query parameter is mandatory when username is specified")
		}
		imageEnv["MYSQL_DATABASE"] = databaseName

		envSecrets = append(envSecrets, EnvSecret{
			EnvName:  "MYSQL_PASSWORD",
			SecretID: secret.SecretID,
			Key:      MySQLUserPasswordSecretKey,
		})
	}

	imageInfo.Environment = imageEnv

	imageInfo.Secrets = envSecrets

	return ImageSet{
		"mysql": imageInfo,
	}, nil, nil
}
