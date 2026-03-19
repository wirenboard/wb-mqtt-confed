.PHONY: all clean test install

PREFIX = /usr
DEB_TARGET_ARCH ?= armhf
WBGO_LOCAL_PATH ?= .

ifeq ($(DEB_TARGET_ARCH),armhf)
GO_ENV := GOARCH=arm GOARM=6 CC_FOR_TARGET=arm-linux-gnueabihf-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),arm64)
GO_ENV := GOARCH=arm64 CC_FOR_TARGET=aarch64-linux-gnu-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),amd64)
GO_ENV := GOARCH=amd64
endif

GO ?= go
GOTEST ?= $(GO) test
GCFLAGS :=
LDFLAGS := -X main.version=`git describe --tags --always --dirty`
GO_FLAGS := -buildvcs=false
GO_TEST_FLAGS = -v -cover -tags test

ifeq ($(DEBUG),)
	LDFLAGS += -s -w
	GO_FLAGS += -trimpath
else
	GCFLAGS += -N -l
endif

GO_FLAGS += $(if $(GCFLAGS),-gcflags=all="$(GCFLAGS)") $(if $(LDFLAGS),-ldflags="$(LDFLAGS)")

all: clean wb-mqtt-confed

clean:
	rm -rf wb-mqtt-confed

amd64:
	$(MAKE) DEB_TARGET_ARCH=amd64

test:
	cp $(WBGO_LOCAL_PATH)/amd64.wbgo.so confed/wbgo.so
	$(GOTEST) $(GO_FLAGS) $(GO_TEST_FLAGS) ./confed

wb-mqtt-confed: main.go confed/*.go
	$(GO_ENV) $(GO) build $(GO_FLAGS)

install:
	mkdir -p $(DESTDIR)/var/lib/wb-mqtt-confed/schemas
	install -Dm0644 confed/interfaces.schema.json -t $(DESTDIR)$(PREFIX)/share/wb-mqtt-confed/schemas
	install -Dm0644 confed/ntp.schema.json -t $(DESTDIR)$(PREFIX)/share/wb-mqtt-confed/schemas
	install -Dm0755 wb-mqtt-confed -t $(DESTDIR)$(PREFIX)/bin
	install -Dm0644 $(WBGO_LOCAL_PATH)/$(DEB_TARGET_ARCH).wbgo.so $(DESTDIR)$(PREFIX)/lib/wb-mqtt-confed/wbgo.so
	install -Dm0755 networkparser -t $(DESTDIR)$(PREFIX)/lib/wb-mqtt-confed/parsers
	install -Dm0755 ntpparser -t $(DESTDIR)$(PREFIX)/lib/wb-mqtt-confed/parsers
	install -Dm0644 wb-mqtt-confed.wbconfigs $(DESTDIR)/etc/wb-configs.d/15wb-mqtt-confed
