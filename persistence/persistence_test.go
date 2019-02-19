package persistence

import (
	"deployment-engine/model"
	"deployment-engine/persistence/memoryrepo"
	"deployment-engine/persistence/mongorepo"
	"flag"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var integrationMongo = flag.Bool("mongo", false, "run MongoDB integration tests")

var depRepos []DeploymentRepository
var productRepos []ProductRepository
var vaults []Vault

func TestMain(m *testing.M) {

	memRepo := memoryrepo.CreateMemoryRepository()
	depRepos = append(depRepos, memRepo)
	productRepos = append(productRepos, memRepo)
	vaults = append(vaults, memRepo)

	if *integrationMongo {
		viper.SetDefault(mongorepo.VaultPassphraseName, "my test passphrase")
		repo, err := mongorepo.CreateRepositoryNative()
		if err != nil {
			log.Fatalf("Error creating repository: %s", err.Error())
		}
		depRepos = append(depRepos, repo)
		productRepos = append(productRepos, repo)
		vaults = append(vaults, repo)
	}

	os.Exit(m.Run())
}

func TestRepository(t *testing.T) {
	if *integrationMongo {
		t.Log("Running MongoDB integration tests")
	}
	t.Run("Deployments", testDeployment)
	t.Run("Vault", testVault)
}

func testStatus(repo DeploymentRepository, ID, status, infrastatus string, t *testing.T) {
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

	if len(resGet.Infrastructures) < 1 {
		t.Fatalf("Infrastructures is empty")
	}

	if resGet.Infrastructures[0].Status != infrastatus {
		t.Fatalf("Unexpected infrastructure status: %s vs expected %s", resGet.Infrastructures[0].Status, infrastatus)
	}
}

func testDeployment(t *testing.T) {

	for _, repo := range depRepos {
		dep := model.DeploymentInfo{
			Infrastructures: []model.InfrastructureDeploymentInfo{
				model.InfrastructureDeploymentInfo{
					ID:     "infra1",
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

		testStatus(repo, res.ID, "running", "creating", t)

		err = repo.UpdateDeploymentStatus(res.ID, "failed")
		if err != nil {
			t.Fatalf("Error updating deployment status: %s", err.Error())
		}

		err = repo.UpdateInfrastructureStatus(res.ID, res.Infrastructures[0].ID, "created")
		if err != nil {
			t.Fatalf("Error updating infrastructure status: %s", err.Error())
		}

		testStatus(repo, res.ID, "failed", "created", t)

		total, err := repo.ListDeployment()
		if err != nil {
			t.Fatalf("Error listing deployments: %s", err.Error())
		}

		if len(total) == 0 {
			t.Fatalf("Got empty list of deployments")
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

func testSecrets(t *testing.T, source, target map[string]string) {
	if len(source) != len(target) {
		t.Fatalf("Wrong length of retrieved secret, expected %d but got %d", len(target), len(source))
	}

	for k, v := range target {
		sourceValue, ok := source[k]
		if !ok {
			t.Fatalf("Found unexpected key %s when comparing secrets", k)
		}
		if sourceValue != v {
			t.Fatalf("Values differ for key %s: Expected %s but got %s", k, v, sourceValue)
		}
	}
}

func testVault(t *testing.T) {
	for _, repo := range vaults {
		testSecret := map[string]string{
			"username": "myuser",
			"password": "mysecretpassword",
		}

		secretId, err := repo.AddSecret(testSecret)
		if err != nil {
			t.Fatalf("Error saving secret %s", err.Error())
		}

		if secretId == "" {
			t.Fatalf("Created secret id is empty")
		}

		var secret map[string]string
		err = repo.GetSecret(secretId, &secret)
		if err != nil {
			t.Fatalf("Error getting secret %s", err.Error())
		}

		testSecrets(t, testSecret, secret)

		newSecret := map[string]string{
			"username": "myuser",
			"password": "mynewpassword",
		}

		err = repo.UpdateSecret(secretId, newSecret)
		if err != nil {
			t.Fatalf("Error updating secret: %s", err.Error())
		}

		err = repo.GetSecret(secretId, &secret)
		if err != nil {
			t.Fatalf("Error getting secret %s", err.Error())
		}

		testSecrets(t, newSecret, secret)

		err = repo.DeleteSecret(secretId)
		if err != nil {
			t.Fatalf("Error deleting secret: %s", err.Error())
		}

		err = repo.GetSecret(secretId, &secret)
		if err == nil {
			t.Fatalf("Secret %s was deleted but could be retrieved", secretId)
		}
	}
}
