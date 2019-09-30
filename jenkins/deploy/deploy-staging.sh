#!/usr/bin/env bash
# Staging environment: 31.171.247.162
# Private key for ssh: /opt/keypairs/ditas-testbed-keypair.pem

# TODO state management? We are killing without caring about any operation the conainer could be doing.

ssh -i /opt/keypairs/ditas-testbed-keypair.pem cloudsigma@31.171.247.162 << 'ENDSSH'
# Ensure that a previously running instance is stopped (-f stops and removes in a single step)
# || true - "docker stop" failt with exit status 1 if image doesn't exists, what makes the Pipeline fail. the "|| true" forces the command to exit with 0.

sudo docker stop --time 20 deployment-engine || true
sudo docker rm --force deployment-engine || true
sudo docker pull ditas/deployment-engine:staging

sudo docker stop --time 20 mongo || true
sudo docker rm --force mongo || true
sudo docker pull mvertes/alpine-mongo:latest

# SET THE PORT MAPPING, link for MongoDB container
sudo docker run -p 27017:27017 -d --name mongo mvertes/alpine-mongo:latest
sudo docker run -p 50012:8090 -v /opt/ditas/dep-engine/config:/root/deployment-engine-config -v /opt/ditas/dep-engine/ssh-certs:/root/.ssh -d --name deployment-engine --link mongo:mongo ditas/deployment-engine:staging
ENDSSH