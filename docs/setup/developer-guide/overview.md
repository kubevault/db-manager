---
title: Overview | Developer Guide
description: Developer Guide Overview
menu:
  product_messenger_0.0.1:
    identifier: developer-guide-readme
    name: Overview
    parent: developer-guide
    weight: 15
product_name: messenger
menu_name: product_messenger_0.0.1
section_menu_id: setup
---

> New to Scanner? Please start [here](/docs/concepts/README.md).

## Development Guide
This document is intended to be the canonical source of truth for things like supported toolchain versions for building Scanner. If you find a requirement that this doc does not capture, please submit an issue on github.

This document is intended to be relative to the branch in which it is found. It is guaranteed that requirements will change over time for the development branch, but release branches of Scanner should not change.

### Build Scanner
Some of the Scanner development helper scripts rely on a fairly up-to-date GNU tools environment, so most recent Linux distros should work just fine out-of-the-box.

#### Setup GO
Scanner is written in Google's GO programming language. Currently, Scanner is developed and tested on **go 1.9.2**. If you haven't set up a GO development environment, please follow [these instructions](https://golang.org/doc/code.html) to install GO.

#### Download Source

```console
$ go get github.com/kubevault/db-manager
$ cd $(go env GOPATH)/src/github.com/kubevault/db-manager
```

#### Install Dev tools
To install various dev tools for Scanner, run the following command:

```console
$ ./hack/builddeps.sh
```

#### Build Binary

```console
$ ./hack/make.py
$ messenger version
```

#### Run Binary Locally

```console
$ messenger run \
    --clair-addr=https://127.0.0.1:6060 \
    --clair-cert-dir=pki/clair \
    --secure-port=8443 \
    --kubeconfig="$HOME/.kube/config" \
    --authorization-kubeconfig="$HOME/.kube/config" \
    --authentication-kubeconfig="$HOME/.kube/config" \
    --authentication-skip-lookup \
    --enable-swagger-ui=true \
    --profiling=true
```

#### Dependency management
Scanner uses [Glide](https://github.com/Masterminds/glide) to manage dependencies. Dependencies are already checked in the `vendor` folder. If you want to update/add dependencies, run:

```console
$ glide slow
```

#### Build Docker images
To build and push your custom Docker image, follow the steps below. To release a new version of Scanner, please follow the [release guide](/docs/setup/developer-guide/release.md).

```console
# Build Docker image
$ ./hack/docker/setup.sh; ./hack/docker/setup.sh push

# Add docker tag for your repository
$ docker tag kubeci/messenger:<tag> <image>:<tag>

# Push Image
$ docker push <image>:<tag>
```

#### Generate CLI Reference Docs

```console
$ ./hack/gendocs/make.sh
```

### Testing Scanner
#### Unit tests

```console
$ ./hack/make.py test unit
```

#### Run e2e tests
Scanner uses [Ginkgo](http://onsi.github.io/ginkgo/) to run e2e tests.

```console
$ ./hack/make.py test e2e
```

To run e2e tests against remote backends, you need to set cloud provider credentials in `./hack/config/.env`. You can see an example file in `./hack/config/.env.example`.
