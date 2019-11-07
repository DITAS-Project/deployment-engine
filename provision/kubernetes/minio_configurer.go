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
	"fmt"

	"github.com/sethvargo/go-password/password"
)

const (
	MinioType               = "minio"
	MinioAccessKeySecretKey = "minio-access-key"
	MinioSecretKeySecretKey = "minio-secret-key"
)

type MinioConfigurer struct {
}

func (c MinioConfigurer) GetSecret(dsID string, args model.Parameters) (SecretData, model.Parameters, error) {

	var secretData SecretData

	accessKey, err := password.Generate(10, 4, 0, false, false)
	if err != nil {
		return secretData, nil, fmt.Errorf("Error generating access key: %w", err)
	}

	secretKey, err := password.Generate(10, 4, 0, false, false)
	if err != nil {
		return secretData, nil, fmt.Errorf("Error generating secret key: %w", err)
	}

	return SecretData{
		SecretID: fmt.Sprintf("%s-secret", dsID),
		Data: map[string]string{
			MinioAccessKeySecretKey: accessKey,
			MinioSecretKeySecretKey: secretKey,
		},
	}, nil, err
}

func (c MinioConfigurer) GetDeploymentConfiguration(dsID string, args model.Parameters, secret SecretData) (ImageSet, model.Parameters, error) {
	envSecrets := []EnvSecret{
		EnvSecret{
			EnvName:  "MINIO_ACCESS_KEY",
			SecretID: secret.SecretID,
			Key:      MinioAccessKeySecretKey,
		},
		EnvSecret{
			EnvName:  "MINIO_SECRET_KEY",
			SecretID: secret.SecretID,
			Key:      MinioSecretKeySecretKey,
		},
	}

	return ImageSet{
		"minio": ImageInfo{
			InternalPort: 9000,
			Image:        "minio/minio",
			Args:         []string{"server", "/data"},
			Secrets:      envSecrets,
		},
	}, nil, nil

}
