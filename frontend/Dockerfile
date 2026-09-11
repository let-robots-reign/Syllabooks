# Development image. compose.yaml bind-mounts the source over /app and keeps
# node_modules in a container volume, so host and container installs never mix.
FROM node:24-slim

RUN npm install -g pnpm@10.27.0

# Pin the store. Otherwise pnpm puts it on the project's filesystem, which is
# the bind mount at runtime: it would write .pnpm-store into the host folder
# and refuse to reuse the node_modules installed at build time.
ENV npm_config_store_dir=/root/.local/share/pnpm/store

WORKDIR /app

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Install again on start: the node_modules volume outlives image rebuilds, and
# this is a no-op when nothing changed.
CMD ["sh", "-c", "pnpm install --frozen-lockfile && exec pnpm dev --host 0.0.0.0"]
