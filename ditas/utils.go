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
 *
 * This is being developed for the DITAS Project: https://www.ditas-project.eu/
 */

package ditas

import (
	"deployment-engine/provision/kubernetes"
	"fmt"
	"os"
	"strconv"

	blueprint "github.com/DITAS-Project/blueprint-go"
	"github.com/google/uuid"
	"github.com/sethvargo/go-password/password"
	"github.com/sirupsen/logrus"
)

const (
	RandomVariable = "random"
)

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

// FillEnvVars will replace the environment variables in the DAL and VDC images that are in the form ${var} or $var with the values in variables[var] if found.
// A special case for VDC images is the ${dal.<dalName>.<imageName>.port} variable which will be replaced by the external port of the DAL if found and not against the variables map.
func FillEnvVars(dals map[string]blueprint.DALImage, vdcImages kubernetes.ImageSet, variables map[string]interface{}) {

	if dals != nil {
		for dalName, dalInfo := range dals {
			if dalInfo.Images != nil {
				for imageName, imageInfo := range dalInfo.Images {
					if imageInfo.ExternalPort != nil && variables != nil {
						// Fill dal.<dalName>.<imageName>.port variable
						varName := fmt.Sprintf("dal.%s.%s.port", dalName, imageName)
						variables[varName] = strconv.FormatInt(*imageInfo.ExternalPort, 10)
					}

					if imageInfo.Environment != nil {
						for varName, varValue := range imageInfo.Environment {
							imageInfo.Environment[varName] = expandVariable(varValue, variables)
						}
					}
					dalInfo.Images[imageName] = imageInfo
				}
				dals[dalName] = dalInfo
			}
		}
	}

	if vdcImages != nil {
		for imageName, vdcImage := range vdcImages {
			if vdcImage.Environment != nil {
				for variable, value := range vdcImage.Environment {
					vdcImage.Environment[variable] = expandVariable(value, variables)
				}
				vdcImages[imageName] = vdcImage
			}
		}
	}
	return
}
