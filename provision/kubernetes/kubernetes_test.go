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

package kubernetes

import "testing"

func TestPorts(t *testing.T) {
	lastPort := 30000
	config := KubernetesConfiguration{
		LastNodePort: lastPort,
		UsedPorts:    make(map[int]bool),
	}

	for i := 0; i < 100; i++ {
		port := config.GetNewFreePort()
		if !(port >= lastPort && port < 30100) {
			t.Fatalf("Port %d out of expected initial range", port)
		}
		used, ok := config.UsedPorts[port]
		if !ok {
			t.Fatalf("Port %d expected to be in use but it's not", port)
		}

		if !used {
			t.Fatalf("Wrong used value for port %d", port)
		}
	}

	if len(config.UsedPorts) != 100 {
		t.Fatalf("Unexpected length for used ports: Expected 100 but got %d", len(config.UsedPorts))
	}

	err := config.ClaimPort(30078)
	if err == nil {
		t.Fatal("Succeeded in claiming port 30078 but it should have failed")
	}

	for i := 30200; i < 30300; i++ {
		err := config.ClaimPort(i)
		if err != nil {
			t.Fatalf("Error claiming port %d: %s", i, err.Error())
		}
	}

	if len(config.UsedPorts) != 200 {
		t.Fatalf("Unexpected length for used ports: Expected 200 but got %d", len(config.UsedPorts))
	}

	for i := 0; i < 500; i++ {
		port := config.GetNewFreePort()
		if port >= 30200 && port < 30300 {
			t.Fatalf("Got free port %d but it should be used", port)
		}
	}

	if len(config.UsedPorts) != 700 {
		t.Fatalf("Expected 700 ports in use but got %d", len(config.UsedPorts))
	}

	for i := 30150; i < 30350; i++ {
		config.LiberatePort(i)
	}

	if len(config.UsedPorts) != 500 {
		t.Fatalf("Expected 500 ports in use but got %d", len(config.UsedPorts))
	}

	if config.LastNodePort != 30150 {
		t.Fatalf("Last node port expected to be 30150 but got %d", config.LastNodePort)
	}

}
