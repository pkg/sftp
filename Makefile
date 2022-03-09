include golang.mk
.DEFAULT_GOAL := install_deps # override default goal set in library makefile

SHELL := /bin/bash
PKG := github.com/Clever/sftp
PKGS := $(shell go list ./... | grep -v /vendor)
EXECUTABLE := $(shell basename $(PKG))
.PHONY: build vendor $(PKGS) $(SCRIPTS)

install_deps:
	go mod vendor

# builds every Go script found in scripts/. prefix is to prevent overlap w/ $(PKGS)
SCRIPTS :=  $(addprefix script/, $(shell go list ./... | grep /scripts))
$(SCRIPTS):
	go build -o bin/$(shell basename $@) $(@:script/%=%)

build: $(SCRIPTS)
	$(call golang-build,$(PKG),$(EXECUTABLE))

$(PKGS): golang-test-all-strict-deps
	$(call golang-test-all-strict,$@)
