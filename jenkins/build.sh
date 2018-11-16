#!/usr/bin/env sh

echo_time() {
    date +"%T $*"
}

echo_time "Setting GOPATH"
XDG_CACHE_HOME=/tmp/.cache
MODULE_NAME=/tmp/deployment-engine
mkdir $MODULE_NAME
#WORKDIR=$GOPATH/src/deployment-engine
#mkdir $WORKDIR
echo_time "Copying sources"
cp -a . $MODULE_NAME
cd $MODULE_NAME
# echo_time "Ensure dependencies"
# dep ensure
#cd src
echo_time "Building"
CGO_ENABLED=0 GOOS=linux go build -a -o deployment-engine
echo_time "Testing"
go test ./...
echo_time "Copying result to workspace"
cp deployment-engine $1
echo_time "Done"