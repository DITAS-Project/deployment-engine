Deployment engine for kubernetes cluster

Swagger api file:
https://app.swaggerhub.com/apis/jacekwachowiak/REST-Kubernetes/1.0.0

REST API in GOlang allows to input POST/GET/DELETE messages and with it to create deployments, view them (one or all) or remove them. Data is stored in a local MySQL database. API calls a python script responsible for creating CloudSigma VMs. Additionally an ansible playbook is called to set up the kubernetes cluster on the VMs.

Steps to go:
-check if multiple deplyments work (no resources - IPs to do that)
-connect VMs python script to the same database to add missing information such as obtained IPs
-if necessary rewrite swagger file and all dependencies in the code to match the standards

