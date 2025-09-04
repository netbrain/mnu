# bwmenu

A fast, secure terminal UI for searching and copying Bitwarden credentials.

- Non-paginated action menu: copy Password, Username, URL, or OTP
- Live search filtering over all items
- Secure clipboard handling with auto-clear and countdown indicator
- Works with the Bitwarden CLI directly or via the `bw serve` HTTP API


## Features

- Item search
  - Type to filter; results update as you type.
  - Esc clears the filter first; Esc again quits.
- Action menu (per item)
  - Shows only relevant actions for the selected item: Password, Username, URL (if present), and OTP (if present).
  - All actions are shown on one screen; no pagination.
  - After copying, a subtle icon and countdown show how long until the clipboard is cleared.
- Secure clipboard
  - Copies via a short-lived subprocess that clears the clipboard after a timeout.
  - Uses a named pipe (FIFO) to cancel previous clearers if you copy again.
  - Clears the clipboard only if the content hasn’t been changed by the user in the meantime.
- Robust TOTP
  - OTP is derived from the item’s TOTP secret (if present) and is copied like any other field.
- Session aware
  - If a Bitwarden session is already available (via keychain or `BW_SESSION`), it’s used.
  - Otherwise, bwmenu prompts for your master password and unlocks Bitwarden.


## Installation

### With Nix (flakes)

This repo includes a flake that builds and runs bwmenu, and ensures the Bitwarden CLI is on PATH for the wrapped binary.

- Build:
  - nix build .#bwmenu
- Run (from this repo):
  - nix run .
- Use in another flake:
  - Add an input to your flake:
    - inputs.bwmenu.url = "github:netbrain/bwmenu"  (or a local path during development)
  - Then reference the package:
    - self.inputs.bwmenu.packages.${system}.bwmenu

The dev shell includes Go, GitHub CLI, and bitwarden-cli:
- nix develop

#### NixOS (flakes) declarative install

Option A: reference the package directly in your NixOS configuration

  # flake.nix
  {
    description = "My host";

    inputs = {
      nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
      bwmenu.url = "github:netbrain/bwmenu"; # or a local path: path:../bwmenu
    };

    outputs = { self, nixpkgs, bwmenu, ... }@inputs: let
      system = "x86_64-linux";
      lib = nixpkgs.lib;
    in {
      nixosConfigurations.my-host = lib.nixosSystem {
        inherit system;
        modules = [
          ({ pkgs, ... }: {
            environment.systemPackages = [
              bwmenu.packages.${system}.bwmenu
            ];
          })
        ];
      };
    };
  }

Option B: expose it via an overlay and install as pkgs.bwmenu

  # flake.nix
  {
    inputs = {
      nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
      bwmenu.url = "github:netbrain/bwmenu";
    };

    outputs = { self, nixpkgs, bwmenu, ... }: let
      system = "x86_64-linux";
      overlays = [ (final: prev: { bwmenu = bwmenu.packages.${final.system}.bwmenu; }) ];
    in {
      nixosConfigurations.my-host = nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          ({ pkgs, ... }: {
            nixpkgs.overlays = overlays;
            environment.systemPackages = [ pkgs.bwmenu ];
          })
        ];
      };
    };
  }

Home Manager (flakes) example

If you manage user packages via Home Manager:

  # flake.nix
  {
    inputs = {
      nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
      home-manager.url = "github:nix-community/home-manager";
      bwmenu.url = "github:netbrain/bwmenu";
    };

    outputs = { self, nixpkgs, home-manager, bwmenu, ... }: let
      system = "x86_64-linux";
    in {
      homeConfigurations.user = home-manager.lib.homeManagerConfiguration {
        inherit system;
        pkgs = import nixpkgs { inherit system; };
        modules = [
          ({ pkgs, ... }: {
            home.packages = [ bwmenu.packages.${pkgs.system}.bwmenu ];
          })
        ];
      };
    };
  }

Notes
- The wrapped bwmenu binary places bitwarden-cli (bw) on PATH automatically; you do not need to add bw separately.
- Replace x86_64-linux with your system (e.g., aarch64-linux) as appropriate.
- For local development, you can use a path input: bwmenu.url = "path:../bwmenu".

### With Go

