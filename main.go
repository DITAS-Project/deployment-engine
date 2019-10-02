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
//go:generate swagger generate spec
package main

import (
	"deployment-engine/ditas"
	"deployment-engine/utils"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
)

const (
	RepositoryProperty              = "repository.type"
	VaultProperty                   = "vault.type"
	FrontendProperty                = "frontent.type"
	FrontendPortProperty            = "frontend.port"
	SSHPublicKeyProperty            = "ssh.public_key"
	SSHPrivateKeyProperty           = "ssh.private_key"
	SSHPrivateKeyPassphraseProperty = "ssh.passphrase"

	RepositoryDefault   = "mongo"
	VaultDefault        = "mongo"
	FrontendDefault     = "default"
	FrontendPortDefault = "8080"
)

func main() {
	home, err := homedir.Dir()
	if err != nil {
		log.WithError(err).Error("Error getting HOME folder")
	}
	sshDir := home + "/.ssh"
	viper.SetDefault(RepositoryProperty, RepositoryDefault)
	viper.SetDefault(VaultProperty, VaultDefault)
	viper.SetDefault(FrontendProperty, FrontendDefault)
	viper.SetDefault(FrontendPortProperty, FrontendPortDefault)
	viper.SetDefault(SSHPublicKeyProperty, sshDir+"/id_rsa.pub")
	viper.SetDefault(SSHPrivateKeyProperty, sshDir+"/id_rsa")

	configFolder, err := utils.ConfigurationFolder()
	if err != nil {
		log.WithError(err).Error("Error getting configuration folder")
		return
	}

	viper.AddConfigPath(configFolder)
	viper.SetConfigName("config")
	viper.ReadInConfig()

	/*repository, err := getRepository(viper.GetString(RepositoryProperty))
	if err != nil {
		log.WithError(err).Error("Error getting repository")
		return
	}

	vault, err := getVault(viper.GetString(VaultProperty), viper.GetString(RepositoryProperty), repository)
	if err != nil {
		log.WithError(err).Error("Error getting vault")
	}

	frontend, err := getFrontend(viper.GetString(FrontendProperty), repository, vault)
	if err != nil {
		log.WithError(err).Error("Error getting provisioner")
		return
	}*/

	frontend, err := ditas.NewDitasFrontend()
	if err != nil {
		log.WithError(err).Error("Error getting frontend")
		return
	}

	port := viper.GetString(FrontendPortProperty)
	log.Infof("Starting deployment engine on port %s", port)

	log.Fatal(frontend.Run(":" + port))
}
