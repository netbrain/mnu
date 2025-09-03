{
  description = "bwmenu - Bitwarden TUI menu";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
      lib = pkgs.lib;
    in {
      packages.${system} = {
        bwmenu = pkgs.buildGoModule {
          pname = "bwmenu";
          version = "unstable";
          src = ./.;
          vendorHash = "sha256-5sSiaeytKCRsY+XWGysMWMgNZMmtED+IRSvEVsh6FEg=";

          # If main package is at repo root, no subPackages needed.
          # subPackages = [ "." ];

          nativeBuildInputs = [ pkgs.makeWrapper ];
          postInstall = ''
            wrapProgram "$out/bin/bwmenu" \
              --prefix PATH : ${lib.makeBinPath [ pkgs.bitwarden-cli ]}
          '';
          meta = with lib; {
            description = "Terminal UI for Bitwarden credentials";
            homepage = "https://github.com/netbrain/bwmenu";
            license = licenses.mit;
            maintainers = [ ];
            mainProgram = "bwmenu";
          };
        };
        default = self.packages.${system}.bwmenu;
      };

      apps.${system}.default = {
        type = "app";
        program = "${self.packages.${system}.bwmenu}/bin/bwmenu";
      };

      devShells.${system}.default = pkgs.mkShell {
        buildInputs = [
          pkgs.go
          pkgs.gh
          pkgs.bitwarden-cli
        ];
      };
    };
}
