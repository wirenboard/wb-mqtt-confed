.PHONY: all prepare clean

DEB_TARGET_ARCH ?= armel

ifeq ($(DEB_TARGET_ARCH),armel)
GO_ENV := GOARCH=arm GOARM=5 CC_FOR_TARGET=arm-linux-gnueabi-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),armhf)
GO_ENV := GOARCH=arm GOARM=5 CC_FOR_TARGET=arm-linux-gnueabihf-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),amd64)
GO_ENV := GOARCH=amd64 CC=x86_64-linux-gnu-gcc
endif
ifeq ($(DEB_TARGET_ARCH),i386)
GO_ENV := GOARCH=386 CC=i586-linux-gnu-gcc
endif

all: clean wb-mqtt-confed

clean:
	rm -rf wb-mqtt-confed

amd64:
	$(MAKE) DEB_TARGET_ARCH=amd64

wb-mqtt-confed: main.go confed/*.go
	$(GO_ENV) glide install
	$(GO_ENV) go build

install:
	mkdir -p $(DESTDIR)/usr/bin/ $(DESTDIR)/etc/init.d/ $(DESTDIR)/usr/share/wb-mqtt-confed/schemas
	install -m 0644 confed/interfaces.schema.json $(DESTDIR)/usr/share/wb-mqtt-confed/schemas/interfaces.schema.json
	install -m 0644 confed/ntp.schema.json $(DESTDIR)/usr/share/wb-mqtt-confed/schemas/ntp.schema.json
	install -m 0755 wb-mqtt-confed $(DESTDIR)/usr/bin/
	install -m 0755 initscripts/wb-mqtt-confed $(DESTDIR)/etc/init.d/wb-mqtt-confed
	install -m 0755 networkparser $(DESTDIR)/usr/bin/
	install -m 0755 ntpparser $(DESTDIR)/usr/bin/

deb: prepare
	CC=arm-linux-gnueabi-gcc dpkg-buildpackage -b -aarmel -us -uc
