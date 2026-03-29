ARCH ?= $(shell uname -m)
OS_VER ?= 13.0

all:
	swiftc -emit-library -static -target $(ARCH)-apple-macosx$(OS_VER) AppUseSnapshot.swift -o libappuse_bridge.a

clean:
	rm -f *.a *.dylib
