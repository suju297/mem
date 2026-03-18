class MemCli < Formula
  desc "Repo-scoped memory for coding agents"
  homepage "https://github.com/suju297/mem"
  url "https://github.com/suju297/mem/archive/refs/tags/v0.2.44.tar.gz"
  sha256 "f88aa784003ee25099ff33ca0116802dac80a6419a2c6dc439605515e7b8caff"
  license "MIT"

  depends_on "go" => :build

  def install
    commit = "9f3cc6e"
    ldflags = %W[
      -s -w
      -X mem/internal/app.Version=#{version}
      -X mem/internal/app.Commit=#{commit}
    ]

    system "go", "build", *std_go_args(output: bin/"mem", ldflags: ldflags), "./cmd/mem"
  end

  test do
    assert_match "mem v#{version}", shell_output("#{bin}/mem --version")
  end
end
