#!/bin/bash
set -eou pipefail

crds=(notifiers)

echo "checking kubeconfig context"
kubectl config current-context || {
  echo "Set a context (kubectl use-context <context>) out of the following:"
  echo
  kubectl config get-contexts
  exit 1
}
echo ""

# http://redsymbol.net/articles/bash-exit-traps/
function cleanup() {
  rm -rf $ONESSL ca.crt ca.key server.crt server.key
}
trap cleanup EXIT

# ref: https://github.com/appscodelabs/libbuild/blob/master/common/lib.sh#L55
inside_git_repo() {
  git rev-parse --is-inside-work-tree >/dev/null 2>&1
  inside_git=$?
  if [ "$inside_git" -ne 0 ]; then
    echo "Not inside a git repository"
    exit 1
  fi
}

detect_tag() {
  inside_git_repo

  # http://stackoverflow.com/a/1404862/3476121
  git_tag=$(git describe --exact-match --abbrev=0 2>/dev/null || echo '')

  commit_hash=$(git rev-parse --verify HEAD)
  git_branch=$(git rev-parse --abbrev-ref HEAD)
  commit_timestamp=$(git show -s --format=%ct)

  if [ "$git_tag" != '' ]; then
    TAG=$git_tag
    TAG_STRATEGY='git_tag'
  elif [ "$git_branch" != 'master' ] && [ "$git_branch" != 'HEAD' ] && [[ "$git_branch" != release-* ]]; then
    TAG=$git_branch
    TAG_STRATEGY='git_branch'
  else
    hash_ver=$(git describe --tags --always --dirty)
    TAG="${hash_ver}"
    TAG_STRATEGY='commit_hash'
  fi

  export TAG
  export TAG_STRATEGY
  export git_tag
  export git_branch
  export commit_hash
  export commit_timestamp
}

# https://stackoverflow.com/a/677212/244009
if [ -x "$(command -v onessl)" ]; then
  export ONESSL=onessl
else
  # ref: https://stackoverflow.com/a/27776822/244009
  case "$(uname -s)" in
    Darwin)
      curl -fsSL -o onessl https://github.com/kubepack/onessl/releases/download/0.3.0/onessl-darwin-amd64
      chmod +x onessl
      export ONESSL=./onessl
      ;;

    Linux)
      curl -fsSL -o onessl https://github.com/kubepack/onessl/releases/download/0.3.0/onessl-linux-amd64
      chmod +x onessl
      export ONESSL=./onessl
      ;;

    CYGWIN* | MINGW32* | MSYS*)
      curl -fsSL -o onessl.exe https://github.com/kubepack/onessl/releases/download/0.3.0/onessl-windows-amd64.exe
      chmod +x onessl.exe
      export ONESSL=./onessl.exe
      ;;
    *)
      echo 'other OS'
      ;;
  esac
fi

# ref: https://stackoverflow.com/a/7069755/244009
# ref: https://jonalmeida.com/posts/2013/05/26/different-ways-to-implement-flags-in-bash/
# ref: http://tldp.org/LDP/abs/html/comparison-ops.html

export MESSENGER_NAMESPACE=kube-system
export MESSENGER_SERVICE_ACCOUNT=messenger-service
export MESSENGER_RUN_ON_MASTER=0
export MESSENGER_ENABLE_VALIDATING_WEBHOOK=false
export MESSENGER_DOCKER_REGISTRY=appscode
export MESSENGER_IMAGE_TAG=0.0.1
export MESSENGER_IMAGE_PULL_SECRET=
export MESSENGER_IMAGE_PULL_POLICY=IfNotPresent
export MESSENGER_ENABLE_ANALYTICS=true
export MESSENGER_UNINSTALL=0
export MESSENGER_PURGE=0

export APPSCODE_ENV=${APPSCODE_ENV:-prod}
export SCRIPT_LOCATION="curl -fsSL https://raw.githubusercontent.com/appscode/messenger/0.0.1/"
if [ "$APPSCODE_ENV" = "dev" ]; then
  detect_tag
  export SCRIPT_LOCATION="cat "
  export MESSENGER_IMAGE_TAG=$TAG
  export MESSENGER_IMAGE_PULL_POLICY=IfNotPresent
fi

KUBE_APISERVER_VERSION=$(kubectl version -o=json | $ONESSL jsonpath '{.serverVersion.gitVersion}')
$ONESSL semver --check='<1.9.0' $KUBE_APISERVER_VERSION || { export MESSENGER_ENABLE_VALIDATING_WEBHOOK=true; }

