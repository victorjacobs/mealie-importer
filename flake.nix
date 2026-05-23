{
  description = "Development environment for mealie-importer";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs =
    { self, nixpkgs, ... }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-darwin"
        "x86_64-linux"
      ];

      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          mealie-importer = pkgs.buildGoModule {
            pname = "mealie-importer";
            version = "0.1.0";
            src = ./.;
            vendorHash = "sha256-0BfipGLNWl/Gy7e2TxU11lU7POTMxFPnRADnb1fgeQs=";

            nativeBuildInputs = [
              pkgs.makeWrapper
            ];

            postInstall = ''
              wrapProgram $out/bin/mealie-importer \
                --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.libheif ]}
            '';
          };
        in
        {
          default = mealie-importer;
          inherit mealie-importer;
          go = pkgs.go_1_26;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.mealie-importer}/bin/mealie-importer";
        };
      });

      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go_1_26
              pkgs.libheif
            ];
          };
        }
      );
    };
}
