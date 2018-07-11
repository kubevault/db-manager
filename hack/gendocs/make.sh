#!/usr/bin/env bash

pushd $GOPATH/src/github.com/kubedb/user-manager/hack/gendocs
go run main.go
popd
