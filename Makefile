.PHONY: all clean

PREFIX = /usr
DEB_TARGET_ARCH ?= armhf

ifeq ($(DEB_TARGET_ARCH),armel)
GO_ENV := GOARCH=arm GOARM=5 CC_FOR_TARGET=arm-linux-gnueabi-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),armhf)
GO_ENV := GOARCH=arm GOARM=6 CC_FOR_TARGET=arm-linux-gnueabihf-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),arm64)
GO_ENV := GOARCH=arm64 CC_FOR_TARGET=aarch64-linux-gnu-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),amd64)
GO_ENV := GOARCH=amd64 CC=x86_64-linux-gnu-gcc
endif
ifeq ($(DEB_TARGET_ARCH),i386)
GO_ENV := GOARCH=386 CC=i586-linux-gnu-gcc
endif

GO ?= go
GO_FLAGS = -ldflags "-s -w -X main.version=`git describe --tags --always --dirty`"

all: clean wb-mqtt-confed

clean:
	rm -rf wb-mqtt-confed

amd64:
	$(MAKE) DEB_TARGET_ARCH=amd64

test:
	cp amd64.wbgo.so confed/wbgo.so
	CC=x86_64-linux-gnu-gcc $(GO) test -v -trimpath -ldflags="-s -w" -tags test -cover ./confed

wb-mqtt-confed: main.go confed/*.go
	$(GO_ENV) $(GO) build -trimpath $(GO_FLAGS)

install:
	mkdir -p $(DESTDIR)/var/lib/wb-mqtt-confed/schemas
	install -Dm0644 confed/interfaces.schema.json -t $(DESTDIR)$(PREFIX)/share/wb-mqtt-confed/schemas
	install -Dm0644 confed/ntp.schema.json -t $(DESTDIR)$(PREFIX)/share/wb-mqtt-confed/schemas
	install -Dm0755 wb-mqtt-confed -t $(DESTDIR)$(PREFIX)/bin
	install -Dm0644 $(DEB_TARGET_ARCH).wbgo.so $(DESTDIR)$(PREFIX)/lib/wb-mqtt-confed/wbgo.so
	install -Dm0755 networkparser -t $(DESTDIR)$(PREFIX)/lib/wb-mqtt-confed/parsers
	install -Dm0755 ntpparser -t $(DESTDIR)$(PREFIX)/lib/wb-mqtt-confed/parsers
	install -Dm0644 wb-mqtt-confed.wbconfigs $(DESTDIR)/etc/wb-configs.d/15wb-mqtt-confed

deb:
	$(GO_ENV) dpkg-buildpackage -b -a$(DEB_TARGET_ARCH) -us -uc
