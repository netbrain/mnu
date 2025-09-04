# mnu

mnu is a small family of terminal tools:

- mnu-bw: Bitwarden TUI for searching, copying credentials, and OTP.
- mnu-run: launcher for executables on PATH (detached).
- mnu-drun: launcher for desktop-entry (.desktop) applications discovered via XDG (detached).

All three are small, fast, and work well together. mnu-bw focuses on secure clipboard handling for secrets.


## Requirements

- mnu-bw
  - Bitwarden CLI (`bw`) available on PATH
  - Clipboard helpers (atotto/clipboard requirements):
    - Linux: typically `xclip` or `xsel`
    - macOS: uses pbcopy/pbpaste (built in)
    - Windows: win32 APIs
- mnu-run: no special requirements beyond a sane PATH
- mnu-drun: an XDG-compliant environment with .desktop files in XDG_DATA_HOME/DIRS


## Install

### With Nix (flakes)

This repository’s root flake builds three packages and apps:

- Build:
  - `nix build .#mnu-bw`
  - `nix build .#mnu-run`
  - `nix build .#mnu-drun`
- Run:
  - `nix run .#mnu-bw`
  - `nix run .#mnu-run`
  - `nix run .#mnu-drun`

Use in another flake:

- Add this repo as an input and reference packages `.#mnu-bw`, `.#mnu-run`, or `.#mnu-drun` from your configuration.

Install all via a single alias (optional):
- The root flake also exposes a default package that symlink-joins all three binaries.
- You can surface it as pkgs.mnu via an overlay and install that one alias, e.g.:

```
  # flake.nix
  {
    inputs = {
      nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
      mnu.url = "github:netbrain/mnu";
    };

    outputs = { self, nixpkgs, mnu, ... }:
    let
      system = "x86_64-linux";
      overlay = (final: prev: {
        # Default package of the mnu flake contains all three binaries
        mnu = mnu.packages.${final.system}.default;
      });
    in {
      nixosConfigurations.my-host = nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          ({ pkgs, ... }: {
            nixpkgs.overlays = [ overlay ];
            environment.systemPackages = [ pkgs.mnu ];
          })
        ];
      };
    };
  }
```

A development shell is provided that includes Go, GitHub CLI, and Bitwarden CLI.

### With Go

- Go 1.21+ recommended
- Install binaries:
  - `go install github.com/netbrain/mnu/cmd/mnu-bw@latest`
  - `go install github.com/netbrain/mnu/cmd/mnu-run@latest`
  - `go install github.com/netbrain/mnu/cmd/mnu-drun@latest`
- Or build locally:
  - `go build -o mnu-bw ./cmd/mnu-bw`
  - `go build -o mnu-run ./cmd/mnu-run`
  - `go build -o mnu-drun ./cmd/mnu-drun`


## Usage

- Bitwarden TUI (single instance):
  - `mnu-bw`
  - Subcommands:
    - `mnu-bw serve` (pre-warm and advertise `bw serve`)
    - `mnu-bw clear-clipboard <seconds> <unique_id> < content` (internal helper; not for direct use)
- PATH launcher:
  - `mnu-run`
- Desktop-entry launcher:
  - `mnu-drun`

Keybindings (TUI):
- Global: Ctrl-C to quit; Esc to clear search or back out
- Search/List: type to filter; Up/Down (or Ctrl-J/Ctrl-K) to navigate; Enter to select
- Action menu: Up/Down to navigate; Enter to execute action; Esc to go back


## Configuration (mnu-bw)

mnu-bw reads configuration from `~/.config/mnu/config.yaml`. If missing, a default is created.

Defaults:

```
clipboard_timeout: 15s
api_mode: true
```

- `clipboard_timeout`: how long clipboard content remains before being cleared (Go duration, e.g., 10s, 30s, 2m)
- `api_mode`: when true, mnu-bw orchestrates `bw serve` and talks HTTP; when false, it uses the `bw` CLI directly

Environment:
- `BW_SESSION`: if set, mnu-bw will use it (no unlock prompt)
- `--debug` flag: logs to `debug.log`


## How it works (high level)

- API vs CLI
  - In `api_mode: true`, `bw serve` is started (or discovered if already advertised) and used for operations.
  - In `api_mode: false`, mnu-bw shells out to the `bw` CLI for status, listing, and secret retrieval.
- Secure clipboard
  - Copy actions stream secret data to an internal helper via stdin (no secrets in argv) and schedule clipboard clearing.
  - A named pipe (FIFO) cancels any previous clearer so the newest copy “wins.”
  - The helper checks the clipboard content hash before clearing to avoid clobbering user changes.
- Runners
  - mnu-run lists executables on PATH (deduplicated) and launches selected entries in the background (detached session).
  - mnu-drun discovers .desktop files via XDG_DATA_HOME and XDG_DATA_DIRS and launches via their `Exec` lines (`/bin/sh -c` to support quoting).


## Contributing

Issues and PRs are welcome.

Potential enhancements:
- Tests for clipboard logic and timers
- Additional CLI flags and UX polish
- OSC52 clipboard fallback for remote shells


## License

MIT
