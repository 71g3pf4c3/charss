{
  description = "charss — a terminal RSS reader (newsboat alternative) with chawan HTML rendering and chafa images";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

    # Used only for the checks (evaluating a Home Manager configuration
    # with the charss module); the module itself only needs lib/mkOption.
    # master tracks nixos-unstable, which our nixpkgs input points at.
    home-manager = {
      url = "github:nix-community/home-manager/master";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      home-manager,
    }:
    let
      lib = nixpkgs.lib;

      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = lib.genAttrs systems;
      pkgsFor = system: import nixpkgs { inherit system; };

      # "20261004153000" -> "2026-10-04T15:30:00Z" (GoReleaser-style date).
      formatDate =
        m:
        let
          s = toString m;
          sub = builtins.substring;
        in
        "${sub 0 4 s}-${sub 4 2 s}-${sub 6 2 s}T${sub 8 2 s}:${sub 10 2 s}:${sub 12 2 s}Z";

      charssFor =
        system:
        (pkgsFor system).callPackage ./nix/package.nix {
          commit = self.shortRev or "unknown";
          date = formatDate (self.lastModifiedDate or "19700101000000");
        };
    in
    {
      packages = forAllSystems (system: rec {
        charss = charssFor system;
        default = charss;
      });

      devShells = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          # chawan and chafa are included so `charss` can be exercised
          # against the real browser/image pipeline inside the shell —
          # rendering features are only "done" when verified in a real
          # terminal (see AGENTS.md).
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gopls
              delve
              gofumpt
              nixfmt-tree
              chawan
              chafa
            ];
          };
        }
      );

      homeManagerModules = {
        charss = import ./nix/hm-module.nix;
        default = self.homeManagerModules.charss;
      };

      checks = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
          # Build the full HM activation with the module enabled and
          # non-default settings/bindings/colors/macros/urls; this
          # exercises option declarations, newsboat-syntax rendering
          # and the default package path end to end.
          home = home-manager.lib.homeManagerConfiguration {
            inherit pkgs;
            modules = [
              ./nix/hm-module.nix
              {
                home.username = "nix-test";
                home.homeDirectory = "/home/nix-test";
                home.stateVersion = "25.05";

                programs.charss = {
                  enable = true;
                  settings = {
                    browser = "chawan";
                    auto-reload = true;
                    reload-time = 30;
                    notify-screen = true;
                    show-read-articles = false;
                  };
                  bindings = [
                    {
                      key = "^";
                      op = "toggle-flag";
                    }
                    {
                      key = "m";
                      op = "toggle-flag";
                      context = "articlelist";
                    }
                  ];
                  colors = [
                    {
                      element = "listnormal";
                      fg = "red";
                      bg = "black";
                    }
                  ];
                  macros = {
                    "," = "toggle-article-read; quit";
                  };
                  extraConfig = ''
                    unbind-key J all
                  '';
                  urls = [
                    ''https://example.com/feed.xml "Example" dev rss''
                  ];
                };
              }
            ];
          };

          # Assert the rendered files match what internal/config's
          # tokenizer expects, line by line, and that the legacy
          # config.toml is not written.
          verifyConfig = pkgs.runCommand "charss-hm-config-verify" { } ''
            conf="${home.activationPackage}/home-files/.config/charss/config"
            urls="${home.activationPackage}/home-files/.config/charss/urls"
            fail() { echo "hm-module content check: $1" >&2; exit 1; }
            line() { grep -Fqx "$2" "$1" || fail "missing line in $1: $2"; }

            [ -f "$conf" ] || fail "config not rendered"
            [ -f "$urls" ] || fail "urls not rendered"
            [ ! -e "${home.activationPackage}/home-files/.config/charss/config.toml" ] \
              || fail "legacy config.toml must not be written"

            line "$conf" 'auto-reload yes'
            line "$conf" 'browser "chawan"'
            line "$conf" 'notify-screen yes'
            line "$conf" 'reload-time 30'
            line "$conf" 'show-read-articles no'
            line "$conf" 'bind-key ^ toggle-flag'
            line "$conf" 'bind-key m toggle-flag articlelist'
            line "$conf" 'color listnormal red black'
            line "$conf" 'macro , toggle-article-read; quit'
            line "$conf" 'unbind-key J all'
            line "$urls" 'https://example.com/feed.xml "Example" dev rss'

            touch "$out"
          '';
        in
        {
          hm-module = home.activationPackage;
          hm-module-config = verifyConfig;
        }
      );

      # nixfmt formats with the RFC 148 style by default; the -tree
      # wrapper walks the flake so bare `nix fmt` works without args.
      formatter = forAllSystems (system: (pkgsFor system).nixfmt-tree);
    };
}
