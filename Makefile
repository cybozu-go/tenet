include Makefile.versions
BIN_DIR := $(shell pwd)/bin
WORKFLOWS_DIR := $(shell pwd)/.github/workflows
MDBOOK := $(BIN_DIR)/mdbook

GH := $(BIN_DIR)/gh
YQ := $(BIN_DIR)/yq

# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: setup
setup: download-tools ## Setup

.PHONY: download-tools
download-tools: $(GH) $(YQ)

$(GH):
	mkdir -p $(BIN_DIR)
	wget -qO - https://github.com/cli/cli/releases/download/v$(GH_VERSION)/gh_$(GH_VERSION)_linux_amd64.tar.gz | tar -zx -O gh_$(GH_VERSION)_linux_amd64/bin/gh > $@
	chmod +x $@

$(YQ):
	mkdir -p $(BIN_DIR)
	wget -qO $@ https://github.com/mikefarah/yq/releases/download/v$(YQ_VERSION)/yq_linux_amd64
	chmod +x $@

##@ Development

.PHONY: manifests
manifests: controller-gen kustomize ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	$(KUSTOMIZE) build config/kustomize-to-helm/overlays/crds > charts/tenet/templates/generated/crds/tenet.cybozu.io_crds.yaml
	$(KUSTOMIZE) build config/kustomize-to-helm/overlays/templates > charts/tenet/templates/generated/generated.yaml

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint:
	if [ -z "$(shell which pre-commit)" ]; then pip3 install pre-commit; fi
	pre-commit install
	pre-commit run --all-files

.PHONY: crds
crds:
	mkdir -p test/crd/
	curl -fsL -o test/crd/ciliumnetworkpolicies.yaml https://github.com/cilium/cilium/raw/v$(CILIUM_VERSION)/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnetworkpolicies.yaml
	curl -fsL -o test/crd/ciliumclusterwidenetworkpolicies.yaml https://github.com/cilium/cilium/raw/v$(CILIUM_VERSION)/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumclusterwidenetworkpolicies.yaml

.PHONY: test
test: manifests generate fmt vet crds setup-envtest ## Run tests.
	source <($(SETUP_ENVTEST) use -p env); \
		go test -v -count 1 -race ./controllers -ginkgo.progress -ginkgo.v -ginkgo.fail-fast -coverprofile controllers-cover.out
	source <($(SETUP_ENVTEST) use -p env); \
		go test -v -count 1 -race ./hooks -ginkgo.progress -ginkgo.v -coverprofile hooks-cover.out

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o $(BIN_DIR)/manager cmd/tenet-controller/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/tenet-controller/main.go

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

CONTROLLER_GEN = $(BIN_DIR)/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v$(CTRL_TOOLS_VERSION))

KUSTOMIZE = $(BIN_DIR)/kustomize
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.

$(KUSTOMIZE):
	mkdir -p $(BIN_DIR)
	curl -fsL https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv$(KUSTOMIZE_VERSION)/kustomize_v$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz | \
	tar -C $(BIN_DIR) -xzf -

HELM := $(BIN_DIR)/helm
.PHONY: helm
helm: $(HELM) ## Download helm locally if necessary.

$(HELM):
	mkdir -p $(BIN_DIR)
	curl -L -sS https://get.helm.sh/helm-v$(HELM_VERSION)-linux-amd64.tar.gz \
	  | tar xz -C $(BIN_DIR) --strip-components 1 linux-amd64/helm

##@ Maintenance
.PHONY: login-gh
login-gh: ## Login to GitHub
	if ! $(GH) auth status 2>/dev/null; then \
		echo; \
		echo '!! You need login to GitHub to proceed. Please follow the next command with "Authenticate Git with your GitHub credentials? (Y)".'; \
		echo; \
		$(GH) auth login -h github.com -p HTTPS -w; \
	fi

.PHONY: logout-gh
logout-gh: ## Logout from GitHub
	$(GH) auth logout

