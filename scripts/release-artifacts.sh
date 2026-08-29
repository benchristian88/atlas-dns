#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
go_command=${GO:-go}
release_version=${ATLAS_DNS_VERSION:?set ATLAS_DNS_VERSION, for example 1.0.1 or 1.0.1-rc.1}
release_version=${release_version#v}
case ${release_version} in
  *[!0-9A-Za-z.+-]*|'') echo "ATLAS_DNS_VERSION contains unsupported characters" >&2; exit 1 ;;
esac

release_commit=${ATLAS_DNS_COMMIT:-$(git -C "${repo_dir}" rev-parse --short=12 HEAD)}
release_built_at=${ATLAS_DNS_BUILT_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
release_dir="${repo_dir}/dist/release/${release_version}"
if [[ -e ${release_dir} ]]; then
  echo "Release output already exists: ${release_dir}" >&2
  exit 1
fi
mkdir -p "${release_dir}"

make -C "${repo_dir}" GO="${go_command}" VERSION="${release_version}" COMMIT="${release_commit}" BUILT_AT="${release_built_at}" build

staging_root=$(mktemp -d)
cleanup() { rm -rf "${staging_root}"; }
trap cleanup EXIT

ldflags="-s -w -X github.com/benchristian88/atlas-dns/internal/version.Version=${release_version} -X github.com/benchristian88/atlas-dns/internal/version.Commit=${release_commit} -X github.com/benchristian88/atlas-dns/internal/version.BuiltAt=${release_built_at}"
for architecture in amd64 arm64; do
  archive_name="atlas-dns_${release_version}_linux_${architecture}"
  archive_root="${staging_root}/${archive_name}"
  mkdir -p "${archive_root}/bin" "${archive_root}/web" "${archive_root}/systemd"

  CGO_ENABLED=0 GOOS=linux GOARCH="${architecture}" "${go_command}" build -trimpath -ldflags "${ldflags}" -o "${archive_root}/bin/atlas-dns" ./cmd/controller
  CGO_ENABLED=0 GOOS=linux GOARCH="${architecture}" "${go_command}" build -trimpath -ldflags "${ldflags}" -o "${archive_root}/bin/atlas-dns-backup" ./cmd/atlas-dns-backup
  CGO_ENABLED=0 GOOS=linux GOARCH="${architecture}" "${go_command}" build -trimpath -ldflags "${ldflags}" -o "${archive_root}/bin/atlas-dns-admin" ./cmd/atlas-dns-admin
  CGO_ENABLED=0 GOOS=linux GOARCH="${architecture}" "${go_command}" build -trimpath -ldflags "${ldflags}" -o "${archive_root}/bin/atlas-dns-migrate" ./cmd/migrate

  cp -a "${repo_dir}/web/dist/." "${archive_root}/web/"
  cp "${repo_dir}/packaging/systemd/atlas-dns.service" "${archive_root}/systemd/atlas-dns.service"
  cp "${repo_dir}/LICENSE" "${archive_root}/LICENSE"
  cp "${repo_dir}/README.md" "${archive_root}/README.md"
  tar -C "${staging_root}" -czf "${release_dir}/${archive_name}.tar.gz" "${archive_name}"
done

cp "${repo_dir}/compose.yaml" "${release_dir}/compose.yaml"
cp "${repo_dir}/.env.example" "${release_dir}/atlas-dns.env.example"
cp "${repo_dir}/scripts/install-systemd.sh" "${release_dir}/install-systemd.sh"
cp "${repo_dir}/LICENSE" "${release_dir}/LICENSE"

(
  cd "${release_dir}"
  shasum -a 256 atlas-dns_*.tar.gz compose.yaml atlas-dns.env.example install-systemd.sh LICENSE > checksums.txt
)

if command -v syft >/dev/null 2>&1; then
  syft dir:"${repo_dir}" -o spdx-json="${release_dir}/atlas-dns_${release_version}_sbom.spdx.json"
fi

echo "Atlas DNS Controller release artifacts written to ${release_dir}"
