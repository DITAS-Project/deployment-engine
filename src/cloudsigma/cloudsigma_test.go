package cloudsigma

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var client *Client
var pubKey string

var integration = flag.Bool("integration", false, "run DS4M integration tests")

func TestMain(m *testing.M) {
	if *integration {
		home, err := homedir.Dir()
		if err != nil {
			msg := fmt.Sprintf("Error getting home folder: %s", err.Error())
			panic(msg)
		}
		viper.SetConfigType("properties")
		viper.SetConfigFile(home + "/.cloudsigma.conf")
		err = viper.ReadInConfig()
		if err == nil {

			pubKeyRaw, err := ioutil.ReadFile(home + "/.ssh/id_rsa.pub")
			if err == nil {
				pubKey = string(pubKeyRaw)
				client = NewClient(viper.GetString("api_endpoint"),
					viper.GetString("username"), viper.GetString("password"), true)
				os.Exit(m.Run())
			}

			msg := fmt.Sprintf("Error reading public key: %s", err.Error())
			panic(msg)

		} else {
			msg := fmt.Sprintf("Error reading configuration: %s", err.Error())
			panic(msg)
		}
	}

}

func waitForStatusChange(t *testing.T, tag string, resourceType string, status string, timeout time.Duration) (RequestResponseType, bool, error) {
	waited := 0 * time.Second
	ready := false
	var response RequestResponseType
	var err error
	for !ready && waited < timeout && err == nil {
		response, err := client.GetByTag(tag, resourceType)
		if err != nil {
			t.Fatalf("Error getting resource type %s by tag %s: %s", resourceType, tag, err.Error())
		}
		if len(response.Objects) == 0 {
			t.Fatalf("Can't find any resource type %s by tag %s", resourceType, tag)
		}
		ready = true
		for _, response := range response.Objects {
			ready = ready && response.Status != status
		}
		if !ready {
			time.Sleep(3 * time.Second)
			waited += 3 * time.Second
		}
	}
	return response, waited >= timeout, err
}

func TestTag(t *testing.T) {
	if *integration {
		tag, err := client.CreateTag("test-tag", []ResourceType{})
		err = client.DeleteTag(tag.UUID)
		tag, err = client.GetTagInformation(tag.UUID)

		if err == nil {
			t.Fatalf("Expected 404 error getting tag but got tag %s", tag.UUID)
		}
	}
}