.PHONY: version
version: login-gh update-kustomize-version ## Update dependent versions
	$(call update-version,actions/checkout,ACTIONS_CHECKOUT_VERSION,1)
	$(call update-version,actions/download-artifact,ACTIONS_DOWNLOAD_ARTIFACT_VERSION,1)
	$(call update-version,actions/setup-go,ACTIONS_SETUP_GO_VERSION,1)
	$(call update-version,actions/setup-python,ACTIONS_SETUP_PYTHON_VERSION,1)
	$(call update-version,actions/upload-artifact,ACTIONS_UPLOAD_ARTIFACT_VERSION,1)
	$(call update-version,azure/setup-helm,AZURE_SETUP_HELM_VERSION,1)
	$(call update-version,GoogleContainerTools/container-structure-test,CST_VERSION)
	$(call update-version,docker/login-action,DOCKER_LOGIN_VERSION,1)
	$(call update-version,docker/setup-buildx-action,DOCKER_SETUP_BUILDX_VERSION,1)
	$(call update-version,docker/setup-qemu-action,DOCKER_SETUP_QEMU_VERSION,1)
	$(call update-version,goreleaser/goreleaser,GORELEASER_VERSION)
	$(call update-version,goreleaser/goreleaser-action,GORELEASER_ACTION_VERSION)
	$(call update-version,helm/chart-testing-action,HELM_CHART_TESTING_VESRION)
	$(call update-hash,helm/chart-testing-action,HELM_CHART_TESTING_HASH)
	$(call update-version,helm/kind-action,HELM_KIND_VERSION)
	$(call update-hash,helm/kind-action,HELM_KIND_HASH)
	$(call update-version,helm/helm,HELM_VERSION)
	$(call update-version,kubernetes-sigs/kind,KIND_VERSION)
	$(call update-version,rust-lang/mdBook,MDBOOK_VERSION)

	$(call update-version-ghcr,cert-manager,CERT_MANAGER_VERSION)
	$(call update-version-ghcr,cilium,CILIUM_VERSION)

.PHONY: update-kustomize-version
update-kustomize-version:
	$(call get-latest-gh-package-tag,argocd)
	NEW_VERSION=$$(docker run ghcr.io/cybozu/argocd:$(latest_tag) kustomize version | cut -c2-); \
	sed -i -e "s/KUSTOMIZE_VERSION := .*/KUSTOMIZE_VERSION := $${NEW_VERSION}/g" Makefile.versions

.PHONY: update-actions
update-actions:
	$(call update-trusted-action,actions/checkout,$(ACTIONS_CHECKOUT_VERSION))
	$(call update-trusted-action,actions/download-artifact,$(ACTIONS_DOWNLOAD_ARTIFACT_VERSION))
	$(call update-trusted-action,actions/setup-go,$(ACTIONS_SETUP_GO_VERSION))
	$(call update-trusted-action,actions/setup-python,$(ACTIONS_SETUP_PYTHON_VERSION))
	$(call update-trusted-action,actions/upload-artifact,$(ACTIONS_UPLOAD_ARTIFACT_VERSION))
	$(call update-trusted-action,azure/setup-helm,$(AZURE_SETUP_HELM_VERSION))
	$(call update-trusted-action,docker/login-action,$(DOCKER_LOGIN_VERSION))
	$(call update-trusted-action,docker/setup-buildx-action,$(DOCKER_SETUP_BUILDX_VERSION))
	$(call update-trusted-action,docker/setup-qemu-action,$(DOCKER_SETUP_QEMU_VERSION))
	$(call update-trusted-action,goreleaser/goreleaser-action,$(GORELEASER_ACTION_VERSION))
	$(call update-normal-action,helm/chart-testing-action,$(HELM_CHART_TESTING_VESRION),$(HELM_CHART_TESTING_HASH))
	$(call update-normal-action,helm/kind-action,$(HELM_KIND_VERSION),$(HELM_KIND_HASH))
	$(call update-goreleaser,$(GORELEASER_VERSION))
	$(call update-helm,$(HELM_VERSION))
	$(call update-kind,$(KIND_VERSION))

.PHONY: maintenance
maintenance: ## Update dependent manifests
	$(MAKE) update-actions

.PHONY: list-actions
list-actions: ## List used GitHub Actions
	@{ for i in $(shell ls $(WORKFLOWS_DIR)); do \
		$(YQ) '.. | select(has("uses")).uses' $(WORKFLOWS_DIR)/$$i; \
	done } | sort | uniq

##@ Test
SETUP_ENVTEST = $(BIN_DIR)/setup-envtest
.PHONY: setup-envtest
setup-envtest: $(SETUP_ENVTEST) ## Download envtest-setup locally if necessary.
$(SETUP_ENVTEST):
	# see https://github.com/kubernetes-sigs/controller-runtime/tree/master/tools/setup-envtest
	GOBIN=$(BIN_DIR) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: book
book: $(MDBOOK)
	rm -rf docs/book
	cd docs; $(MDBOOK) build

$(MDBOOK):
	mkdir -p $(BIN_DIR)
	curl -fsL https://github.com/rust-lang/mdBook/releases/download/v$(MDBOOK_VERSION)/mdbook-v$(MDBOOK_VERSION)-x86_64-unknown-linux-gnu.tar.gz | tar -C $(BIN_DIR) -xzf -

