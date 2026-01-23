#!/bin/sh
set -e

# Handle signals gracefully during setup
cleanup() {
    echo "Received signal, shutting down..."
    exit 0
}
trap cleanup TERM INT

# Fix ownership of mounted volumes (runs as root)
if [ -d "/data" ]; then
    chown -R worker:worker /data
fi

# Remove trap before exec (tini handles signals for the main process)
trap - TERM INT

# Drop privileges and exec the main process as non-root user
exec gosu worker "$@"
