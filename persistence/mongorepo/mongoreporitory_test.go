package mongorepo

import (
	"deployment-engine/model"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
)

var repo *MongoRepositoryNative

func TestMain(m *testing.M) {
	var err error
	repo, err = CreateRepositoryNative()
	if err != nil {
		log.Errorf("Error creating repository: %s", err.Error())
	}
	os.Exit(m.Run())
}

func TestRepository(t *testing.T) {
	t.Run("CreateDeployment", testDeployment)
}

func testDeployment(t *testing.T) {
	res, err := repo.Save(model.DeploymentInfo{})

	if err != nil {
		t.Errorf("Error saving deployment: %s", err.Error())
	}

	if res.ID == "" {
		t.Error("Null id for inserted deployment")
	}

	res.Status = "running"

	res, err = repo.Update(res)

	if err != nil {
		t.Errorf("Error updating deployment: %s", err.Error())
	}

	resGet, errGet := repo.Get(res.ID)

	if errGet != nil {
		t.Errorf("Error getting deployment: %s", errGet.Error())
	}

	if resGet.ID != res.ID {
		t.Errorf("Original and recovered Ids do not match: %s vs %s", res.ID, resGet.ID)
	}

	if resGet.Status != "running" {
		t.Errorf("Unexpected status: %s", resGet.Status)
	}

	total, err := repo.List()
	if err != nil {
		t.Errorf("Error listing deployments: %s", err.Error())
	}

	if len(total) == 0 {
		t.Errorf("Got empty list of deployments")
	}
}
