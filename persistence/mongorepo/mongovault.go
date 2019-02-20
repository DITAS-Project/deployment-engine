package mongorepo

import (
	"crypto/rand"
	"deployment-engine/model"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/mongodb/mongo-go-driver/bson/primitive"

	"github.com/google/uuid"
)

const secretsCollection = "secrets"

type SecretEntry struct {
	ID     string       `json:"id" bson:"_id"`
	Secret model.Secret `json:"secret"`
	Nonce  []byte       `json:"nonce"`
}

func (v *MongoRepository) Encrypt(secret model.Secret) (SecretEntry, error) {
	if v.cipher == nil {
		return SecretEntry{}, errors.New("No cipher has been configured and the secret can't be saved")
	}

	result := SecretEntry{
		ID:     uuid.New().String(),
		Secret: secret,
	}

	jsonValue, err := json.Marshal(secret.Content)
	if err != nil {
		return result, err
	}

	nonce := make([]byte, v.cipher.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return result, err
	}

	ciphertext := v.cipher.Seal(nil, nonce, jsonValue, nil)

	result.Nonce = nonce
	result.Secret.Content = ciphertext

	return result, nil
}

func (v *MongoRepository) DeserializeSecret(entry *SecretEntry, plaintext []byte, secret interface{}) error {
	err := json.Unmarshal(plaintext, secret)
	if err != nil {
		return err
	}

	entry.Secret.Content = secret
	return nil
}

func (v *MongoRepository) Decrypt(secret *SecretEntry) error {

	if v.cipher == nil {
		return errors.New("No cipher has been configured and the secret can't be retrieved")
	}

	content, ok := secret.Secret.Content.(primitive.Binary)

	if !ok {
		return fmt.Errorf("Invalid content type in secret %s", secret.ID)
	}

	plaintext, err := v.cipher.Open(nil, secret.Nonce, content.Data, nil)
	if err != nil {
		return err
	}

	switch secret.Secret.Format {
	case model.BasicAuthType:
		var deSecret model.BasicAuthSecret
		err = json.Unmarshal(plaintext, &deSecret)
		secret.Secret.Content = deSecret
	case model.OAuth2Type:
		var deSecret model.OAuth2Secret
		err = json.Unmarshal(plaintext, &deSecret)
		secret.Secret.Content = deSecret
	case model.PKIType:
		var deSecret model.PKISecret
		err = json.Unmarshal(plaintext, &deSecret)
		secret.Secret.Content = deSecret
	default:
		var deSecret map[string]interface{}
		err = json.Unmarshal(plaintext, &deSecret)
		secret.Secret.Content = deSecret
	}

	return err
}

// AddSecret adds a new secret to the vault, returning its identifier
func (v *MongoRepository) AddSecret(secret model.Secret) (string, error) {
	encrypted, err := v.Encrypt(secret)
	if err != nil {
		return "", err
	}

	return encrypted.ID, v.insert(secretsCollection, encrypted)
}

// UpdateSecret updates a secret replacing its content if it exists or returning an error if not
func (v *MongoRepository) UpdateSecret(secretID string, secret model.Secret) error {
	var existing SecretEntry
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
func (v *MongoRepository) GetSecret(secretID string) (model.Secret, error) {
	var existing SecretEntry
	err := v.get(secretsCollection, secretID, &existing)
	if err != nil {
		return existing.Secret, err
	}

	err = v.Decrypt(&existing)
	return existing.Secret, err
}

// DeleteSecret deletes a secret from the vault given its identifier
func (v *MongoRepository) DeleteSecret(secretID string) error {
	return v.delete(secretsCollection, secretID)
}
