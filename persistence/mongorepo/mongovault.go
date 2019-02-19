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

func (v *MongoRepository) Encrypt(secret interface{}) (Secret, error) {

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

func (v *MongoRepository) Decrypt(secret Secret, result interface{}) error {
	plaintext, err := v.cipher.Open(nil, secret.Nonce, secret.Content, nil)
	if err != nil {
		return err
	}

	return json.Unmarshal(plaintext, result)
}

// AddSecret adds a new secret to the vault, returning its identifier
func (v *MongoRepository) AddSecret(secret interface{}) (string, error) {
	encrypted, err := v.Encrypt(secret)
	if err != nil {
		return "", err
	}

	return encrypted.ID, v.insert(secretsCollection, encrypted)
}

// UpdateSecret updates a secret replacing its content if it exists or returning an error if not
func (v *MongoRepository) UpdateSecret(secretID string, secret interface{}) error {
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
	return v.replace(secretsCollection, existing.ID, newSecret, &existing)
}

// GetSecret gets a secret information given its identifier
func (v *MongoRepository) GetSecret(secretID string, secretOut interface{}) error {
	var existing Secret
	err := v.get(secretsCollection, secretID, &existing)
	if err != nil {
		return err
	}

	return v.Decrypt(existing, secretOut)

}

// DeleteSecret deletes a secret from the vault given its identifier
func (v *MongoRepository) DeleteSecret(secretID string) error {
	return v.delete(secretsCollection, secretID)
}
