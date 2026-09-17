#!/bin/sh

set -eu
umask 077

: "${DATABASE_URL:?DATABASE_URL must point to the Syllabooks database}"

backup_dir=${BACKUP_DIR:-/var/backups/syllabooks}
retention_count=14
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
archive="$backup_dir/syllabooks-$timestamp.dump"
temporary="$backup_dir/.syllabooks-$timestamp-$$.dump.tmp"
listing="$backup_dir/.syllabooks-retention-$$.tmp"

cleanup() {
	rm -f -- "$temporary" "$listing"
}
trap cleanup EXIT HUP INT TERM

command -v pg_dump >/dev/null 2>&1 || {
	echo "pg_dump is not installed" >&2
	exit 1
}
command -v pg_restore >/dev/null 2>&1 || {
	echo "pg_restore is not installed" >&2
	exit 1
}

mkdir -p "$backup_dir"
chmod 700 "$backup_dir"

if [ -e "$archive" ]; then
	echo "backup already exists: $archive" >&2
	exit 1
fi

pg_dump \
	--format=custom \
	--no-owner \
	--no-privileges \
	--file="$temporary" \
	"$DATABASE_URL"

# Do not publish or prune anything unless pg_restore can read the archive.
pg_restore --list "$temporary" >/dev/null
mv "$temporary" "$archive"

find "$backup_dir" \
	-mindepth 1 \
	-maxdepth 1 \
	-type f \
	-name 'syllabooks-*.dump' \
	-print | LC_ALL=C sort >"$listing"

archive_count=$(wc -l <"$listing" | tr -d ' ')
if [ "$archive_count" -gt "$retention_count" ]; then
	remove_count=$((archive_count - retention_count))
	sed -n "1,${remove_count}p" "$listing" | while IFS= read -r old_archive; do
		rm -f -- "$old_archive"
	done
fi

echo "$archive"
