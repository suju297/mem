class MemCli < Formula
  desc "Repo-scoped memory for coding agents"
  homepage "https://github.com/suju297/mem"
  url "https://github.com/suju297/mem/archive/refs/tags/v0.2.42.tar.gz"
  sha256 "e39434b3736d308766565b26c73b4494565efa3c4437a6419aff04e1cfcd2938"
  license "MIT"

  depends_on "go" => :build

  def install
    commit = "4906386"
    ldflags = %W[
      -s -w
      -X mem/internal/app.Version=#{version}
      -X mem/internal/app.Commit=#{commit}
    ]

    system "go", "build", *std_go_args(ldflags: ldflags), "./cmd/mem"
  end

  test do
    assert_match "mem v#{version}", shell_output("#{bin}/mem --version")
  end
end
