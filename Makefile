.PHONY: all prepare clean

DEB_TARGET_ARCH ?= armel

ifeq ($(DEB_TARGET_ARCH),armel)
GO_ENV := GOARCH=arm GOARM=5 CC_FOR_TARGET=arm-linux-gnueabi-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),armhf)
GO_ENV := GOARCH=arm GOARM=6 CC_FOR_TARGET=arm-linux-gnueabihf-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),amd64)
GO_ENV := GOARCH=amd64 CC=x86_64-linux-gnu-gcc
endif
ifeq ($(DEB_TARGET_ARCH),i386)
GO_ENV := GOARCH=386 CC=i586-linux-gnu-gcc
endif

GO ?= go

all: clean wb-mqtt-confed

clean:
	rm -rf wb-mqtt-confed

amd64:
	$(MAKE) DEB_TARGET_ARCH=amd64

wb-mqtt-confed: main.go confed/*.go
	$(GO_ENV) $(GO) build -trimpath -ldflags "-w -X main.version=`git describe --tags --always --dirty`"

install:
	mkdir -p $(DESTDIR)/var/lib/wb-mqtt-confed/schemas
	install -D -m 0644 confed/interfaces.schema.json $(DESTDIR)/usr/share/wb-mqtt-confed/schemas/interfaces.schema.json
	install -D -m 0644 confed/ntp.schema.json $(DESTDIR)/usr/share/wb-mqtt-confed/schemas/ntp.schema.json
	install -D -m 0755 wb-mqtt-confed $(DESTDIR)/usr/bin/wb-mqtt-confed
	install -D -m 0644 $(DEB_TARGET_ARCH).wbgo.so $(DESTDIR)/usr/lib/wb-mqtt-confed/wbgo.so
	install -D -m 0755 ntpparser $(DESTDIR)/usr/lib/wb-mqtt-confed/parsers/ntpparser

deb:
	$(GO_ENV) dpkg-buildpackage -b -a$(DEB_TARGET_ARCH) -us -uc
