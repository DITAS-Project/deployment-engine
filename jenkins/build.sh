#!/usr/bin/env sh

echo_time() {
    date +"%R $*"
}

echo_time "Setting GOPATH"
WORKDIR=$GOPATH/src/deployment-engine
mkdir $WORKDIR
echo_time "Copying sources"
cp -a . $WORKDIR
cd $WORKDIR
echo_time "Ensure dependencies"
dep ensure
cd src
echo_time "Building"
CGO_ENABLED=0 GOOS=linux go build -a -o deployment-engine
echo_time "Testing"
go test ./...
echo_time "Copying result to workspace"
cp deployment-engine $1/src
echo_time "Done"