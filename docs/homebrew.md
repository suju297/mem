# Homebrew Shipping

This repo ships to Homebrew through a separate tap repo, not through this source repo.

Recommended layout:

- Source repo: `https://github.com/suju297/mem`
- Tap repo: `https://github.com/suju297/homebrew-mem`
- Formula name: `mem-cli`

## Current release

- Version: `v0.2.42`
- Source tarball: `https://github.com/suju297/mem/archive/refs/tags/v0.2.42.tar.gz`
- Source sha256: `e39434b3736d308766565b26c73b4494565efa3c4437a6419aff04e1cfcd2938`
- Commit: `4906386`
- Release page: `https://github.com/suju297/mem/releases/tag/v0.2.42`

The tap-ready formula for this release lives at `packaging/homebrew/Formula/mem-cli.rb`.

## Create the tap

Create a separate repo named `homebrew-mem`, then add the formula under `Formula/mem-cli.rb`.

Example:

```bash
mkdir -p ~/code/homebrew-mem/Formula
cp packaging/homebrew/Formula/mem-cli.rb ~/code/homebrew-mem/Formula/
cd ~/code/homebrew-mem
git init
git remote add origin git@github.com:suju297/homebrew-mem.git
git add Formula/mem-cli.rb
git commit -m "Add mem-cli formula for v0.2.42"
git push -u origin main
```

## Install from the tap

```bash
brew tap suju297/mem
brew install suju297/mem/mem-cli
```

If you prefer a local dry run before publishing the tap:

```bash
brew install --build-from-source ./Formula/mem-cli.rb
brew test mem-cli
```

## Audit the formula

```bash
export HOMEBREW_NO_INSTALL_FROM_API=1
brew audit --strict --formula ./Formula/mem-cli.rb
```

## Update for the next release

For each new release:

1. Change `url` to the new tag tarball.
2. Recompute `sha256` from the source tarball.
3. Change `commit` in `install`.
4. Commit the formula change in `homebrew-mem`.

Example checksum command:

```bash
curl -fsSL https://github.com/suju297/mem/archive/refs/tags/vX.Y.Z.tar.gz -o /tmp/mem.tar.gz
shasum -a 256 /tmp/mem.tar.gz
```
