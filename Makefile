SHELL ?= /bin/bash

.DEFAULT_GOAL := build

################################################################################
# Version details                                                              #
################################################################################

# This will reliably return the short SHA1 of HEAD or, if the working directory
# is dirty, will return that + "-dirty"
GIT_VERSION = $(shell git describe --always --abbrev=7 --dirty --match=NeVeRmAtCh)

################################################################################
# Containerized development environment-- or lack thereof                      #
################################################################################

ifneq ($(SKIP_DOCKER),true)
	PROJECT_ROOT := $(dir $(realpath $(firstword $(MAKEFILE_LIST))))
	GO_DEV_IMAGE := brigadecore/go-tools:v0.8.0

	GO_DOCKER_CMD := docker run \
		-it \
		--rm \
		-e SKIP_DOCKER=true \
		-e GITHUB_TOKEN=$${GITHUB_TOKEN} \
		-e GOCACHE=/workspaces/brigade-cloudevents-gateway/.gocache \
		-v $(PROJECT_ROOT):/workspaces/brigade-cloudevents-gateway \
		-w /workspaces/brigade-cloudevents-gateway \
		$(GO_DEV_IMAGE)

	HELM_IMAGE := brigadecore/helm-tools:v0.4.0

	HELM_DOCKER_CMD := docker run \
	  -it \
		--rm \
		-e SKIP_DOCKER=true \
		-e HELM_PASSWORD=$${HELM_PASSWORD} \
		-v $(PROJECT_ROOT):/workspaces/brigade-cloudevents-gateway \
		-w /workspaces/brigade-cloudevents-gateway \
		$(HELM_IMAGE)
endif

################################################################################
# Binaries and Docker images we build and publish                              #
################################################################################

ifdef DOCKER_REGISTRY
	DOCKER_REGISTRY := $(DOCKER_REGISTRY)/
endif

ifdef DOCKER_ORG
	DOCKER_ORG := $(DOCKER_ORG)/
endif

DOCKER_IMAGE_NAME := $(DOCKER_REGISTRY)$(DOCKER_ORG)brigade-cloudevents-gateway

ifdef HELM_REGISTRY
	HELM_REGISTRY := $(HELM_REGISTRY)/
endif

ifdef HELM_ORG
	HELM_ORG := $(HELM_ORG)/
endif

HELM_CHART_NAME := $(HELM_REGISTRY)$(HELM_ORG)brigade-cloudevents-gateway

ifdef VERSION
	MUTABLE_DOCKER_TAG := latest
else
	VERSION            := $(GIT_VERSION)
	MUTABLE_DOCKER_TAG := edge
endif

IMMUTABLE_DOCKER_TAG := $(VERSION)

################################################################################
# Tests                                                                        #
################################################################################

.PHONY: lint
lint:
	$(GO_DOCKER_CMD) sh -c ' \
		golangci-lint run --config golangci.yaml \
	'

.PHONY: test-unit
test-unit:
	$(GO_DOCKER_CMD) sh -c ' \
		go test \
			-v \
			-timeout=60s \
			-race \
			-coverprofile=coverage.txt \
			-covermode=atomic \
			./... \
	'

.PHONY: lint-chart
lint-chart:
	$(HELM_DOCKER_CMD) sh -c ' \
		cd charts/brigade-cloudevents-gateway && \
		helm dep up && \
		helm lint . \
	'

################################################################################
# Upload Code Coverage Reports                                                 #
################################################################################

.PHONY: upload-code-coverage
upload-code-coverage:
	$(GO_DOCKER_CMD) codecov

################################################################################
# Build                                                                        #
################################################################################

.PHONY: build
build:
	docker buildx build \
		-t $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG) \
		-t $(DOCKER_IMAGE_NAME):$(MUTABLE_DOCKER_TAG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(GIT_VERSION) \
		--platform linux/amd64,linux/arm64 \
		.

################################################################################
# Image security                                                               #
################################################################################

.PHONY: scan
scan:
	grype $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG) -f medium

.PHONY: generate-sbom
generate-sbom:
	syft $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG) \
		-o spdx-json \
		--file ./artifacts/brigade-cloudevents-gateway-$(VERSION)-SBOM.json

