{
  description = "mnu workspace";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    gomod2nix.url = "github:nix-community/gomod2nix";
    gomod2nix.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, gomod2nix }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system:
        let
          pkgs = import nixpkgs {
            inherit system;
              overlays = [
              gomod2nix.overlays.default
            ];
          };
          #pkgs = nixpkgs.legacyPackages.${system};
          #g = gomod2nix.legacyPackages.${system};
          #buildGoApplication = g.buildGoApplication;
          buildGoApplication = gomod2nix.legacyPackages.${system}.buildGoApplication;
        in f { inherit pkgs buildGoApplication; }
      );
    in {
      packages = forAllSystems ({ pkgs, buildGoApplication }: {
        mnu-bw = buildGoApplication {
          pname = "mnu-bw";
          version = "unstable";
          src = ./.;
          pwd = ./.;
          modules = ./gomod2nix.toml;
          subPackages = [ "cmd/mnu-bw" ];
          propagatedBuildInputs = [ pkgs.bitwarden-cli ];
        };
        mnu-run = buildGoApplication {
          pname = "mnu-run";
          version = "unstable";
          src = ./.;
          pwd = ./.;
          modules = ./gomod2nix.toml;
          subPackages = [ "cmd/mnu-run" ];
        };
        mnu-drun = buildGoApplication {
          pname = "mnu-drun";
          version = "unstable";
          src = ./.;
          pwd = ./.;
          modules = ./gomod2nix.toml;
          subPackages = [ "cmd/mnu-drun" ];
        };
        # A convenience package that contains all three binaries in one output
        default = pkgs.symlinkJoin {
          name = "mnu";
          paths = [ self.packages.${pkgs.system}.mnu-bw self.packages.${pkgs.system}.mnu-run self.packages.${pkgs.system}.mnu-drun ];
        };
      });

      apps = forAllSystems ({ pkgs }: {
        mnu-bw = {
          type = "app";
          program = "${self.packages.${pkgs.system}.mnu-bw}/bin/mnu-bw";
        };
        mnu-run = {
          type = "app";
          program = "${self.packages.${pkgs.system}.mnu-run}/bin/mnu-run";
        };
        mnu-drun = {
          type = "app";
          program = "${self.packages.${pkgs.system}.mnu-drun}/bin/mnu-drun";
        };
      });

      devShells = forAllSystems ({ pkgs, buildGoApplication }: {
        default = pkgs.mkShell {
          buildInputs = [ pkgs.go pkgs.gh pkgs.bitwarden-cli pkgs.gomod2nix ];
        };
      });
    };
}
