package persistence

import (
	"deployment-engine/model"
	"deployment-engine/persistence/memoryrepo"
	"deployment-engine/persistence/mongorepo"
	"flag"
	"os"
	"reflect"
	"testing"

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
	if *integrationMongo {
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
	}
	t.Run("Deployments", testDeployment)
	t.Run("Vault", testVault)
}

func testStatus(repo DeploymentRepository, ID, status, infraID, infrastatus string, t *testing.T) {
	resGet, errGet := repo.GetDeployment(ID)

	if errGet != nil {
		t.Fatalf("Error getting deployment: %s", errGet.Error())
	}

	if resGet.ID != ID {
		t.Fatalf("Original and recovered Ids do not match: %s vs %s", ID, resGet.ID)
	}

	if resGet.Status != status {
		t.Fatalf("Unexpected status: %s vs expected %s", resGet.Status, status)
	}

	if infraID != "" {

		if resGet.Infrastructures == nil || len(resGet.Infrastructures) == 0 {
			t.Fatalf("Infrastructures is empty")
		}

		infra, ok := resGet.Infrastructures[infraID]
		if !ok {
			t.Fatalf("Can't find infrastructure %s in deployment %s", infraID, ID)
		}

		if infra.Status != infrastatus {
			t.Fatalf("Unexpected infrastructure status: %s vs expected %s", infra.Status, infrastatus)
		}
	}
}

func testDeployment(t *testing.T) {
	t.Logf("Testing %d deployment repositories", len(depRepos))

	for _, repo := range depRepos {
		infraId := "infra1"
		dep := model.DeploymentInfo{
			Infrastructures: map[string]model.InfrastructureDeploymentInfo{
				infraId: model.InfrastructureDeploymentInfo{
					ID:     infraId,
					Status: "creating",
				}},
		}
		res, err := repo.SaveDeployment(dep)

		if err != nil {
			t.Fatalf("Error saving deployment: %s", err.Error())
		}

		if res.ID == "" {
			t.Fatal("Null id for inserted deployment")
		}

		resGet, err := repo.GetDeployment(res.ID)

		if err != nil {
			t.Fatalf("Error retrieving deployment: %s", err.Error())
		}

		if resGet.ID != res.ID {
			t.Fatalf("Retrieved bad deployment ID. Expected %s but got %s", res.ID, resGet.ID)
		}

		res.Status = "running"

		res, err = repo.UpdateDeployment(res)

		if err != nil {
			t.Fatalf("Error updating deployment: %s", err.Error())
		}

		testStatus(repo, res.ID, "running", infraId, "creating", t)

		res, err = repo.UpdateDeploymentStatus(res.ID, "failed")
		if err != nil {
			t.Fatalf("Error updating deployment status: %s", err.Error())
		}

		res, err = repo.UpdateInfrastructureStatus(res.ID, infraId, "created")
		if err != nil {
			t.Fatalf("Error updating infrastructure status: %s", err.Error())
		}

		testStatus(repo, res.ID, "failed", infraId, "created", t)

		total, err := repo.ListDeployment()
		if err != nil {
			t.Fatalf("Error listing deployments: %s", err.Error())
		}

		if len(total) == 0 {
			t.Fatalf("Got empty list of deployments")
		}

		extraInfraId := "infra2"

		res, err = repo.AddInfrastructure(res.ID, model.InfrastructureDeploymentInfo{
			ID:     extraInfraId,
			Status: "creating",
		})

		if err != nil {
			t.Fatalf("Error adding new infrastructure: %s", err.Error())
		}

		if len(res.Infrastructures) != 2 {
			t.Fatalf("After adding new infrastructure found %d infrastructures but expected 2", len(resGet.Infrastructures))
		}

		infra, err := repo.FindInfrastructure(res.ID, extraInfraId)

		if err != nil {
			t.Fatalf("Error finding infrastructure: %s", err.Error())
		}

		if infra.ID != extraInfraId {
			t.Fatalf("Found wrong infrastructure. Expected %s but found %s", extraInfraId, infra.ID)
		}

		res, err = repo.AddProductToInfrastructure(res.ID, extraInfraId, "kubernetes", make(map[string]string))
		if err != nil {
			t.Fatalf("Error adding product to infrastructure: %s", err.Error())
		}

		if _, ok := res.Infrastructures[extraInfraId].Products["kubernetes"]; !ok {
			t.Fatal("Product not found in response")
		}

		res, err = repo.DeleteInfrastructure(res.ID, extraInfraId)
		if err != nil {
			t.Fatalf("Error deleting infrastructure: %s", err.Error())
		}

		if _, ok := res.Infrastructures[extraInfraId]; ok {
			t.Fatalf("Deleted infrastructure but then found in deployment")
		}

		err = repo.DeleteDeployment(res.ID)
		if err != nil {
			t.Fatalf("Error deleting deployment %s: %s", res.ID, err.Error())
		}

		_, errGet := repo.GetDeployment(res.ID)
		if errGet == nil {
			t.Fatalf("Got previously deleted deplyment %s", res.ID)
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
