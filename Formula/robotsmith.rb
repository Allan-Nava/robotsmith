# Homebrew formula, tapped straight from this repository:
#
#   brew tap Allan-Nava/robotsmith https://github.com/Allan-Nava/robotsmith
#   brew install robotsmith
#
# ⚠️ `url` and `sha256` are rewritten by .github/workflows/release.yml at tag time — a formula
# updated by hand goes stale after the first release nobody remembers to edit.
class Robotsmith < Formula
  desc "Verifies a robots.txt and advises how to write it from your real traffic"
  homepage "https://github.com/Allan-Nava/robotsmith"
  url "https://github.com/Allan-Nava/robotsmith/archive/refs/tags/v0.3.0.tar.gz"
  sha256 "964ccf2f84d1bf4f93072e188444bcaf7f621dd7fd71ae97a31a88e2db98a0f6"
  license "MIT"
  head "https://github.com/Allan-Nava/robotsmith.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-X main.version=v#{version}")
  end

  test do
    assert_match "robotsmith", shell_output("#{bin}/robotsmith version")

    # A clean file must lint clean, and a structural defect must exit 1: the two ends of the
    # contract, checked at install time.
    (testpath/"robots.txt").write "User-agent: *\nDisallow: /admin/\n"
    assert_match "no structural defect", shell_output("#{bin}/robotsmith lint #{testpath}/robots.txt")

    (testpath/"orphan.txt").write "User-agent: *\n\nDisallow: /admin/\n"
    output = shell_output("#{bin}/robotsmith lint #{testpath}/orphan.txt", 1)
    assert_match "BLANK line", output

    (testpath/"ua.txt").write "500 Mozilla/5.0 (compatible; GPTBot/1.4)\n"
    assert_match "User-agent: GPTBot",
      shell_output("#{bin}/robotsmith advise --ua-counts #{testpath}/ua.txt 2>/dev/null")
  end
end
