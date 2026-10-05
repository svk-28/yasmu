VERSION := 1.0.0
ARCH    := amd64

BIN     := bin/yasmu
DEB_DIR := build/yasmu_$(VERSION)_$(ARCH)
RPM_DIR := ~/rpmbuild

.PHONY: run build static deb rpm clean

run:
	go run main.go config.go

build:
	go build -ldflags "-s -w" -o bin/yasmu main.go config.go

static:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -ldflags "-s -w" -o bin/yasmu-static  main.go config.go

deb: build
	mkdir -p $(DEB_DIR)/DEBIAN
	mkdir -p $(DEB_DIR)/usr/bin
	mkdir -p $(DEB_DIR)/etc/yasmu
	mkdir -p $(DEB_DIR)/lib/systemd/system

	cp $(BIN) $(DEB_DIR)/usr/bin/yasmu

	./bin/yasmu -config build/config.yaml -make-config

	cp build/config.yaml $(DEB_DIR)/etc/yasmu/config.yml
	cp pkg/yasmu.service $(DEB_DIR)/lib/systemd/system/

	cp pkg/debian-control $(DEB_DIR)/DEBIAN/control
	sed -i "s/@VERSION@/$(VERSION)/" $(DEB_DIR)/DEBIAN/control

	dpkg-deb --build $(DEB_DIR)
	@echo "DEB: $(DEB_DIR).deb"


clean:
	rm -rf bin/ build/
