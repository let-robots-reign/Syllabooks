#!/bin/sh

# One-time production bootstrap for syllabooks.ru on Ubuntu. Run as root after
# the release and ops files have been copied to /tmp by the zotov SSH account.
set -eu
umask 077

source_env=/tmp/syllabooks.source.env
test -r "$source_env"
test -x /tmp/syllabooks-linux-amd64

read_env() {
	awk -v key="$1" 'index($0, key "=") == 1 { print substr($0, length(key) + 2); exit }' "$source_env"
}

# This VPS has less than 1 GiB of physical RAM. Swap protects package upgrades
# and short-lived PostgreSQL maintenance from the kernel OOM killer.
if ! swapon --show=NAME --noheadings | grep -qx /swapfile; then
	if [ ! -f /swapfile ]; then
		fallocate -l 1G /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=1024
		chmod 600 /swapfile
		mkswap /swapfile
	fi
	swapon /swapfile
fi
grep -qF '/swapfile none swap sw 0 0' /etc/fstab || \
	printf '%s\n' '/swapfile none swap sw 0 0' >>/etc/fstab

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl postgresql postgresql-client cron

# Ubuntu's repository carries an old Caddy release. Use Caddy's signed stable
# package repository, which also installs the maintained systemd unit.
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
	| gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
	-o /etc/apt/sources.list.d/caddy-stable.list
chmod o+r /usr/share/keyrings/caddy-stable-archive-keyring.gpg /etc/apt/sources.list.d/caddy-stable.list
apt-get update
apt-get install -y caddy

if ! id syllabooks >/dev/null 2>&1; then
	useradd --system --home-dir /var/lib/syllabooks --shell /usr/sbin/nologin syllabooks
fi
install -d -o root -g root -m 0755 /opt/syllabooks /opt/syllabooks/ops /etc/syllabooks
install -d -o syllabooks -g syllabooks -m 0700 /var/backups/syllabooks

db_password=$(openssl rand -hex 24)
shelf_code=$(openssl rand -hex 32)

systemctl enable --now postgresql
runuser -u postgres -- psql --set=ON_ERROR_STOP=1 --set=db_password="$db_password" <<'SQL'
SELECT format('CREATE ROLE syllabooks LOGIN PASSWORD %L', :'db_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'syllabooks') \gexec
ALTER ROLE syllabooks PASSWORD :'db_password';
SELECT 'CREATE DATABASE syllabooks OWNER syllabooks'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'syllabooks') \gexec
ALTER SYSTEM SET max_connections = '20';
ALTER SYSTEM SET shared_buffers = '64MB';
ALTER SYSTEM SET work_mem = '1MB';
ALTER SYSTEM SET maintenance_work_mem = '32MB';
SQL
systemctl restart postgresql

yandex_client_id=$(read_env YANDEX_CLIENT_ID)
yandex_client_secret=$(read_env YANDEX_CLIENT_SECRET)
vk_client_id=$(read_env VK_CLIENT_ID)
google_books_api_key=$(read_env GOOGLE_BOOKS_API_KEY)

env_tmp=$(mktemp /etc/syllabooks/syllabooks.env.XXXXXX)
{
	printf '%s\n' 'LISTEN_ADDR=127.0.0.1:8080'
	printf '%s\n' 'PUBLIC_URL=https://syllabooks.ru'
	printf 'DATABASE_URL=postgres://syllabooks:%s@127.0.0.1:5432/syllabooks?sslmode=disable\n' "$db_password"
	printf '%s\n' 'BOOK_COVERS_DIR=/var/lib/syllabooks/book-covers'
	printf '%s\n' 'LOAN_DAYS=21'
	printf '%s\n' 'GOMEMLIMIT=192MiB'
	printf 'SHELF_CODE=%s\n' "$shelf_code"
	printf 'YANDEX_CLIENT_ID=%s\n' "$yandex_client_id"
	printf 'YANDEX_CLIENT_SECRET=%s\n' "$yandex_client_secret"
	printf 'VK_CLIENT_ID=%s\n' "$vk_client_id"
	printf 'GOOGLE_BOOKS_API_KEY=%s\n' "$google_books_api_key"
} >"$env_tmp"
chown root:root "$env_tmp"
chmod 600 "$env_tmp"
mv "$env_tmp" /etc/syllabooks/syllabooks.env

backup_env_tmp=$(mktemp /etc/syllabooks/backup.env.XXXXXX)
{
	printf "export DATABASE_URL='postgres://syllabooks:%s@127.0.0.1:5432/syllabooks?sslmode=disable'\n" "$db_password"
	printf "%s\n" "export BACKUP_DIR='/var/backups/syllabooks'"
} >"$backup_env_tmp"
chown syllabooks:syllabooks "$backup_env_tmp"
chmod 600 "$backup_env_tmp"
mv "$backup_env_tmp" /etc/syllabooks/backup.env

install -o root -g root -m 0755 /tmp/syllabooks-linux-amd64 /opt/syllabooks/syllabooks
install -o root -g root -m 0755 /tmp/backup.sh /opt/syllabooks/ops/backup.sh
install -o root -g root -m 0755 /tmp/syllabooks-deploy /usr/local/sbin/syllabooks-deploy
install -o root -g root -m 0644 /tmp/syllabooks.service /etc/systemd/system/syllabooks.service
install -o root -g root -m 0644 /tmp/Caddyfile /etc/caddy/Caddyfile

caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
systemctl daemon-reload
systemctl enable syllabooks cron caddy
systemctl restart syllabooks
systemctl reload caddy

# Keep local logs from consuming a material share of the 10 GB disk.
install -d -o root -g root -m 0755 /etc/systemd/journald.conf.d
printf '%s\n' '[Journal]' 'SystemMaxUse=100M' > /etc/systemd/journald.conf.d/syllabooks-storage.conf
systemctl restart systemd-journald

crontab -u syllabooks /tmp/crontab.example
runuser -u syllabooks -- sh -c '. /etc/syllabooks/backup.env && /opt/syllabooks/ops/backup.sh'

# Future `make deploy` runs go through the fixed, root-owned deployment command
# and need only these exact privileged commands.
visudo -cf /tmp/sudoers.example
install -o root -g root -m 0440 /tmp/sudoers.example /etc/sudoers.d/syllabooks-deploy

rm -f "$source_env"

systemctl --no-pager --full status syllabooks
systemctl --no-pager --full status caddy
free -h
df -h /
