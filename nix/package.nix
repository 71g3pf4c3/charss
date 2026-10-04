# Shared charss package definition.
#
# Used by flake.nix (packages.<system>.charss) and as the default for
# programs.charss.package in nix/hm-module.nix, so both build the same thing.
#
# Callers may override version/commit/date; the defaults produce the same
# metadata shape GoReleaser injects (see .goreleaser.yaml ldflags).
# NB: do not add a `src` parameter — pkgs.callPackage would inject
# nixpkgs' (unrelated) `src` package as the argument.
{
  lib,
  buildGoModule,
  version ? "0.1.0",
  commit ? "unknown",
  date ? "1970-01-01T00:00:00Z",
}:

buildGoModule {
  pname = "charss";
  inherit version;
  # Repo root (Go module root), one level up from this file.
  src = ../.;

  # Keep in sync with the GoReleaser build env (CGO_ENABLED=0) and ldflags
  # in .goreleaser.yaml.
  env.CGO_ENABLED = 0;

  ldflags = [
    "-s"
    "-w"
    "-X github.com/71g3pf4c3/charss/internal/version.version=${version}"
    "-X github.com/71g3pf4c3/charss/internal/version.commit=${commit}"
    "-X github.com/71g3pf4c3/charss/internal/version.date=${date}"
  ];

  vendorHash = "sha256-zegWRNdkOUI78q2zTu5Tnsj+1BF8WM5LaEctuUbO7jk=";

  meta = {
    description = "Terminal RSS reader (newsboat alternative) with HTML rendering via chawan and images via chafa";
    homepage = "https://github.com/71g3pf4c3/charss";
    license = lib.licenses.mit;
    mainProgram = "charss";
    # Matches the GoReleaser target matrix; windows is out of scope by design.
    platforms = lib.platforms.linux ++ lib.platforms.darwin;
  };
}
