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

package ansible

import (
	"deployment-engine/utils"
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
)

func ExecutePlaybook(logger *log.Entry, script string, inventory string, extravars map[string]string) error {
	args := make([]string, 1)
	args[0] = script

	if inventory != "" {
		args = append(args, fmt.Sprintf("--inventory=%s", inventory))
	}

	if extravars != nil && len(extravars) > 0 {
		args = append(args, "--extra-vars")
		vars, err := json.Marshal(extravars)
		if err != nil {
			return fmt.Errorf("Error marshaling ansible variables: %w", err)
		}
		args = append(args, string(vars))
	}

	return utils.ExecuteCommand(logger, "ansible-playbook", args...)
}
