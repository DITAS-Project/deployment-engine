#!/usr/bin/env sh
echo "Setting GOPATH"
WORKDIR=$GOPATH/src/deployment-engine
mkdir $WORKDIR
echo "Copying sources"
cp -a . $WORKDIR
cd $WORKDIR
echo "Ensure dependencies"
dep ensure
cd src
echo "Building"
CGO_ENABLED=0 GOOS=linux go build -a -o deployment-engine
echo "Testing"
go test ./...
echo "Copying result to workspace"
cp deployment-engine $1/src
echo "Done"