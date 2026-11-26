# Admin Panel

The Weather Service admin panel provides a web interface for managing the server, users, settings, and monitoring system health.

## Accessing the Admin Panel

Default path: `/server/administration` (the operator may configure another admin path).

### First-Time Setup

On first run, Weather Service launches the setup wizard:

1. Navigate to `/server/administration/setup` in your browser
2. The setup wizard automatically opens
3. Create the primary admin account:
   - Username (email address)
   - Strong password (min 12 characters)
4. Configure basic server settings
5. Complete setup
6. Register a passkey from the admin profile to require a second authentication factor at sign-in

The setup wizard is a one-time process. After completion, access the admin panel at `/server/administration`.

## Authentication

### Login

1. Go to `/server/administration`
2. Enter admin username and password
3. If the account has a passkey registered, complete the passkey prompt. Otherwise, enter the authenticator code or an unused recovery code when TOTP is enabled.
4. Click **Login**

### Passkeys

Register a passkey to add a second factor to password sign-in:

1. Go to your admin profile's **Security** section
2. Enter your current password and register a passkey using the browser prompt
3. Remove a passkey from the same page when it is no longer trusted

### TOTP and recovery codes

Open your admin profile's **Security** page, confirm your current password, then scan the setup
QR code or enter its manual key in an authenticator app. Confirm setup with the current six-digit
code and acknowledge that you saved the ten recovery codes. Each recovery code works once. Store
them offline; they are shown only when created or regenerated. To regenerate them, enter a current
authenticator code. Disabling TOTP requires an authenticator code or an unused recovery code.

The profile API exposes the same controls under
`/api/v1/server/{admin_path}/{admin_username}/profile/security/totp`.

Admins may also register or remove an armored OpenPGP public key from the profile Security page.
This personal key receives encrypted security-report maintainer notifications; it is not the
project key shown on the public security page.

### Password Requirements

Admin passwords must meet these criteria:

- Minimum 12 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one number
- At least one special character (!@#$%^&*)

## Admin Panel Sections

### URL layout

Everything under `/server/{admin_path}/` (default `administration`) is either the dashboard, the
admin's own area (`/server/{admin_path}/{admin_username}/profile`, `/preferences`,
`/notifications`) or a fixed management section named directly: `settings`, `users`, `admins`,
`security`, `network`, `email`, `notifications`, `web`, `scheduler`, `logs`, `backup`, `updates`,
`ssl`, and so on. There is no `config` segment. The API mirrors the same layout under
`/api/v1/server/{admin_path}/`. An admin username can never equal a section name.

Logging out uses `/server/auth/logout` for admins and users alike.

The `/server/{admin_path}/admins` page shows the signed-in admin's own account and the total
admin count. It does not list other admins' usernames, email addresses, sessions, or credentials.
Use `/server/{admin_path}/admins/invite` to invite another admin. Each admin's profile and
security settings are under `/server/{admin_path}/{admin_username}/`.

Admin web forms use the admin session cookie, CSRF protection, and POST-redirect-GET feedback.
They remain usable with JavaScript disabled. The separate `/api/v1/server/{admin_path}/` routes
are Bearer-token APIs and are not form targets.

### Dashboard

The dashboard shows total user accounts, administrator accounts, saved
locations, and service status. Its quick links open the server-rendered user,
invite, settings, security, database, SSL, backup, token, log, scheduler, and
administrator pages. Each management page owns its own forms and feedback.

### Roles

The Roles page reads and updates `server.users.roles`. It lists configured roles
and assigned user counts, sets the default role for new accounts, and lets
admins create custom roles with `read`, `write`, and `moderate` permissions.
Custom permissions can be updated in place. Built-in `admin` and `user` roles
are locked. A custom role cannot be deleted while assigned to users or while it
is the default role.

**Quick Actions:**
- Restart server (graceful shutdown; a service manager or container restart policy starts the new process, a manually started server re-executes itself)
- Clear cache
- Download logs
- Run backup

### Users

