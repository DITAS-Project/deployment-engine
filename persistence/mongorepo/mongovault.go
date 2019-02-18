package mongorepo

import (
	"crypto/rand"
	"encoding/json"
	"io"

	"github.com/google/uuid"
)

const secretsCollection = "secrets"

type Secret struct {
	ID      string `json:"id" bson:"_id"`
	Content []byte `json:"content"`
	Nonce   []byte `json:"nonce"`
}

func (v *MongoRepositoryNative) Encrypt(secret map[string]string) (Secret, error) {

	result := Secret{
		ID: uuid.New().String(),
	}

	jsonValue, err := json.Marshal(secret)
	if err != nil {
		return result, err
	}

	nonce := make([]byte, v.cipher.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return result, err
	}

	ciphertext := v.cipher.Seal(nil, nonce, jsonValue, nil)

	result.Nonce = nonce
	result.Content = ciphertext

	return result, nil
}

func (v *MongoRepositoryNative) Decrypt(secret Secret) (map[string]string, error) {
	plaintext, err := v.cipher.Open(nil, secret.Nonce, secret.Content, nil)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	err = json.Unmarshal(plaintext, &result)
	return result, err
}

// AddSecret adds a new secret to the vault, returning its identifier
func (v *MongoRepositoryNative) AddSecret(secret map[string]string) (string, error) {
	encrypted, err := v.Encrypt(secret)
	if err != nil {
		return "", err
	}

	err = v.insert(secretsCollection, encrypted)
	return encrypted.ID, err
}

// UpdateSecret updates a secret replacing its content if it exists or returning an error if not
func (v *MongoRepositoryNative) UpdateSecret(secretID string, secret map[string]string) error {
	var existing Secret
	err := v.get(secretsCollection, secretID, &existing)
	if err != nil {
		return err
	}

	newSecret, err := v.Encrypt(secret)
	if err != nil {
		return err
	}

	newSecret.ID = existing.ID
	err = v.replace(secretsCollection, existing.ID, newSecret, &existing)
	return err
}

// GetSecret gets a secret information given its identifier
func (v *MongoRepositoryNative) GetSecret(secretID string) (map[string]string, error) {
	var existing Secret
	err := v.get(secretsCollection, secretID, &existing)
	if err != nil {
		return nil, err
	}
	return v.Decrypt(existing)

}

// DeleteSecret deletes a secret from the vault given its identifier
func (v *MongoRepositoryNative) DeleteSecret(secretID string) error {
	return v.delete(secretsCollection, secretID)
}
