# Home Manager module for charss.
#
# Renders, when non-empty:
#   $XDG_CONFIG_HOME/charss/config.toml  <- programs.charss.settings
#   $XDG_CONFIG_HOME/charss/urls         <- programs.charss.urls
#
# The urls file is intentionally not part of the TOML config, mirroring
# newsboat's config/urls split (see AGENTS.md product invariants).

{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.programs.charss;
  # lib.generators has no TOML writer; pkgs.formats.toml is the idiomatic
  # way to render a TOML config file with proper escaping.
  tomlFormat = pkgs.formats.toml { };
in
{
  options.programs.charss = {
    enable = lib.mkEnableOption "charss";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./package.nix { };
      description = ''
        The charss package to use. By default it is built from the
        package definition bundled with this module (nix/package.nix)
        against the current pkgs. When consuming this module from the
        flake, prefer passing `packages.<system>.charss` explicitly.
      '';
    };

    settings = lib.mkOption {
      type = lib.types.attrsOf lib.types.anything;
      default = { };
      example = {
        browser = "chawan";
        chafa = "chafa";
      };
      description = ''
        Configuration written to $XDG_CONFIG_HOME/charss/config.toml.

        Known keys (see internal/config): `browser` — the HTML rendering
        browser used to display articles (chawan, https://chawan.net, by
        default) — and `chafa` — the image-to-terminal converter used for
        image previews. Unknown keys are passed through for forward
        compatibility.

        An empty attrset means no config file is written and charss falls
        back to its built-in defaults.
      '';
    };

    urls = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      example = [ ''https://example.com/feed.xml "Example" dev rss'' ];
      description = ''
        Feed list written to $XDG_CONFIG_HOME/charss/urls. Each entry is
        one ready urls-file line (newsboat-compatible): URL, optional
        quoted title, tags and per-feed "key: value" pairs.

        An empty list means no urls file is written; charss then starts
        with an empty feed list, which is not an error.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    xdg.configFile."charss/config.toml" = lib.mkIf (cfg.settings != { }) {
      source = tomlFormat.generate "charss-config.toml" cfg.settings;
    };

    xdg.configFile."charss/urls" = lib.mkIf (cfg.urls != [ ]) {
      text = lib.concatStringsSep "\n" cfg.urls + "\n";
    };
  };
}