show_help() {
  echo "messenger.sh - install messenger operator"
  echo " "
  echo "messenger.sh [options]"
  echo " "
  echo "options:"
  echo "-h, --help                         show brief help"
  echo "-n, --namespace=NAMESPACE          specify namespace (default: kube-system)"
  echo "    --docker-registry              docker registry used to pull messenger images (default: appscode)"
  echo "    --image-pull-secret            name of secret used to pull messenger operator images"
  echo "    --run-on-master                run messenger operator on master"
  echo "    --enable-validating-webhook    enable/disable validating webhooks for Messenger CRDs"
  echo "    --enable-analytics             send usage events to Google Analytics (default: true)"
  echo "    --uninstall                    uninstall messenger"
  echo "    --purge                        purges messenger crd objects and crds"
}

while test $# -gt 0; do
  case "$1" in
    -h | --help)
      show_help
      exit 0
      ;;
    -n)
      shift
      if test $# -gt 0; then
        export MESSENGER_NAMESPACE=$1
      else
        echo "no namespace specified"
        exit 1
      fi
      shift
      ;;
    --namespace*)
      export MESSENGER_NAMESPACE=$(echo $1 | sed -e 's/^[^=]*=//g')
      shift
      ;;
    --docker-registry*)
      export MESSENGER_DOCKER_REGISTRY=$(echo $1 | sed -e 's/^[^=]*=//g')
      shift
      ;;
    --image-pull-secret*)
      secret=$(echo $1 | sed -e 's/^[^=]*=//g')
      export MESSENGER_IMAGE_PULL_SECRET="name: '$secret'"
      shift
      ;;
    --enable-validating-webhook*)
      val=$(echo $1 | sed -e 's/^[^=]*=//g')
      if [ "$val" = "false" ]; then
        export MESSENGER_ENABLE_VALIDATING_WEBHOOK=false
      fi
      shift
      ;;
    --enable-analytics*)
      val=$(echo $1 | sed -e 's/^[^=]*=//g')
      if [ "$val" = "false" ]; then
        export MESSENGER_ENABLE_ANALYTICS=false
      fi
      shift
      ;;
    --run-on-master)
      export MESSENGER_RUN_ON_MASTER=1
      shift
      ;;
    --uninstall)
      export MESSENGER_UNINSTALL=1
      shift
      ;;
    --purge)
      export MESSENGER_PURGE=1
      shift
      ;;
    *)
      show_help
      exit 1
      ;;
  esac
done

