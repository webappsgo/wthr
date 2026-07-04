#!/bin/bash
set -e

# Set timezone
if [ -n "$TZ" ]; then
    ln -snf /usr/share/zoneinfo/$TZ /etc/localtime
    echo $TZ > /etc/timezone
fi

# Setup directories for EXTERNAL services only (PostgreSQL, Valkey)
# NOTE: App directories (config, data, sqlite, logs) are created by the server binary
# External services need special ownership that binary can't set
mkdir -p /data/db/postgres /data/db/valkey /run/postgresql /run/valkey /data/log/postgres
chown -R postgres:postgres /data/db/postgres /run/postgresql /data/log/postgres
chmod 700 /data/db/postgres
chmod 755 /run/valkey

# Initialize PostgreSQL if not already done
if [ ! -f /data/db/postgres/PG_VERSION ]; then
    echo "Initializing PostgreSQL database..."
    su - postgres -c "initdb -D /data/db/postgres"

    # Copy optimized config from /config/postgres/
    cp /config/postgres/postgresql.conf /data/db/postgres/postgresql.conf

    # Start PostgreSQL temporarily to create database and user
    su - postgres -c "pg_ctl -D /data/db/postgres -l /data/log/postgres/init.log start"
    sleep 3

    # Create application database and user
    su - postgres -c "psql -c \"CREATE USER ${DB_USER:-wthr} WITH PASSWORD '${DB_PASSWORD:-wthr}';\""
    su - postgres -c "psql -c \"CREATE DATABASE ${DB_NAME:-wthr} OWNER ${DB_USER:-wthr};\""
    su - postgres -c "psql -c \"GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME:-wthr} TO ${DB_USER:-wthr};\""

    # Stop PostgreSQL (supervisor will start it)
    su - postgres -c "pg_ctl -D /data/db/postgres stop"
fi

# Set Tor enabled flag for supervisor
export TOR_ENABLED="${TOR_ENABLED:-false}"

# Start supervisor (manages postgresql + valkey + tor + app)
exec /usr/bin/supervisord -c /etc/supervisor/supervisord.conf
