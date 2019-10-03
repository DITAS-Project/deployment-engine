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

import (
	"math/rand"
	"sort"
	"testing"
)

func TestPorts(t *testing.T) {
	lastPort := 30000
	config := KubernetesConfiguration{
		UsedPorts: make(sort.IntSlice, 0),
	}

	for i := 0; i < 10; i++ {
		port := config.GetNewFreePort()
		if !(port >= lastPort && port < 30010) {
			t.Fatalf("Port %d out of expected initial range", port)
		}
	}

	if len(config.UsedPorts) != 10 {
		t.Fatalf("Unexpected length for used ports: Expected 100 but got %d", len(config.UsedPorts))
	}

	if !sort.IntsAreSorted(config.UsedPorts) {
		t.Fatalf("Ports not sorted after initial load")
	}

	err := config.ClaimPort(30008)
	if err == nil {
		t.Fatal("Succeeded in claiming port 30078 but it should have failed")
	}

	for i := 30020; i < 30030; i++ {
		err := config.ClaimPort(i)
		if err != nil {
			t.Fatalf("Error claiming port %d: %s", i, err.Error())
		}
	}

	if !sort.IntsAreSorted(config.UsedPorts) {
		t.Fatalf("Ports not sorted after batch claim")
	}

	if len(config.UsedPorts) != 20 {
		t.Fatalf("Unexpected length for used ports: Expected 200 but got %d", len(config.UsedPorts))
	}

	for i := 0; i < 50; i++ {
		port := config.GetNewFreePort()
		if port >= 30020 && port < 30030 {
			t.Fatalf("Got free port %d but it should be used", port)
		}
	}

	if !sort.IntsAreSorted(config.UsedPorts) {
		t.Fatalf("Ports not sorted after batch port finding")
	}

	if len(config.UsedPorts) != 70 {
		t.Fatalf("Expected 700 ports in use but got %d", len(config.UsedPorts))
	}

	for i := 30015; i < 30035; i++ {
		config.LiberatePort(i)
	}

	if len(config.UsedPorts) != 50 {
		t.Fatalf("Expected 500 ports in use but got %d", len(config.UsedPorts))
	}

	if !sort.IntsAreSorted(config.UsedPorts) {
		t.Fatalf("Ports not sorted after free")
	}

	newPorts := 0
	max := NodePortEnd - NodePortStart
	for i := 0; i < 50; i++ {
		randPort := NodePortStart + rand.Intn(max)
		err = config.ClaimPort(randPort)
		if err == nil {
			newPorts++
		}
	}

	if len(config.UsedPorts) != 50+newPorts {
		t.Fatalf("Expected %d ports in use but got %d", 500+newPorts, len(config.UsedPorts))
	}

	if !sort.IntsAreSorted(config.UsedPorts) {
		t.Fatalf("Ports not sorted after random claim")
	}

}