func TestListDrives(t *testing.T) {

	if *integration {
		drive, err := client.GetLibDrive(map[string]string{
			"version": "16.04 DITAS",
		})

		if err != nil {
			t.Fatalf("Error finding DITAS drive %s", err.Error())
		}

		if drive.UUID == "" {
			t.Fatal("Invalid UUID found")
		}

		if drive.Version != "16.04 DITAS" {
			t.Fatalf("Invalid version found: %s", drive.Version)
		}

		clone, err := client.CloneDrive(drive.UUID, nil)

		if err != nil {
			t.Fatalf("Error cloning DITAS drive %s", err.Error())
		}

		if clone.UUID == "" {
			t.Fatal("Invalid UUID found")
		}

		if clone.UUID == drive.UUID {
			t.Fatalf("Same UUID found in drive and clone")
		}

		tag, err := client.CreateTag("test-vdc", []ResourceType{clone})
		if err != nil {
			t.Fatalf("Error creating test tag")
		}

		if tag.UUID == "" {
			t.Fatalf("UUID not found for tag")
		}

		if len(tag.Resources) == 0 {
			t.Fatalf("Resources not added to tag")
		}

		fmt.Printf("Waiting for drive to be ready\n")
		timeout := 60 * time.Second
		result, timedOut, err := waitForStatusChange(t, tag.UUID, DrivesType, "cloning_dst", timeout)

		if err != nil {
			t.Fatalf("Error waiting for drive to be ready: %s", err.Error())
		}

		if timedOut {
			t.Fatalf("Timeout reached waiting for drive to be ready")
		}

		for _, drive := range result.Objects {
			if drive.Status != "unmounted" {
				t.Fatalf("Unexpected status for cloned drive %s: %s", drive.UUID, drive.Status)
			}
		}

		resources := RequestResponseType{
			Objects: []ResourceType{ResourceType{
				Name:        "test-server",
				CPU:         2000,
				Mem:         4096 * 1024 * 1024,
				VNCPassword: "test-password",
				Drives: []ServerDriveType{ServerDriveType{
					BootOrder:  1,
					DevChannel: "0:0",
					Device:     "virtio",
					Drive:      clone,
				}},
				NICS: []ServerNICType{ServerNICType{
					IPV4Conf: ServerIPV4ConfType{
						Conf: "dhcp",
					},
					Model: "virtio",
				}},
				Meta: map[string]string{
					"ssh_public_key": pubKey,
				},
				Tags: []ResourceType{tag},
			}},
		}

		availableServers, err := client.CreateServers(resources)
		if err != nil {
			t.Fatalf("Error creating servers: %s", err.Error())
		}

		if len(availableServers.Objects) == 0 {
			t.Fatalf("Empty list of servers found")
		}

		var found ResourceType
		for _, server := range availableServers.Objects {
			if server.Name == "test-server" {
				found = server
			}
		}

		if found.Name != "test-server" {
			t.Fatalf("Created server not found")
		}

		if found.UUID == "" {
			t.Fatal("Invalid UUID found for created server")
		}

		if len(found.Tags) == 0 {
			t.Fatal("Server created without tags")
		}

		if found.Tags[0].UUID != tag.UUID {
			t.Fatalf("Server tag %s is different than expected %s", found.Tags[0].UUID, tag.UUID)
		}

		serverInfo, err := client.GetServerDetails(found.UUID)

		if err != nil {
			t.Errorf("Error reading individual server %s information: %s", found.UUID, err.Error())
		}

		if serverInfo.UUID != found.UUID {
			t.Errorf("Invalid information returned for individual server %s. Found UUID %s", found.UUID, serverInfo.UUID)
		}

		if serverInfo.Status == "" {
			t.Errorf("Empty status returned for individual server %s", found.UUID)
		}

		actionResult, err := client.ExecuteServerAction(found.UUID, ServerStartAction)

		if err != nil {
			t.Fatalf("Error starting server %s", err.Error())
		}

		if actionResult.Result != "success" {
			t.Fatalf("Unexpected state of start operation: %s", actionResult.Result)
		}

		fmt.Printf("Waiting for server to be ready\n")
		result, timedOut, err = waitForStatusChange(t, tag.UUID, ServersType, "starting", timeout)

		if err != nil {
			t.Fatalf("Error waiting for server to start: %s", err.Error())
		}

		if timedOut {
			t.Fatalf("Timeout reached waiting for server to start")
		}

		for _, server := range result.Objects {
			if server.Status != "running" {
				t.Fatalf("Server start status in unexpected state: %s", server.Status)
			}
		}

		actionResult, err = client.ExecuteServerAction(found.UUID, ServerStopAction)

		if err != nil {
			t.Fatalf("Error stopping server %s", err.Error())
		}

		if actionResult.Result != "success" {
			t.Fatalf("Unexpected state of stop operation: %s", actionResult.Result)
		}

		fmt.Printf("Waiting for server to stop\n")
		result, timedOut, err = waitForStatusChange(t, tag.UUID, ServersType, "stopping", timeout)

		if err != nil {
			t.Fatalf("Error waiting for server to stop: %s", err.Error())
		}

		if timedOut {
			t.Fatalf("Timeout reached waiting for server to stop")
		}

		for _, server := range result.Objects {
			if server.Status != "stopped" {
				t.Fatalf("Server stop status in unexpected state: %s", server.Status)
			}
		}

		err = client.DeleteServerWithDrives(found.UUID)

		if err != nil {
			t.Fatalf("Error deleting server: %s", err.Error())
		}

		resources, err = client.GetByTag(tag.UUID, ServersType)

		if err != nil {
			t.Fatalf("Error getting servers after delete: %s", err.Error())
		}

		if len(resources.Objects) > 0 {
			t.Fatalf("Found server %s after deletion", resources.Objects[0].UUID)
		}

		resources, err = client.GetByTag(tag.UUID, DrivesType)

		if err != nil {
			t.Fatalf("Error getting drives after delete: %s", err.Error())
		}

		if len(resources.Objects) > 0 {
			t.Fatalf("Found drive %s after deletion", resources.Objects[0].UUID)
		}

		err = client.DeleteTag(tag.UUID)

		if err != nil {
			t.Fatalf("Error deleting tag: %s", err.Error())
		}

		tag, err = client.GetTagInformation(tag.UUID)

		if err == nil {
			t.Fatalf("Expected 404 error getting tag but got tag %s", tag.UUID)
		}
	}

}
