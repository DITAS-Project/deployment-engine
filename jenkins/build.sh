#!/usr/bin/env sh
WORKDIR=$GOPATH/src/deployment-engine
mkdir $WORKDIR
cp -a . $WORKDIR
cd $WORKDIR
rm -rf vendor
dep ensure
cd src
CGO_ENABLED=0 GOOS=linux go build -a -o deployment-engine
go test ./...
cp deployment-engine $1/src