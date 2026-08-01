# Local install / lifecycle for mxmoused.
#
# The daemon needs root (evdev grab + uinput), so it runs as a system unit.
# The binary lives in the user's ~/.local/bin to match configs/mxmoused.service;
# only the unit file install and systemctl operations need sudo.

BINARY      := mxmoused
BIN_DIR     := $(HOME)/.local/bin
CONFIG_DIR  := $(HOME)/.config/mxmoused
CONFIG_FILE := $(CONFIG_DIR)/config.yaml
UNIT_NAME   := mxmoused.service
UNIT_DIR    := /etc/systemd/system
UNIT_SRC    := configs/mxmoused.service
EXAMPLE_CFG := configs/mxmoused.example.yaml

GO    ?= go
SUDO  ?= sudo

.PHONY: build test install reinstall restart stop start status logs uninstall

build:
	$(GO) build -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	$(GO) test ./...

# Full install: binary + config seed + systemd unit + enable & start.
install: build
	install -Dm0755 bin/$(BINARY) $(BIN_DIR)/$(BINARY)
	install -d $(CONFIG_DIR)
	[ -f $(CONFIG_FILE) ] || install -m0644 $(EXAMPLE_CFG) $(CONFIG_FILE)
	$(SUDO) install -Dm0644 $(UNIT_SRC) $(UNIT_DIR)/$(UNIT_NAME)
	$(SUDO) systemctl daemon-reload
	$(SUDO) systemctl enable --now $(UNIT_NAME)

# Rebuild + swap binary + restart. Use this for the edit-rebuild-test loop.
reinstall: build
	install -Dm0755 bin/$(BINARY) $(BIN_DIR)/$(BINARY)
	$(SUDO) systemctl restart $(UNIT_NAME)

restart:
	$(SUDO) systemctl restart $(UNIT_NAME)

stop:
	$(SUDO) systemctl stop $(UNIT_NAME)

start:
	$(SUDO) systemctl start $(UNIT_NAME)

status:
	systemctl status $(UNIT_NAME) --no-pager

logs:
	journalctl -u $(UNIT_NAME) -f

uninstall:
	-$(SUDO) systemctl disable --now $(UNIT_NAME)
	-$(SUDO) rm -f $(UNIT_DIR)/$(UNIT_NAME)
	$(SUDO) systemctl daemon-reload
	-rm -f $(BIN_DIR)/$(BINARY)
