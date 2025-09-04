{
  description = "mnu workspace";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f (import nixpkgs { inherit system; }));
    in {
      packages = forAllSystems (pkgs: {
        mnu-bw = pkgs.buildGoModule {
          pname = "mnu-bw";
          version = "unstable";
          src = ./.;
          # Replace this with the suggested hash after first nix build
vendorHash = "sha256-5sSiaeytKCRsY+XWGysMWMgNZMmtED+IRSvEVsh6FEg=";
          subPackages = [ "cmd/mnu-bw" ];
          propagatedBuildInputs = [ pkgs.bitwarden-cli ];
        };
        mnu-run = pkgs.buildGoModule {
          pname = "mnu-run";
          version = "unstable";
          src = ./.;
vendorHash = "sha256-5sSiaeytKCRsY+XWGysMWMgNZMmtED+IRSvEVsh6FEg=";
          subPackages = [ "cmd/mnu-run" ];
        };
        mnu-drun = pkgs.buildGoModule {
          pname = "mnu-drun";
          version = "unstable";
          src = ./.;
vendorHash = "sha256-5sSiaeytKCRsY+XWGysMWMgNZMmtED+IRSvEVsh6FEg=";
          subPackages = [ "cmd/mnu-drun" ];
        };
        # A convenience package that contains all three binaries in one output
        default = pkgs.symlinkJoin {
          name = "mnu";
          paths = [ self.packages.${pkgs.system}.mnu-bw self.packages.${pkgs.system}.mnu-run self.packages.${pkgs.system}.mnu-drun ];
        };
      });

      apps = forAllSystems (pkgs: {
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
        default = {
          type = "app";
          program = "${self.packages.${pkgs.system}.mnu-bw}/bin/mnu-bw";
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          buildInputs = [ pkgs.go pkgs.gh pkgs.bitwarden-cli ];
        };
      });
    };
}
