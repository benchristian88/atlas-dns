#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID} -ne 0 ]]; then
  echo "Run this installer as root." >&2
  exit 1
fi

for command_name in curl sha256sum tar install openssl runuser systemctl pg_dump pg_restore psql getent groupadd useradd uname awk; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "Required command is missing: ${command_name}" >&2
    exit 1
  }
done

pg_dump_version=$(pg_dump --version | awk '{print $NF}')
pg_restore_version=$(pg_restore --version | awk '{print $NF}')
if [[ ${pg_dump_version%%.*} != 17 || ${pg_restore_version%%.*} != 17 ]]; then
  echo "PostgreSQL 17 pg_dump and pg_restore are required for portable backup compatibility." >&2
  exit 1
fi

public_base_url=${PUBLIC_BASE_URL:-}
if [[ -z ${public_base_url} ]]; then
  echo "Set PUBLIC_BASE_URL to the browser-visible origin, for example https://controller.example.test" >&2
  exit 1
fi

case $(uname -m) in
  x86_64|amd64) architecture=amd64 ;;
  aarch64|arm64) architecture=arm64 ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

repository_url=https://github.com/benchristian88/atlas-dns
requested_version=${ATLAS_DNS_VERSION:-latest}
if [[ ${requested_version} == latest ]]; then
  effective_url=$(curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error --output /dev/null --write-out '%{url_effective}' "${repository_url}/releases/latest")
  release_tag=${effective_url##*/}
else
  release_tag=v${requested_version#v}
fi
release_version=${release_tag#v}
if [[ ! ${release_version} =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "Resolved release version is invalid: ${release_version}" >&2
  exit 1
fi

archive_name="atlas-dns_${release_version}_linux_${architecture}.tar.gz"
release_base="${repository_url}/releases/download/v${release_version}"
download_dir=$(mktemp -d)
cleanup() { rm -rf "${download_dir}"; }
trap cleanup EXIT

curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error --output "${download_dir}/${archive_name}" "${release_base}/${archive_name}"
curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error --output "${download_dir}/checksums.txt" "${release_base}/checksums.txt"
awk -v file="${archive_name}" '$2 == file || $2 == "*" file { print }' "${download_dir}/checksums.txt" > "${download_dir}/expected-checksum.txt"
if [[ ! -s ${download_dir}/expected-checksum.txt ]]; then
  echo "Release checksum does not contain ${archive_name}." >&2
  exit 1
fi
(
  cd "${download_dir}"
  sha256sum --check expected-checksum.txt
)

tar -C "${download_dir}" -xzf "${download_dir}/${archive_name}"
bundle_dir="${download_dir}/${archive_name%.tar.gz}"
for required in bin/atlas-dns bin/atlas-dns-backup bin/atlas-dns-migrate web/index.html systemd/atlas-dns.service LICENSE; do
  if [[ ! -f ${bundle_dir}/${required} ]]; then
    echo "Verified release archive is missing ${required}." >&2
    exit 1
  fi
done

service_user=atlas-dns
service_group=atlas-dns
database_name=${POSTGRES_DB:-atlas_dns}
database_user=${POSTGRES_USER:-atlas_dns}
environment_dir=/etc/atlas-dns
environment_file=${environment_dir}/atlas-dns.env
state_dir=/var/lib/atlas-dns
web_dir=/usr/local/share/atlas-dns/web

if ! getent group "${service_group}" >/dev/null; then
  groupadd --system "${service_group}"
fi
if ! id "${service_user}" >/dev/null 2>&1; then
  useradd --system --gid "${service_group}" --home-dir "${state_dir}" --shell /usr/sbin/nologin "${service_user}"
fi

install -d -o root -g root -m 0755 "${environment_dir}" /usr/local/share/atlas-dns
install -d -o "${service_user}" -g "${service_group}" -m 0750 "${state_dir}"
install -d -o root -g root -m 0755 "${web_dir}"
install -o root -g root -m 0755 "${bundle_dir}/bin/atlas-dns" /usr/local/bin/atlas-dns
install -o root -g root -m 0755 "${bundle_dir}/bin/atlas-dns-backup" /usr/local/bin/atlas-dns-backup
install -o root -g root -m 0755 "${bundle_dir}/bin/atlas-dns-migrate" /usr/local/bin/atlas-dns-migrate
cp -a "${bundle_dir}/web/." "${web_dir}/"
chown -R root:root "${web_dir}"
install -o root -g root -m 0644 "${bundle_dir}/LICENSE" /usr/local/share/atlas-dns/LICENSE

if [[ ! -f ${environment_file} ]]; then
  database_password=${POSTGRES_PASSWORD:-$(openssl rand -hex 24)}
  if [[ ! ${database_name} =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ || ! ${database_user} =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
    echo "POSTGRES_DB and POSTGRES_USER must be PostgreSQL identifiers." >&2
    exit 1
  fi
  if [[ ! ${database_password} =~ ^[a-zA-Z0-9._~-]+$ ]]; then
    echo "POSTGRES_PASSWORD must contain only URL-safe letters, numbers, dot, underscore, tilde, or hyphen." >&2
    exit 1
  fi
  session_secret=$(openssl rand -base64 48)
  credential_key=$(openssl rand -base64 32)
  {
    printf "\\set db_name '%s'\n" "${database_name}"
    printf "\\set db_user '%s'\n" "${database_user}"
    printf "\\set db_password '%s'\n" "${database_password}"
    printf '%s\n' \
      "SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'db_user', :'db_password')" \
      "WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'db_user') \\gexec" \
      "SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', :'db_user', :'db_password') \\gexec" \
      "SELECT format('CREATE DATABASE %I OWNER %I', :'db_name', :'db_user')" \
      "WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = :'db_name') \\gexec"
  } | runuser -u postgres -- psql --set=ON_ERROR_STOP=1
  umask 077
  {
    printf 'APP_ENV=production\n'
    printf 'INSTALLATION_TYPE=native_systemd\n'
    printf 'HTTP_ADDR=:8080\n'
    printf 'DATABASE_URL=postgres://%s:%s@127.0.0.1:5432/%s?sslmode=disable\n' "${database_user}" "${database_password}" "${database_name}"
    printf 'PUBLIC_BASE_URL=%s\n' "${public_base_url}"
    printf 'SESSION_SECRET=%s\n' "${session_secret}"
    printf 'CREDENTIAL_ENCRYPTION_KEY=%s\n' "${credential_key}"
    printf 'AUTO_MIGRATE=true\n'
  } >"${environment_file}"
  chmod 0600 "${environment_file}"
else
  echo "Preserving existing ${environment_file}"
fi

install -o root -g root -m 0644 "${bundle_dir}/systemd/atlas-dns.service" /etc/systemd/system/atlas-dns.service
systemctl daemon-reload
systemctl enable atlas-dns.service
systemctl restart atlas-dns.service
systemctl is-active --quiet atlas-dns.service

ready=false
for _ in {1..30}; do
  if curl --fail --silent http://127.0.0.1:8080/ready >/dev/null; then
    ready=true
    break
  fi
  sleep 1
done
if [[ ${ready} != true ]]; then
  systemctl --no-pager --full status atlas-dns.service || true
  echo "Atlas DNS Controller did not become ready after installation." >&2
  exit 1
fi

echo "Atlas DNS Controller ${release_version} is ready. Open ${public_base_url} to continue."
