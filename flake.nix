{
  description = "cronx - a small, reliable, portable and secure local job scheduler";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      # Systems for which the development environment and the package are exposed.
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems = f:
        nixpkgs.lib.genAttrs systems (system: f system nixpkgs.legacyPackages.${system});
    in
    {
      # Reproducible development environment.
      #
      # Provides exactly what is required to develop and test cronx: a pinned Go
      # toolchain, Git and the SQLite CLI (for manual inspection of the runtime
      # state during development). The application does not depend on Nix.
      devShells = forAllSystems (system: pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go_1_26
            pkgs.git
            pkgs.sqlite
          ];
        };
      });

      # The cronx binary, built as a normal portable Go executable.
      packages = forAllSystems (system: pkgs: {
        default = pkgs.buildGoModule {
          pname = "cronx";
          version = "0.5.0";
          src = ./.;
          # Hash of the vendored Go dependencies. Update it whenever the
          # dependency set changes (Nix reports the expected value on mismatch).
          vendorHash = "sha256-4oVBrZtpOI4U98ugjh22SMAGIzPMzGLMSQraxjDzZv0=";
          meta = {
            description = "A small, reliable, portable and secure local job scheduler";
            license = pkgs.lib.licenses.mit;
            mainProgram = "cronx";
          };
        };
      });

      # Building the package runs the Go test suite (buildGoModule check phase).
      checks = forAllSystems (system: pkgs: {
        default = self.packages.${system}.default;
      });
    };
}
