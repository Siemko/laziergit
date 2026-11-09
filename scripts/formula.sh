#!/bin/sh
# Prints the Homebrew formula of a release, with the checksums of its
# tarballs, for the tap (Siemko/homebrew-tap, Formula/laziergit.rb):
#
#	scripts/formula.sh 0.1.0 > ../homebrew-tap/Formula/laziergit.rb
set -eu
version=${1:?usage: formula.sh VERSION}
repo=Siemko/laziergit
sums=$(gh release download "v$version" --repo "$repo" --pattern SHA256SUMS --output -)
sum() {
	echo "$sums" | awk -v f="laziergit-$version-darwin-$1.tar.gz" '$2 == f { print $1 }' | grep . ||
		{ echo "no checksum of $1 in the release" >&2; exit 1; }
}
arm=$(sum arm64)
intel=$(sum amd64)
cat <<RUBY
class Laziergit < Formula
  desc "Native macOS app with lazygit's functionality"
  homepage "https://github.com/$repo"
  version "$version"
  license "MIT"

  on_arm do
    url "https://github.com/$repo/releases/download/v$version/laziergit-$version-darwin-arm64.tar.gz"
    sha256 "$arm"
  end
  on_intel do
    url "https://github.com/$repo/releases/download/v$version/laziergit-$version-darwin-amd64.tar.gz"
    sha256 "$intel"
  end

  depends_on macos: :monterey

  def install
    prefix.install "laziergit.app"
    bin.write_exec_script prefix/"laziergit.app/Contents/MacOS/laziergit"
  end

  def caveats
    <<~EOS
      Run laziergit in a repository, or put the app in Applications:
        ln -sf #{opt_prefix}/laziergit.app /Applications/laziergit.app
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/laziergit --version")
  end
end
RUBY
