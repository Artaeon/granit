# Maintainer: Artaeon <raphael.lugmayr@stoicera.com>
pkgname=granit
pkgver=0.1.0
pkgrel=1
pkgdesc="A blazing-fast, AI-powered terminal knowledge manager — fully Obsidian compatible"
arch=('x86_64' 'aarch64')
url="https://github.com/artaeon/granit"
license=('MIT')
makedepends=('go>=1.23')
optdepends=(
  'ollama: local AI provider'
  'aspell: spell checking'
  'hunspell: spell checking (alternative)'
  'pandoc: PDF export'
  'xclip: system clipboard (X11)'
  'wl-clipboard: system clipboard (Wayland)'
  'git: version control features'
)
source=("${pkgname}-${pkgver}.tar.gz::${url}/archive/v${pkgver}.tar.gz")
sha256sums=('SKIP')

build() {
  cd "${pkgname}-${pkgver}"
  export CGO_ENABLED=0
  export GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
  go build -ldflags "-s -w -X main.version=${pkgver}" -o "${pkgname}" ./cmd/granit/
}

package() {
  cd "${pkgname}-${pkgver}"
  install -Dm755 "${pkgname}" "${pkgdir}/usr/bin/${pkgname}"
  install -Dm644 LICENSE "${pkgdir}/usr/share/licenses/${pkgname}/LICENSE"
  install -Dm644 README.md "${pkgdir}/usr/share/doc/${pkgname}/README.md"
}
