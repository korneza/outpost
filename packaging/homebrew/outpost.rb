class Outpost < Formula
  desc "MCP proxy for agent reliability and security visibility"
  homepage "https://github.com/korneza/outpost"
  version "0.9.0"
  license "Apache-2.0"

  on_macos do
    on_arm do
      url "https://github.com/korneza/outpost/releases/download/v0.9.0/outpost_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_REAL_SHA256_FROM_checksums.txt"
    end
    on_intel do
      url "https://github.com/korneza/outpost/releases/download/v0.9.0/outpost_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_REAL_SHA256_FROM_checksums.txt"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/korneza/outpost/releases/download/v0.9.0/outpost_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_REAL_SHA256_FROM_checksums.txt"
    end
    on_intel do
      url "https://github.com/korneza/outpost/releases/download/v0.9.0/outpost_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_REAL_SHA256_FROM_checksums.txt"
    end
  end

  def install
    bin.install "outpost"
  end

  test do
    assert_match "outpost", shell_output("#{bin}/outpost version")
  end
end
