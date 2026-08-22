/**
 * Weather Service - Application JavaScript
 * Provides utilities for modals, alerts, toasts, and interactive components
 */

(function() {
  'use strict';

  // ============================================
  // UTILITY FUNCTIONS
  // ============================================

  const Utils = {
    /**
     * Create and dispatch a custom event
     */
    dispatchEvent: function(eventName, detail = {}) {
      const event = new CustomEvent(eventName, { detail, bubbles: true });
      document.dispatchEvent(event);
    },

    /**
     * Debounce function calls
     */
    debounce: function(func, wait) {
      let timeout;
      return function executedFunction(...args) {
        const later = () => {
          clearTimeout(timeout);
          func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
      };
    },

    /**
     * Generate unique ID
     */
    generateId: function() {
      return 'id_' + Math.random().toString(36).substr(2, 9);
    }
  };

  // ============================================
  // MODAL SYSTEM
  // ============================================

  const Modal = {
    /**
     * Open a modal
     */
    open: function(modalId) {
      const overlay = document.getElementById(modalId);
      if (overlay) {
        overlay.classList.add('active');
        document.body.style.overflow = 'hidden';
        Utils.dispatchEvent('modal:opened', { modalId });
      }
    },

    /**
     * Close a modal with animation
     */
    close: function(modalId) {
      const overlay = document.getElementById(modalId);
      if (overlay) {
        // Clear auto-close interval if exists
        const intervalId = overlay.getAttribute('data-auto-close-interval');
        if (intervalId) {
          clearInterval(parseInt(intervalId));
        }

        // Add closing animation
        overlay.classList.add('closing');
        overlay.classList.remove('active');

        // Remove from DOM after animation
        setTimeout(() => {
          overlay.remove();
          document.body.style.overflow = '';
          Utils.dispatchEvent('modal:closed', { modalId });
        }, 200);
      }
    },

    /**
     * Create and show a modal programmatically
     */
    create: function(options) {
      const {
        title = 'Modal',
        body = '',
        footer = '',
        onClose = null,
        size = 'md',
        autoClose = 0,  // Auto-close after N seconds (0 = no auto-close)
        closeable = true
      } = options;

      const modalId = Utils.generateId();
      const autoCloseHtml = autoClose > 0
        ? `<div class="modal-auto-close">Auto-closing in <span id="${modalId}-countdown">${autoClose}</span>s</div>`
        : '';

      const modalHTML = `
        <div id="${modalId}" class="modal-overlay">
          <div class="modal modal-${size}">
            <div class="modal-header">
              <h3 class="modal-title">${title}</h3>
              ${closeable ? `<button class="modal-close" data-action="modal-close" aria-label="Close">&times;</button>` : ''}
            </div>
            <div class="modal-body">
              ${body}
              ${autoCloseHtml}
            </div>
            ${footer ? `<div class="modal-footer">${footer}</div>` : ''}
          </div>
        </div>
      `;

      document.body.insertAdjacentHTML('beforeend', modalHTML);

      // Close on overlay click (if closeable)
      const overlay = document.getElementById(modalId);

      // Prevent clicks on modal content from closing the modal
      const modalContent = overlay.querySelector('.modal');
      if (modalContent) {
        modalContent.addEventListener('click', function(e) {
          e.stopPropagation();
        });
      }

      if (closeable) {
        overlay.addEventListener('click', function(e) {
          if (e.target === overlay) {
            Modal.close(modalId);
            if (onClose) onClose();
          }
        });
      }

      // Close on Escape key (if closeable)
      if (closeable) {
        const escapeHandler = function(e) {
          if (e.key === 'Escape') {
            Modal.close(modalId);
            if (onClose) onClose();
            document.removeEventListener('keydown', escapeHandler);
          }
        };
        document.addEventListener('keydown', escapeHandler);
      }

      // Auto-close countdown
      if (autoClose > 0) {
        let remaining = autoClose;
        const countdownEl = document.getElementById(`${modalId}-countdown`);

        const interval = setInterval(() => {
          remaining--;
          if (countdownEl) {
            countdownEl.textContent = remaining;
          }

          if (remaining <= 0) {
            clearInterval(interval);
            Modal.close(modalId);
            if (onClose) onClose();
          }
        }, 1000);

        // Store interval ID to clear if manually closed
        overlay.setAttribute('data-auto-close-interval', interval);
      }

      Modal.open(modalId);
      return modalId;
    }
  };

  // ============================================
  // TOAST NOTIFICATIONS
  // ============================================

  const Toast = {
    container: null,

    /**
     * Initialize toast container
     */
    init: function() {
      if (!this.container) {
        this.container = document.createElement('div');
        this.container.className = 'toast-container';
        document.body.appendChild(this.container);
      }
    },

    /**
     * Show a toast notification
     */
    show: function(message, options = {}) {
      this.init();

      const {
        type = 'info',  // 'success', 'error', 'warning', 'info'
        title = '',
        duration = 5000,
        dismissible = true
      } = options;

      const icons = {
        success: '✓',
        error: '✗',
        warning: '⚠',
        info: 'ℹ'
      };

      const toastId = Utils.generateId();
      const toastHTML = `
        <div id="${toastId}" class="toast toast-${type}">
          <div class="toast-icon">${icons[type]}</div>
          <div class="toast-content">
            ${title ? `<div class="toast-title">${title}</div>` : ''}
            <div class="toast-message">${message}</div>
          </div>
          ${dismissible ? '<button class="toast-close" data-action="toast-dismiss" aria-label="Dismiss">&times;</button>' : ''}
        </div>
      `;

      this.container.insertAdjacentHTML('beforeend', toastHTML);

      if (duration > 0) {
        setTimeout(() => this.dismiss(toastId), duration);
      }

      Utils.dispatchEvent('toast:shown', { toastId, type, message });
      return toastId;
    },

    /**
     * Dismiss a toast
     */
    dismiss: function(toastId) {
      const toast = document.getElementById(toastId);
      if (toast) {
        toast.style.opacity = '0';
        toast.style.transform = 'translateX(100%)';
        setTimeout(() => toast.remove(), 300);
        Utils.dispatchEvent('toast:dismissed', { toastId });
      }
    },

    /**
     * Convenience methods
     */
    success: function(message, options = {}) {
      return this.show(message, { ...options, type: 'success' });
    },

    error: function(message, options = {}) {
      return this.show(message, { ...options, type: 'error' });
    },

    warning: function(message, options = {}) {
      return this.show(message, { ...options, type: 'warning' });
    },

    info: function(message, options = {}) {
      return this.show(message, { ...options, type: 'info' });
    }
  };

  // ============================================
  // ALERT SYSTEM
  // ============================================

  const Alert = {
    /**
     * Create an alert element
     */
    create: function(message, options = {}) {
      const {
        type = 'info',  // 'success', 'error', 'warning', 'info'
        title = '',
        dismissible = true,
        container = null
      } = options;

      const icons = {
        success: '✓',
        error: '✗',
        warning: '⚠',
        info: 'ℹ'
      };

      const alertId = Utils.generateId();
      const alertHTML = `
        <div id="${alertId}" class="alert alert-${type} ${dismissible ? 'alert-dismissible' : ''}">
          <div class="alert-icon">${icons[type]}</div>
          <div class="alert-content">
            ${title ? `<div class="alert-title">${title}</div>` : ''}
            <div class="alert-message">${message}</div>
          </div>
          ${dismissible ? '<button class="alert-close" data-action="alert-dismiss" aria-label="Dismiss">&times;</button>' : ''}
        </div>
      `;

      if (container) {
        const containerEl = typeof container === 'string'
          ? document.getElementById(container) || document.querySelector(container)
          : container;

        if (containerEl) {
          containerEl.insertAdjacentHTML('beforeend', alertHTML);
        }
      }

      return alertId;
    },

    /**
     * Dismiss an alert
     */
    dismiss: function(alertId) {
      const alert = document.getElementById(alertId);
      if (alert) {
        alert.style.opacity = '0';
        setTimeout(() => alert.remove(), 300);
      }
    }
  };

  // ============================================
  // DROPDOWN SYSTEM
  // ============================================

  const Dropdown = {
    activeDropdown: null,

    /**
     * Toggle dropdown visibility
     */
    toggle: function(dropdownId, triggerId = null) {
      const dropdown = document.getElementById(dropdownId);
      if (!dropdown) return;

      // Check if using hidden attribute or active class
      const usesHidden = dropdown.hasAttribute('hidden');
      const isActive = usesHidden ? !dropdown.hidden : dropdown.classList.contains('active');

      // Close any other open dropdowns
      this.closeAll();

      if (!isActive) {
        // Open dropdown
        if (usesHidden) {
          dropdown.removeAttribute('hidden');
        } else {
          dropdown.classList.add('active');
        }
        this.activeDropdown = dropdownId;

        // Fetch notifications when notification dropdown opens
        if (dropdownId === 'notification-dropdown' && typeof Notifications !== 'undefined') {
          Notifications.fetchList();
        }

        // CSS handles positioning via position:absolute + right:0 on the dropdown.
        // Do not override with viewport-relative inline styles — the dropdown is
        // position:absolute inside a position:relative parent, so right:0 already
        // aligns it correctly. Overriding broke mobile (pushed off-screen left).

        Utils.dispatchEvent('dropdown:opened', { dropdownId });
      }
    },

    /**
     * Close a specific dropdown
     */
    close: function(dropdownId) {
      const dropdown = document.getElementById(dropdownId);
      if (dropdown) {
        if (dropdown.hasAttribute('hidden') || dropdown.hidden === false) {
          dropdown.setAttribute('hidden', '');
        } else {
          dropdown.classList.remove('active');
        }
        if (this.activeDropdown === dropdownId) {
          this.activeDropdown = null;
        }
        Utils.dispatchEvent('dropdown:closed', { dropdownId });
      }
    },

    /**
     * Close all dropdowns
     */
    closeAll: function() {
      // Close class-based dropdowns
      document.querySelectorAll('.profile-dropdown.active, .dropdown.active').forEach(dropdown => {
        dropdown.classList.remove('active');
      });
      // Close hidden-attribute dropdowns
      document.querySelectorAll('.notification-dropdown:not([hidden])').forEach(dropdown => {
        dropdown.setAttribute('hidden', '');
      });
      this.activeDropdown = null;
    }
  };

  // ============================================
  // MOBILE MENU
  // ============================================

  const MobileMenu = {
    /**
     * Toggle mobile menu
     */
    toggle: function() {
      const menu = document.querySelector('.navbar-menu-mobile');
      if (menu) {
        menu.classList.toggle('active');
        Utils.dispatchEvent('mobile-menu:toggled', {
          isActive: menu.classList.contains('active')
        });
      }
    },

    /**
     * Close mobile menu
     */
    close: function() {
      const menu = document.querySelector('.navbar-menu-mobile');
      if (menu) {
        menu.classList.remove('active');
      }
    }
  };

  // ============================================
  // NOTIFICATION SYSTEM
  // ============================================

  const Notifications = {
    unreadCount: 0,
    notifications: [],

    /**
     * Get API base path
     */
    getBasePath: function() {
      return (window.API_PATHS && window.API_PATHS.notifications)
        ? window.API_PATHS.notifications
        : '/api/v1/users/notifications';
    },

    /**
     * Update notification badge count
     */
    updateBadge: function(count) {
      this.unreadCount = count;
      const badge = document.getElementById('notification-badge');

      if (badge) {
        if (count > 0) {
          badge.textContent = count > 9 ? '9+' : count;
        } else {
          badge.textContent = '';
        }
      }

      Utils.dispatchEvent('notifications:updated', { count });
    },

    /**
     * Fetch and update notification count
     */
    fetch: async function() {
      try {
        const response = await fetch(this.getBasePath() + '/unread', {
          credentials: 'same-origin'
        });

        // Stop polling if unauthorized (user logged out)
        if (response.status === 401) {
          this.stopPolling();
          return;
        }

        if (response.ok) {
          const data = await response.json();
          this.updateBadge(data.unread_count || 0);
        }
      } catch (error) {
        console.error('Failed to fetch notifications:', error);
      }
    },

    /**
     * Fetch notifications list for dropdown
     */
    fetchList: async function() {
      try {
        const response = await fetch(this.getBasePath() + '?limit=5', {
          credentials: 'same-origin'
        });

        if (response.ok) {
          const data = await response.json();
          this.notifications = data.notifications || [];
          this.renderList();
        }
      } catch (error) {
        console.error('Failed to fetch notifications list:', error);
      }
    },

    /**
     * Render notifications in dropdown
     */
    renderList: function() {
      const list = document.getElementById('notification-list');
      if (!list) return;

      if (this.notifications.length === 0) {
        list.innerHTML = '<div class="notification-empty">No notifications</div>';
        return;
      }

      list.innerHTML = this.notifications.map(function(n) {
        var icon = n.type === 'error' ? '❌' : n.type === 'warning' ? '⚠️' : n.type === 'success' ? '✅' : 'ℹ️';
        var unreadClass = n.read ? '' : ' unread';
        var timeAgo = Notifications.formatTimeAgo(n.created_at);

        return '<a href="' + (n.link || '/users/notifications') + '" class="notification-item' + unreadClass + '" data-id="' + n.id + '">' +
          '<span class="notification-dot"></span>' +
          '<span class="notification-icon">' + icon + '</span>' +
          '<div class="notification-content">' +
            '<span class="notification-title">' + (n.title || 'Notification') + '</span>' +
            '<span class="notification-message">' + (n.message || '') + '</span>' +
            '<span class="notification-time">' + timeAgo + '</span>' +
          '</div>' +
        '</a>';
      }).join('');
    },

    /**
     * Format timestamp to relative time
     */
    formatTimeAgo: function(timestamp) {
      if (!timestamp) return '';
      var date = new Date(timestamp);
      var now = new Date();
      var diff = Math.floor((now - date) / 1000);

      if (diff < 60) return 'Just now';
      if (diff < 3600) return Math.floor(diff / 60) + 'm ago';
      if (diff < 86400) return Math.floor(diff / 3600) + 'h ago';
      if (diff < 172800) return 'Yesterday';
      return Math.floor(diff / 86400) + 'd ago';
    },

    /**
     * Mark all notifications as read
     */
    markAllRead: async function() {
      try {
        const response = await fetch(this.getBasePath() + '/read-all', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (response.ok) {
          this.updateBadge(0);
          this.notifications.forEach(function(n) { n.read = true; });
          this.renderList();
          Toast.show('All notifications marked as read', 'success');
        }
      } catch (error) {
        console.error('Failed to mark notifications as read:', error);
      }
    },

    /**
     * Start polling for notifications
     */
    startPolling: function(interval = 30000) {
      this.fetch(); // Initial fetch
      this.pollInterval = setInterval(() => this.fetch(), interval);
    },

    /**
     * Stop polling
     */
    stopPolling: function() {
      if (this.pollInterval) {
        clearInterval(this.pollInterval);
        this.pollInterval = null;
      }
    }
  };

  // ============================================
  // FORM UTILITIES
  // ============================================

  const Forms = {
    /**
     * Serialize form data to JSON
     */
    serializeToJSON: function(formElement) {
      const formData = new FormData(formElement);
      const json = {};

      for (const [key, value] of formData.entries()) {
        if (json[key]) {
          if (!Array.isArray(json[key])) {
            json[key] = [json[key]];
          }
          json[key].push(value);
        } else {
          json[key] = value;
        }
      }

      return json;
    },

    /**
     * Validate form
     */
    validate: function(formElement) {
      if (!formElement.checkValidity()) {
        formElement.reportValidity();
        return false;
      }
      return true;
    },

    /**
     * Show form errors
     */
    showErrors: function(formElement, errors) {
      // Clear existing errors
      formElement.querySelectorAll('.form-error').forEach(el => el.remove());

      // Add new errors
      Object.entries(errors).forEach(([field, message]) => {
        const input = formElement.querySelector(`[name="${field}"]`);
        if (input) {
          const error = document.createElement('div');
          error.className = 'form-error';
          error.textContent = message;
          input.parentNode.appendChild(error);
        }
      });
    }
  };

  // ============================================
  // LOADING STATES
  // ============================================

  const Loading = {
    /**
     * Show loading spinner
     */
    show: function(element, size = 'md') {
      const spinner = document.createElement('div');
      spinner.className = `spinner spinner-${size}`;
      spinner.setAttribute('data-loading', 'true');

      if (typeof element === 'string') {
        element = document.querySelector(element);
      }

      if (element) {
        element.appendChild(spinner);
      }
    },

    /**
     * Hide loading spinner
     */
    hide: function(element) {
      if (typeof element === 'string') {
        element = document.querySelector(element);
      }

      if (element) {
        const spinner = element.querySelector('[data-loading="true"]');
        if (spinner) {
          spinner.remove();
        }
      }
    },

    /**
     * Set button loading state
     */
    setButtonLoading: function(button, isLoading) {
      if (typeof button === 'string') {
        button = document.querySelector(button);
      }

      if (!button) return;

      if (isLoading) {
        button.disabled = true;
        button.setAttribute('data-original-text', button.textContent);
        button.innerHTML = '<span class="loading-dots"><span class="loading-dot"></span><span class="loading-dot"></span><span class="loading-dot"></span></span>';
      } else {
        button.disabled = false;
        const originalText = button.getAttribute('data-original-text');
        if (originalText) {
          button.textContent = originalText;
          button.removeAttribute('data-original-text');
        }
      }
    }
  };

  // ============================================
  // GLOBAL EVENT HANDLERS
  // ============================================

  document.addEventListener('DOMContentLoaded', function() {
    // Close dropdowns when clicking outside
    document.addEventListener('click', function(e) {
      if (!e.target.closest('.profile-avatar') && !e.target.closest('.notification-bell') && !e.target.closest('.dropdown')) {
        Dropdown.closeAll();
      }
    });

    // Close mobile menu when clicking links
    document.querySelectorAll('.navbar-menu-mobile .navbar-link').forEach(link => {
      link.addEventListener('click', () => MobileMenu.close());
    });

    // Handle modal close on overlay click
    document.querySelectorAll('.modal-overlay').forEach(overlay => {
      overlay.addEventListener('click', function(e) {
        if (e.target === overlay) {
          Modal.close(overlay.id);
        }
      });
    });

    // Handle ESC key to close modals
    document.addEventListener('keydown', function(e) {
      if (e.key === 'Escape') {
        // Close active modal
        const activeModal = document.querySelector('.modal-overlay.active');
        if (activeModal) {
          Modal.close(activeModal.id);
        }

        // Close dropdowns
        Dropdown.closeAll();

        // Close mobile menu
        MobileMenu.close();
      }
    });
  });

  // ============================================
  // MODERN ALERT & CONFIRM REPLACEMENTS
  // ============================================

  /**
   * Modern alert replacement using modals
   */
  window.showAlert = function(message, title = 'Alert') {
    return new Promise((resolve) => {
      Modal.create({
        title: title,
        body: `<p class="modal-body-text">${message}</p>`,
        footer: `
          <button class="btn btn-primary" data-action="dialog-alert-ok">
            OK
          </button>
        `,
        size: 'sm',
        onClose: () => resolve()
      });
      window._alertResolve = resolve;
    });
  };

  /**
   * Modern confirm replacement using modals
   */
  window.showConfirm = function(message, title = 'Confirm') {
    return new Promise((resolve) => {
      Modal.create({
        title: title,
        body: `<p class="modal-body-text">${message}</p>`,
        footer: `
          <button class="btn btn-secondary" data-action="dialog-confirm-cancel">
            Cancel
          </button>
          <button class="btn btn-primary" data-action="dialog-confirm-ok">
            OK
          </button>
        `,
        size: 'sm',
        onClose: () => resolve(false)
      });
      window._confirmResolve = resolve;
    });
  };

  /**
   * Modern prompt replacement using modals
   */
  window.showPrompt = function(message, defaultValue = '', title = 'Input') {
    return new Promise((resolve) => {
      const inputId = Utils.generateId();
      const modalId = Modal.create({
        title: title,
        body: `
          <p class="modal-body-text-spacing">${message}</p>
          <input type="text" id="${inputId}" class="modal-input-full" value="${defaultValue}"
                 placeholder="Enter value...">
        `,
        footer: `
          <button class="btn btn-secondary" data-action="dialog-prompt-cancel">
            Cancel
          </button>
          <button class="btn btn-primary" data-action="dialog-prompt-ok" data-input-id="${inputId}">
            OK
          </button>
        `,
        size: 'sm',
        onClose: () => resolve(null)
      });

      window._promptResolve = resolve;

      // Focus input and allow Enter to submit
      setTimeout(() => {
        const input = document.getElementById(inputId);
        if (input) {
          input.focus();
          input.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
              const value = input.value;
              Modal.close(modalId);
              resolve(value);
            }
          });
        }
      }, 100);
    });
  };

  // ============================================
  // EXPOSE TO GLOBAL SCOPE
  // ============================================

  window.Modal = Modal;
  window.Toast = Toast;
  window.Alert = Alert;
  window.Dropdown = Dropdown;
  window.MobileMenu = MobileMenu;
  window.Notifications = Notifications;
  window.Forms = Forms;
  window.Loading = Loading;
  window.Utils = Utils;

  // Auto-start notification polling if user is authenticated
  // Only poll if notification bell exists (user is logged in)
  if (document.querySelector('.notification-bell')) {
    Notifications.startPolling();
  }

  // ============================================
  // ADMIN PANEL (AI.md PART 18)
  // ============================================

  const AdminPanel = {
    /**
     * Initialize admin panel
     */
    init: function() {
      this.initializeKeyboardShortcuts();
    },

    /**
     * Keyboard shortcuts per AI.md PART 18
     */
    initializeKeyboardShortcuts: function() {
      document.addEventListener('keydown', function(e) {
        // Ctrl/Cmd + K: Focus the admin search field, which submits to the
        // server-side global search route
        if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
          e.preventDefault();
          document.getElementById('admin-search-input')?.focus();
        }

        // Ctrl/Cmd + B: Toggle sidebar
        if ((e.ctrlKey || e.metaKey) && e.key === 'b') {
          e.preventDefault();
          AdminPanel.toggleSidebar();
        }
      });
    },

    /**
     * Toggle the sidebar drawer.
     * The drawer is a pure-CSS checkbox in the admin chrome, so it already
     * works without JavaScript; this only adds the keyboard shortcut.
     */
    toggleSidebar: function() {
      const drawer = document.getElementById('admin-drawer');
      if (drawer) {
        drawer.checked = !drawer.checked;
      }
    }
  };

  // ============================================
  // ADMIN AUTH SETTINGS (AI.md PART 16, 11)
  // ============================================

  const AdminAuthSettings = {
    /**
     * Append a blank OIDC provider row to #oidcProviders.
     * Bound via data-action delegation (CSP blocks inline onclick).
     */
    addOIDCProvider: function() {
      const container = document.getElementById('oidcProviders');
      if (!container) return;

      const index = container.children.length;
      const labels = container.dataset;
      const row = document.createElement('div');
      row.className = 'oidc-provider-row';
      row.dataset.index = String(index);
      row.innerHTML = `
        <div class="form-group">
          <label class="form-label">${labels.labelName || 'Name'}</label>
          <input class="form-input" type="text" name="oidc_provider_name">
        </div>
        <div class="form-group">
          <label class="form-label">${labels.labelClientId || 'Client ID'}</label>
          <input class="form-input" type="text" name="oidc_provider_client_id">
        </div>
        <div class="form-group">
          <label class="form-label">${labels.labelClientSecret || 'Client Secret'}</label>
          <input class="form-input" type="password" name="oidc_provider_client_secret">
        </div>
        <div class="form-group">
          <label class="form-label">${labels.labelIssuerUrl || 'Issuer URL'}</label>
          <input class="form-input" type="text" name="oidc_provider_issuer_url">
        </div>
        <div class="form-group">
          <label class="form-label">${labels.labelRedirectUrl || 'Redirect URL'}</label>
          <input class="form-input" type="text" name="oidc_provider_redirect_url">
        </div>
        <button type="button" class="btn btn-sm btn-danger" data-action="remove-oidc-provider">${labels.labelRemove || 'Remove'}</button>
      `;
      container.appendChild(row);
    },

    /**
     * Remove the OIDC provider row containing the clicked button.
     */
    removeOIDCProvider: function(button) {
      const row = button.closest('.oidc-provider-row');
      if (row) row.remove();
    },

    /**
     * Read every .oidc-provider-row into the OIDCProvider JSON shape
     * expected by UpdateAuthSettings.
     */
    collectOIDCProviders: function(form) {
      const rows = form.querySelectorAll('#oidcProviders .oidc-provider-row');
      return Array.from(rows).map(function(row) {
        return {
          name: row.querySelector('[name="oidc_provider_name"]').value,
          client_id: row.querySelector('[name="oidc_provider_client_id"]').value,
          client_secret: row.querySelector('[name="oidc_provider_client_secret"]').value,
          issuer_url: row.querySelector('[name="oidc_provider_issuer_url"]').value,
          redirect_url: row.querySelector('[name="oidc_provider_redirect_url"]').value
        };
      });
    },

    /**
     * Serialize #authSettingsForm into the request shape UpdateAuthSettings expects.
     */
    serialize: function(form) {
      const data = new FormData(form);
      return {
        oidc_enabled: form.querySelector('[name="oidc_enabled"]').checked,
        oidc_providers: AdminAuthSettings.collectOIDCProviders(form),
        ldap_enabled: form.querySelector('[name="ldap_enabled"]').checked,
        ldap_server: data.get('ldap_server') || '',
        ldap_port: parseInt(data.get('ldap_port'), 10) || 0,
        ldap_bind_dn: data.get('ldap_bind_dn') || '',
        ldap_bind_password: data.get('ldap_bind_password') || '',
        ldap_base_dn: data.get('ldap_base_dn') || '',
        ldap_user_filter: data.get('ldap_user_filter') || '',
        totp_enabled: form.querySelector('[name="totp_enabled"]').checked,
        totp_issuer: data.get('totp_issuer') || '',
        totp_digits: parseInt(data.get('totp_digits'), 10) || 0,
        totp_period: parseInt(data.get('totp_period'), 10) || 0,
        passkeys_enabled: form.querySelector('[name="passkeys_enabled"]').checked,
        passkeys_rp_id: data.get('passkeys_rp_id') || '',
        passkeys_rp_name: data.get('passkeys_rp_name') || ''
      };
    },

    /**
     * Serialize and POST #authSettingsForm to its action URL, showing
     * saving/saved/error state via the button and Toast (i18n-driven
     * data-label- and data-msg- attributes, no hardcoded strings).
     */
    submitForm: function(form) {
      const submitBtn = form.querySelector('button[type="submit"]');
      const csrfInput = form.querySelector('[name="csrf_token"]');
      const savingLabel = submitBtn ? submitBtn.dataset.labelSaving : '';
      const savedLabel = submitBtn ? submitBtn.dataset.labelSave : '';

      if (submitBtn) {
        submitBtn.disabled = true;
        if (savingLabel) submitBtn.textContent = savingLabel;
      }

      fetch(form.action, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': csrfInput ? csrfInput.value : ''
        },
        body: JSON.stringify(AdminAuthSettings.serialize(form))
      })
        .then(function(response) {
          return response.json().then(function(payload) {
            return { ok: response.ok && payload.ok, payload: payload };
          });
        })
        .then(function(result) {
          if (result.ok) {
            Toast.show(form.dataset.msgSaved, 'success');
          } else {
            Toast.show((result.payload && result.payload.message) || form.dataset.msgError, 'error');
          }
        })
        .catch(function() {
          Toast.show(form.dataset.msgError, 'error');
        })
        .finally(function() {
          if (submitBtn) {
            submitBtn.disabled = false;
            if (savedLabel) submitBtn.textContent = savedLabel;
          }
        });
    }
  };

  document.addEventListener('DOMContentLoaded', function() {
    const authSettingsForm = document.getElementById('authSettingsForm');
    if (authSettingsForm) {
      authSettingsForm.addEventListener('submit', function(e) {
        e.preventDefault();
        AdminAuthSettings.submitForm(authSettingsForm);
      });
    }

    document.addEventListener('click', function(e) {
      const btn = e.target.closest('[data-action]');
      if (!btn) return;

      switch (btn.dataset.action) {
        case 'add-oidc-provider':
          AdminAuthSettings.addOIDCProvider();
          break;
        case 'remove-oidc-provider':
          AdminAuthSettings.removeOIDCProvider(btn);
          break;
        case 'modal-close':
        case 'modal-cancel': {
          const overlay = btn.closest('.modal-overlay');
          if (overlay) Modal.close(overlay.id);
          break;
        }
        case 'toast-dismiss': {
          const toast = btn.closest('.toast');
          if (toast) Toast.dismiss(toast.id);
          break;
        }
        case 'alert-dismiss': {
          const alertEl = btn.closest('.alert');
          if (alertEl) Alert.dismiss(alertEl.id);
          break;
        }
        case 'dialog-alert-ok': {
          const overlay = btn.closest('.modal-overlay');
          if (overlay) Modal.close(overlay.id);
          if (window._alertResolve) window._alertResolve();
          break;
        }
        case 'dialog-confirm-cancel': {
          const overlay = btn.closest('.modal-overlay');
          if (overlay) Modal.close(overlay.id);
          if (window._confirmResolve) window._confirmResolve(false);
          break;
        }
        case 'dialog-confirm-ok': {
          const overlay = btn.closest('.modal-overlay');
          if (overlay) Modal.close(overlay.id);
          if (window._confirmResolve) window._confirmResolve(true);
          break;
        }
        case 'dialog-prompt-cancel': {
          const overlay = btn.closest('.modal-overlay');
          if (overlay) Modal.close(overlay.id);
          if (window._promptResolve) window._promptResolve(null);
          break;
        }
        case 'dialog-prompt-ok': {
          const overlay = btn.closest('.modal-overlay');
          const input = document.getElementById(btn.dataset.inputId);
          const value = input ? input.value : null;
          if (overlay) Modal.close(overlay.id);
          if (window._promptResolve) window._promptResolve(value);
          break;
        }
      }
    });
  });

  window.AdminAuthSettings = AdminAuthSettings;

  /**
   * Confirmation dialog (replaces JS alerts per AI.md PART 18)
   */
  window.confirmAction = function(message, callback) {
    Modal.create({
      title: 'Confirm Action',
      body: `<p class="modal-body-text">${message}</p>`,
      footer: `
        <button class="btn btn-secondary" data-action="modal-cancel">Cancel</button>
        <button class="btn btn-danger" id="confirmActionBtn">Confirm</button>
      `,
      size: 'sm',
      onClose: function() {}
    });

    // Set up confirm button handler after modal is created
    setTimeout(function() {
      const confirmBtn = document.getElementById('confirmActionBtn');
      if (confirmBtn) {
        confirmBtn.addEventListener('click', function() {
          const overlay = this.closest('.modal-overlay');
          if (overlay) Modal.close(overlay.id);
          if (callback) callback();
        });
      }
    }, 50);
  };

  // ============================================
  // ADMIN SIDEBAR STATE (AI.md PART 17)
  // "Remember state | Persist expanded/collapsed state"
  // ============================================

  /**
   * Persists which admin sidebar sections are expanded.
   * The markup is <details open> so navigation still works with JavaScript
   * disabled; this only restores the admin's last choice on top of it.
   * A UI preference is the only thing stored - never a session token.
   */
  const AdminSidebarState = {
    storageKey: 'admin.sidebar.sections',

    /**
     * Read the saved section map, tolerating unavailable or corrupt storage.
     */
    read: function() {
      try {
        const raw = window.localStorage.getItem(this.storageKey);
        if (!raw) return {};
        const parsed = JSON.parse(raw);
        return (parsed && typeof parsed === 'object') ? parsed : {};
      } catch (e) {
        return {};
      }
    },

    /**
     * Persist the section map, ignoring quota or private-mode failures.
     */
    write: function(state) {
      try {
        window.localStorage.setItem(this.storageKey, JSON.stringify(state));
      } catch (e) {
        return;
      }
    },

    /**
     * Apply the saved state to the sidebar, then keep it in sync on toggle.
     */
    init: function() {
      const sections = document.querySelectorAll('.admin-sidebar [data-nav-section]');
      if (!sections.length) return;

      const state = this.read();
      sections.forEach(section => {
        const key = section.dataset.navSection;
        if (Object.prototype.hasOwnProperty.call(state, key)) {
          section.open = state[key] === true;
        }
      });

      // The toggle event does not bubble, so listen in the capture phase.
      document.addEventListener('toggle', function(e) {
        const section = e.target;
        if (!section.dataset || !section.dataset.navSection) return;
        if (!section.closest('.admin-sidebar')) return;

        const current = AdminSidebarState.read();
        current[section.dataset.navSection] = section.open;
        AdminSidebarState.write(current);
      }, true);
    }
  };

  // Expose AdminPanel
  window.AdminPanel = AdminPanel;

  // Initialize admin panel if on admin page
  if (document.querySelector('.admin-sidebar')) {
    AdminPanel.init();
  }

  // Sidebar state restores on every admin page that renders the shared chrome
  AdminSidebarState.init();

  // ============================================
  // THEME SYSTEM (AI.md PART 16)
  // Dark theme is DEFAULT per AI.md spec
  // ============================================

  const Theme = {
    // Available themes: dark (default), light, auto
    THEMES: ['dark', 'light', 'auto'],
    COOKIE_NAME: 'theme',

    /**
     * Read a cookie value by name (AI.md PART 16: theme preference is
     * persisted server-readable in the theme cookie, never localStorage)
     */
    getCookie: function(name) {
      var match = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'));
      return match ? decodeURIComponent(match[1]) : null;
    },

    /**
     * Write the theme cookie - mirrors what SetThemeHandler (POST /theme)
     * would set server-side, so the class swaps instantly without a
     * reload while staying server-readable on the next navigation
     */
    setCookie: function(theme) {
      var secure = window.location.protocol === 'https:' ? '; Secure' : '';
      document.cookie = this.COOKIE_NAME + '=' + encodeURIComponent(theme) +
        '; path=/; max-age=31536000; SameSite=Strict' + secure;
    },

    /**
     * Get current theme from the server-readable theme cookie. The
     * server already rendered the correct theme-* class on <html> from
     * this same cookie (or the DB preference) with zero JS - this getter
     * exists only so client-side JS enhancements (toggle/cycle) know the
     * current selection.
     */
    get: function() {
      var cookieTheme = this.getCookie(this.COOKIE_NAME);
      return this.THEMES.includes(cookieTheme) ? cookieTheme : 'dark';
    },

    /**
     * Set theme and persist to the theme cookie (server-readable)
     */
    set: function(theme) {
      if (!this.THEMES.includes(theme)) {
        theme = 'dark';
      }
      this.setCookie(theme);
      this.apply(theme);
      Utils.dispatchEvent('theme:changed', { theme });
    },

    /**
     * Apply theme to document
     */
    apply: function(theme) {
      var effectiveTheme = theme;
      if (theme === 'auto') {
        effectiveTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
      }
      document.documentElement.className = 'theme-' + effectiveTheme;
      // Update theme-color meta tag
      var themeColor = effectiveTheme === 'dark' ? '#282a36' : '#ffffff';
      var metaThemeColor = document.querySelector('meta[name="theme-color"]');
      if (metaThemeColor) {
        metaThemeColor.setAttribute('content', themeColor);
      }
      // Update active button in profile dropdown
      this.updateActiveButton(theme);
    },

    /**
     * Update active theme button in profile dropdown
     */
    updateActiveButton: function(theme) {
      // Remove active class from all theme buttons
      document.querySelectorAll('.theme-btn').forEach(function(btn) {
        btn.classList.remove('active');
      });
      // Add active class to current theme button
      var activeBtn = document.querySelector('.theme-btn[data-theme="' + theme + '"]');
      if (activeBtn) {
        activeBtn.classList.add('active');
      }
    },

    /**
     * Toggle between dark and light themes
     */
    toggle: function() {
      var current = this.get();
      var next = current === 'dark' ? 'light' : 'dark';
      this.set(next);
      return next;
    },

    /**
     * Cycle through all themes: dark -> light -> auto -> dark
     */
    cycle: function() {
      var current = this.get();
      var index = this.THEMES.indexOf(current);
      var next = this.THEMES[(index + 1) % this.THEMES.length];
      this.set(next);
      return next;
    },

    /**
     * Initialize theme system
     */
    init: function() {
      // Apply saved theme
      var currentTheme = this.get();
      this.apply(currentTheme);

      // Listen for system preference changes when using auto theme
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function(e) {
        if (Theme.get() === 'auto') {
          Theme.apply('auto');
        }
      });
    }
  };

  // Expose Theme globally
  window.Theme = Theme;

  // Initialize theme system
  Theme.init();

})();
