# Production deployment

Production is one static `linux/amd64` binary containing the React app, with
PostgreSQL and Caddy on the same VPS. Node, pnpm, Go, Docker, and the repository
are not needed on the server.

The commands below target a current Debian or Ubuntu VPS. Run them once before
the first `make deploy`. They assume the SSH administration account is named
`zotov`; the web application itself runs as the separate locked-down
`syllabooks` service account.

## 1. DNS, firewall, and packages

Point the domain's `A` record at the VPS before starting Caddy. Add an `AAAA`
record only when IPv6 is configured on the server. Allow inbound TCP 22, 80,
and 443; do not expose ports 5432 or 8080.

Install PostgreSQL and its client tools. Install Caddy using its
[official Debian/Ubuntu instructions](https://caddyserver.com/docs/install#debian-ubuntu-raspbian),
then enable PostgreSQL and Caddy.

The installed `pg_dump` major version must be at least as new as the PostgreSQL
server's major version.

## 2. Service account and directories

```sh
sudo useradd --system --home /var/lib/syllabooks --shell /usr/sbin/nologin syllabooks
sudo install -d -o root -g root -m 0755 /opt/syllabooks /etc/syllabooks
sudo install -d -o syllabooks -g syllabooks -m 0700 /var/backups/syllabooks
```

`systemd` creates `/var/lib/syllabooks` when the application starts. It holds
uploaded book covers and is the application's only writable state outside
PostgreSQL.

## 3. PostgreSQL

Generate a long URL-safe password locally. In `psql` as the PostgreSQL
administrator, create a login and database (replace the password first):

```sql
CREATE ROLE syllabooks LOGIN PASSWORD 'replace-with-a-long-random-password';
CREATE DATABASE syllabooks OWNER syllabooks;
```

PostgreSQL should listen on loopback only. The default local `scram-sha-256`
host authentication is suitable. The release binary applies pending embedded
migrations before every service start; goose records applied versions and
does nothing when the schema is current.

## 4. Application configuration

Copy the templates and replace every placeholder:

```sh
sudo install -o root -g root -m 0600 ops/syllabooks.env.example /etc/syllabooks/syllabooks.env
sudo install -o root -g root -m 0644 ops/syllabooks.service /etc/systemd/system/syllabooks.service
sudo systemctl daemon-reload
sudo systemctl enable syllabooks
```

The password inside `DATABASE_URL` must be percent-encoded if it contains URL
punctuation. `PUBLIC_URL` must exactly equal the final origin and must use
`https://`; that setting is also what enables Secure session cookies.

Create the same redirect URLs in the provider dashboards:

- Yandex: `https://syllabooks.ru/api/auth/callback/yandex`
- VK ID: `https://syllabooks.ru/api/auth/callback/vk`

Use `https://syllabooks.ru/privacy` as the privacy-policy URL where a provider
asks for one.

## 5. Caddy and HTTPS

Edit the hostname in `ops/Caddyfile`, then validate and install it:

```sh
caddy validate --config ops/Caddyfile --adapter caddyfile
sudo install -o root -g root -m 0644 ops/Caddyfile /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Caddy obtains and renews the certificate automatically after public DNS and
ports 80/443 work. The application remains reachable only through Caddy on
loopback port 8080.

## 6. Deployment command

Install the fixed deployment command as a root-owned file. The `zotov` SSH
user needs narrowly scoped passwordless `sudo` only for the exact `install`,
`mv`, and `systemctl` commands it uses. Check the command paths in
`ops/sudoers.example` against `command -v install mv systemctl` on the VPS,
then install both files:

```sh
sudo install -o root -g root -m 0755 ops/syllabooks-deploy /usr/local/sbin/syllabooks-deploy
sudo visudo -cf ops/sudoers.example
sudo install -o root -g root -m 0440 ops/sudoers.example /etc/sudoers.d/syllabooks-deploy
```

Deploy from the development machine (without a trailing slash in the URL):

```sh
make deploy DEPLOY_HOST=zotov@31.77.173.53 DEPLOY_URL=https://syllabooks.ru
```

The target builds the frontend, cross-compiles a static Linux AMD64 binary,
streams that one file to the fixed deployment command, installs it atomically,
restarts the service, and checks the public HTTPS health endpoint. It keeps the replaced binary as
`/opt/syllabooks/syllabooks.previous`. `ExecStartPre` applies migrations before
the new process starts. A failed restart automatically restores the previous
binary and starts it again.

## 7. Automatic deployment from GitHub

`.github/workflows/deploy.yml` runs the same `make deploy` target after every
push to `master` or `main`, and can also be started manually from GitHub's
Actions page. Runs are serialized so two releases cannot modify production at
the same time.

Create a dedicated Ed25519 key and store its private key in the repository
Actions secret named `DEPLOY_SSH_KEY`. Add the public key to
`/home/zotov/.ssh/authorized_keys` with this prefix, all on one line:

```text
restrict,command="/usr/local/sbin/syllabooks-deploy" ssh-ed25519 ... syllabooks-github-actions
```

The forced command means this key cannot open a shell or select a different
command or destination. It can only replace the Syllabooks release and restart
the service. The committed `.github/known_hosts` pins the VPS host key so the
runner does not accept an unknown or impersonated SSH server.

Useful checks:

```sh
sudo systemctl status syllabooks
sudo journalctl -u syllabooks -n 100 --no-pager
curl --fail https://your-domain/api/health
```

To roll back application code:

```sh
sudo mv /opt/syllabooks/syllabooks.previous /opt/syllabooks/syllabooks
sudo systemctl restart syllabooks
```

Database migrations are intentionally forward-only in routine deploys; restore
a tested backup for disaster recovery instead of running automatic
down-migrations.

## 8. Nightly backups

Install the existing backup script and private environment file:

```sh
sudo install -d -o root -g root -m 0755 /opt/syllabooks/ops
sudo install -o root -g root -m 0755 ops/backup.sh /opt/syllabooks/ops/backup.sh
sudo install -o syllabooks -g syllabooks -m 0600 ops/backup.env.example /etc/syllabooks/backup.env
sudoedit /etc/syllabooks/backup.env
sudo -u syllabooks sh -c '. /etc/syllabooks/backup.env && /opt/syllabooks/ops/backup.sh'
sudo -u syllabooks crontab -e
```

Add the line from `ops/crontab.example`. It keeps 14 verified database dumps.
Copy dumps and `/var/lib/syllabooks/book-covers/` off the VPS periodically; a
backup stored only on the VPS does not protect against losing the VPS. Perform
the restore test from `README.md` before launch and after PostgreSQL upgrades.

## 9. Launch verification

- `https://your-domain/api/health` returns `ok`.
- The certificate is valid on a real phone, not just a desktop browser.
- Yandex and VK return to their configured callback URLs and leave the user
  signed in.
- A phone can grant camera permission and read a real EAN-13 barcode.
- A cover upload survives a service restart.
- A manual backup succeeds, `pg_restore --list` accepts it, and an off-server
  copy exists.
- PostgreSQL and the application are bound only to loopback; only 22/80/443 are
  publicly reachable.