if [ "$MESSENGER_UNINSTALL" -eq 1 ]; then
  # delete webhooks and apiservices
  kubectl delete validatingwebhookconfiguration -l app=messenger || true
  kubectl delete mutatingwebhookconfiguration -l app=messenger || true
  kubectl delete apiservice -l app=messenger
  # delete messenger operator
  kubectl delete deployment -l app=messenger --namespace $MESSENGER_NAMESPACE
  kubectl delete service -l app=messenger --namespace $MESSENGER_NAMESPACE
  kubectl delete secret -l app=messenger --namespace $MESSENGER_NAMESPACE
  # delete RBAC objects, if --rbac flag was used.
  kubectl delete serviceaccount -l app=messenger --namespace $MESSENGER_NAMESPACE
  kubectl delete clusterrolebindings -l app=messenger
  kubectl delete clusterrole -l app=messenger
  kubectl delete rolebindings -l app=messenger --namespace $MESSENGER_NAMESPACE
  kubectl delete role -l app=messenger --namespace $MESSENGER_NAMESPACE

  echo "waiting for messenger operator pod to stop running"
  for (( ; ; )); do
    pods=($(kubectl get pods --all-namespaces -l app=messenger -o jsonpath='{range .items[*]}{.metadata.name} {end}'))
    total=${#pods[*]}
    if [ $total -eq 0 ]; then
      break
    fi
    sleep 2
  done

  # https://github.com/kubernetes/kubernetes/issues/60538
  if [ "$MESSENGER_PURGE" -eq 1 ]; then
    for crd in "${crds[@]}"; do
      pairs=($(kubectl get ${crd}.users.kubedb.com --all-namespaces -o jsonpath='{range .items[*]}{.metadata.name} {.metadata.namespace} {end}' || true))
      total=${#pairs[*]}

      # save objects
      if [ $total -gt 0 ]; then
        echo "dumping ${crd} objects into ${crd}.yaml"
        kubectl get ${crd}.users.kubedb.com --all-namespaces -o yaml >${crd}.yaml
      fi

      for ((i = 0; i < $total; i += 2)); do
        name=${pairs[$i]}
        namespace=${pairs[$i + 1]}
        # delete crd object
        echo "deleting ${crd} $namespace/$name"
        kubectl delete ${crd}.users.kubedb.com $name -n $namespace
      done

      # delete crd
      kubectl delete crd ${crd}.users.kubedb.com || true
    done

    # delete user roles
    kubectl delete clusterroles appscode:messenger:edit appscode:messenger:view
  fi

  echo
  echo "Successfully uninstalled Messenger!"
  exit 0
fi

echo "checking whether extended apiserver feature is enabled"
$ONESSL has-keys configmap --namespace=kube-system --keys=requestheader-client-ca-file extension-apiserver-authentication || {
  echo "Set --requestheader-client-ca-file flag on Kubernetes apiserver"
  exit 1
}
echo ""

export KUBE_CA=
if [ "$MESSENGER_ENABLE_VALIDATING_WEBHOOK" = true ]; then
  $ONESSL get kube-ca >/dev/null 2>&1 || {
    echo "Admission webhooks can't be used when kube apiserver is accesible without verifying its TLS certificate (insecure-skip-tls-verify : true)."
    echo
    exit 1
  }
  export KUBE_CA=$($ONESSL get kube-ca | $ONESSL base64)
fi

env | sort | grep MESSENGER*
echo ""

# create necessary TLS certificates:
# - a local CA key and cert
# - a webhook server key and cert signed by the local CA
$ONESSL create ca-cert
$ONESSL create server-cert server --domains=messenger-service.$MESSENGER_NAMESPACE.svc
export SERVICE_SERVING_CERT_CA=$(cat ca.crt | $ONESSL base64)
export TLS_SERVING_CERT=$(cat server.crt | $ONESSL base64)
export TLS_SERVING_KEY=$(cat server.key | $ONESSL base64)

${SCRIPT_LOCATION}hack/deploy/operator.yaml | $ONESSL envsubst | kubectl apply -f -

# create rbac objects
${SCRIPT_LOCATION}hack/deploy/service-account.yaml | $ONESSL envsubst | kubectl apply -f -
${SCRIPT_LOCATION}hack/deploy/rbac-list.yaml | $ONESSL envsubst | kubectl auth reconcile -f -
${SCRIPT_LOCATION}hack/deploy/user-roles.yaml | $ONESSL envsubst | kubectl auth reconcile -f -

if [ "$MESSENGER_RUN_ON_MASTER" -eq 1 ]; then
  kubectl patch deploy messenger-service -n $MESSENGER_NAMESPACE \
    --patch="$(${SCRIPT_LOCATION}hack/deploy/run-on-master.yaml)"
fi

if [ "$MESSENGER_ENABLE_VALIDATING_WEBHOOK" = true ]; then
  ${SCRIPT_LOCATION}hack/deploy/apiservices.yaml | $ONESSL envsubst | kubectl apply -f -
fi
if [ "$MESSENGER_ENABLE_VALIDATING_WEBHOOK" = true ]; then
  ${SCRIPT_LOCATION}hack/deploy/validating-webhook.yaml | $ONESSL envsubst | kubectl apply -f -
fi

echo
echo "waiting until messenger operator deployment is ready"
$ONESSL wait-until-ready deployment messenger-service --namespace $MESSENGER_NAMESPACE || {
  echo "Messenger operator deployment failed to be ready"
  exit 1
}

if [ "$MESSENGER_ENABLE_VALIDATING_WEBHOOK" = true ]; then
  echo "waiting until messenger apiservice is available"
  $ONESSL wait-until-ready apiservice v1alpha1.admission.users.kubedb.com || {
    echo "Messenger apiservice failed to be ready"
    exit 1
  }
fi

echo "waiting until messenger crds are ready"
for crd in "${crds[@]}"; do
  $ONESSL wait-until-ready crd ${crd}.users.kubedb.com || {
    echo "$crd crd failed to be ready"
    exit 1
  }
done

echo
echo "Successfully installed Messenger in $MESSENGER_NAMESPACE namespace!"
