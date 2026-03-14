# Homebrew Shipping

This repo ships to Homebrew through a separate tap repo, not through this source repo.

Recommended layout:

- Source repo: `https://github.com/suju297/mem`
- Tap repo: `https://github.com/suju297/homebrew-mem`
- Formula name: `mem-cli`

The tap-ready formula example for the most recently prepared release lives at
`packaging/homebrew/Formula/mem-cli.rb`.

## Release automation

Tagged releases can update the tap automatically through `.github/workflows/release.yml`.
The workflow:

- downloads the GitHub tag source tarball
- computes the source `sha256`
- regenerates `Formula/mem-cli.rb`
- updates the tap README's `Current packaged release` line
- commits and pushes the change to `suju297/homebrew-mem`

To enable this, add a repository secret in `suju297/mem`:

- Name: `HOMEBREW_TAP_GITHUB_TOKEN`
- Scope: token with `contents:write` access to `suju297/homebrew-mem`

If the secret is missing, the release still succeeds and the Homebrew update job
logs a skip message instead of failing the release.

The workflow uses `scripts/render_homebrew_formula.sh` to render the formula.

## Create the tap manually

Create a separate repo named `homebrew-mem`, then add the formula under `Formula/mem-cli.rb`.

Example:

```bash
mkdir -p ~/code/homebrew-mem/Formula
cp packaging/homebrew/Formula/mem-cli.rb ~/code/homebrew-mem/Formula/
cd ~/code/homebrew-mem
git init
git remote add origin git@github.com:suju297/homebrew-mem.git
git add Formula/mem-cli.rb
git commit -m "Add mem-cli formula for vX.Y.Z"
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
brew audit --strict --new suju297/mem/mem-cli
```

## Update for the next release

If automation is configured, tagged releases update the tap automatically.
If you need to update it manually:

1. Change `url` to the new tag tarball.
2. Recompute `sha256` from the source tarball.
3. Change `commit` in `install`.
4. Commit the formula change in `homebrew-mem`.

Example checksum command:

```bash
curl -fsSL https://github.com/suju297/mem/archive/refs/tags/vX.Y.Z.tar.gz -o /tmp/mem.tar.gz
shasum -a 256 /tmp/mem.tar.gz
```
