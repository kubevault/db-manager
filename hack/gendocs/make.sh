#!/usr/bin/env bash

pushd $GOPATH/src/github.com/kubevault/db-manager/hack/gendocs
go run main.go
popd
