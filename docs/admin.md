# Admin Panel

The Weather Service admin panel provides a web interface for managing the server, users, settings, and monitoring system health.

## Accessing the Admin Panel

Default URL: `https://wthr.top/admin`

### First-Time Setup

On first run, Weather Service launches the setup wizard:

1. Navigate to `https://wthr.top/admin/server/setup` in your browser
2. The setup wizard automatically opens
3. Create the primary admin account:
   - Username (email address)
   - Strong password (min 12 characters)
   - Enable 2FA (recommended, optional)
4. Configure basic server settings
5. Complete setup

The setup wizard is a one-time process. After completion, access the admin panel at `/admin`.

## Authentication

### Login

1. Go to `https://wthr.top/admin`
2. Enter admin username and password
3. If 2FA is enabled, enter the TOTP code
4. Click **Login**

### Two-Factor Authentication (2FA)

Enable 2FA for enhanced security:

1. Go to **Admin Panel** → **Security**
2. Click **Enable Two-Factor Authentication**
3. Scan QR code with authenticator app (Google Authenticator, Authy, etc.)
4. Enter verification code
5. Save recovery codes in a safe place

### Password Requirements

Admin passwords must meet these criteria:

- Minimum 12 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one number
- At least one special character (!@#$%^&*)

## Admin Panel Sections

### Dashboard

The dashboard provides an overview of server status and activity.

**Displays:**
- Server uptime and version
- Active users and sessions
- Recent API requests
- System resource usage (CPU, memory, disk)
- Weather data cache statistics
- GeoIP database status
- Recent errors and warnings

**Quick Actions:**
- Restart server
- Clear cache
- Download logs
- Run backup

### Users

Manage user accounts.

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

- **Enable GeoIP** - Auto-location via IP address
- **Auto-Update** - Automatically update GeoIP databases
- **Update Interval** - How often to update (default: 7 days)
- **Database Sources** - CDN URLs for GeoIP data

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
- **Rate Limiting** - API rate limits
- **IP Blocking** - Block specific IPs or countries
- **CORS** - Cross-origin resource sharing settings

#### SSL/TLS Settings

- **Enable HTTPS** - Use SSL/TLS
- **Certificate Path** - SSL certificate file
- **Key Path** - SSL private key file
- **Let's Encrypt** - Automatic SSL via Let's Encrypt
  - Email address
  - Domain names
  - Auto-renewal

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

### Scheduler

View and manage scheduled tasks.

**Scheduled Tasks:**
- **GeoIP Update** - Update GeoIP databases (weekly)
- **Database Vacuum** - Optimize database (daily)
- **Log Rotation** - Rotate log files (daily)
- **Notification Cleanup** - Clean old notifications (daily)
- **Session Cleanup** - Remove expired sessions (hourly)
- **Cache Cleanup** - Clear stale cache entries (hourly)

**Task Actions:**
- View last run time and status
- Run task immediately
- Enable/disable task
- Change schedule
- View task history

### Backup & Restore

Create and restore backups.

**Backup Features:**
- Manual backup creation
- Automated scheduled backups
- Backup to local filesystem
- Download backups
- View backup history
- Restore from backup

**Creating a Backup:**

1. Go to **Admin Panel** → **Backup**
2. Click **Create Backup**
3. Wait for backup to complete
4. Download backup file or leave on server

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

View audit logs at **Admin Panel** → **Logs** → **Audit**

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
- **Enable 2FA** - Protect admin access with TOTP
- **Regular password rotation** - Change passwords every 90 days
- **Unique passwords** - Don't reuse passwords from other services
- **Secure password storage** - Use a password manager

### Session Security

- Sessions expire after 7 days of inactivity (configurable)
- Sessions are invalidated on password change
- Logout from all devices available
- Session cookies are HTTP-only and secure

### Access Control

- Only server admins can access `/admin`
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
2. Run setup wizard reset: `wthr --maintenance setup`
3. Create new admin account
4. Start the server

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
