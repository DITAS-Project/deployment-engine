package mongorepo

import (
	"deployment-engine/model"
	"flag"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var integrationMongo = flag.Bool("mongo", false, "run MongoDB integration tests")
var integrationVault = flag.Bool("vault", false, "run MongoDB Vault integration tests")

var repo *MongoRepositoryNative

func TestMain(m *testing.M) {
	viper.SetDefault(VaultPassphraseName, "my test passphrase")
	var err error
	repo, err = CreateRepositoryNative()
	if err != nil {
		log.Errorf("Error creating repository: %s", err.Error())
	}
	os.Exit(m.Run())
}

func TestRepository(t *testing.T) {
	t.Run("Deployments", testDeployment)
	t.Run("Vault", testVault)
}

func testStatus(ID, status, infrastatus string, t *testing.T) {
	resGet, errGet := repo.GetDeployment(ID)

	if errGet != nil {
		t.Errorf("Error getting deployment: %s", errGet.Error())
	}

	if resGet.ID != ID {
		t.Errorf("Original and recovered Ids do not match: %s vs %s", ID, resGet.ID)
	}

	if resGet.Status != status {
		t.Errorf("Unexpected status: %s vs expected %s", resGet.Status, status)
	}

	if len(resGet.Infrastructures) < 1 {
		t.Errorf("Infrastructures is empty")
	}

	if resGet.Infrastructures[0].Status != infrastatus {
		t.Errorf("Unexpected infrastructure status: %s vs expected %s", resGet.Infrastructures[0].Status, infrastatus)
	}
}

func testDeployment(t *testing.T) {
	if *integrationMongo {
		t.Log("Running MongoDB integration tests")
		dep := model.DeploymentInfo{
			Infrastructures: []model.InfrastructureDeploymentInfo{
				model.InfrastructureDeploymentInfo{
					ID:     "infra1",
					Status: "creating",
				}},
		}
		res, err := repo.SaveDeployment(dep)

		if err != nil {
			t.Errorf("Error saving deployment: %s", err.Error())
		}

		if res.ID == "" {
			t.Error("Null id for inserted deployment")
		}

		res.Status = "running"

		res, err = repo.UpdateDeployment(res)

		if err != nil {
			t.Errorf("Error updating deployment: %s", err.Error())
		}

		testStatus(res.ID, "running", "creating", t)

		err = repo.UpdateDeploymentStatus(res.ID, "failed")
		if err != nil {
			t.Errorf("Error updating deployment status: %s", err.Error())
		}

		err = repo.UpdateInfrastructureStatus(res.ID, res.Infrastructures[0].ID, "created")
		if err != nil {
			t.Errorf("Error updating infrastructure status: %s", err.Error())
		}

		testStatus(res.ID, "failed", "created", t)

		total, err := repo.ListDeployment()
		if err != nil {
			t.Errorf("Error listing deployments: %s", err.Error())
		}

		if len(total) == 0 {
			t.Errorf("Got empty list of deployments")
		}

		err = repo.DeleteDeployment(res.ID)
		if err != nil {
			t.Errorf("Error deleting deployment %s: %s", res.ID, err.Error())
		}

		_, errGet := repo.GetDeployment(res.ID)
		if errGet == nil {
			t.Errorf("Got previously deleted deplyment %s", res.ID)
		}
	}
}

func testSecrets(t *testing.T, source, target map[string]string) {
	if len(source) != len(target) {
		t.Errorf("Wrong length of retrieved secret, expected %d but got %d", len(target), len(source))
	}

	for k, v := range target {
		sourceValue, ok := source[k]
		if !ok {
			t.Errorf("Found unexpected key %s when comparing secrets", k)
		}
		if sourceValue != v {
			t.Errorf("Values differ for key %s: Expected %s but got %s", k, v, sourceValue)
		}
	}
}

func testVault(t *testing.T) {
	if *integrationVault {
		t.Log("Running Vault integration tests")
		testSecret := map[string]string{
			"username": "myuser",
			"password": "mysecretpassword",
		}

		secretId, err := repo.AddSecret(testSecret)
		if err != nil {
			t.Errorf("Error saving secret %s", err.Error())
		}

		if secretId == "" {
			t.Errorf("Created secret id is empty")
		}

		secret, err := repo.GetSecret(secretId)
		if err != nil {
			t.Errorf("Error getting secret %s", err.Error())
		}

		testSecrets(t, testSecret, secret)

		newSecret := map[string]string{
			"username": "myuser",
			"password": "mynewpassword",
		}

		err = repo.UpdateSecret(secretId, newSecret)
		if err != nil {
			t.Errorf("Error updating secret: %s", err.Error())
		}

		secret, err = repo.GetSecret(secretId)
		if err != nil {
			t.Errorf("Error getting secret %s", err.Error())
		}

		testSecrets(t, newSecret, secret)

		err = repo.DeleteSecret(secretId)
		if err != nil {
			t.Errorf("Error deleting secret: %s", err.Error())
		}

		secret, err = repo.GetSecret(secretId)
		if err == nil {
			t.Errorf("Secret %s was deleted but could be retrieved", secretId)
		}
	}
}