Open `/server/{admin_path}/moderation/users` to search and filter accounts. The
user detail page shows the selected account's profile, two-factor status, active
session metadata, and recent activity. Session-authenticated forms can disable
or enable the account, revoke all of its sessions, and delete it. Delete requires
an explicit confirmation checkbox. These forms use CSRF protection and
POST-redirect-GET feedback, so the moderation pages remain usable without
JavaScript.

The **Impersonate user** action creates a separate 15-minute user session and
keeps the admin session in its own cookie. The user session does not contain
admin privileges. A notice with a **Stop impersonating and return to admin**
form appears in the user navigation; submitting it revokes the impersonation
session and returns to the admin panel. Start and stop are recorded in the
audit log with the acting admin and target user. The Bearer-token API remains
separate from these browser forms.

### API token registry

The API token page lists user-token names, owner email addresses, token prefixes,
creation and use times, and expiry. Admins can revoke a token through a
CSRF-protected form. Creating a token displays its plaintext once in the POST
response; copy it before leaving the page because it cannot be retrieved later.
The stored token remains hashed. Browser token forms do not call the Bearer API.

**Features:**
- List all registered users
- Create new users
- Edit user profiles
- Reset user passwords
- Enable/disable user accounts
- Delete users
- View user activity logs
- Manage user locations

**User Table Columns:**
- Username/Email
- Registration date
- Last login
- Status (active/disabled)
- 2FA enabled
- Saved locations count
- Actions (edit, disable, delete)

**Creating a User:**

1. Click **Add User**
2. Enter user details:
   - Username
   - Email address
   - Password (auto-generated or manual)
3. Set user status (active/disabled)
4. Click **Create User**
5. Send credentials to user (manually or via email)

### Settings

Configure server-wide settings through the web UI.

Admin settings and operational changes use session-authenticated HTML POST forms with CSRF
tokens and redirect-after-save feedback, so those controls remain usable with JavaScript disabled.
JavaScript adds live dashboards, previews, and other optional enhancements.

#### General Settings

- **Server Name** - Display name for the service
- **Server Mode** - Production or development
- **Debug Mode** - Enable verbose logging
- **Maintenance Mode** - Put server in maintenance mode

#### Weather Settings

- **Enable Weather** - Toggle weather forecasts
- **Enable Earthquakes** - Toggle earthquake data
- **Enable Hurricanes** - Toggle hurricane tracking
- **Enable Severe Weather** - Toggle severe weather alerts
- **Enable Moon Phase** - Toggle lunar information
- **Cache Duration** - Weather data cache TTL

#### GeoIP Settings

- **Enable GeoIP** - Enable lookups; the built-in scheduler updates databases weekly
- **Deny Countries** - Block selected ISO 3166-1 alpha-2 country codes
- **Allow Countries** - Allow only selected codes; this takes precedence over the deny list
- **Saved Country Lists** - Save operator-defined lists as named presets and load them into either list

The controls are available at `/server/{admin_path}/network/geoip`. Database paths and update
schedules are built in and are not configurable there. Missing databases and lookup failures fail
open for GeoIP while the remaining security checks still run. IPs in `server.security.allowlist`
bypass country blocking.

#### Firewall and Trusted IPs

Use `/server/{admin_path}/security/firewall` for trusted entries, or
`/server/{admin_path}/security/blocked-ips` for the same page focused on IP blocks. Trusted entries
need a description and are stored in `{config_dir}/yaml.d/security.yml`;
they bypass IP blocks, rate limits, and country blocking while still requiring admin
authentication. Manual blocks require a reason and can be temporary (one hour), extended (24
hours), or permanent. Changes apply immediately.

#### Authentication Settings

- **Session Duration** - How long sessions last
- **Password Requirements** - Enforce password complexity
- **Login Rate Limit** - Max login attempts
- **Registration** - Enable/disable user registration

#### Notification Settings

- **Enable Notifications** - WebSocket notifications
- **Toast Notifications** - Pop-up toast messages
- **Banner Notifications** - Top banner messages
- **Notification Center** - Notification history panel
- **Sound Alerts** - Audio notification alerts

