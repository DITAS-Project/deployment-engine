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
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/sethvargo/go-password/password"
	"github.com/sirupsen/logrus"
)

const (
	RandomVariable = "random"
)

func GetRepositoryFromImageName(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) > 1 && (strings.Contains(parts[0], ":") || strings.Contains(parts[0], ".")) {
		return parts[0]
	}
	return ""
}

// expandVariable gets a potential variable name and a set of values indexed by variable name and returns its value if found.
// The variable varValue can be:
// - A variable surrounded by ${var} or $var in which case it will be replaced by values[var] if found.
// - Any other string, in which case it will be returned as is
func expandVariable(varValue string, values map[string]interface{}) string {
	if values != nil {
		return os.Expand(varValue, func(variable string) string {
			if variable == RandomVariable {
				pw, err := password.Generate(10, 3, 1, false, false)
				if err != nil {
					logrus.WithError(err).Error("Error generating random password for environment variable. Generating a UUID instead")
					return uuid.New().String()
				}
				return pw
			}
			val, ok := values[variable]
			if ok {
				return fmt.Sprintf("%v", val)
			}
			return variable
		})
	}
	return varValue
}

// FillEnvVars gets a set of environment variables by replacing its values for the ones found in the variables parameter if found when their value is set in the form ${var} or $var.
func FillEnvVars(images ImageSet, variables map[string]interface{}) map[string]map[string]string {
	result := make(map[string]map[string]string)
	if images != nil {
		for imageName, vdcImage := range images {
			imageVars := make(map[string]string)
			if vdcImage.Environment != nil {
				for variable, value := range vdcImage.Environment {
					imageVars[variable] = expandVariable(value, variables)
				}
			}
			result[imageName] = imageVars
		}
	}
	return result
}

// ReplaceEnvVars will replace the environment variables in the DAL and VDC images that are in the form ${var} or $var with the values in variables[var] if found.
func ReplaceEnvVars(images ImageSet, variables map[string]interface{}) {
	result := FillEnvVars(images, variables)
	for imageName, imageInfo := range images {
		vars, ok := result[imageName]
		if ok {
			imageInfo.Environment = vars
		}
		images[imageName] = imageInfo
	}
}