CONTAINER_STRUCTURE_TEST = $(BIN_DIR)/container-structure-test
.PHONY: container-structure-test
container-structure-test: $(CONTAINER_STRUCTURE_TEST)
	$(CONTAINER_STRUCTURE_TEST) test --image ghcr.io/cybozu-go/tenet:$(shell git describe --tags --abbrev=0 --match "v*" || echo v0.0.0)-next-amd64 --config cst.yaml

$(CONTAINER_STRUCTURE_TEST):
	mkdir -p $(BIN_DIR)
	curl -fsL -o $(CONTAINER_STRUCTURE_TEST) https://storage.googleapis.com/container-structure-test/v$(CST_VERSION)/container-structure-test-linux-amd64
	chmod +x $(CONTAINER_STRUCTURE_TEST)

# usage: get-latest-gh OWNER/REPO
define get-latest-gh
	$(eval latest_gh := $(shell $(GH) release list --repo $1 | grep Latest | cut -f3))
endef

# usage: get-latest-gh-package-tag NAME
define get-latest-gh-package-tag
$(eval latest_tag := $(shell curl -sSf -H "Authorization: Bearer $(shell curl -sSf "https://ghcr.io/token?scope=repository%3Acybozu%2F$1%3Apull&service=ghcr.io" | jq -r .token)" https://ghcr.io/v2/cybozu/$1/tags/list | jq -r '.tags[]' | sort -Vr | head -n 1))
endef

# usage: get-release-hash OWNER/REPO VERSION
# do not indent because it appears on output
define get-release-hash
$(shell TEMP_DIR=$$(mktemp -d); \
git clone https://github.com/$1.git $${TEMP_DIR}; \
cd $${TEMP_DIR}; \
git rev-parse $2; \
rm -rf $${TEMP_DIR})
endef

# usage: upstream-tag 1.2.3.4
# do not indent because it appears on output
define upstream-tag
$(shell echo $1 | sed -E 's/^(.*)\.[[:digit:]]+$$/v\1/')
endef

# usage: update-version OWNER/REPO VAR MAJOR
define update-version
	$(call get-latest-gh,$1)
	NEW_VERSION=$$(echo $(latest_gh) | if [ -z "$3" ]; then cut -b 2-; else cut -b 2; fi); \
	sed -i -e "s/^$2 := .*/$2 := $${NEW_VERSION}/g" Makefile.versions
endef

# usage: update-version-ghcr NAME VAR
define update-version-ghcr
	$(call get-latest-gh-package-tag,$1)
	NEW_VERSION=$$(echo $(call upstream-tag,$(latest_tag)) | cut -b 2-); \
	sed -i -e "s/$2 := .*/$2 := $${NEW_VERSION}/g" Makefile.versions
endef

# usage: update-hash OWNER/REPO VAR
# this function must be called immediate after update-version
define update-hash
	NEW_HASH=$(call get-release-hash,$1,$(latest_gh)); \
	sed -i -e "s/$2 := .*/$2 := $${NEW_HASH}/g" Makefile.versions
endef

# usage: update-trusted-action OWNER/REPO VERSION
define update-trusted-action
	for i in $(shell ls $(WORKFLOWS_DIR)); do \
		$(YQ) -i '(.. | select(has("uses")) | select(.uses | contains("$1"))).uses = "$1@v$2"' $(WORKFLOWS_DIR)/$$i; \
	done
endef

# usage: update-normal-action OWNER/REPO VERSION HASH
define update-normal-action
	for i in $(shell ls $(WORKFLOWS_DIR)); do \
		$(YQ) -i '(.. | select(has("uses")) | select(.uses | contains("$1"))).uses = "$1@$3"' $(WORKFLOWS_DIR)/$$i; \
		$(YQ) -i '(.. | select(has("uses")) | select(.uses | contains("$1"))).uses line_comment="$2"' $(WORKFLOWS_DIR)/$$i; \
	done
endef

# usage: update-goreleaser VERSION
define update-goreleaser
	for i in $(shell ls $(WORKFLOWS_DIR)); do \
		$(YQ) -i '(.. | select(has("uses")) | select(.uses | contains("goreleaser/goreleaser-action"))).with.version = "v$1"'  $(WORKFLOWS_DIR)/$$i; \
	done
endef

# usage: update-helm VERSION
define update-helm
	for i in $(shell ls $(WORKFLOWS_DIR)); do \
		$(YQ) -i '(.. | select(has("uses")) | select(.uses | contains("azure/setup-helm"))).with.version = "v$1"'  $(WORKFLOWS_DIR)/$$i; \
	done
endef

# usage: update-kind VERSION
define update-kind
	for i in $(shell ls $(WORKFLOWS_DIR)); do \
		$(YQ) -i '(.. | select(has("uses")) | select(.uses | contains("helm/kind-action"))).with.version = "v$1"'  $(WORKFLOWS_DIR)/$$i; \
	done
endef
