# Home Manager module for charss.
#
# Renders, when non-empty:
#   $XDG_CONFIG_HOME/charss/config  <- programs.charss.{settings,bindings,colors,macros,extraConfig}
#   $XDG_CONFIG_HOME/charss/urls     <- programs.charss.urls
#
# The config file is newsboat syntax (plain `name value` lines plus
# bind-key/color/macro/include directives), NOT TOML — charss dropped the
# legacy TOML config; a leftover config.toml is ignored with a warning.
# String values are rendered quoted with \" and \\ escaped, the exact
# inverse of internal/config's tokenizer. The urls file is intentionally
# separate, mirroring newsboat's config/urls split (see AGENTS.md product
# invariants).

{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.programs.charss;

  # One newsboat config token: "..." with embedded \ and " escaped —
  # the inverse of internal/config tokenize().
  quote = s: ''"${lib.strings.escape [ "\\" "\"" ] s}"'';

  renderValue =
    v:
    if lib.isBool v then
      (if v then "yes" else "no")
    else if lib.isInt v then
      toString v
    else if lib.isString v then
      quote v
    else
      # listOf str: one quoted token per element (multi-token value).
      lib.concatStringsSep " " (map quote v);

  renderBinding =
    b:
    if b.context == null then "bind-key ${b.key} ${b.op}" else "bind-key ${b.key} ${b.op} ${b.context}";

  renderColor =
    c:
    lib.concatStringsSep " " (
      [
        "color"
        c.element
        c.fg
        c.bg
      ]
      ++ c.attrs
    );

  # All structured parts, rendered in order: options, bindings, colors,
  # macros, then extraConfig verbatim. Empty parts contribute nothing.
  configLines =
    lib.mapAttrsToList (name: value: "${name} ${renderValue value}") cfg.settings
    ++ map renderBinding cfg.bindings
    ++ map renderColor cfg.colors
    ++ lib.mapAttrsToList (key: body: "macro ${key} ${body}") cfg.macros
    ++ lib.optional (cfg.extraConfig != "") cfg.extraConfig;

  hasConfig =
    cfg.settings != { }
    || cfg.bindings != [ ]
    || cfg.colors != [ ]
    || cfg.macros != { }
    || cfg.extraConfig != "";
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
      type = lib.types.attrsOf (
        lib.types.oneOf [
          lib.types.bool
          lib.types.int
          lib.types.str
          (lib.types.listOf lib.types.str)
        ]
      );
      default = { };
      example = {
        browser = "chawan";
        auto-reload = true;
        reload-time = 30;
        notify-screen = true;
      };
      description = ''
        Plain `name value` options for $XDG_CONFIG_HOME/charss/config
        (newsboat syntax). Booleans render as yes/no, integers plainly,
        strings quoted (with escaping), string lists as one quoted token
        per element.

        Known keys (see internal/config): browser, chafa, feed-sort-order,
        article-sort-order, show-read-articles, auto-reload, reload-time,
        notify-screen. Unknown keys are passed through (charss warns but
        does not fail).

        An empty attrset (together with empty bindings/colors/macros/
        extraConfig) means no config file is written and charss falls
        back to its built-in defaults.
      '';
    };

    bindings = lib.mkOption {
      type = lib.types.listOf (
        lib.types.submodule {
          options = {
            key = lib.mkOption {
              type = lib.types.str;
              description = "Key name: a rune (case-sensitive), ENTER, ESC, UP, DOWN, PAGEUP, PAGEDOWN, or ^X-style control.";
            };
            op = lib.mkOption {
              type = lib.types.str;
              description = "Operation name in newsboat spelling, e.g. open, quit, toggle-flag, run-macro.";
            };
            context = lib.mkOption {
              type = lib.types.nullOr lib.types.str;
              default = null;
              description = ''
                Binding context: feedlist, articlelist, article, help,
                dialog, podcast, or all. null means all contexts
                (newsboat's default).
              '';
            };
          };
        }
      );
      default = [ ];
      example = [
        {
          key = "m";
          op = "toggle-flag";
        }
        {
          key = "^";
          op = "toggle-flag";
          context = "articlelist";
        }
      ];
      description = ''
        bind-key directives, written in list order. Order matters when
        combined with unbind-key lines in extraConfig (directives are
        replayed in file order).
      '';
    };

    colors = lib.mkOption {
      type = lib.types.listOf (
        lib.types.submodule {
          options = {
            element = lib.mkOption {
              type = lib.types.str;
              description = "Color element, e.g. listnormal, listfocus, article.";
            };
            fg = lib.mkOption {
              type = lib.types.str;
              description = "Foreground color name.";
            };
            bg = lib.mkOption {
              type = lib.types.str;
              description = "Background color name.";
            };
            attrs = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              default = [ ];
              description = "Optional attributes: bold, underline, standout, reverse, blink, dim.";
            };
          };
        }
      );
      default = [ ];
      example = [
        {
          element = "listfocus";
          fg = "black";
          bg = "yellow";
          attrs = [ "bold" ];
        }
      ];
      description = "color directives, written in list order (last rule per element wins).";
    };

    macros = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { };
      example = {
        "," = "toggle-article-read; quit";
      };
      description = ''
        macro directives: macro key -> raw body. The body is written
        verbatim after the key, so use newsboat macro syntax:
        semicolon-separated operations, with operation arguments (flag
        letters, save paths) inline.
      '';
    };

    extraConfig = lib.mkOption {
      type = lib.types.lines;
      default = "";
      description = ''
        Extra lines appended verbatim to the config file — for include,
        unbind-key, or directives not yet modelled by the structured
        options.
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

    xdg.configFile."charss/config" = lib.mkIf hasConfig {
      text = lib.concatStringsSep "\n" configLines + "\n";
    };

    xdg.configFile."charss/urls" = lib.mkIf (cfg.urls != [ ]) {
      text = lib.concatStringsSep "\n" cfg.urls + "\n";
    };
  };
}