#### Email/SMTP Settings

- **Enable Email** - Send email notifications
- **SMTP Host** - Mail server hostname
- **SMTP Port** - Mail server port (25, 587, 465)
- **SMTP Username** - Authentication username
- **SMTP Password** - Authentication password
- **From Address** - Email sender address
- **Use TLS** - Enable TLS encryption

#### Security Settings

- **CSRF Protection** - Enable CSRF tokens
- **Rate Limiting** - Always-on limits by request class, editable at `/server/{admin_path}/security/ratelimit`; there is no enable/disable switch.
- **IP Blocking** - Block specific IPs or countries
- **CORS** - Cross-origin settings on the **Web** page: allow all origins or list origins, with a preview of the headers sent

#### SSL/TLS Settings

- View certificate status, validity dates, domains, and renewal information.
- Obtain or renew certificates through HTTP-01, TLS-ALPN-01, or a supported DNS-01 provider.
- Start auto-renewal for a domain list and challenge type, or revoke a certificate.
- Obtain, renew, auto-renew, DNS provider save, and revoke actions use CSRF-protected POST forms.

### Branding and SEO

The `/server/{admin_path}/branding` page manages site identity, SEO metadata, accent color,
and operator-authored page content. Changes are stored in `yaml.d/branding.yml`. Logo and favicon
uploads are validated and saved under the private config `assets/` directory; the server generates
responsive logo sizes, favicon sizes, an Apple touch icon, and an Open Graph image variant. Remote
image URLs are also supported and are fetched through the validated image pipeline. See
[Configuration](configuration.md#branding-and-seo) for the supported settings and generated sizes.

### Web (robots.txt, security.txt, feeds)

`/server/administration/web` manages:

- **robots.txt** - one allowed path and one denied path per line; `{admin_path}` stands for the
  configured admin path. A read-only preview shows the generated file. The sitemap never lists a
  path that `robots.txt` disallows.
- **Syndication feeds** - entries per feed (1 to 200) and the feed title for `/feed.xml` and
  `/rss.xml`, which are always served.
- **security.txt** - Contact, preferred languages, policy URL, and encryption key URL.
- **MCP endpoint** - Always available at the displayed endpoint URL and authenticated with the
  same Bearer tokens as the API. AI.md does not define an admin toggle for MCP.

### Security reports

`/server/administration/security/` holds the coordinated-disclosure tools: `reports` (list, filter,
decrypted view, status, comment and disclosure date), `policy` (edit the public policy page),
`thanks` (hall of fame entries) and `keypair` (the project PGP key: generate, rotate, publish to
keyservers, download the public key, export or import the private key, delete). Exporting or
importing the private key and deleting the keypair ask for your password; the export also needs a
reason and is limited to once per hour. See [Security](security.md#security-reporting).

### Settings owned by drop-in files

Settings that a file in `yaml.d/` sets are listed at the top of the Settings page with the file
that owns them. The panel never writes them into `server.yml`; edit the drop-in file instead.

### Logs

View and manage server logs.

**Log Types:**
- **Application Logs** - Server events and errors
- **Access Logs** - HTTP request logs
- **Audit Logs** - Admin actions and security events
- **Error Logs** - Error-only logs

**Features:**
- Real-time log streaming
- Filter by log level (debug, info, warn, error)
- Search logs by keyword
- Download logs as files
- Clear old logs
- Configure log retention

**Log Levels:**
- **DEBUG** - Detailed debugging information
- **INFO** - General informational messages
- **WARN** - Warning messages (non-critical)
- **ERROR** - Error messages (failures)

### Metrics

Monitor system performance and usage.

**Metrics Displayed:**
- API request rate (requests/minute)
- Response time percentiles (p50, p95, p99)
- Cache hit/miss ratio
- Database query performance
- Memory usage over time
- Active WebSocket connections
- GeoIP lookup performance
- External API call statistics

**Charts:**
- Request rate over time
- Response time distribution
- Memory usage timeline
- Cache performance
- Top API endpoints
- Geographic distribution of requests

The Metrics page's settings form works without JavaScript. Its no-JavaScript links open the
registered-metrics listing and the Prometheus, JSON, or OpenMetrics export through the
session-authenticated admin web routes.

### Scheduler

The built-in scheduler always runs; there is no switch to turn it off and no external cron is used.
The page at **Scheduler** lists every task with its status, schedule, last run and next run, all
shown in the scheduler timezone. It works without JavaScript: every button is a plain form.

**Built-in tasks:**
- **SSL Renewal**, **Session Cleanup**, **Token Cleanup**, **Log Rotation**, **Health Check**,
  **Tor Health**, **I2P Health** and **Cluster Heartbeat** are critical and can never be disabled
- **GeoIP Update**, **Blocklist Update**, **CVE Update**, **Update Check** and **Backup Daily** can
  be disabled
- **Backup Hourly** is an hourly incremental and is disabled by default
- Weather, earthquake, hurricane and notification tasks run on their own schedules

**Task actions:**
- Run a task immediately
- Enable or disable a task (critical tasks cannot be disabled)
- Change a task's schedule: a cron expression, `@hourly`, `@daily`, `@weekly`, `@monthly` or
  `@every 15m`
- Adjust only the settings exposed for that task; backup verification and health-task restart
  behavior are fixed by the task definition
- View a task's last 100 runs, with duration and error text

**Scheduler settings:** the timezone that schedules are evaluated in (default `America/New_York`)
and the catch-up window (default `1h`): a run missed while the server was down is executed at
startup when it was due inside that window. Changes are saved to `server.yml` and applied
immediately, without a restart.

### Backup & Restore

Create and restore backups.

**Backup Features:**
- Manual backup creation
- Automated scheduled backups
- Optional backup encryption; set or change the password during setup or from the Backup page
- Enabling any `server.compliance` standard makes encryption mandatory; the Backup page requires
  a password before creating backups, and scheduled jobs skip with an audit warning until one is saved
- Backup to local filesystem
- Download backups
- View backup history
- Restore from backup

**Creating a Backup:**

1. Go to **Admin Panel** → **Backup**
2. Click **Create Backup**
3. Wait for backup to complete
4. Download backup file or leave on server

The backup password is encrypted at rest in `{config_dir}/secrets.yml` using the server encryption
key. It is not returned by an API or displayed again. Scheduled backups use the saved password;
restoring an encrypted archive still requires the password to be supplied for that restore.

Backups include databases and configuration, including account credential hashes and application
secrets. Without a backup password, the archive is unencrypted and its contents are readable by
anyone who obtains the file. Store backups with access controls appropriate for credentials, or
set a backup encryption password before creating one.

**Restoring a Backup:**

!!! danger "Service Interruption"
    Restoring a backup will stop the server and replace all data.
    All current sessions will be terminated.

1. Go to **Admin Panel** → **Backup**
2. Select backup file
3. Click **Restore**
4. Confirm restoration
5. Server will restart with restored data

### Notifications

Manage admin panel notifications.

The **Notify** page (`/server/{admin_path}/notify`) configures Admin, Security, Abuse,
and General contact email and webhook transports. Its test action sends a sample through
the configured transports. Every webhook request carries an HMAC signature, timestamp,
UUID v7 delivery ID, event type, and project User-Agent. If delivery fails, it retries at
1 minute, 5 minutes, 15 minutes, 1 hour, 6 hours, and 24 hours using the same delivery
ID, then drops the event. Webhook URLs and signing keys are never displayed; read the
signing key from `{config_dir}/secrets.yml` (mode `0600`).

The admin preferences page links to `/server/preferences#language`; language changes are
made only in that public preferences page.

**Notification Types:**
- **Success** - Operation completed successfully
- **Info** - Informational message
- **Warning** - Warning or caution
- **Error** - Error or failure
- **Security** - Security-related events

**Display Modes:**
- **Toast** - Pop-up notification (auto-dismiss)
- **Banner** - Top banner (requires dismiss)
- **Center** - Notification center panel

**Features:**
- View notification history
- Mark notifications as read
- Dismiss notifications
- Configure notification preferences
- Enable/disable notification types

### Tor Hidden Service

Configure Tor hidden service (if enabled).

**Features:**
- Enable/disable Tor hidden service
- View .onion address
- Configure hidden service port
- Monitor Tor connection status
- Regenerate .onion address

**Enabling Tor:**

1. Ensure Tor is installed (included in Docker image)
2. Go to **Admin Panel** → **Tor**
3. Click **Enable Tor Hidden Service**
4. Wait for .onion address generation (may take 1-2 minutes)
5. Copy .onion address for use

Your weather service will be accessible at:
```
http://abc123def456.onion
```

## Admin Panel Features

### Live Settings Updates

Most settings can be updated without restarting the service:

1. Change setting in admin panel
2. Click **Save**
3. Setting takes effect immediately

Settings requiring restart:
- Listen address/port
- Database paths
- SSL certificates

### Audit Trail

All admin actions are logged:

- User created/edited/deleted
- Settings changed
- Backups created/restored
- Domains added/verified/deleted
- System maintenance performed

View audit logs at **Admin Panel** → **Logs** → **Audit**. Each JSONL entry includes
`prev`, the lowercase SHA-256 hash of the preceding exact log line (excluding its
newline). The first entry starts with 64 zeroes; after rotation, the new file starts
from the previous file's final line hash. Startup and rotation verify the chain. A
mismatch produces one critical `security.audit_chain_broken` event with only the file
name and line number; the server continues running. By default, rotation deletes the
closed archive after verification and stores its final hash in `audit.log.chain` so the
next active file remains verifiable. This detects edits, removals, and reordering, but
cannot prevent someone with file access from rewriting the entire chain and checkpoint
and recomputing their hashes.

### Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl + K` | Global search |
| `Ctrl + /` | Show shortcuts |
| `Esc` | Close modals/dialogs |
| `g d` | Go to dashboard |
| `g u` | Go to users |
| `g s` | Go to settings |

### Mobile Support

The admin panel is fully responsive and works on:
- Desktop browsers
- Tablets
- Mobile phones (portrait and landscape)

## Security Best Practices

### Admin Account Security

- **Use strong passwords** - At least 12 characters with complexity
- **Passkeys** - Add a WebAuthn passkey as a second factor for admin sign-in
- **Regular password rotation** - Change passwords every 90 days
- **Unique passwords** - Don't reuse passwords from other services
- **Secure password storage** - Use a password manager

### Session Security

- Sessions expire after 7 days of inactivity (configurable)
- Sessions are invalidated on password change
- Logout from all devices available
- Session cookies are HTTP-only and secure

### Access Control

- Only server admins can access `/server/administration`
- Regular users cannot access admin panel
- Failed login attempts are rate-limited
- Suspicious activity triggers security notifications

### Audit and Monitoring

- Review audit logs regularly
- Monitor failed login attempts
- Check for unusual API activity
- Review user account changes

## Troubleshooting

### Cannot Access Admin Panel

**Problem:** Admin panel shows "Access Denied"

**Solution:**
- Ensure you're logged in as an admin
- Check that admin role is set correctly in database
- Verify session cookie is present

### Forgot Admin Password

**Problem:** Cannot remember admin password

**Solution:**
1. Stop the server
2. Run the recovery reset as root or with a valid setup token: `wthr --maintenance setup`. It removes every admin account, API token, passkey and admin session, keeps all user accounts, user data and settings, and prints a new one-time setup token
3. Start the server, open the admin URL and enter the setup token
4. Create the new admin account in the setup wizard

### Settings Not Saving

**Problem:** Changes to settings don't persist

**Solution:**
- Check file permissions on config directory
- Verify database is writable
- Check server logs for errors
- Ensure maintenance mode is off

## Next Steps

- [Configuration](configuration.md) - Manual configuration file editing
- [CLI Reference](cli.md) - Command-line management
- [API Reference](api.md) - Programmatic access
- [Development](development.md) - Contributing to Weather Service
