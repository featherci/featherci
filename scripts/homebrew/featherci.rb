class Featherci < Formula
  desc "Lightweight self-hosted CI/CD system"
  homepage "https://github.com/featherci/featherci"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/featherci/featherci/releases/download/v#{version}/featherci-darwin-arm64.tar.gz"
      sha256 "0b1d9b61135de3cf4f0e734837cd023634de30392716ad84145dfafa54b53a50"
    else
      url "https://github.com/featherci/featherci/releases/download/v#{version}/featherci-darwin-amd64.tar.gz"
      sha256 "3a815eaec57fea242d143048e00ccf3db37e746b02cfb4f9e3859ef557645ed0"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/featherci/featherci/releases/download/v#{version}/featherci-linux-arm64.tar.gz"
      sha256 "a7e40882de8088e4bd1b9e714dd25969429584be3dcf62e838eac3134e2de3f0"
    else
      url "https://github.com/featherci/featherci/releases/download/v#{version}/featherci-linux-amd64.tar.gz"
      sha256 "cbc381a86d416b9090241d4dce9ba95c47677eb33e071e590bc3735b6cad6d77"
    end
  end

  depends_on "docker" => :optional

  def install
    bin.install "featherci"
    (share/"featherci").install "config.yaml.example"
  end

  def post_install
    (var/"featherci").mkpath
    (var/"log/featherci").mkpath
  end

  def caveats
    <<~EOS
      To start FeatherCI, first generate an encryption key:
        featherci --generate-key

      Then create a config file at #{etc}/featherci/config.yaml:
        mkdir -p #{etc}/featherci
        cp #{opt_share}/featherci/config.yaml.example #{etc}/featherci/config.yaml

      Edit the config file with your settings (secret key, OAuth, admins).

      Start the service:
        brew services start featherci

      Or run manually:
        featherci --config #{etc}/featherci/config.yaml
    EOS
  end

  service do
    run [opt_bin/"featherci", "--config", etc/"featherci/config.yaml"]
    keep_alive true
    working_dir var/"featherci"
    log_path var/"log/featherci/featherci.log"
    error_log_path var/"log/featherci/featherci.error.log"
  end

  test do
    assert_match "FeatherCI", shell_output("#{bin}/featherci --version")
  end
end
