include ../Makefile.versions

PROJECT_NAME = tenet
BIN_DIR = $(dir $(shell pwd))/bin
KIND = $(BIN_DIR)/kind
KUBECTL = $(BIN_DIR)/kubectl
HELM = $(BIN_DIR)/helm
KUBECONFIG = $(shell pwd)/.kubeconfig
KIND_CONFIG = kind-config.yaml
export KUBECTL KUBECONFIG

.PHONY: help
help:
	@echo "Choose one of the following target"
	@echo
	@echo "start      Start kind cluster and install tenet"
	@echo "test       Run e2e tests"
	@echo "logs       Save logs as logs.tar.gz"
	@echo "stop       Stop the kind cluster"

.PHONY: start
start: $(KIND) $(KUBECTL) $(HELM)
	$(KIND) create cluster --name=$(PROJECT_NAME) --config=$(KIND_CONFIG) --image=kindest/node:v$(KUBERNETES_VERSION) --wait 1m
	cd ..; CGO_ENABLED=0 go build -o $(PROJECT_NAME) cmd/tenet-controller/main.go
	cd ..; docker build --no-cache -t $(PROJECT_NAME):dev .
	$(KIND) load docker-image $(PROJECT_NAME):dev --name=$(PROJECT_NAME)
	$(KUBECTL) apply -f https://github.com/jetstack/cert-manager/releases/download/v$(CERT_MANAGER_VERSION)/cert-manager.yaml
	$(KUBECTL) -n cert-manager wait --for=condition=available --timeout=180s --all deployments
	$(KUBECTL) apply -f https://github.com/cilium/cilium/raw/$(CILIUM_VERSION)/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnetworkpolicies.yaml
	$(HELM) install --create-namespace --namespace $(PROJECT_NAME) $(PROJECT_NAME) ../charts/$(PROJECT_NAME) -f values.yaml
	$(KUBECTL) -n $(PROJECT_NAME) wait --for=condition=available --timeout=180s --all deployments
	$(KUBECTL) -n kube-system wait --for=condition=available --timeout=180s --all deployments

.PHONY: test
test:
	env PATH=$$(pwd)/../bin:$$PATH RUN_E2E=1 \
		go test -v -race . -ginkgo.progress -ginkgo.v -ginkgo.failFast

.PHONY: logs
logs:
	rm -rf logs.tar.gz logs
	$(KIND) export logs --name=$(PROJECT_NAME) ./logs
	tar czf logs.tar.gz logs
	rm -rf logs

.PHONY: stop
stop: $(KIND) 
	$(KIND) delete cluster --name=$(PROJECT_NAME)
	-docker image rm $(PROJECT_NAME):dev
	-docker image prune -f

$(KIND):
	mkdir -p ../bin
	curl -sfL -o $@ https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-linux-amd64
	chmod a+x $@

$(KUBECTL):
	mkdir -p ../bin
	curl -sfL -o $@ https://dl.k8s.io/release/v$(KUBERNETES_VERSION)/bin/linux/amd64/kubectl
	chmod a+x $@

$(HELM):
	$(MAKE) -C .. helm
