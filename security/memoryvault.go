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

package security

import (
	"fmt"

	"github.com/google/uuid"
)

// MemoryVault implements a basic vault in memory.
// WARNING: It stores credentials and private keys in memory and UNENCRYPTED which is a VERY bad practice and it's strongly discouraged to be used in production. Use it for development and test but change later for a secure vault implementation.
type MemoryVault struct {
	vault map[string]map[string]string
}

// CreateMemoryVault creates a new vault in memory
func CreateMemoryVault() *MemoryVault {
	return &MemoryVault{
		vault: make(map[string]map[string]string),
	}
}

func (v *MemoryVault) getSecret(secretID string) (map[string]string, error) {
	secret, ok := v.vault[secretID]
	if !ok {
		return secret, fmt.Errorf("Secret %s not found in vault", secretID)
	}
	return secret, nil
}

// AddSecret adds a new secret to the vault, returning its identifier
func (v *MemoryVault) AddSecret(secret map[string]string) (string, error) {
	id := uuid.New().String()
	v.vault[id] = secret
	return id, nil
}

// UpdateSecret updates a secret replacing its content if it exists or returning an error if not
func (v *MemoryVault) UpdateSecret(secretID string, secret map[string]string) error {
	secret, err := v.getSecret(secretID)
	if err == nil {
		v.vault[secretID] = secret
	}
	return err
}

// GetSecret gets a secret information given its identifier
func (v *MemoryVault) GetSecret(secretID string) (map[string]string, error) {
	return v.getSecret(secretID)
}

// DeleteSecret deletes a secret from the vault given its identifier
func (v *MemoryVault) DeleteSecret(secretID string) error {
	_, err := v.getSecret(secretID)
	if err == nil {
		delete(v.vault, secretID)
	}
	return err
}
