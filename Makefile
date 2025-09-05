# Simple helper Makefile to regenerate Go module deps (gomod2nix) and build
# Usage:
#   make           # regen deps and build nix packages
#   make deps      # regenerate deps.nix from go.mod/go.sum
#   make go-build  # local Go build of all commands
#   make build     # nix build all packages in the flake
#   make clean     # remove local build artifacts

.PHONY: all deps build go-build clean

all: deps build

# Regenerate deps.nix using nix-community/gomod2nix without needing it installed
deps:
	nix run github:nix-community/gomod2nix -- generate

# Local Go build (useful for quick iteration)
go-build:
	go build ./cmd/mnu-bw ./cmd/mnu-run ./cmd/mnu-drun

# Nix build of all three packages
build:
	nix build .#mnu-bw .#mnu-run .#mnu-drun

clean:
	rm -f mnu-bw mnu-run mnu-drun