- go 1.21+ recommended
- Install:
  - go install github.com/netbrain/bwmenu@latest
- Or build locally from this repo:
  - go build ./...


## Requirements

- Bitwarden CLI (bw)
  - On Nix: provided via the package wrapper.
  - Otherwise: install from Bitwarden; ensure `bw` is in PATH.
- Clipboard helpers
  - Linux: atotto/clipboard typically requires `xclip` or `xsel` in PATH.
  - macOS: uses pbcopy/pbpaste (built-in).
  - Windows: supported via win32 APIs.


## Usage

- Start the TUI (single instance):
  - bwmenu
  - If another bwmenu TUI is already running, a new invocation exits immediately.
- Pre-warm Bitwarden API server in the background for faster startups:
  - bwmenu serve    # starts a background advertiser for `bw serve`; run it with & to keep it in the background
- App runner (search PATH commands and execute):
  - bwmenu apps     # lists executables found in PATH; Enter to run

- Keybindings (default):
  - Global
    - Ctrl-C: quit
    - Esc: clear search if non-empty; otherwise quit
  - Search/List
    - Type to filter items
    - Up/Down, Ctrl-J/Ctrl-K: navigate items
    - Enter: open the action menu for the selected item
  - Action menu
    - Up/Down, Ctrl-J/Ctrl-K: navigate actions
    - Enter: copy the selected action’s value
    - Esc: go back to the item list


## Configuration

bwmenu reads configuration from `~/.config/bwmenu/config.yaml`. If no file is found, it writes a default one.

- Defaults (applied and written when missing):
  - clipboard_timeout: 15s
  - api_mode: true

- Example `~/.config/bwmenu/config.yaml`:

  clipboard_timeout: 30s
  api_mode: true

- Options
  - clipboard_timeout
    - How long the copied value remains on the clipboard before being cleared.
    - Accepts Go duration strings (e.g., 10s, 45s, 2m).
  - api_mode
    - When true, bwmenu orchestrates `bw serve` and communicates over HTTP.
    - When false, bwmenu uses the `bw` CLI directly for all operations.

Environment
- BW_SESSION: if set, bwmenu will use it (and not prompt for password).
- --debug: verbose logs to `debug.log`.


## How it works (high level)

- Startup
  - Loads config (creates a default file if missing).
  - If `api_mode: true`, starts a `bw serve` subprocess (internal/serve) and talks to it over HTTP.
  - Otherwise, invokes the `bw` CLI for status, listing items, and retrieving secrets.
  - Reuses an existing Bitwarden session from the keychain or `BW_SESSION` when available; otherwise prompts for master password and unlocks Bitwarden.

- UI
  - Built with Bubble Tea/Bubbles and Lip Gloss for styling and input fields sized to the terminal.
  - Search input and item list on the main screen; action menu shows all copy actions for the selected item.

- Clipboard security
  - Copying spawns a helper subprocess of bwmenu with a hidden `clear-clipboard` subcommand.
  - The secret is streamed via stdin to avoid leaking in arguments.
  - The helper schedules a clear after the configured timeout but checks the clipboard content hash first to avoid overwriting user changes.
  - A named pipe allows canceling an in-flight clearer if you copy again (so the newest copy wins).
  - The UI shows an icon and per-item countdown until clearing; multiple copies are disambiguated with a generation token.


## Troubleshooting

- "bw not found": install Bitwarden CLI and ensure it’s on PATH. With Nix, it’s wrapped automatically.
- Clipboard errors on Linux: make sure `xclip` or `xsel` is installed and available in PATH.
- Not seeing OTP: only items with a TOTP secret expose the OTP action.
- No Username/Password actions: those actions appear only when the selected item has that field.
- API mode issues: try setting `api_mode: false` to use the CLI directly, or ensure `bw serve` runs locally without errors.
- Session prompts too often: verify that `BW_SESSION` is exported and/or the session key can be stored/retrieved by the keychain backend.


## Contributing

Issues and pull requests are welcome. Ideas for future improvements:
- OSC52 clipboard fallback for SSH/tmux environments
- Tests around clipboard management and timer logic
- More item details and additional actions
- Subtle progress indicators or fade-out effects for the countdown


## License

MIT
