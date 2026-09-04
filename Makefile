APP := w4gns-logger
BUILD_DIR := bin
BUILD_APP := $(BUILD_DIR)/$(APP)
CMD_DIR := ./cmd/w4gns-logger
LOCAL_BIN_DIR ?= $(HOME)/.local/bin
LOCAL_APP := $(LOCAL_BIN_DIR)/$(APP)

.PHONY: build install test

# build always refreshes the repository binary. The installed command is a
# symlink to this file, so it immediately follows every successful rebuild.
build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_APP).tmp $(CMD_DIR)
	mv -f $(BUILD_APP).tmp $(BUILD_APP)

install: build
	@mkdir -p $(LOCAL_BIN_DIR)
	ln -sfn $(CURDIR)/$(BUILD_APP) $(LOCAL_APP)

test:
	go test ./...
	go vet ./...
