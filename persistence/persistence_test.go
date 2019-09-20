package persistence

import (
	"deployment-engine/model"
	"deployment-engine/persistence/memoryrepo"
	"deployment-engine/persistence/mongorepo"
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/go-test/deep"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var integrationMongo = flag.Bool("mongo", false, "run MongoDB integration tests")

var depRepos []DeploymentRepository
var vaults []Vault

func TestMain(m *testing.M) {

	memRepo := memoryrepo.CreateMemoryRepository()
	depRepos = append(depRepos, memRepo)
	vaults = append(vaults, memRepo)

	os.Exit(m.Run())
}

func TestRepository(t *testing.T) {
	//if *integrationMongo {
	t.Log("Running MongoDB integration tests")
	viper.SetDefault(mongorepo.VaultPassphraseName, "my test passphrase")
	repo, err := mongorepo.CreateRepositoryNative()
	if err != nil {
		log.Fatalf("Error creating repository: %s", err.Error())
	}
	repo.SetDatabase("deployment_engine_test")
	err = repo.ClearDatabase()
	if err != nil {
		log.Fatalf("Error clearing database")
	}
	depRepos = append(depRepos, repo)
	vaults = append(vaults, repo)
	//}
	t.Run("Deployments", testDeployment)
	t.Run("Vault", testVault)
}

func readInfra(path string) (model.InfrastructureDeploymentInfo, error) {
	var result model.InfrastructureDeploymentInfo
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return result, err
	}

	err = json.Unmarshal(content, &result)
	return result, err
}

func testInfra(t *testing.T, function func() (model.InfrastructureDeploymentInfo, error), infra model.InfrastructureDeploymentInfo, errorMsg string) model.InfrastructureDeploymentInfo {
	after, err := function()
	if err != nil {
		t.Fatalf("%s: %s", errorMsg, err.Error())
	}
	if diff := deep.Equal(after, infra); diff != nil {
		t.Error(errorMsg)
		t.Fatal(diff)
	}
	return after
}

func testDeployment(t *testing.T) {
	t.Logf("Testing %d deployment repositories", len(depRepos))
	for _, repo := range depRepos {
		infra, err := readInfra("../resources/test_infra1.json")
		if err != nil {
			t.Fatalf("Error reading input infrastructure: %s", err.Error())
		}
		infra.Products = make(map[string]interface{})

		after, err := repo.AddInfrastructure(infra)
		if err != nil {
			t.Fatalf("Error inserting infrastructure %v: %s", infra, err.Error())
		}

		if infra.ID == "" {
			infra.ID = after.ID
		}

		if diff := deep.Equal(infra, after); diff != nil {
			t.Fatal(diff)
		}

		after = testInfra(t, func() (model.InfrastructureDeploymentInfo, error) {
			return repo.FindInfrastructure(infra.ID)
		}, infra, "Error finding infrastructure")

		after.Status = "completed"
		after.Name = "New Name"
		infra.Status = "completed"
		infra.Name = "New Name"
		after = testInfra(t, func() (model.InfrastructureDeploymentInfo, error) {
			return repo.UpdateInfrastructure(after)
		}, infra, "Error updating infrastructure")

		infra.Status = "done"
		after = testInfra(t, func() (model.InfrastructureDeploymentInfo, error) {
			return repo.UpdateInfrastructureStatus(infra.ID, "done")
		}, infra, "Error updating infrastructure status")

		testConfig := map[string]interface{}{
			"testProperty": "testValue",
		}
		infra.Products = map[string]interface{}{
			"kubernetes": testConfig,
		}
		after = testInfra(t, func() (model.InfrastructureDeploymentInfo, error) {
			return repo.AddProductToInfrastructure(infra.ID, "kubernetes", testConfig)
		}, infra, "Error adding product to infrastructure")

		after = testInfra(t, func() (model.InfrastructureDeploymentInfo, error) {
			return repo.DeleteInfrastructure(infra.ID)
		}, infra, "Error adding product to infrastructure")

		after, err = repo.FindInfrastructure(infra.ID)
		if err == nil {
			t.Fatalf("Retrieved infrastructure %s when it was deleted", infra.ID)
		}
	}
}

func testVault(t *testing.T) {
	t.Logf("Testing %d vaults", len(vaults))
	for _, repo := range vaults {
		testSecret := model.Secret{
			Description: "Test secret",
			Format:      model.BasicAuthType,
			Content: model.BasicAuthSecret{
				Username: "someuser",
				Password: "somepassword",
			},
		}

		secretId, err := repo.AddSecret(testSecret)
		if err != nil {
			t.Fatalf("Error saving secret %s", err.Error())
		}

		if secretId == "" {
			t.Fatalf("Created secret id is empty")
		}

		secret, err := repo.GetSecret(secretId)
		if err != nil {
			t.Fatalf("Error getting secret %s", err.Error())
		}

		if !reflect.DeepEqual(testSecret, secret) {
			t.Fatalf("Retrieved secret %s is different than the original one %s", secret, testSecret)
		}

		newSecret := model.Secret{
			Description: "OAuth test secret",
			Format:      model.OAuth2Type,
			Content: model.OAuth2Secret{
				ClientID:     "myclientId",
				ClientSecret: "MySecret",
			},
		}

		err = repo.UpdateSecret(secretId, newSecret)
		if err != nil {
			t.Fatalf("Error updating secret: %s", err.Error())
		}

		secret, err = repo.GetSecret(secretId)
		if err != nil {
			t.Fatalf("Error getting secret %s", err.Error())
		}

		if !reflect.DeepEqual(newSecret, secret) {
			t.Fatalf("Retrieved secret %v is different than the updated one %s", secret, newSecret)
		}

		err = repo.DeleteSecret(secretId)
		if err != nil {
			t.Fatalf("Error deleting secret: %s", err.Error())
		}

		secret, err = repo.GetSecret(secretId)
		if err == nil {
			t.Fatalf("Secret %s was deleted but could be retrieved", secretId)
		}
	}
}
