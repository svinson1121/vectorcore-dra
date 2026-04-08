.PHONY: ui build test clean dev-ui all install uninstall

BINARY=dra
PREFIX=/opt/vectorcore
BINDIR=$(PREFIX)/bin
ETCDIR=$(PREFIX)/etc
LOGDIR=$(PREFIX)/log
SYSTEMD=/lib/systemd/system/


all: ui build

# Build the React UI (required before `make build`)
ui:
	cd web && ([ -f package-lock.json ] && npm ci || npm install) && npm run build

# Build the Go binary (includes embedded UI if web/dist exists)
build:
	go build -o bin/dra ./cmd/dra

# Run tests
test:
	go test ./...

# Start Vite dev server (proxies API to localhost:8080)
dev-ui:
	cd web && npm run dev

clean:
	rm -rf bin/ web/dist/

install: build
	install -d $(BINDIR)
	install -d $(ETCDIR)
	install -d $(LOGDIR)

	install -m755 bin/$(BINARY) $(BINDIR)/$(BINARY)

	if [ ! -f $(ETCDIR)/hss.yaml ]; then \
		install -m644 config/dra.yaml $(ETCDIR)/dra.yaml; \
	fi

	touch $(LOGDIR)/dra.log
	chmod 644 $(LOGDIR)/dra.log

	install -m644 systemd/vectorcore-dra.service $(SYSTEMD)/vectorcore-dra.service

	systemctl daemon-reload
	systemctl enable vectorcore-dra
	systemctl start vectorcore-dra


uninstall:
	systemctl stop vectorcore-dra || true
	systemctl disable vectorcore-dra || true

	rm -f $(BINDIR)/$(BINARY)
	rm -f $(SYSTEMD)/vectorcore-dra.service

	systemctl daemon-reload

