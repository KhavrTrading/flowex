# Build the flowex C-ABI shared library.
# Output: ../flowex.<ext> + ../flowex.h at repo root.

UNAME_S := $(shell uname -s 2>/dev/null)
ifeq ($(OS),Windows_NT)
    EXT := dll
else ifeq ($(UNAME_S),Darwin)
    EXT := dylib
else
    EXT := so
endif

OUT := ../flowex.$(EXT)

.PHONY: build clean

build:
	CGO_ENABLED=1 go build -buildmode=c-shared -ldflags="-s -w" -o $(OUT) ./

clean:
	rm -f $(OUT) ../flowex.h
