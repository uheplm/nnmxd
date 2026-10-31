APP         := nnmxd
BIN_DIR     := $(HOME)/.local/bin
UNIT_DIR    := $(HOME)/.config/systemd/user
UNIT_NAME   := nnmxd.service
UNIT_SOURCE := configs/nnmxd.service
GO          ?= go

.PHONY: build test install reinstall enable restart stop start status logs uninstall

build:
	mkdir -p bin
	$(GO) build -o bin/$(APP) ./cmd/$(APP)

test:
	$(GO) test ./...

install: build
	install -Dm0755 bin/$(APP) $(BIN_DIR)/$(APP)
	install -Dm0644 $(UNIT_SOURCE) $(UNIT_DIR)/$(UNIT_NAME)
	systemctl --user daemon-reload
	systemctl --user enable --now $(UNIT_NAME)

reinstall: build
	install -Dm0755 bin/$(APP) $(BIN_DIR)/$(APP)
	systemctl --user restart $(UNIT_NAME)

enable:
	systemctl --user enable --now $(UNIT_NAME)

restart:
	systemctl --user restart $(UNIT_NAME)

stop:
	systemctl --user stop $(UNIT_NAME)

start:
	systemctl --user start $(UNIT_NAME)

status:
	systemctl --user status $(UNIT_NAME) --no-pager

logs:
	journalctl --user -u $(UNIT_NAME) -f

uninstall:
	-systemctl --user disable --now $(UNIT_NAME)
	rm -f $(UNIT_DIR)/$(UNIT_NAME) $(BIN_DIR)/$(APP)
	systemctl --user daemon-reload
