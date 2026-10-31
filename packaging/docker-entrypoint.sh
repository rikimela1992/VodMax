#!/bin/sh
# Docker entrypoint. Normally the image runs as its own user (1000). Started
# as root with PUID/PGID (Unraid, NAS: "--user 0" and PUID=99 PGID=100), it
# gives the data folder to that user and runs VodMax as that user.
set -e
if [ "$(id -u)" = 0 ] && [ -n "$PUID" ]; then
  PGID=${PGID:-$PUID}
  data=/var/lib/vodmax
  # Only when the owner is another: recordings can be many files.
  if [ "$(stat -c %u:%g "$data")" != "$PUID:$PGID" ]; then
    chown -R "$PUID:$PGID" "$data"
  fi
  exec su-exec "$PUID:$PGID" vodmax-server "$@"
fi
exec vodmax-server "$@"