.PHONY: publish-sbom
publish-sbom: generate-sbom
	ghr \
		-u $(GITHUB_ORG) \
		-r $(GITHUB_REPO) \
		-c $$(git rev-parse HEAD) \
		-t $${GITHUB_TOKEN} \
		-n ${VERSION} \
		${VERSION} ./artifacts/brigade-cloudevents-gateway-$(VERSION)-SBOM.json

################################################################################
# Publish                                                                      #
################################################################################

.PHONY: publish
publish: push publish-chart

.PHONY: push
push:
	docker buildx build \
		-t $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG) \
		-t $(DOCKER_IMAGE_NAME):$(MUTABLE_DOCKER_TAG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(GIT_VERSION) \
		--platform linux/amd64,linux/arm64 \
		--push \
		.

.PHONY: sign
sign:
	docker pull $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG)
	docker pull $(DOCKER_IMAGE_NAME):$(MUTABLE_DOCKER_TAG)
	docker trust sign $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG)
	docker trust sign $(DOCKER_IMAGE_NAME):$(MUTABLE_DOCKER_TAG)
	docker trust inspect --pretty $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG)
	docker trust inspect --pretty $(DOCKER_IMAGE_NAME):$(MUTABLE_DOCKER_TAG)

.PHONY: publish-chart
publish-chart:
	$(HELM_DOCKER_CMD) sh	-c ' \
		helm registry login $(HELM_REGISTRY) -u $(HELM_USERNAME) -p $${HELM_PASSWORD} && \
		cd charts/brigade-cloudevents-gateway && \
		helm dep up && \
		helm package . --version $(VERSION) --app-version $(VERSION) && \
		helm push brigade-cloudevents-gateway-$(VERSION).tgz oci://$(HELM_REGISTRY)$(HELM_ORG) \
	'

################################################################################
# Targets to facilitate hacking on Brigade CloudEvents Gateway.                  #
################################################################################

.PHONY: hack-kind-up
hack-kind-up:
	ctlptl apply -f hack/kind/cluster.yaml
	HELM_EXPERIMENTAL_OCI=1 helm upgrade brigade \
		oci://ghcr.io/brigadecore/brigade \
		--version v2.3.1 \
		--install \
		--create-namespace \
		--namespace brigade \
		--wait \
		--timeout 300s

.PHONY: hack-kind-down
hack-kind-down:
	ctlptl delete -f hack/kind/cluster.yaml

.PHONY: hack-build
hack-build:
	docker build \
		-t $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG) \
		-t $(DOCKER_IMAGE_NAME):$(MUTABLE_DOCKER_TAG) \
		--build-arg VERSION='$(VERSION)' \
		--build-arg COMMIT='$(GIT_VERSION)' \
		.

.PHONY: hack-push
hack-push: hack-build
	docker push $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG)
	docker push $(DOCKER_IMAGE_NAME):$(MUTABLE_DOCKER_TAG)

IMAGE_PULL_POLICY ?= Always

.PHONY: hack-deploy
hack-deploy:
ifndef BRIGADE_API_TOKEN
	@echo "BRIGADE_API_TOKEN must be defined" && false
endif
	helm dep up charts/brigade-cloudevents-gateway && \
	helm upgrade brigade-cloudevents-gateway charts/brigade-cloudevents-gateway \
		--install \
		--create-namespace \
		--namespace brigade-cloudevents-gateway \
		--timeout 60s \
		--set image.repository=$(DOCKER_IMAGE_NAME) \
		--set image.tag=$(IMMUTABLE_DOCKER_TAG) \
		--set image.pullPolicy=$(IMAGE_PULL_POLICY) \
		--set brigade.apiToken=$(BRIGADE_API_TOKEN)

.PHONY: hack
hack: hack-push hack-deploy

# Convenience target for loading image into a KinD cluster
.PHONY: hack-load-image
hack-load-image:
	@echo "Loading $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG)"
	@kind load docker-image $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG) \
			|| echo >&2 "kind not installed or error loading image: $(DOCKER_IMAGE_NAME):$(IMMUTABLE_DOCKER_TAG)"
