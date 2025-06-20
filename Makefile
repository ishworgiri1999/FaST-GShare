DOCKER_USER=docker.io/ishworgiri
ARCH=linux/amd64

GOOS?=linux
GOARCH?=amd64

CGO_ENABLED?=1
CLI_VERSION_PACKAGE := main
COMMIT ?= $(shell git describe --dirty --long --always --abbrev=15)
# VERSION := $(shell cat ./VERSION)
CGO_LDFLAGS_ALLOW := "-Wl,--unresolved-symbols=ignore-in-object-file"

GIT_COMMIT := $(COMMIT)
CLI_VERSION ?= 0.1.0

.PHONY: clean_crd_gen
clean_crd_gen:
	rm -r pkg/client && rm -r pkg/apis/fastgshare.caps.in.tum/v1/zz_generated.deepcopy.go

.PHONY: update_crd
update_crd:
	bash code-gen.sh

.PHONY: code_gen_crd
code_frist_gen_crd:
	cd .. && git clone https://github.com/kubernetes/code-generator.git && cd code-generator && git checkout release-1.23
	bash code-gen.sh


### create dummPod_container
.PHONY: dummyPod_container
dummyPod_container:
	docker build -t ${DOCKER_USER}/dummycontainer:release -f docker/dummyPod/Dockerfile .


### ------------------------ fastpod-controller-manager ----------------------- ###
### create fastpod-controller-manager and corresponding container image
CTR_MGR_OUTPUT_DIR := cmd/fastpod-controller-manager
CTR_MGR_BUILD_DIR := cmd/fastpod-controller-manager
CTR_MGR_BINARY_NAME := fastpodcontrollermanager

.PHONY: ctr_mgr_clean
ctr_mgr_clean:
	@echo "Cleaning up..."
	@rm -f $(CTR_MGR_OUTPUT_DIR)/$(CTR_MGR_BINARY_NAME)
	@echo "Clean complete."

.PHONY: fastpod-controller-manager
fastpod-controller-manager:
	@echo "Building module [fastpod-controller-manager] ..."
	@cd $(CTR_MGR_BUILD_DIR) && go build -o $(CTR_MGR_BINARY_NAME)
	@echo "Build complete. Binary is located at $(CTR_MGR_OUTPUT_DIR)/$(CTR_MGR_BINARY_NAME)"

PLATFORMS ?= linux/amd64


.PHONY: build-fastpod-controller-manager-container
build-fastpod-controller-manager-container:
	docker buildx build --platform=$(PLATFORMS) -t ${DOCKER_USER}/fastpod-controller-manager:release -f docker/fastpod-controller-manager/Dockerfile . --push



.PHONY: upload-fastpod-controller-manager-image
upload-fastpod-controller-manager-image:
	docker push ${DOCKER_USER}/fastpod-controller-manager:release 

.PHONY: clean-ctr-fastpod-controller-manager-image
clean-ctr-fastpod-controller-manager-image:
	sudo ctr -n k8s.io i ls | grep ${DOCKER_USER}/fastpod-controller-manager | awk '{print $$1}' | xargs -I {} sudo ctr -n k8s.io i rm {}


### ------------------------  fast-configurator ------------------------- ###
### create fastpod-controller-manager and corresponding container image
FAST_CONFIG_OUTPUT_DIR := cmd/fast-configurator
FAST_CONFIG_BUILD_DIR := cmd/fast-configurator
FAST_CONFIG_BINARY_NAME := fast-configurator

.PHONY: fast-configurator
fast-configurator:
	@echo "Building module [fast-configurator] ..."
	@cd $(FAST_CONFIG_BUILD_DIR) && \
	  CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files' \
	  CGO_ENABLED=1 GOOS=$(GOOS) \
	  go build \
	    -ldflags "-extldflags=-Wl,-z,lazy -s -w -X $(CLI_VERSION_PACKAGE).gitCommit=$(GIT_COMMIT) -X $(CLI_VERSION_PACKAGE).version=$(CLI_VERSION)" \
	    -o $(FAST_CONFIG_BINARY_NAME)
	@echo "Build complete. Binary is located at $(FAST_CONFIG_OUTPUT_DIR)/$(FAST_CONFIG_BINARY_NAME)"


.PHONY: build-fast-configurator-container
build-fast-configurator-container:
	docker buildx build --build-arg GOLANG_VERSION=1.24.1  --platform ${ARCH} --push -t ${DOCKER_USER}/fast-configurator:release -f docker/fast-configurator/Dockerfile .

.PHONY: upload-fast-configurator-image
upload-fast-configurator-image:
	docker push ${DOCKER_USER}/fast-configurator:release 

.PHONY: clean-ctr-fast-configurator-image
clean-ctr-fast-configurator-image:
	sudo ctr -n k8s.io i rm ${DOCKER_USER}/fast-configurator:release


# ----------------------------------------- debug fast-configurator and fastpodcontrollermanager ---------------------
.PHONY: test_fastconfigurator_fastpodcontrollermanager
test_fastconfigurator_fastpodcontrollermanager: clean-ctr-fast-configurator-image build-fast-configurator-container upload-fast-configurator-image \
clean-ctr-fastpod-controller-manager-image build-fastpod-controller-manager-container upload-fastpod-controller-manager-image



## --------------------------------------- openfaas fast-gshare dockerfile build ---------------------------------------------------
FAST_GSHARE_IMAGE_NAME := fast-gshare-faas
FAST_GSHARE_IMAGE_TAG=test
.PHONY: build-fast-gshare-faas-image
build-fast-gshare-faas-image:
	@echo "Building module [fast-gshare-faas] ..."
	docker build -t ${DOCKER_USER}/${FAST_GSHARE_IMAGE_NAME}:${FAST_GSHARE_IMAGE_TAG} -f Dockerfile .

.PHONY: upload-fast-gshare-faas-image
upload-fast-gshare-faas-image:
	docker push ${DOCKER_USER}/${FAST_GSHARE_IMAGE_NAME}:${FAST_GSHARE_IMAGE_TAG}

.PHONY: clean-fast-gshare-faas-image
clean-fast-gshare-faas-image:
	sudo ctr -n k8s.io i ls | grep -i ${FAST_GSHARE_IMAGE_NAME} | awk '{print $$1}' | xargs -I {} sudo ctr -n k8s.io i rm {} 

.PHONY: test-fast-gshare-faas-image
test-fast-gshare-faas-image: clean-fast-gshare-faas-image build-fast-gshare-faas-image upload-fast-gshare-faas-image
	




##------------------------------------ helm install the fast-gshare-fn system -------------------------------- ##
.PHONY: install-helm
install-helm:
	helm install fast-gshare ./chart/fastgshare --namespace fast-gshare --set functionNamespace=fast-gshare-fn

.PHONY: uninstall-helm
uninstall-helm:
	helm uninstall fast-gshare --namespace fast-gshare
