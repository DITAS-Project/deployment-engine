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
	"strings"
)

func GetRepositoryFromImageName(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) > 1 && (strings.Contains(parts[0], ":") || strings.Contains(parts[0], ".")) {
		return parts[0]
	}
	return ""
}
