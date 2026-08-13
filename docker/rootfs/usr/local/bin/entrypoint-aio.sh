#!/bin/bash
# shellcheck shell=bash
# - - - - - - - - - - - - - - - - - - - - - - - -
##@Version           :  202608131924-git
# @@Author           :  Jason Hempstead
# @@Contact          :  git-admin@casjaysdev.pro
# @@License          :  WTFPL
# @@ReadME           :  {scriptname --help | README.md}
# @@Copyright        :  Copyright: (c) 2026 Jason Hempstead, Casjays Developments
# @@Created          :  Thursday, August 13, 2026 19:24 EDT
# @@File             :  entrypoint-aio.sh
# @@Description      :  Container entrypoint for the all-in-one image; initializes PostgreSQL/Valkey and execs supervisord
# @@Changelog        :  Bring script into CasjaysDev header and lint compliance
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  yes
# @@Template         :  shell/bash
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
set -e

# Set timezone
if [ -n "$TZ" ]; then
    ln -snf "/usr/share/zoneinfo/$TZ" /etc/localtime
    echo "$TZ" > /etc/timezone
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

# ex: ts=2 sw=2 et filetype=sh
