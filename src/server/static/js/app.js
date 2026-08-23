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
    },

    /**
     * Escape a value for safe use inside an HTML attribute value, e.g.
     * `data-filename="' + Utils.escapeAttr(name) + '"`. Text-node escaping
     * (textContent -> innerHTML) does not encode quote characters, so it is
     * NOT safe for attribute context on its own - use this instead whenever
     * an untrusted string is concatenated into an attribute value.
     */
    escapeAttr: function(value) {
      const str = value === null || value === undefined ? '' : String(value);
      return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
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

  const AdminSettingsPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminSettingsPage.dataPayload) {
        const el = document.getElementById('admin-settings-data');
        AdminSettingsPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '', adminApiPath: '' };
      }
      return AdminSettingsPage.dataPayload;
    },

    switchTab: function(btn, tabName) {
      document.querySelectorAll('.tab-content').forEach(function(tab) {
        tab.classList.remove('active');
      });
      document.querySelectorAll('.tab').forEach(function(tabBtn) {
        tabBtn.classList.remove('active');
      });
      const target = document.getElementById('tab-' + tabName);
      if (target) target.classList.add('active');
      btn.classList.add('active');
    },

    saveSettings: function(form, category) {
      const formData = new FormData(form);
      const settings = {};
      for (const [key, value] of formData.entries()) {
        if (form.elements[key].type === 'checkbox') {
          settings[key] = form.elements[key].checked;
        } else {
          settings[key] = value;
        }
      }
      const adminApiPath = AdminSettingsPage.getData().adminApiPath;
      const updates = Object.entries(settings).map(function(entry) {
        const key = entry[0];
        const value = entry[1];
        return fetch(adminApiPath + '/server/settings/' + key, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ value: value })
        }).then(function(response) {
          if (!response.ok) throw new Error('Failed to update ' + key);
        });
      });
      Promise.all(updates)
        .then(function() {
          Toast.success(category + ' settings saved successfully');
          setTimeout(function() {
            window.location.reload();
          }, 1500);
        })
        .catch(function(error) {
          Toast.error('Failed to save settings: ' + error.message);
        });
    },

    init: function() {
      document.querySelectorAll('form[data-settings-category]').forEach(function(form) {
        form.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminSettingsPage.saveSettings(form, form.dataset.settingsCategory);
        });
      });
      if (document.getElementById('admin-settings-data')) {
        Notifications.fetch();
        Notifications.startPolling(60000);
      }
    }
  };

  const AdminDatabasePage = {
    dataPayload: null,

    getData: function() {
      if (!AdminDatabasePage.dataPayload) {
        const el = document.getElementById('admin-database-data');
        AdminDatabasePage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '', adminApiPath: '' };
      }
      return AdminDatabasePage.dataPayload;
    },

    formatBytes: function(bytes) {
      if (bytes === 0) return '0 Bytes';
      const k = 1024;
      const sizes = ['Bytes', 'KB', 'MB', 'GB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
    },

    showOperationResult: function(message, type) {
      const result = document.getElementById('operation-result');
      result.className = 'alert alert-result alert-' + (type === 'success' ? 'success' : 'danger');
      result.textContent = message;
      result.classList.remove('hidden');
      setTimeout(function() {
        result.classList.add('hidden');
      }, 5000);
    },

    showCacheResult: function(message, type) {
      const result = document.getElementById('cache-result');
      result.className = 'alert alert-result alert-' + (type === 'success' ? 'success' : 'danger');
      result.textContent = message;
      result.classList.remove('hidden');
      setTimeout(function() {
        result.classList.add('hidden');
      }, 5000);
    },

    showNotification: function(message, type) {
      const alertClass = type === 'success' ? 'alert-success' : 'alert-danger';
      const notification = document.createElement('div');
      notification.className = 'alert ' + alertClass + ' toast-notification';
      notification.textContent = message;
      document.body.appendChild(notification);
      setTimeout(function() {
        notification.remove();
      }, 4000);
    },

    showDatabaseConfigResult: function(message, type) {
      const result = document.getElementById('db-config-result');
      result.className = 'alert alert-result alert-' + (type === 'success' ? 'success' : type === 'info' ? 'info' : 'danger');
      result.textContent = message;
      result.classList.remove('hidden');
      setTimeout(function() {
        result.classList.add('hidden');
      }, 5000);
    },

    loadDatabaseStats: function() {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      fetch(adminApiPath + '/server/stats')
        .then(function(response) {
          return response.json();
        })
        .then(function(data) {
          if (data.database) {
            document.getElementById('db-status').innerHTML = '<span class="badge badge-success">✅ Connected</span>';
            document.getElementById('db-type').textContent = data.database.type || 'SQLite';
            document.getElementById('db-size').textContent = AdminDatabasePage.formatBytes(data.database.size || 0);
            document.getElementById('db-tables').textContent = data.database.tables || '0';
          } else {
            document.getElementById('db-status').innerHTML = '<span class="badge badge-danger">❌ Error</span>';
          }
        })
        .catch(function(error) {
          console.error('Failed to load database stats:', error);
          document.getElementById('db-status').innerHTML = '<span class="badge badge-danger">❌ Error</span>';
        });
    },

    loadCacheStats: function() {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      fetch(adminApiPath + '/server/stats')
        .then(function(response) {
          return response.json();
        })
        .then(function(data) {
          if (data.cache && data.cache.enabled) {
            document.getElementById('cache-status').innerHTML = '<span class="badge badge-success">✅ Enabled</span>';
            document.getElementById('cache-type').textContent = data.cache.type || 'Redis';
            document.getElementById('cache-hitrate').textContent = (data.cache.hit_rate || 0) + '%';
          } else {
            document.getElementById('cache-status').innerHTML = '<span class="badge badge-secondary">⚪ Disabled</span>';
            document.getElementById('cache-type').textContent = 'None';
            document.getElementById('cache-hitrate').textContent = 'N/A';
          }
        })
        .catch(function(error) {
          console.error('Failed to load cache stats:', error);
        });
    },

    loadSettings: function() {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      fetch(adminApiPath + '/server/settings/all')
        .then(function(response) {
          return response.json();
        })
        .then(function(data) {
          if (data.settings) {
            Object.entries(data.settings).forEach(function(entry) {
              const key = entry[0];
              const setting = entry[1];
              const input = document.querySelector('[name="' + key + '"]');
              if (input) {
                if (input.type === 'checkbox') {
                  input.checked = setting.value === 'true';
                } else {
                  input.value = setting.value || '';
                }
              }
            });
          }
        })
        .catch(function(error) {
          console.error('Failed to load settings:', error);
        });
    },

    optimizeDatabase: function() {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      const btn = document.getElementById('optimize-btn');
      btn.disabled = true;
      btn.textContent = '⏳ Optimizing...';
      fetch(adminApiPath + '/server/database/optimize', { method: 'POST' })
        .then(function(response) {
          return response.json().then(function(data) {
            return { ok: response.ok, data: data };
          });
        })
        .then(function(result) {
          if (result.ok) {
            AdminDatabasePage.showOperationResult('✅ Database optimized successfully! Performance improved.', 'success');
            AdminDatabasePage.refreshStats();
          } else {
            throw new Error((result.data.error && result.data.error.message) || 'Optimization failed');
          }
        })
        .catch(function(error) {
          AdminDatabasePage.showOperationResult('❌ ' + error.message, 'error');
        })
        .finally(function() {
          btn.disabled = false;
          btn.textContent = '⚡ Optimize Now';
        });
    },

    vacuumDatabase: function() {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      const btn = document.getElementById('vacuum-btn');
      btn.disabled = true;
      btn.textContent = '⏳ Vacuuming...';
      fetch(adminApiPath + '/server/database/vacuum', { method: 'POST' })
        .then(function(response) {
          return response.json().then(function(data) {
            return { ok: response.ok, data: data };
          });
        })
        .then(function(result) {
          if (result.ok) {
            AdminDatabasePage.showOperationResult('✅ Database vacuumed successfully! Space reclaimed.', 'success');
            AdminDatabasePage.refreshStats();
          } else {
            throw new Error((result.data.error && result.data.error.message) || 'Vacuum failed');
          }
        })
        .catch(function(error) {
          AdminDatabasePage.showOperationResult('❌ ' + error.message, 'error');
        })
        .finally(function() {
          btn.disabled = false;
          btn.textContent = '🧹 Vacuum Now';
        });
    },

    testConnection: function() {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      const btn = document.getElementById('test-btn');
      btn.disabled = true;
      btn.textContent = '⏳ Testing...';
      fetch(adminApiPath + '/server/database/test', { method: 'POST' })
        .then(function(response) {
          return response.json().then(function(data) {
            return { ok: response.ok, data: data };
          });
        })
        .then(function(result) {
          if (result.ok) {
            AdminDatabasePage.showOperationResult('✅ Connection test passed! Response time: ' + (result.data.response_time || 'N/A'), 'success');
          } else {
            throw new Error((result.data.error && result.data.error.message) || 'Connection test failed');
          }
        })
        .catch(function(error) {
          AdminDatabasePage.showOperationResult('❌ ' + error.message, 'error');
        })
        .finally(function() {
          btn.disabled = false;
          btn.textContent = '🔍 Test Connection';
        });
    },

    clearCache: function() {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      showConfirm('Clear all cached data? This will temporarily slow down the application until cache rebuilds.', 'Clear Cache')
        .then(function(confirmed) {
          if (!confirmed) return;
          const btn = document.getElementById('clear-cache-btn');
          btn.disabled = true;
          btn.textContent = '⏳ Clearing...';
          fetch(adminApiPath + '/server/cache/clear', { method: 'POST' })
            .then(function(response) {
              return response.json().then(function(data) {
                return { ok: response.ok, data: data };
              });
            })
            .then(function(result) {
              if (result.ok) {
                AdminDatabasePage.showCacheResult('✅ Cache cleared successfully!', 'success');
                AdminDatabasePage.refreshCacheStats();
              } else {
                throw new Error((result.data.error && result.data.error.message) || 'Failed to clear cache');
              }
            })
            .catch(function(error) {
              AdminDatabasePage.showCacheResult('❌ ' + error.message, 'error');
            })
            .finally(function() {
              btn.disabled = false;
              btn.textContent = '🗑️ Clear All Cache';
            });
        });
    },

    refreshStats: function() {
      AdminDatabasePage.loadDatabaseStats();
    },

    refreshCacheStats: function() {
      AdminDatabasePage.loadCacheStats();
    },

    submitSettingsForm: function(form) {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      const formData = new FormData(form);
      const settings = {};
      for (const [key, value] of formData.entries()) {
        settings[key] = value;
      }
      settings['database.auto_optimize'] = document.getElementById('db_auto_optimize').checked ? 'true' : 'false';
      fetch(adminApiPath + '/server/settings/bulk', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ settings: settings })
      })
        .then(function(response) {
          return response.json().then(function(data) {
            return { ok: response.ok, data: data };
          });
        })
        .then(function(result) {
          if (result.ok) {
            AdminDatabasePage.showNotification('Database settings saved successfully', 'success');
          } else {
            throw new Error((result.data.error && result.data.error.message) || 'Failed to save settings');
          }
        })
        .catch(function(error) {
          AdminDatabasePage.showNotification(error.message, 'error');
        });
    },

    loadDatabaseConfig: function() {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      fetch(adminApiPath + '/server/settings/all')
        .then(function(response) {
          return response.json();
        })
        .then(function(data) {
          if (data.settings) {
            const driver = data.settings['database.driver'] || 'file';
            document.getElementById('db_driver').value = driver;
            document.getElementById('db_host').value = data.settings['database.host'] || '';
            document.getElementById('db_port').value = data.settings['database.port'] || '';
            document.getElementById('db_name').value = data.settings['database.name'] || '';
            document.getElementById('db_username').value = data.settings['database.username'] || '';
            document.getElementById('db_password').value = data.settings['database.password'] || '';
            document.getElementById('db_sslmode').value = data.settings['database.sslmode'] || 'disable';
            AdminDatabasePage.updateDatabaseFieldVisibility(driver);
          }
        })
        .catch(function(error) {
          console.error('Failed to load database configuration:', error);
        });
    },

    setupDatabaseDriverListener: function() {
      const driverSelect = document.getElementById('db_driver');
      driverSelect.addEventListener('change', function() {
        AdminDatabasePage.updateDatabaseFieldVisibility(this.value);
      });
    },

    updateDatabaseFieldVisibility: function(driver) {
      const remoteFields = document.getElementById('remote-db-fields');
      const postgresFields = document.getElementById('postgres-fields');
      if (driver === 'file' || driver === 'sqlite') {
        remoteFields.classList.add('hidden');
        postgresFields.classList.add('hidden');
      } else {
        remoteFields.classList.remove('hidden');
        if (driver === 'postgres' || driver === 'postgresql') {
          postgresFields.classList.remove('hidden');
        } else {
          postgresFields.classList.add('hidden');
        }
      }
    },

    togglePasswordVisibility: function() {
      const passwordInput = document.getElementById('db_password');
      const toggleButton = document.getElementById('toggle-password');
      if (passwordInput.type === 'password') {
        passwordInput.type = 'text';
        toggleButton.textContent = '👁️‍🗨️';
      } else {
        passwordInput.type = 'password';
        toggleButton.textContent = '👁️';
      }
    },

    testDatabaseConnectionConfig: function() {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      const btn = document.getElementById('test-db-connection-btn');
      btn.disabled = true;
      btn.textContent = '⏳ Testing...';
      const driver = document.getElementById('db_driver').value;
      const config = { driver: driver };
      if (driver !== 'file' && driver !== 'sqlite') {
        config.host = document.getElementById('db_host').value;
        config.port = parseInt(document.getElementById('db_port').value, 10);
        config.name = document.getElementById('db_name').value;
        config.username = document.getElementById('db_username').value;
        config.password = document.getElementById('db_password').value;
        if (driver === 'postgres' || driver === 'postgresql') {
          config.sslmode = document.getElementById('db_sslmode').value;
        }
      }
      fetch(adminApiPath + '/server/database/test-config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config)
      })
        .then(function(response) {
          return response.json().then(function(data) {
            return { ok: response.ok, data: data };
          });
        })
        .then(function(result) {
          if (result.ok && result.data.success) {
            AdminDatabasePage.showDatabaseConfigResult('✅ Connection test passed! Database is accessible.', 'success');
          } else {
            throw new Error(result.data.error || 'Connection test failed');
          }
        })
        .catch(function(error) {
          AdminDatabasePage.showDatabaseConfigResult('❌ ' + error.message, 'error');
        })
        .finally(function() {
          btn.disabled = false;
          btn.textContent = '🔍 Test Connection';
        });
    },

    cancelDatabaseConfig: function() {
      AdminDatabasePage.loadDatabaseConfig();
      AdminDatabasePage.showDatabaseConfigResult('Configuration changes cancelled', 'info');
    },

    submitConfigForm: function(form) {
      const adminApiPath = AdminDatabasePage.getData().adminApiPath;
      const driver = document.getElementById('db_driver').value;
      const settings = { 'database.driver': driver };
      if (driver !== 'file' && driver !== 'sqlite') {
        settings['database.host'] = document.getElementById('db_host').value;
        settings['database.port'] = document.getElementById('db_port').value;
        settings['database.name'] = document.getElementById('db_name').value;
        settings['database.username'] = document.getElementById('db_username').value;
        settings['database.password'] = document.getElementById('db_password').value;
        if (driver === 'postgres' || driver === 'postgresql') {
          settings['database.sslmode'] = document.getElementById('db_sslmode').value;
        }
        if (!settings['database.host']) {
          AdminDatabasePage.showDatabaseConfigResult('❌ Host is required for remote databases', 'error');
          return;
        }
        if (!settings['database.port']) {
          AdminDatabasePage.showDatabaseConfigResult('❌ Port is required for remote databases', 'error');
          return;
        }
        if (!settings['database.name']) {
          AdminDatabasePage.showDatabaseConfigResult('❌ Database name is required for remote databases', 'error');
          return;
        }
      }
      fetch(adminApiPath + '/server/settings/database', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings)
      })
        .then(function(response) {
          return response.json().then(function(data) {
            return { ok: response.ok, data: data };
          });
        })
        .then(function(result) {
          if (result.ok) {
            AdminDatabasePage.showDatabaseConfigResult('✅ Database configuration saved successfully! Please restart the server for changes to take effect.', 'success');
          } else {
            throw new Error(result.data.error || 'Failed to save database configuration');
          }
        })
        .catch(function(error) {
          AdminDatabasePage.showDatabaseConfigResult('❌ ' + error.message, 'error');
        });
    },

    init: function() {
      if (!document.getElementById('admin-database-data')) return;
      AdminDatabasePage.loadDatabaseStats();
      AdminDatabasePage.loadCacheStats();
      AdminDatabasePage.loadSettings();
      AdminDatabasePage.loadDatabaseConfig();
      AdminDatabasePage.setupDatabaseDriverListener();

      const settingsForm = document.getElementById('db-settings-form');
      if (settingsForm) {
        settingsForm.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminDatabasePage.submitSettingsForm(settingsForm);
        });
      }

      const configForm = document.getElementById('db-config-form');
      if (configForm) {
        configForm.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminDatabasePage.submitConfigForm(configForm);
        });
      }
    }
  };

  const AdminBackupPage = {
    dataPayload: null,
    refreshTimer: null,

    getData: function() {
      if (!AdminBackupPage.dataPayload) {
        const el = document.getElementById('admin-backup-data');
        AdminBackupPage.dataPayload = el ? JSON.parse(el.textContent) : { adminApiPath: '', text: {} };
      }
      return AdminBackupPage.dataPayload;
    },

    backupApi: function() {
      return AdminBackupPage.getData().adminApiPath + '/config/backup';
    },

    request: function(url, options) {
      return fetch(url, options).then(function(response) {
        return response.json().catch(function() {
          return null;
        }).then(function(payload) {
          if (!response.ok) {
            throw new Error(payload && payload.message ? payload.message : String(response.status));
          }
          return payload;
        });
      });
    },

    formatBytes: function(bytes) {
      const value = Number(bytes) || 0;
      const units = ['B', 'KB', 'MB', 'GB', 'TB'];
      let index = 0;
      let size = value;
      while (size >= 1024 && index < units.length - 1) {
        size /= 1024;
        index += 1;
      }
      return (index === 0 ? size : size.toFixed(1)) + ' ' + units[index];
    },

    formatMoment: function(value) {
      const text = AdminBackupPage.getData().text;
      if (!value) {
        return text.never;
      }
      const parsed = new Date(value);
      return isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
    },

    escapeText: function(value) {
      const holder = document.createElement('span');
      holder.textContent = value === null || value === undefined ? '' : String(value);
      return holder.innerHTML;
    },

    loadStats: function() {
      const text = AdminBackupPage.getData().text;
      AdminBackupPage.request(AdminBackupPage.backupApi() + '/stats')
        .then(function(stats) {
          document.getElementById('totalBackups').textContent = stats.count || 0;
          document.getElementById('totalSize').textContent = AdminBackupPage.formatBytes(stats.total_size);
          document.getElementById('lastBackup').textContent = AdminBackupPage.formatMoment(stats.last_backup);
          document.getElementById('nextBackup').textContent = stats.next_backup ? AdminBackupPage.formatMoment(stats.next_backup) : '-';
        })
        .catch(function(error) {
          AdminBackupPage.showError(text.loadFailed + ' ' + error.message);
        });
    },

    loadBackups: function() {
      const text = AdminBackupPage.getData().text;
      const container = document.getElementById('backupList');
      AdminBackupPage.request(AdminBackupPage.backupApi())
        .then(function(backups) {
          if (!Array.isArray(backups) || backups.length === 0) {
            container.innerHTML = '<p class="text-center text-muted p-3">' + AdminBackupPage.escapeText(text.empty) + '</p>';
            return;
          }
          container.innerHTML = backups.map(function(backup) {
            const encrypted = backup.filename.endsWith('.enc') ? text.encryptedYes : text.encryptedNo;
            const name = AdminBackupPage.escapeText(backup.filename);
            const nameAttr = Utils.escapeAttr(backup.filename);
            return '<div class="backup-item">' +
              '<div class="backup-info">' +
                '<h3>' + name + '</h3>' +
                '<div class="backup-meta">' + AdminBackupPage.escapeText(AdminBackupPage.formatBytes(backup.size)) + ' &bull; ' + AdminBackupPage.escapeText(AdminBackupPage.formatMoment(backup.created)) + ' &bull; ' + AdminBackupPage.escapeText(encrypted) + '</div>' +
              '</div>' +
              '<div class="btn-group">' +
                '<button type="button" class="btn btn-sm btn-primary" data-action="download-backup" data-filename="' + nameAttr + '">⬇️ ' + AdminBackupPage.escapeText(text.downloadAction) + '</button>' +
                '<button type="button" class="btn btn-sm btn-success" data-action="restore-backup" data-filename="' + nameAttr + '">🔄 ' + AdminBackupPage.escapeText(text.restoreAction) + '</button>' +
                '<button type="button" class="btn btn-sm btn-danger" data-action="delete-backup" data-filename="' + nameAttr + '">🗑️ ' + AdminBackupPage.escapeText(text.deleteAction) + '</button>' +
              '</div>' +
            '</div>';
          }).join('');
        })
        .catch(function(error) {
          AdminBackupPage.showError(text.loadFailed + ' ' + error.message);
        });
    },

    handleListClick: function(filename, action) {
      if (action === 'download-backup') {
        window.location.href = AdminBackupPage.backupApi() + '/' + encodeURIComponent(filename) + '/download';
      } else if (action === 'restore-backup') {
        AdminBackupPage.restoreBackup(filename);
      } else if (action === 'delete-backup') {
        AdminBackupPage.deleteBackup(filename);
      }
    },

    submitCreateForm: function(form) {
      const text = AdminBackupPage.getData().text;
      const button = document.getElementById('createBtn');
      button.disabled = true;
      button.textContent = text.creating;
      AdminBackupPage.request(AdminBackupPage.backupApi(), {
        method: 'POST',
        body: new FormData(form)
      })
        .then(function() {
          AdminBackupPage.showSuccess(text.created);
          document.getElementById('backupPassword').value = '';
          AdminBackupPage.loadBackups();
          AdminBackupPage.loadStats();
        })
        .catch(function(error) {
          AdminBackupPage.showError(text.createFailed + ' ' + error.message);
        })
        .finally(function() {
          button.disabled = false;
          button.textContent = text.createAction;
        });
    },

    restoreBackup: function(filename) {
      const text = AdminBackupPage.getData().text;
      showConfirm(text.restoreConfirm + ' ' + filename, text.restoreAction).then(function(confirmed) {
        if (!confirmed) return;
        const body = new FormData();
        body.append('filename', filename);
        body.append('csrf_token', document.querySelector('#createForm [name="csrf_token"]').value);
        if (filename.endsWith('.enc')) {
          body.append('password', document.getElementById('backupPassword').value);
        }
        AdminBackupPage.request(AdminBackupPage.backupApi() + '/restore', { method: 'POST', body: body })
          .then(function() {
            AdminBackupPage.showSuccess(text.restored);
          })
          .catch(function(error) {
            AdminBackupPage.showError(text.restoreFailed + ' ' + error.message);
          });
      });
    },

    deleteBackup: function(filename) {
      const text = AdminBackupPage.getData().text;
      showConfirm(text.deleteConfirm + ' ' + filename, text.deleteAction).then(function(confirmed) {
        if (!confirmed) return;
        AdminBackupPage.request(AdminBackupPage.backupApi() + '/' + encodeURIComponent(filename), { method: 'DELETE' })
          .then(function() {
            AdminBackupPage.showSuccess(text.deleted);
            AdminBackupPage.loadBackups();
            AdminBackupPage.loadStats();
          })
          .catch(function(error) {
            AdminBackupPage.showError(text.deleteFailed + ' ' + error.message);
          });
      });
    },

    loadSchedule: function() {
      const text = AdminBackupPage.getData().text;
      AdminBackupPage.request(AdminBackupPage.backupApi() + '/schedule')
        .then(function(schedule) {
          document.getElementById('autoBackup').checked = schedule.enabled !== false;
          document.getElementById('interval').value = schedule.interval;
          document.getElementById('retention').value = schedule.retention;
        })
        .catch(function(error) {
          AdminBackupPage.showError(text.loadFailed + ' ' + error.message);
        });
    },

    submitScheduleForm: function() {
      const text = AdminBackupPage.getData().text;
      const button = document.getElementById('scheduleBtn');
      button.disabled = true;
      const payload = {
        enabled: document.getElementById('autoBackup').checked,
        interval: parseInt(document.getElementById('interval').value, 10),
        retention: parseInt(document.getElementById('retention').value, 10)
      };
      AdminBackupPage.request(AdminBackupPage.backupApi() + '/schedule', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      })
        .then(function() {
          AdminBackupPage.showSuccess(text.scheduleSaved);
          AdminBackupPage.loadStats();
        })
        .catch(function(error) {
          AdminBackupPage.showError(text.scheduleFailed + ' ' + error.message);
        })
        .finally(function() {
          button.disabled = false;
          button.textContent = text.saveScheduleAction;
        });
    },

    showSuccess: function(msg) {
      const alert = document.getElementById('successAlert');
      alert.textContent = msg;
      alert.classList.add('show');
      setTimeout(function() {
        alert.classList.remove('show');
      }, 5000);
    },

    showError: function(msg) {
      const alert = document.getElementById('errorAlert');
      alert.textContent = msg;
      alert.classList.add('show');
      setTimeout(function() {
        alert.classList.remove('show');
      }, 5000);
    },

    init: function() {
      if (!document.getElementById('admin-backup-data')) return;

      AdminBackupPage.loadStats();
      AdminBackupPage.loadBackups();
      AdminBackupPage.loadSchedule();

      document.getElementById('backupList').addEventListener('click', function(event) {
        const button = event.target.closest('[data-action]');
        if (!button) return;
        AdminBackupPage.handleListClick(button.getAttribute('data-filename'), button.getAttribute('data-action'));
      });

      document.getElementById('createForm').addEventListener('submit', function(event) {
        event.preventDefault();
        AdminBackupPage.submitCreateForm(event.target);
      });

      document.getElementById('scheduleForm').addEventListener('submit', function(event) {
        event.preventDefault();
        AdminBackupPage.submitScheduleForm();
      });

      AdminBackupPage.refreshTimer = setInterval(function() {
        AdminBackupPage.loadStats();
        AdminBackupPage.loadBackups();
      }, 30000);
    }
  };

  const AdminEmailEditorPage = {
    dataPayload: null,
    currentTemplate: 'welcome',
    templates: {},

    sampleData: {
      app_name: 'Weather API Manager',
      app_url: 'https://weather.example.com',
      admin_email: 'admin@example.com',
      current_year: new Date().getFullYear(),
      reset_link: 'https://weather.example.com/reset?token=abc123',
      backup_file: 'weather_backup_2025-12-13.sql.gz',
      error_message: 'Database connection timeout',
      days_until_expiry: '7',
      ip_address: '192.168.1.100',
      timestamp: new Date().toLocaleString()
    },

    getData: function() {
      if (!AdminEmailEditorPage.dataPayload) {
        const el = document.getElementById('admin-email-editor-data');
        AdminEmailEditorPage.dataPayload = JSON.parse(el.textContent);
      }
      return AdminEmailEditorPage.dataPayload;
    },

    loadTemplate: function(templateName) {
      const apiPath = AdminEmailEditorPage.getData().adminApiPath;
      fetch(apiPath + '/email/templates/' + templateName)
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to load template');
          return response.json();
        })
        .then(function(data) {
          AdminEmailEditorPage.templates[templateName] = data;
          document.getElementById('templateSubject').value = data.subject || '';
          document.getElementById('templateBody').value = data.body || '';
          document.getElementById('editorTitle').textContent = 'Edit: ' + templateName.replace(/_/g, ' ').replace(/\b\w/g, function(l) {
            return l.toUpperCase();
          });
          AdminEmailEditorPage.updatePreview();
        })
        .catch(function(error) {
          AdminEmailEditorPage.showError('Failed to load template: ' + error.message);
        });
    },

    updatePreview: function() {
      const subject = document.getElementById('templateSubject').value;
      const body = document.getElementById('templateBody').value;
      let previewSubject = subject;
      let previewBody = body;

      for (const [key, value] of Object.entries(AdminEmailEditorPage.sampleData)) {
        const regex = new RegExp('\\{' + key + '\\}', 'g');
        previewSubject = previewSubject.replace(regex, value);
        previewBody = previewBody.replace(regex, value);
      }

      document.getElementById('previewSubject').textContent = previewSubject || 'Subject will appear here';
      document.getElementById('previewBody').textContent = previewBody || 'Email body will appear here...';
    },

    saveTemplate: function() {
      const apiPath = AdminEmailEditorPage.getData().adminApiPath;
      const saveBtn = document.getElementById('saveBtn');
      const originalText = saveBtn.textContent;
      saveBtn.disabled = true;
      saveBtn.innerHTML = '<span class="loading"></span> Saving...';

      const subject = document.getElementById('templateSubject').value;
      const body = document.getElementById('templateBody').value;

      fetch(apiPath + '/email/templates/' + AdminEmailEditorPage.currentTemplate, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ subject: subject, body: body })
      })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to save template');
          AdminEmailEditorPage.showSuccess('Template saved successfully!');
        })
        .catch(function(error) {
          AdminEmailEditorPage.showError('Failed to save template: ' + error.message);
        })
        .finally(function() {
          saveBtn.disabled = false;
          saveBtn.textContent = originalText;
        });
    },

    reloadTemplate: function() {
      showConfirm('Reload template from disk? Any unsaved changes will be lost.', 'Reload Template')
        .then(function(confirmed) {
          if (!confirmed) return;
          AdminEmailEditorPage.loadTemplate(AdminEmailEditorPage.currentTemplate);
          AdminEmailEditorPage.showSuccess('Template reloaded from disk');
        });
    },

    sendTestEmail: function() {
      const apiPath = AdminEmailEditorPage.getData().adminApiPath;
      const testBtn = document.getElementById('testBtn');
      const originalText = testBtn.textContent;
      testBtn.disabled = true;
      testBtn.innerHTML = '<span class="loading"></span> Sending...';

      fetch(apiPath + '/email/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ template: AdminEmailEditorPage.currentTemplate })
      })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to send test email');
          AdminEmailEditorPage.showSuccess('Test email sent successfully! Check your inbox.');
        })
        .catch(function(error) {
          AdminEmailEditorPage.showError('Failed to send test email: ' + error.message);
        })
        .finally(function() {
          testBtn.disabled = false;
          testBtn.textContent = originalText;
        });
    },

    showSuccess: function(message) {
      const alert = document.getElementById('successAlert');
      alert.textContent = message;
      alert.classList.add('show');
      setTimeout(function() {
        alert.classList.remove('show');
      }, 5000);
    },

    showError: function(message) {
      const alert = document.getElementById('errorAlert');
      alert.textContent = message;
      alert.classList.add('show');
      setTimeout(function() {
        alert.classList.remove('show');
      }, 5000);
    },

    init: function() {
      if (!document.getElementById('admin-email-editor-data')) return;

      document.querySelectorAll('.template-item').forEach(function(item) {
        item.addEventListener('click', function() {
          document.querySelectorAll('.template-item').forEach(function(i) {
            i.classList.remove('active');
            i.setAttribute('aria-pressed', 'false');
          });
          this.classList.add('active');
          this.setAttribute('aria-pressed', 'true');
          AdminEmailEditorPage.currentTemplate = this.dataset.template;
          AdminEmailEditorPage.loadTemplate(AdminEmailEditorPage.currentTemplate);
        });

        item.addEventListener('keydown', function(e) {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            this.click();
          }
        });
      });

      document.querySelectorAll('.variable-tag').forEach(function(tag) {
        tag.addEventListener('click', function() {
          const textarea = document.getElementById('templateBody');
          const cursorPos = textarea.selectionStart;
          const textBefore = textarea.value.substring(0, cursorPos);
          const textAfter = textarea.value.substring(textarea.selectionEnd);

          textarea.value = textBefore + this.dataset.var + textAfter;
          textarea.focus();
          textarea.setSelectionRange(cursorPos + this.dataset.var.length, cursorPos + this.dataset.var.length);

          AdminEmailEditorPage.updatePreview();
        });
      });

      document.getElementById('templateSubject').addEventListener('input', AdminEmailEditorPage.updatePreview);
      document.getElementById('templateBody').addEventListener('input', AdminEmailEditorPage.updatePreview);

      document.getElementById('saveBtn').addEventListener('click', AdminEmailEditorPage.saveTemplate);
      document.getElementById('reloadBtn').addEventListener('click', AdminEmailEditorPage.reloadTemplate);
      document.getElementById('testBtn').addEventListener('click', AdminEmailEditorPage.sendTestEmail);

      AdminEmailEditorPage.loadTemplate(AdminEmailEditorPage.currentTemplate);
    }
  };

  const AdminEmailPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminEmailPage.dataPayload) {
        const el = document.getElementById('admin-email-data');
        AdminEmailPage.dataPayload = JSON.parse(el.textContent);
      }
      return AdminEmailPage.dataPayload;
    },

    toggleSMTPFields: function() {
      const enabled = document.getElementById('smtp_enabled').checked;
      const fields = document.getElementById('smtp-fields');
      fields.style.opacity = enabled ? '1' : '0.5';
      fields.style.pointerEvents = enabled ? 'auto' : 'none';
    },

    loadSettings: function() {
      const adminApiPath = AdminEmailPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/settings/all')
        .then(function(response) {
          return response.json();
        })
        .then(function(data) {
          if (!data.settings) return;
          for (const [key, setting] of Object.entries(data.settings)) {
            const input = document.querySelector('[name="' + key + '"]');
            if (input) {
              if (input.type === 'checkbox') {
                input.checked = setting.value === 'true';
              } else {
                input.value = setting.value || '';
              }
            }
          }
          AdminEmailPage.toggleSMTPFields();
        })
        .catch(function(error) {
          AdminEmailPage.showNotification('Failed to load settings', 'error');
        });
    },

    submitSMTPForm: function(form) {
      const adminApiPath = AdminEmailPage.getData().adminApiPath;
      const formData = new FormData(form);
      const settings = {};

      for (const [key, value] of formData.entries()) {
        settings[key] = value;
      }

      settings['smtp.enabled'] = document.getElementById('smtp_enabled').checked ? 'true' : 'false';

      fetch(adminApiPath + '/server/settings/bulk', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ settings: settings })
      })
        .then(function(response) {
          return response.json().then(function(data) {
            if (!response.ok) {
              throw new Error((data.error && data.error.message) || 'Failed to save settings');
            }
            AdminEmailPage.showNotification('SMTP settings saved successfully', 'success');
          });
        })
        .catch(function(error) {
          AdminEmailPage.showNotification(error.message, 'error');
        });
    },

    sendTestEmail: function() {
      const adminApiPath = AdminEmailPage.getData().adminApiPath;
      const btn = document.getElementById('test-email-btn');
      btn.disabled = true;
      btn.textContent = 'Sending...';

      fetch(adminApiPath + '/server/test/email', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
      })
        .then(function(response) {
          return response.json().then(function(data) {
            if (!response.ok) {
              throw new Error((data.error && data.error.message) || 'Failed to send test email');
            }
            AdminEmailPage.showTestResult('Test email sent successfully! Check your inbox.', 'success');
          });
        })
        .catch(function(error) {
          AdminEmailPage.showTestResult(error.message, 'error');
        })
        .finally(function() {
          btn.disabled = false;
          btn.textContent = 'Send Test Email';
        });
    },

    autodetectSMTP: function() {
      const adminApiPath = AdminEmailPage.getData().adminApiPath;
      const btn = document.getElementById('autodetect-btn');
      btn.disabled = true;
      btn.textContent = 'Detecting...';

      fetch(adminApiPath + '/server/smtp/autodetect', { method: 'POST' })
        .then(function(response) {
          return response.json().then(function(data) {
            return { ok: response.ok, data: data };
          });
        })
        .then(function(result) {
          if (result.ok && result.data.detected) {
            AdminEmailPage.showNotification('SMTP server detected at ' + result.data.host, 'success');
            AdminEmailPage.loadSettings();
          } else {
            AdminEmailPage.showNotification('No SMTP server detected on local network', 'info');
          }
        })
        .catch(function(error) {
          AdminEmailPage.showNotification('Auto-detection failed', 'error');
        })
        .finally(function() {
          btn.disabled = false;
          btn.textContent = 'Auto-Detect';
        });
    },

    showNotification: function(message, type) {
      const alertClass = type === 'success' ? 'alert-success' : type === 'error' ? 'alert-danger' : 'alert-info';
      const notification = document.createElement('div');
      notification.className = 'alert ' + alertClass + ' toast-notification';
      notification.textContent = message;

      document.body.appendChild(notification);

      setTimeout(function() {
        notification.remove();
      }, 4000);
    },

    showTestResult: function(message, type) {
      const result = document.getElementById('test-result');
      result.className = 'alert alert-' + (type === 'success' ? 'success' : 'danger');
      result.textContent = message;
      result.style.display = 'block';

      setTimeout(function() {
        result.style.display = 'none';
      }, 5000);
    },

    init: function() {
      if (!document.getElementById('admin-email-data')) return;

      AdminEmailPage.loadSettings();
      AdminEmailPage.toggleSMTPFields();

      document.getElementById('smtp_enabled').addEventListener('change', AdminEmailPage.toggleSMTPFields);

      document.getElementById('smtp-form').addEventListener('submit', function(e) {
        e.preventDefault();
        AdminEmailPage.submitSMTPForm(e.target);
      });

      document.getElementById('test-email-btn').addEventListener('click', AdminEmailPage.sendTestEmail);
      document.getElementById('autodetect-btn').addEventListener('click', AdminEmailPage.autodetectSMTP);
    }
  };

  const AdminLogsPage = {
    dataPayload: null,
    logs: [],
    filteredLogs: [],
    autoRefreshInterval: null,

    getData: function() {
      if (!AdminLogsPage.dataPayload) {
        const el = document.getElementById('admin-logs-data');
        AdminLogsPage.dataPayload = JSON.parse(el.textContent);
      }
      return AdminLogsPage.dataPayload;
    },

    fetchLogs: function() {
      const adminApiPath = AdminLogsPage.getData().adminApiPath;
      const refreshBtn = document.getElementById('refreshBtn');
      const originalText = refreshBtn.innerHTML;
      refreshBtn.disabled = true;
      refreshBtn.innerHTML = '<span class="loading"></span> Loading...';

      const tailLines = document.getElementById('tailLines').value;

      fetch(adminApiPath + '/logs?tail=' + tailLines)
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to fetch logs');
          return response.json();
        })
        .then(function(data) {
          AdminLogsPage.logs = data.logs || [];
          AdminLogsPage.applyFilters();
          AdminLogsPage.updateStats();
        })
        .catch(function(error) {
          AdminLogsPage.showNoLogs('Error loading logs: ' + error.message);
        })
        .finally(function() {
          refreshBtn.disabled = false;
          refreshBtn.innerHTML = originalText;
        });
    },

    applyFilters: function() {
      const level = document.getElementById('logLevel').value;
      const source = document.getElementById('logSource').value;
      const search = document.getElementById('searchQuery').value.toLowerCase();

      AdminLogsPage.filteredLogs = AdminLogsPage.logs.filter(function(log) {
        if (level !== 'all' && log.level !== level) return false;
        if (source !== 'all' && log.source !== source) return false;
        if (search && !log.message.toLowerCase().includes(search)) return false;
        return true;
      });

      AdminLogsPage.renderLogs();
    },

    renderLogs: function() {
      const container = document.getElementById('logsContainer');

      if (AdminLogsPage.filteredLogs.length === 0) {
        AdminLogsPage.showNoLogs('No logs match the current filters');
        return;
      }

      const search = document.getElementById('searchQuery').value;
      let html = '';

      AdminLogsPage.filteredLogs.forEach(function(log) {
        let message = AdminLogsPage.escapeHtml(log.message);

        if (search) {
          const regex = new RegExp('(' + AdminLogsPage.escapeRegex(search) + ')', 'gi');
          message = message.replace(regex, '<span class="search-highlight">$1</span>');
        }

        html += '<div class="log-entry">' +
          '<span class="log-timestamp">' + log.timestamp + '</span>' +
          '<span class="log-level log-level-' + log.level + '">' + log.level + '</span>' +
          '<span class="log-message">' + message + '</span>' +
          '</div>';
      });

      container.innerHTML = html;
      container.scrollTop = container.scrollHeight;
    },

    showNoLogs: function(message) {
      const container = document.getElementById('logsContainer');
      container.innerHTML = '<div class="no-logs"><p>' + message + '</p></div>';
    },

    updateStats: function() {
      document.getElementById('totalEntries').textContent = AdminLogsPage.logs.length;
      document.getElementById('filteredEntries').textContent = AdminLogsPage.filteredLogs.length;

      const errorCount = AdminLogsPage.logs.filter(function(log) {
        return log.level === 'ERROR' || log.level === 'FATAL';
      }).length;
      const warnCount = AdminLogsPage.logs.filter(function(log) {
        return log.level === 'WARN';
      }).length;

      document.getElementById('errorCount').textContent = errorCount;
      document.getElementById('warnCount').textContent = warnCount;
    },

    downloadLogs: function() {
      const adminApiPath = AdminLogsPage.getData().adminApiPath;
      fetch(adminApiPath + '/logs/download')
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to download logs');
          return response.blob();
        })
        .then(function(blob) {
          const url = window.URL.createObjectURL(blob);
          const a = document.createElement('a');
          a.href = url;
          a.download = 'logs_' + new Date().toISOString().split('T')[0] + '.log';
          document.body.appendChild(a);
          a.click();
          document.body.removeChild(a);
          window.URL.revokeObjectURL(url);
        })
        .catch(function(error) {
          Toast.show('Error downloading logs: ' + error.message, 'error');
        });
    },

    clearLogs: function() {
      const adminApiPath = AdminLogsPage.getData().adminApiPath;
      showConfirm('Are you sure you want to clear all logs? This action cannot be undone.', 'Clear Logs')
        .then(function(confirmed) {
          if (!confirmed) return;

          fetch(adminApiPath + '/logs', { method: 'DELETE' })
            .then(function(response) {
              if (!response.ok) throw new Error('Failed to clear logs');
              AdminLogsPage.logs = [];
              AdminLogsPage.filteredLogs = [];
              AdminLogsPage.renderLogs();
              AdminLogsPage.updateStats();
              Toast.show('Logs cleared successfully', 'success');
            })
            .catch(function(error) {
              Toast.show('Error clearing logs: ' + error.message, 'error');
            });
        });
    },

    clearFilters: function() {
      document.getElementById('logLevel').value = 'all';
      document.getElementById('logSource').value = 'all';
      document.getElementById('searchQuery').value = '';
      AdminLogsPage.applyFilters();
    },

    toggleAutoRefresh: function() {
      const enabled = document.getElementById('autoRefresh').checked;

      if (enabled) {
        AdminLogsPage.autoRefreshInterval = setInterval(AdminLogsPage.fetchLogs, 5000);
      } else if (AdminLogsPage.autoRefreshInterval) {
        clearInterval(AdminLogsPage.autoRefreshInterval);
        AdminLogsPage.autoRefreshInterval = null;
      }
    },

    escapeHtml: function(text) {
      const div = document.createElement('div');
      div.textContent = text;
      return div.innerHTML;
    },

    escapeRegex: function(text) {
      return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    },

    init: function() {
      if (!document.getElementById('admin-logs-data')) return;

      document.getElementById('logLevel').addEventListener('change', AdminLogsPage.applyFilters);
      document.getElementById('logSource').addEventListener('change', AdminLogsPage.applyFilters);
      document.getElementById('searchQuery').addEventListener('input', AdminLogsPage.applyFilters);
      document.getElementById('tailLines').addEventListener('change', AdminLogsPage.fetchLogs);
      document.getElementById('refreshBtn').addEventListener('click', AdminLogsPage.fetchLogs);
      document.getElementById('downloadBtn').addEventListener('click', AdminLogsPage.downloadLogs);
      document.getElementById('clearLogsBtn').addEventListener('click', AdminLogsPage.clearLogs);
      document.getElementById('clearFilterBtn').addEventListener('click', AdminLogsPage.clearFilters);
      document.getElementById('autoRefresh').addEventListener('change', AdminLogsPage.toggleAutoRefresh);

      AdminLogsPage.fetchLogs();

      window.addEventListener('beforeunload', function() {
        if (AdminLogsPage.autoRefreshInterval) {
          clearInterval(AdminLogsPage.autoRefreshInterval);
        }
      });
    }
  };

  const AdminMetricsPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminMetricsPage.dataPayload) {
        AdminMetricsPage.dataPayload = JSON.parse(document.getElementById('admin-metrics-data').textContent);
      }
      return AdminMetricsPage.dataPayload;
    },

    switchTab: function(tab) {
      document.querySelectorAll('.admin-tab').forEach(function(t) { t.classList.remove('active'); });
      document.querySelectorAll('.admin-tab-content').forEach(function(c) { c.classList.remove('active'); });

      tab.classList.add('active');
      document.getElementById(tab.dataset.tab).classList.add('active');
    },

    loadStats: function() {
      fetch(AdminMetricsPage.getData().adminApiPath + '/server/metrics/stats')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          document.getElementById('totalMetrics').textContent = data.total || 0;
          document.getElementById('enabledMetrics').textContent = data.enabled || 0;
          document.getElementById('customMetrics').textContent = data.custom || 0;
        })
        .catch(function(error) {
          console.error('Failed to load stats:', error);
        });
    },

    saveConfig: function(e) {
      e.preventDefault();

      const config = {
        enabled: document.getElementById('prometheusEnabled').checked,
        path: document.getElementById('metricsPath').value,
        namespace: document.getElementById('namespace').value,
        subsystem: document.getElementById('subsystem').value,
        includeGoMetrics: document.getElementById('includeGoMetrics').checked,
        includeProcessMetrics: document.getElementById('includeProcessMetrics').checked
      };

      fetch(AdminMetricsPage.getData().adminApiPath + '/server/metrics/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config)
      })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to save configuration');
          AdminMetricsPage.showSuccess('Prometheus configuration saved successfully!');
        })
        .catch(function(error) {
          AdminMetricsPage.showError('Failed to save configuration: ' + error.message);
        });
    },

    testEndpoint: function() {
      fetch('/metrics')
        .then(function(response) {
          return response.text().then(function(text) {
            if (response.ok) {
              AdminMetricsPage.showSuccess('Prometheus endpoint is working! Check browser console for output.');
              console.log('Metrics output:', text);
            } else {
              AdminMetricsPage.showError('Endpoint returned error: ' + response.status);
            }
          });
        })
        .catch(function(error) {
          AdminMetricsPage.showError('Failed to test endpoint: ' + error.message);
        });
    },

    exportPrometheus: function() {
      fetch('/metrics')
        .then(function(response) { return response.text(); })
        .then(function(text) {
          document.getElementById('exportPreview').textContent = text;
        });
    },

    exportJson: function() {
      fetch(AdminMetricsPage.getData().adminApiPath + '/server/metrics/export?format=json')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          document.getElementById('exportPreview').textContent = JSON.stringify(data, null, 2);
        });
    },

    showSuccess: function(msg) {
      const alert = document.getElementById('successAlert');
      alert.textContent = msg;
      alert.classList.add('show');
      setTimeout(function() { alert.classList.remove('show'); }, 5000);
    },

    showError: function(msg) {
      const alert = document.getElementById('errorAlert');
      alert.textContent = msg;
      alert.classList.add('show');
      setTimeout(function() { alert.classList.remove('show'); }, 5000);
    },

    init: function() {
      if (!document.getElementById('admin-metrics-data')) return;

      document.querySelectorAll('.admin-tab').forEach(function(tab) {
        tab.addEventListener('click', function() { AdminMetricsPage.switchTab(tab); });
      });

      document.getElementById('prometheusForm').addEventListener('submit', AdminMetricsPage.saveConfig);
      document.getElementById('testPrometheusBtn').addEventListener('click', AdminMetricsPage.testEndpoint);
      document.getElementById('exportPrometheusBtn').addEventListener('click', AdminMetricsPage.exportPrometheus);
      document.getElementById('exportJsonBtn').addEventListener('click', AdminMetricsPage.exportJson);

      AdminMetricsPage.loadStats();
    }
  };

  const AdminNotificationsPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminNotificationsPage.dataPayload) {
        AdminNotificationsPage.dataPayload = JSON.parse(document.getElementById('admin-notifications-data').textContent);
      }
      return AdminNotificationsPage.dataPayload;
    },

    updateDurationLabel: function(type, value) {
      const label = type.charAt(0).toUpperCase() + type.slice(1);
      document.getElementById('adminDurationLabel' + label).textContent = value + 's';
    },

    loadPreferences: function() {
      fetch(AdminNotificationsPage.getData().adminApiPath + '/notifications/preferences')
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to load preferences');
          return response.json();
        })
        .then(function(prefs) {
          document.getElementById('adminEnableToast').checked = prefs.enable_toast !== false;
          document.getElementById('adminEnableBanner').checked = prefs.enable_banner !== false;
          document.getElementById('adminEnableCenter').checked = prefs.enable_center !== false;
          document.getElementById('adminEnableSound').checked = prefs.enable_sound === true;

          const successDuration = prefs.toast_duration_success || 5;
          const infoDuration = prefs.toast_duration_info || 5;
          const warningDuration = prefs.toast_duration_warning || 10;

          document.getElementById('adminToastDurationSuccess').value = successDuration;
          document.getElementById('adminToastDurationInfo').value = infoDuration;
          document.getElementById('adminToastDurationWarning').value = warningDuration;

          AdminNotificationsPage.updateDurationLabel('success', successDuration);
          AdminNotificationsPage.updateDurationLabel('info', infoDuration);
          AdminNotificationsPage.updateDurationLabel('warning', warningDuration);
        })
        .catch(function(error) {
          console.error('Failed to load admin notification preferences:', error);
        });
    },

    savePreferences: function(e) {
      e.preventDefault();

      const preferences = {
        enable_toast: document.getElementById('adminEnableToast').checked,
        enable_banner: document.getElementById('adminEnableBanner').checked,
        enable_center: document.getElementById('adminEnableCenter').checked,
        enable_sound: document.getElementById('adminEnableSound').checked,
        toast_duration_success: parseInt(document.getElementById('adminToastDurationSuccess').value),
        toast_duration_info: parseInt(document.getElementById('adminToastDurationInfo').value),
        toast_duration_warning: parseInt(document.getElementById('adminToastDurationWarning').value)
      };

      fetch(AdminNotificationsPage.getData().adminApiPath + '/notifications/preferences', {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(preferences)
      })
        .then(function(response) {
          if (!response.ok) {
            return response.json().then(function(error) {
              throw new Error(error.error || 'Failed to update preferences');
            });
          }

          if (typeof notificationManager !== 'undefined') {
            notificationManager.preferences = preferences;
            notificationManager.showToast({
              type: 'success',
              title: 'Preferences Saved',
              message: 'Your notification preferences have been updated successfully.',
              display: 'toast'
            });
          } else {
            Toast.show('Notification preferences updated successfully!', 'success');
          }
        })
        .catch(function(error) {
          if (typeof notificationManager !== 'undefined') {
            notificationManager.showToast({
              type: 'error',
              title: 'Save Failed',
              message: 'Failed to update preferences: ' + error.message,
              display: 'toast'
            });
          } else {
            Toast.show('Failed to update preferences: ' + error.message, 'error');
          }
        });
    },

    init: function() {
      if (!document.getElementById('admin-notifications-data')) return;

      document.getElementById('adminToastDurationSuccess').addEventListener('input', function() {
        AdminNotificationsPage.updateDurationLabel('success', this.value);
      });
      document.getElementById('adminToastDurationInfo').addEventListener('input', function() {
        AdminNotificationsPage.updateDurationLabel('info', this.value);
      });
      document.getElementById('adminToastDurationWarning').addEventListener('input', function() {
        AdminNotificationsPage.updateDurationLabel('warning', this.value);
      });

      document.getElementById('adminNotificationPreferencesForm').addEventListener('submit', AdminNotificationsPage.savePreferences);

      AdminNotificationsPage.loadPreferences();
    }
  };

  const AdminPasskeyLoginPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminPasskeyLoginPage.dataPayload) {
        AdminPasskeyLoginPage.dataPayload = JSON.parse(document.getElementById('admin-passkey-login-data').textContent);
      }
      return AdminPasskeyLoginPage.dataPayload;
    },

    bufferToBase64url: function(buffer) {
      const bytes = new Uint8Array(buffer);
      let binary = '';
      for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
      return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
    },

    base64urlToBuffer: function(b64url) {
      const padded = (b64url + '==='.slice((b64url.length + 3) % 4))
        .replace(/-/g, '+').replace(/_/g, '/');
      const binary = atob(padded);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
      return bytes.buffer;
    },

    normalizeOptions: function(options) {
      const pk = options.publicKey || options;
      pk.challenge = AdminPasskeyLoginPage.base64urlToBuffer(pk.challenge);
      if (Array.isArray(pk.allowCredentials)) {
        pk.allowCredentials = pk.allowCredentials.map(function(cred) {
          return Object.assign({}, cred, { id: AdminPasskeyLoginPage.base64urlToBuffer(cred.id) });
        });
      }
      return pk;
    },

    serializeAssertion: function(assertion, ceremonyToken) {
      return {
        ceremony_token: ceremonyToken,
        id: assertion.id,
        rawId: AdminPasskeyLoginPage.bufferToBase64url(assertion.rawId),
        type: assertion.type,
        response: {
          authenticatorData: AdminPasskeyLoginPage.bufferToBase64url(assertion.response.authenticatorData),
          clientDataJSON: AdminPasskeyLoginPage.bufferToBase64url(assertion.response.clientDataJSON),
          signature: AdminPasskeyLoginPage.bufferToBase64url(assertion.response.signature),
          userHandle: assertion.response.userHandle
            ? AdminPasskeyLoginPage.bufferToBase64url(assertion.response.userHandle)
            : null,
        },
      };
    },

    setStatus: function(msg, cls) {
      const box = document.getElementById('passkeyStatus');
      if (!box) return;
      box.className = 'status-box ' + cls;
      box.innerHTML = msg;
    },

    run: function() {
      const pendingToken = AdminPasskeyLoginPage.getData().pendingToken;
      const adminPath = AdminPasskeyLoginPage.getData().adminPath;

      if (!pendingToken) {
        AdminPasskeyLoginPage.setStatus('Session expired or invalid. <a href="/server/auth/login" class="inline-link">Please log in again.</a>', 'status-error');
        return;
      }
      if (!window.PublicKeyCredential || !navigator.credentials || !navigator.credentials.get) {
        AdminPasskeyLoginPage.setStatus('This browser does not support passkeys. Please use a modern browser.', 'status-error');
        return;
      }

      let challengeData;
      fetch('/api/v1/server/auth/admin/passkey/challenge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ session_token: pendingToken }),
      })
        .then(function(challengeResp) {
          return challengeResp.json().then(function(data) {
            if (!challengeResp.ok || !data.ok) {
              throw new Error(data.error || 'Failed to start passkey authentication');
            }
            return data;
          });
        })
        .then(function(data) {
          challengeData = data;
          AdminPasskeyLoginPage.setStatus('<span class="spinner"></span> Touch your security key or use your device passkey to continue…', 'status-info');
          return navigator.credentials.get({
            publicKey: AdminPasskeyLoginPage.normalizeOptions(challengeData.options),
          });
        })
        .then(function(credential) {
          if (!credential) {
            throw new Error('Passkey authentication was cancelled');
          }
          AdminPasskeyLoginPage.setStatus('<span class="spinner"></span> Verifying credential…', 'status-info');
          const payload = AdminPasskeyLoginPage.serializeAssertion(credential, challengeData.ceremony_token);
          return fetch('/api/v1/server/auth/admin/passkey/verify', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
          });
        })
        .then(function(verifyResp) {
          return verifyResp.json().then(function(verifyData) {
            if (!verifyResp.ok || !verifyData.ok) {
              throw new Error(verifyData.error || 'Passkey verification failed');
            }
            AdminPasskeyLoginPage.setStatus('Authentication successful. Redirecting…', 'status-success');
            window.location.href = '/' + adminPath + '/dashboard';
          });
        })
        .catch(function(err) {
          AdminPasskeyLoginPage.setStatus('Passkey authentication failed: ' + (err.message || String(err)) +
            ' <a href="/server/auth/login" class="inline-link">Try again</a>', 'status-error');
        });
    },

    init: function() {
      if (!document.getElementById('admin-passkey-login-data')) return;
      AdminPasskeyLoginPage.run();
    }
  };

  const AdminSchedulerPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminSchedulerPage.dataPayload) {
        AdminSchedulerPage.dataPayload = JSON.parse(document.getElementById('admin-scheduler-data').textContent);
      }
      return AdminSchedulerPage.dataPayload;
    },

    previewCron: function(taskId, expr) {
      const preview = document.getElementById(taskId + '_preview');
      if (!preview) return;
      if (!expr || expr.trim() === '') {
        preview.style.display = 'none';
        return;
      }

      let description = '';
      if (expr === '@hourly') {
        description = 'Every hour at the start of the hour';
      } else if (expr === '@daily') {
        description = 'Every day at midnight';
      } else if (expr === '@weekly') {
        description = 'Every Sunday at midnight';
      } else if (expr.startsWith('@every ')) {
        description = 'Every ' + expr.substring(7);
      } else if (expr.match(/^\d+ \d+ \* \* \*$/)) {
        const parts = expr.split(' ');
        description = 'Daily at ' + parts[1].padStart(2, '0') + ':' + parts[0].padStart(2, '0');
      } else if (expr.match(/^\d+ \d+ \* \* \d+$/)) {
        const parts = expr.split(' ');
        const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
        description = 'Every ' + days[parseInt(parts[4])] + ' at ' + parts[1].padStart(2, '0') + ':' + parts[0].padStart(2, '0');
      } else {
        description = 'Custom cron expression: ' + expr;
      }

      preview.innerHTML = '<div class="cron-preview-title">Next Run:</div><div class="cron-preview-times">' + description + '</div>';
      preview.style.display = 'block';
    },

    showSuccess: function(msg) {
      const alert = document.getElementById('successAlert');
      alert.textContent = msg;
      alert.classList.add('show');
      setTimeout(function() { alert.classList.remove('show'); }, 5000);
    },

    showError: function(msg) {
      const alert = document.getElementById('errorAlert');
      alert.textContent = msg;
      alert.classList.add('show');
      setTimeout(function() { alert.classList.remove('show'); }, 5000);
    },

    submitForm: function(e) {
      e.preventDefault();

      const adminPath = AdminSchedulerPage.getData().adminPath;
      const formData = new FormData(e.target);
      const config = {};

      for (const [key, value] of formData.entries()) {
        const parts = key.split('.');
        if (parts.length === 1) {
          config[key] = value;
        } else if (parts.length === 2) {
          if (!config[parts[0]]) config[parts[0]] = {};
          config[parts[0]][parts[1]] = value;
        }
      }

      document.querySelectorAll('input[type="checkbox"]').forEach(function(cb) {
        const parts = cb.name.split('.');
        if (parts.length === 2) {
          if (!config[parts[0]]) config[parts[0]] = {};
          config[parts[0]][parts[1]] = cb.checked;
        }
      });

      fetch(adminPath + '/config/scheduler', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(config)
      })
        .then(function(response) {
          if (!response.ok) {
            return response.json().then(function(error) {
              throw new Error(error.error || 'Failed to save configuration');
            });
          }
          AdminSchedulerPage.showSuccess('Scheduler configuration saved successfully!');
          setTimeout(function() {
            window.location.href = adminPath + '/config/scheduler';
          }, 1500);
        })
        .catch(function(error) {
          AdminSchedulerPage.showError('Failed to save configuration: ' + error.message);
        });
    },

    init: function() {
      if (!document.getElementById('admin-scheduler-data')) return;

      document.querySelectorAll('.cron-schedule-input').forEach(function(input) {
        input.addEventListener('change', function() {
          AdminSchedulerPage.previewCron(this.dataset.taskId, this.value);
        });
      });

      document.getElementById('schedulerForm').addEventListener('submit', AdminSchedulerPage.submitForm);
    }
  };

  const AdminSecurityPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminSecurityPage.dataPayload) {
        AdminSecurityPage.dataPayload = JSON.parse(document.getElementById('admin-security-data').textContent);
      }
      return AdminSecurityPage.dataPayload;
    },

    toggleCORSFields: function() {
      const enabled = document.getElementById('cors_enabled').checked;
      const fields = document.getElementById('cors-fields');
      fields.style.opacity = enabled ? '1' : '0.5';
      fields.style.pointerEvents = enabled ? 'auto' : 'none';
    },

    toggleRateLimitFields: function() {
      const enabled = document.getElementById('ratelimit_enabled').checked;
      const fields = document.getElementById('ratelimit-fields');
      fields.style.opacity = enabled ? '1' : '0.5';
      fields.style.pointerEvents = enabled ? 'auto' : 'none';
    },

    loadSettings: function() {
      const adminApiPath = AdminSecurityPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/settings/all')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (data.settings) {
            Object.entries(data.settings).forEach(function(entry) {
              const key = entry[0];
              const setting = entry[1];
              const input = document.querySelector('[name="' + key + '"]');
              if (input) {
                if (input.type === 'checkbox') {
                  input.checked = setting.value === 'true';
                } else {
                  input.value = setting.value || '';
                }
              }
            });
            AdminSecurityPage.toggleCORSFields();
            AdminSecurityPage.toggleRateLimitFields();
          }
        })
        .catch(function(error) {
          console.error('Failed to load settings:', error);
          AdminSecurityPage.showNotification('Failed to load settings', 'error');
        });
    },

    saveFormSettings: function(form, successMessage) {
      const adminApiPath = AdminSecurityPage.getData().adminApiPath;
      const formData = new FormData(form);
      const settings = {};

      formData.forEach(function(value, key) {
        settings[key] = value;
      });

      form.querySelectorAll('input[type="checkbox"]').forEach(function(cb) {
        settings[cb.name] = cb.checked ? 'true' : 'false';
      });

      let responseOk = false;
      fetch(adminApiPath + '/server/settings/bulk', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ settings: settings })
      })
        .then(function(response) {
          responseOk = response.ok;
          return response.json();
        })
        .then(function(data) {
          if (responseOk) {
            AdminSecurityPage.showNotification(successMessage, 'success');
          } else {
            throw new Error((data.error && data.error.message) || 'Failed to save settings');
          }
        })
        .catch(function(error) {
          console.error('Save error:', error);
          AdminSecurityPage.showNotification(error.message, 'error');
        });
    },

    showNotification: function(message, type) {
      const alertClass = type === 'success' ? 'alert-success' : 'alert-danger';
      const notification = document.createElement('div');
      notification.className = 'alert ' + alertClass + ' floating-notification';
      notification.textContent = message;

      document.body.appendChild(notification);

      setTimeout(function() {
        notification.remove();
      }, 4000);
    },

    pkBufToBase64url: function(buffer) {
      const bytes = new Uint8Array(buffer);
      let binary = '';
      bytes.forEach(function(b) { binary += String.fromCharCode(b); });
      return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
    },

    pkBase64urlToBuf: function(b64url) {
      const padded = (b64url + '==='.slice((b64url.length + 3) % 4))
        .replace(/-/g, '+').replace(/_/g, '/');
      const binary = atob(padded);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
      return bytes.buffer;
    },

    pkNormalizeCreation: function(options) {
      const pk = options.publicKey || options;
      pk.challenge = AdminSecurityPage.pkBase64urlToBuf(pk.challenge);
      pk.user.id = AdminSecurityPage.pkBase64urlToBuf(pk.user.id);
      if (Array.isArray(pk.excludeCredentials)) {
        pk.excludeCredentials = pk.excludeCredentials.map(function(c) {
          return Object.assign({}, c, { id: AdminSecurityPage.pkBase64urlToBuf(c.id) });
        });
      }
      return pk;
    },

    pkSerializeCredential: function(cred, ceremonyToken) {
      return {
        ceremony_token: ceremonyToken,
        id: cred.id,
        rawId: AdminSecurityPage.pkBufToBase64url(cred.rawId),
        type: cred.type,
        response: {
          attestationObject: AdminSecurityPage.pkBufToBase64url(cred.response.attestationObject),
          clientDataJSON: AdminSecurityPage.pkBufToBase64url(cred.response.clientDataJSON),
          publicKey: cred.response.getPublicKey
            ? AdminSecurityPage.pkBufToBase64url(cred.response.getPublicKey())
            : undefined,
          authenticatorData: cred.response.getAuthenticatorData
            ? AdminSecurityPage.pkBufToBase64url(cred.response.getAuthenticatorData())
            : undefined,
        },
      };
    },

    pkSetOpStatus: function(msg, cls) {
      const el = document.getElementById('passkey-op-status');
      if (!el) return;
      el.classList.remove('passkey-op-status-hidden');
      el.className = 'alert ' + (cls || 'alert-info');
      el.textContent = msg;
    },

    escHtml: function(str) {
      return String(str)
        .replace(/&/g, '&amp;').replace(/</g, '&lt;')
        .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    },

    loadAdminPasskeys: function() {
      const adminApiPath = AdminSecurityPage.getData().adminApiPath;
      const statusEl = document.getElementById('passkeys-status');
      const listEl = document.getElementById('passkeys-list');
      const itemsEl = document.getElementById('passkeys-items');
      let respOk = false;
      fetch(adminApiPath + '/profile/security/passkeys')
        .then(function(resp) {
          respOk = resp.ok;
          return resp.json();
        })
        .then(function(data) {
          if (!respOk || !data.ok) throw new Error(data.error || 'Failed to load passkeys');

          const passkeys = data.passkeys || [];
          if (passkeys.length === 0) {
            statusEl.className = 'alert alert-info';
            statusEl.textContent = 'No passkeys registered yet. Add one below to enable passkey-based login.';
            listEl.classList.add('passkeys-list-hidden');
          } else {
            statusEl.className = 'alert alert-success';
            statusEl.textContent = passkeys.length + ' passkey' + (passkeys.length === 1 ? '' : 's') + ' registered.';
            itemsEl.innerHTML = passkeys.map(function(pk) {
              const name = pk.name || pk.Name || 'Unnamed';
              const createdAt = pk.created_at || pk.CreatedAt || '';
              const lastUsedAt = pk.last_used_at || pk.LastUsedAt;
              const id = pk.id || pk.ID;
              return '<div class="passkey-item">' +
                '<div>' +
                '<div class="passkey-item-name">' + AdminSecurityPage.escHtml(name) + '</div>' +
                '<div class="passkey-item-meta">' +
                'Added ' + AdminSecurityPage.escHtml(createdAt) +
                (lastUsedAt ? ' · Last used ' + AdminSecurityPage.escHtml(lastUsedAt) : '') +
                '</div>' +
                '</div>' +
                '<button type="button" class="btn btn-danger passkey-item-delete-btn" ' +
                'data-action="delete-admin-passkey" ' +
                'data-passkey-id="' + AdminSecurityPage.escHtml(id) + '" ' +
                'data-passkey-name="' + AdminSecurityPage.escHtml(name) + '" ' +
                'aria-label="Delete passkey ' + AdminSecurityPage.escHtml(name) + '"' +
                '>Delete</button>' +
                '</div>';
            }).join('');
            listEl.classList.remove('passkeys-list-hidden');
          }
        })
        .catch(function(err) {
          statusEl.className = 'alert alert-error';
          statusEl.textContent = 'Failed to load passkeys: ' + err.message;
        });
    },

    registerAdminPasskey: function() {
      if (!window.PublicKeyCredential || !navigator.credentials || !navigator.credentials.create) {
        AdminSecurityPage.pkSetOpStatus('This browser does not support passkeys.', 'alert-error');
        return;
      }

      const adminApiPath = AdminSecurityPage.getData().adminApiPath;
      const name = document.getElementById('adminPasskeyName').value.trim();
      const password = document.getElementById('adminPasskeyPassword').value;
      if (!name || !password) {
        AdminSecurityPage.pkSetOpStatus('Passkey name and admin password are required.', 'alert-error');
        return;
      }

      const btn = document.getElementById('registerPasskeyBtn');
      btn.disabled = true;
      AdminSecurityPage.pkSetOpStatus('Starting passkey registration…', 'alert-info');

      let startData = null;
      let startResp = null;
      fetch(adminApiPath + '/profile/security/passkeys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name, password: password }),
      })
        .then(function(resp) {
          startResp = resp;
          return resp.json();
        })
        .then(function(data) {
          startData = data;
          if (!startResp.ok || !startData.ok) {
            throw new Error(startData.error || 'Failed to start registration');
          }

          AdminSecurityPage.pkSetOpStatus('Touch your security key or use your device passkey prompt…', 'alert-info');

          return navigator.credentials.create({
            publicKey: AdminSecurityPage.pkNormalizeCreation(startData.options),
          });
        })
        .then(function(credential) {
          if (!credential) throw new Error('Passkey registration was cancelled');

          AdminSecurityPage.pkSetOpStatus('Completing registration…', 'alert-info');

          let finishResp = null;
          return fetch(adminApiPath + '/profile/security/passkeys', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(AdminSecurityPage.pkSerializeCredential(credential, startData.ceremony_token)),
          })
            .then(function(resp) {
              finishResp = resp;
              return resp.json();
            })
            .then(function(finishData) {
              if (!finishResp.ok || !finishData.ok) {
                throw new Error(finishData.error || 'Failed to complete registration');
              }

              AdminSecurityPage.pkSetOpStatus('Passkey registered successfully.', 'alert-success');
              document.getElementById('adminPasskeyName').value = '';
              document.getElementById('adminPasskeyPassword').value = '';
              AdminSecurityPage.loadAdminPasskeys();
            });
        })
        .catch(function(err) {
          AdminSecurityPage.pkSetOpStatus('Registration failed: ' + err.message, 'alert-error');
        })
        .finally(function() {
          btn.disabled = false;
        });
    },

    deleteAdminPasskey: function(passkeyID, name) {
      showConfirm('Delete passkey "' + name + '"? You will no longer be able to use it for login.').then(function(confirmed) {
        if (!confirmed) return;

        const adminApiPath = AdminSecurityPage.getData().adminApiPath;
        AdminSecurityPage.pkSetOpStatus('Deleting passkey…', 'alert-info');
        let respOk = false;
        fetch(adminApiPath + '/profile/security/passkeys/' + encodeURIComponent(passkeyID), {
          method: 'DELETE',
        })
          .then(function(resp) {
            respOk = resp.ok;
            return resp.json();
          })
          .then(function(data) {
            if (!respOk || !data.ok) throw new Error(data.error || 'Failed to delete passkey');

            AdminSecurityPage.pkSetOpStatus('Passkey deleted.', 'alert-success');
            AdminSecurityPage.loadAdminPasskeys();
          })
          .catch(function(err) {
            AdminSecurityPage.pkSetOpStatus('Delete failed: ' + err.message, 'alert-error');
          });
      });
    },

    init: function() {
      if (!document.getElementById('admin-security-data')) return;

      AdminSecurityPage.loadSettings();
      AdminSecurityPage.loadAdminPasskeys();

      document.getElementById('cors_enabled').addEventListener('change', AdminSecurityPage.toggleCORSFields);
      document.getElementById('ratelimit_enabled').addEventListener('change', AdminSecurityPage.toggleRateLimitFields);

      document.getElementById('headers-form').addEventListener('submit', function(e) {
        e.preventDefault();
        AdminSecurityPage.saveFormSettings(this, 'Security headers saved successfully');
      });

      document.getElementById('cors-form').addEventListener('submit', function(e) {
        e.preventDefault();
        AdminSecurityPage.saveFormSettings(this, 'CORS settings saved successfully');
      });

      document.getElementById('ratelimit-form').addEventListener('submit', function(e) {
        e.preventDefault();
        AdminSecurityPage.saveFormSettings(this, 'Rate limiting settings saved successfully');
      });

      document.getElementById('auth-form').addEventListener('submit', function(e) {
        e.preventDefault();
        AdminSecurityPage.saveFormSettings(this, 'Authentication settings saved successfully');
      });

      document.getElementById('registerPasskeyBtn').addEventListener('click', AdminSecurityPage.registerAdminPasskey);
    }
  };

  const AdminSslPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminSslPage.dataPayload) {
        AdminSslPage.dataPayload = JSON.parse(document.getElementById('admin-ssl-data').textContent);
      }
      return AdminSslPage.dataPayload;
    },

    loadCertStatus: function() {
      const adminApiPath = AdminSslPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/ssl/status')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          AdminSslPage.renderCertStatus(data);
          AdminSslPage.updateSchedule(data);
        })
        .catch(function(error) {
          Toast.show('Failed to load certificate status: ' + error.message, 'error');
        });
    },

    renderCertStatus: function(data) {
      const container = document.getElementById('certStatus');

      if (!data.certificate) {
        container.innerHTML =
          '<div class="text-center p-4">' +
          '<div class="status-badge status-none">No Certificate</div>' +
          '<p class="mt-3 text-comment">No SSL certificate installed</p>' +
          '</div>';
        return;
      }

      const cert = data.certificate;
      const daysUntilExpiry = Math.floor((new Date(cert.notAfter) - new Date()) / (1000 * 60 * 60 * 24));

      let statusClass = 'status-active';
      let statusText = 'Active';

      if (daysUntilExpiry < 0) {
        statusClass = 'status-expired';
        statusText = 'Expired';
      } else if (daysUntilExpiry < 30) {
        statusClass = 'status-expiring';
        statusText = 'Expiring Soon';
      }

      container.innerHTML =
        '<div class="text-center mb-3">' +
        '<div class="status-badge ' + statusClass + '">' + statusText + '</div>' +
        '</div>' +
        '<div class="info-grid">' +
        '<div class="info-row"><span class="info-label">Subject:</span><span class="info-value">' + (cert.subject || '-') + '</span></div>' +
        '<div class="info-row"><span class="info-label">Issuer:</span><span class="info-value">' + (cert.issuer || '-') + '</span></div>' +
        '<div class="info-row"><span class="info-label">Valid From:</span><span class="info-value">' + new Date(cert.notBefore).toLocaleDateString() + '</span></div>' +
        '<div class="info-row"><span class="info-label">Valid Until:</span><span class="info-value">' + new Date(cert.notAfter).toLocaleDateString() + '</span></div>' +
        '<div class="info-row"><span class="info-label">Days Remaining:</span><span class="info-value">' + daysUntilExpiry + ' days</span></div>' +
        '</div>';
    },

    updateSchedule: function(data) {
      document.getElementById('nextCheck').textContent = data.nextCheck || '-';
      document.getElementById('nextRenewal').textContent = data.nextRenewal || '-';
      document.getElementById('lastRenewal').textContent = data.lastRenewal || 'Never';
    },

    updateProgress: function(percent, text) {
      document.getElementById('progressFill').style.width = percent + '%';
      document.getElementById('progressText').textContent = text;
    },

    submitLetsEncryptForm: function(e) {
      e.preventDefault();

      const adminApiPath = AdminSslPage.getData().adminApiPath;
      const obtainBtn = document.getElementById('obtainBtn');
      const progress = document.getElementById('obtainProgress');

      obtainBtn.disabled = true;
      progress.classList.remove('d-none');

      const domain = document.getElementById('domain').value;
      const email = document.getElementById('email').value;
      const altNames = document.getElementById('altNames').value.split('\n').filter(function(n) { return n.trim(); });

      AdminSslPage.updateProgress(10, 'Validating domain...');

      let responseOk = false;
      fetch(adminApiPath + '/server/ssl/obtain', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ domain: domain, email: email, altNames: altNames })
      })
        .then(function(response) {
          responseOk = response.ok;
          if (!responseOk) throw new Error('Failed to obtain certificate');

          AdminSslPage.updateProgress(100, 'Certificate obtained successfully!');
          Toast.show('SSL certificate obtained and installed successfully!', 'success');

          setTimeout(function() {
            progress.classList.add('d-none');
            AdminSslPage.loadCertStatus();
          }, 2000);
        })
        .catch(function(error) {
          Toast.show('Failed to obtain certificate: ' + error.message, 'error');
          progress.classList.add('d-none');
        })
        .finally(function() {
          obtainBtn.disabled = false;
        });
    },

    renewCert: function() {
      showConfirm('Renew the SSL certificate now?', 'Renew Certificate').then(function(confirmed) {
        if (!confirmed) return;

        const adminApiPath = AdminSslPage.getData().adminApiPath;
        const renewBtn = document.getElementById('renewBtn');
        renewBtn.disabled = true;

        fetch(adminApiPath + '/server/ssl/renew', { method: 'POST' })
          .then(function(response) {
            if (!response.ok) throw new Error('Failed to renew certificate');

            Toast.show('Certificate renewed successfully!', 'success');
            AdminSslPage.loadCertStatus();
          })
          .catch(function(error) {
            Toast.show('Failed to renew certificate: ' + error.message, 'error');
          })
          .finally(function() {
            renewBtn.disabled = false;
          });
      });
    },

    verifyCert: function() {
      const adminApiPath = AdminSslPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/ssl/verify', { method: 'POST' })
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (data.valid) {
            Toast.show('Certificate is valid and properly configured!', 'success');
          } else {
            Toast.show('Certificate validation failed: ' + data.error, 'error');
          }
        })
        .catch(function(error) {
          Toast.show('Verification failed: ' + error.message, 'error');
        });
    },

    saveSettings: function() {
      const adminApiPath = AdminSslPage.getData().adminApiPath;
      const settings = {
        autoRenewal: document.getElementById('autoRenewal').checked,
        renewalDays: parseInt(document.getElementById('renewalDays').value, 10),
        emailNotifications: document.getElementById('emailNotifications').checked
      };

      fetch(adminApiPath + '/server/ssl/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings)
      })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to save settings');

          Toast.show('Settings saved successfully!', 'success');
        })
        .catch(function(error) {
          Toast.show('Failed to save settings: ' + error.message, 'error');
        });
    },

    init: function() {
      if (!document.getElementById('admin-ssl-data')) return;

      document.getElementById('letsEncryptForm').addEventListener('submit', AdminSslPage.submitLetsEncryptForm);
      document.getElementById('renewBtn').addEventListener('click', AdminSslPage.renewCert);
      document.getElementById('refreshBtn').addEventListener('click', AdminSslPage.loadCertStatus);
      document.getElementById('verifyBtn').addEventListener('click', AdminSslPage.verifyCert);
      document.getElementById('saveSettingsBtn').addEventListener('click', AdminSslPage.saveSettings);

      AdminSslPage.loadCertStatus();
      setInterval(AdminSslPage.loadCertStatus, 30000);
    }
  };

  const AdminSystemPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminSystemPage.dataPayload) {
        AdminSystemPage.dataPayload = JSON.parse(document.getElementById('admin-system-data').textContent);
      }
      return AdminSystemPage.dataPayload;
    },

    loadSystemInfo: function() {
      const adminApiPath = AdminSystemPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/stats')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (!data) return;

          document.getElementById('uptime').textContent = AdminSystemPage.formatUptime(data.uptime_seconds || 0);
          document.getElementById('version').textContent = data.version || 'dev';
          document.getElementById('build-date').textContent = data.build_date || 'unknown';

          document.getElementById('os').textContent = data.os || '-';
          document.getElementById('arch').textContent = data.arch || '-';
          document.getElementById('go-version').textContent = data.go_version || '-';
          document.getElementById('hostname').textContent = data.hostname || '-';
          document.getElementById('working-dir').textContent = data.working_dir || '-';
          document.getElementById('pid').textContent = data.pid || '-';

          document.getElementById('mode').textContent = data.mode || 'production';
          document.getElementById('debug').textContent = data.debug ? 'Yes' : 'No';
        })
        .catch(function(error) {
          console.error('Failed to load system info:', error);
        });
    },

    loadResourceUsage: function() {
      const adminApiPath = AdminSystemPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/stats')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (!data.memory) return;

          document.getElementById('memory-alloc').textContent = AdminSystemPage.formatBytes(data.memory.alloc || 0);
          document.getElementById('memory-sys').textContent = AdminSystemPage.formatBytes(data.memory.sys || 0);
          document.getElementById('goroutines').textContent = data.goroutines || '0';
          document.getElementById('gc-runs').textContent = data.memory.num_gc || '0';
        })
        .catch(function(error) {
          console.error('Failed to load resource usage:', error);
        });
    },

    loadNetworkInfo: function() {
      const adminApiPath = AdminSystemPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/stats')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          document.getElementById('http-port').textContent = data.http_port || '-';
          document.getElementById('https-port').textContent = data.https_port || '-';
          document.getElementById('https-enabled').textContent = data.https_enabled ? 'Yes' : 'No';
          document.getElementById('server-url').textContent = data.server_url || window.location.origin;

          if (data.tor) {
            document.getElementById('tor-status').innerHTML = data.tor.enabled ?
              '<span class="badge badge-success">✅ Running</span>' :
              '<span class="badge badge-secondary">⚪ Disabled</span>';
            document.getElementById('onion-address').textContent = data.tor.address || 'Not configured';
          }
        })
        .catch(function(error) {
          console.error('Failed to load network info:', error);
        });
    },

    loadSettings: function() {
      const adminApiPath = AdminSystemPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/settings/all')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (!data.settings) return;

          document.getElementById('data-dir').textContent = (data.settings['server.data_dir'] && data.settings['server.data_dir'].value) || '-';
          document.getElementById('config-dir').textContent = (data.settings['server.config_dir'] && data.settings['server.config_dir'].value) || '-';
          document.getElementById('log-dir').textContent = (data.settings['server.log_dir'] && data.settings['server.log_dir'].value) || '-';
          document.getElementById('timezone').textContent = (data.settings['server.timezone'] && data.settings['server.timezone'].value) || '-';
        })
        .catch(function(error) {
          console.error('Failed to load settings:', error);
        });
    },

    triggerGC: function() {
      fetch('/debug/gc', { method: 'POST' })
        .then(function(response) {
          if (!response.ok) throw new Error('GC trigger failed');

          AdminSystemPage.showActionResult('✅ Garbage collection completed', 'success');
          setTimeout(AdminSystemPage.loadResourceUsage, 1000);
        })
        .catch(function(error) {
          AdminSystemPage.showActionResult('❌ Failed to trigger GC: ' + error.message, 'error');
        });
    },

    reloadConfig: function() {
      const adminApiPath = AdminSystemPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/reload', { method: 'POST' })
        .then(function(response) {
          return response.json().then(function(data) {
            if (!response.ok) throw new Error((data.error && data.error.message) || 'Reload failed');

            AdminSystemPage.showActionResult('✅ Configuration reloaded successfully', 'success');
          });
        })
        .catch(function(error) {
          AdminSystemPage.showActionResult('❌ ' + error.message, 'error');
        });
    },

    viewAllRoutes: function() {
      window.open('/debug/routes', '_blank');
    },

    exportSystemInfo: function() {
      const adminApiPath = AdminSystemPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/stats')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
          const url = URL.createObjectURL(blob);
          const a = document.createElement('a');
          a.href = url;
          a.download = 'system-info-' + new Date().toISOString().split('T')[0] + '.json';
          document.body.appendChild(a);
          a.click();
          document.body.removeChild(a);
          URL.revokeObjectURL(url);
          AdminSystemPage.showActionResult('✅ System info exported', 'success');
        })
        .catch(function(error) {
          AdminSystemPage.showActionResult('❌ Export failed: ' + error.message, 'error');
        });
    },

    refreshSystemInfo: function() {
      AdminSystemPage.loadSystemInfo();
      AdminSystemPage.loadNetworkInfo();
      AdminSystemPage.loadSettings();
    },

    refreshResourceUsage: function() {
      AdminSystemPage.loadResourceUsage();
    },

    showActionResult: function(message, type) {
      const result = document.getElementById('action-result');
      result.className = 'alert alert-' + (type === 'success' ? 'success' : 'danger');
      result.textContent = message;
      result.classList.remove('d-none');

      setTimeout(function() {
        result.classList.add('d-none');
      }, 5000);
    },

    formatUptime: function(seconds) {
      const days = Math.floor(seconds / 86400);
      const hours = Math.floor((seconds % 86400) / 3600);
      const minutes = Math.floor((seconds % 3600) / 60);

      if (days > 0) {
        return days + 'd ' + hours + 'h ' + minutes + 'm';
      } else if (hours > 0) {
        return hours + 'h ' + minutes + 'm';
      } else {
        return minutes + 'm';
      }
    },

    formatBytes: function(bytes) {
      if (bytes === 0) return '0 B';
      const k = 1024;
      const sizes = ['B', 'KB', 'MB', 'GB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
    },

    init: function() {
      if (!document.getElementById('admin-system-data')) return;

      AdminSystemPage.loadSystemInfo();
      AdminSystemPage.loadResourceUsage();
      AdminSystemPage.loadNetworkInfo();
      AdminSystemPage.loadSettings();

      setInterval(AdminSystemPage.loadResourceUsage, 30000);
    }
  };

  const AdminTasksPage = {
    dataPayload: null,
    tasks: [],
    currentTab: 'timeline',

    escHtml: function(str) {
      return String(str)
        .replace(/&/g, '&amp;').replace(/</g, '&lt;')
        .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    },

    getData: function() {
      if (!AdminTasksPage.dataPayload) {
        AdminTasksPage.dataPayload = JSON.parse(document.getElementById('admin-tasks-data').textContent);
      }
      return AdminTasksPage.dataPayload;
    },

    loadTasks: function() {
      const adminApiPath = AdminTasksPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/tasks')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          AdminTasksPage.tasks = data.tasks || [];

          AdminTasksPage.updateStats();
          AdminTasksPage.renderTaskList();
          AdminTasksPage.renderTimeline();
        })
        .catch(function(error) {
          AdminTasksPage.showError('Failed to load tasks: ' + error.message);
        });
    },

    updateStats: function() {
      const tasks = AdminTasksPage.tasks;
      const enabled = tasks.filter(function(t) { return t.enabled; }).length;
      const running = tasks.filter(function(t) { return t.running; }).length;
      const total = tasks.length;

      document.getElementById('totalTasks').textContent = total;
      document.getElementById('enabledTasks').textContent = enabled;
      document.getElementById('runningTasks').textContent = running;

      const successRate = 95;
      document.getElementById('successRate').textContent = successRate + '%';
    },

    renderTaskList: function() {
      const container = document.getElementById('taskList');
      const tasks = AdminTasksPage.tasks;

      if (tasks.length === 0) {
        container.innerHTML = '<div class="text-center text-muted p-3">No tasks configured</div>';
        return;
      }

      container.innerHTML = tasks.map(function(task) {
        const statusClasses = 'task-status ' + (task.enabled ? 'enabled' : 'disabled') + ' ' + (task.running ? 'running' : '');
        const toggleClass = task.enabled ? 'btn-danger' : 'btn-success';
        const toggleLabel = task.enabled ? '⏸️ Disable' : '▶️ Enable';
        const name = AdminTasksPage.escHtml(task.name);

        return '' +
          '<div class="task-item">' +
          '<div class="' + statusClasses + '"></div>' +
          '<div class="task-info">' +
          '<div class="task-name">' + name + '</div>' +
          '<div class="task-schedule">Every ' + AdminTasksPage.escHtml(AdminTasksPage.formatInterval(task.interval)) + '</div>' +
          '</div>' +
          '<div class="task-stats">' +
          '<span>Last: ' + AdminTasksPage.escHtml(task.lastRun || 'Never') + '</span>' +
          '<span>Next: ' + AdminTasksPage.escHtml(task.nextRun || '-') + '</span>' +
          '</div>' +
          '<div class="task-actions">' +
          '<button class="btn btn-sm btn-primary" data-action="trigger-task" data-name="' + name + '">▶️ Run</button>' +
          '<button class="btn btn-sm ' + toggleClass + '" data-action="toggle-task" data-name="' + name + '" data-enabled="' + task.enabled + '">' + toggleLabel + '</button>' +
          '</div>' +
          '</div>';
      }).join('');
    },

    renderTimeline: function() {
      const grid = document.getElementById('timelineGrid');
      const tasksContainer = document.getElementById('timelineTasks');

      let gridHTML = '';
      for (let i = 0; i < 24; i++) {
        gridHTML += '<div class="timeline-hour"><span class="timeline-label">' + i + ':00</span></div>';
      }
      grid.innerHTML = gridHTML;

      tasksContainer.innerHTML = AdminTasksPage.tasks.map(function(task, index) {
        const hourPos = Math.min(100, Math.max(0, Math.round(AdminTasksPage.getHourPosition(task.interval))));
        const topBucket = index % 3;
        const runningClass = task.running ? ' running' : '';
        const name = AdminTasksPage.escHtml(task.name);

        return '<div class="timeline-task' + runningClass + ' timeline-left-' + hourPos + ' timeline-top-' + topBucket + '" title="' + name + '">' + name + '</div>';
      }).join('');
    },

    getHourPosition: function(interval) {
      const hours = AdminTasksPage.parseInterval(interval);
      return (hours / 24) * 100;
    },

    parseInterval: function(interval) {
      if (interval.includes('h')) return parseInt(interval, 10);
      if (interval.includes('m')) return parseInt(interval, 10) / 60;
      if (interval.includes('s')) return parseInt(interval, 10) / 3600;
      return 1;
    },

    formatInterval: function(interval) {
      return interval || '1 hour';
    },

    loadHistory: function() {
      const adminApiPath = AdminTasksPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/tasks/history')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          const tbody = document.getElementById('historyTable').querySelector('tbody');
          tbody.innerHTML = data.history.map(function(h) {
            const statusClass = h.status === 'success' ? 'success' : 'danger';
            return '' +
              '<tr>' +
              '<td>' + AdminTasksPage.escHtml(h.task) + '</td>' +
              '<td><span class="badge badge-' + statusClass + '">' + AdminTasksPage.escHtml(h.status) + '</span></td>' +
              '<td>' + new Date(h.startedAt).toLocaleString() + '</td>' +
              '<td>' + AdminTasksPage.escHtml(h.duration) + '</td>' +
              '<td>' + AdminTasksPage.escHtml(h.result || '-') + '</td>' +
              '</tr>';
          }).join('');
        })
        .catch(function(error) {
          AdminTasksPage.showError('Failed to load history: ' + error.message);
        });
    },

    renderDependencyGraph: function() {
      const graph = document.getElementById('dependencyGraph');
      graph.innerHTML =
        '<div class="text-center text-muted p-4">' +
        '<p>Task Dependency Graph</p>' +
        '<p class="mt-2 text-sm">Most tasks run independently. Complex dependencies can be configured via API.</p>' +
        '</div>';
    },

    triggerTask: function(name) {
      showConfirm('Manually trigger task: ' + name + '?', 'Trigger Task').then(function(confirmed) {
        if (!confirmed) return;

        const adminApiPath = AdminTasksPage.getData().adminApiPath;
        fetch(adminApiPath + '/server/tasks/' + name + '/trigger', { method: 'POST' })
          .then(function(response) {
            if (!response.ok) throw new Error('Failed to trigger task');

            AdminTasksPage.showSuccess('Task "' + name + '" triggered successfully!');
            setTimeout(AdminTasksPage.loadTasks, 1000);
          })
          .catch(function(error) {
            AdminTasksPage.showError('Failed to trigger task: ' + error.message);
          });
      });
    },

    toggleTask: function(name, currentlyEnabled) {
      const action = currentlyEnabled ? 'disable' : 'enable';
      const adminApiPath = AdminTasksPage.getData().adminApiPath;

      fetch(adminApiPath + '/server/tasks/' + name + '/' + action, { method: 'POST' })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to ' + action + ' task');

          AdminTasksPage.showSuccess('Task "' + name + '" ' + action + 'd successfully!');
          AdminTasksPage.loadTasks();
        })
        .catch(function(error) {
          AdminTasksPage.showError('Failed to ' + action + ' task: ' + error.message);
        });
    },

    showSuccess: function(msg) {
      const alert = document.getElementById('successAlert');
      alert.textContent = msg;
      alert.classList.add('show');
      setTimeout(function() { alert.classList.remove('show'); }, 5000);
    },

    showError: function(msg) {
      const alert = document.getElementById('errorAlert');
      alert.textContent = msg;
      alert.classList.add('show');
      setTimeout(function() { alert.classList.remove('show'); }, 5000);
    },

    init: function() {
      if (!document.getElementById('admin-tasks-data')) return;

      document.querySelectorAll('.tab').forEach(function(tab) {
        tab.addEventListener('click', function() {
          document.querySelectorAll('.tab').forEach(function(t) { t.classList.remove('active'); });
          document.querySelectorAll('.tab-content').forEach(function(c) { c.classList.remove('active'); });

          tab.classList.add('active');
          document.getElementById(tab.dataset.tab).classList.add('active');
          AdminTasksPage.currentTab = tab.dataset.tab;

          if (AdminTasksPage.currentTab === 'history') AdminTasksPage.loadHistory();
          if (AdminTasksPage.currentTab === 'dependencies') AdminTasksPage.renderDependencyGraph();
        });
      });

      AdminTasksPage.loadTasks();
      setInterval(AdminTasksPage.loadTasks, 10000);
    }
  };

  const AddLocationPage = {
    dataPayload: null,
    searchTimeout: null,
    selectedLocation: null,
    searchResults: [],

    getData: function() {
      if (!AddLocationPage.dataPayload) {
        const el = document.getElementById('add-location-data');
        AddLocationPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '' };
      }
      return AddLocationPage.dataPayload;
    },

    escHtml: function(str) {
      const div = document.createElement('div');
      div.textContent = str;
      return div.innerHTML;
    },

    switchTab: function(tabName, btn) {
      document.querySelectorAll('#add-location-tabs .tab-content').forEach(function(tab) {
        tab.classList.remove('active');
      });
      document.querySelectorAll('#add-location-tabs .tab').forEach(function(tabBtn) {
        tabBtn.classList.remove('active');
      });
      document.getElementById(tabName + '-tab').classList.add('active');
      if (btn) btn.classList.add('active');
      AddLocationPage.resetForm();
    },

    searchCities: function(query) {
      clearTimeout(AddLocationPage.searchTimeout);

      if (query.length < 2) {
        document.getElementById('searchResults').classList.remove('active');
        return;
      }

      AddLocationPage.searchTimeout = setTimeout(function() {
        fetch(AddLocationPage.getData().apiPath + '/locations/search?q=' + encodeURIComponent(query))
          .then(function(response) { return response.json(); })
          .then(function(results) { AddLocationPage.displaySearchResults(results); })
          .catch(function(error) { console.error('Search failed:', error); });
      }, 300);
    },

    displaySearchResults: function(results) {
      const container = document.getElementById('searchResults');
      AddLocationPage.searchResults = results || [];

      if (!results || results.length === 0) {
        container.innerHTML = '<div class="result-item">No results found</div>';
        container.classList.add('active');
        return;
      }

      container.innerHTML = results.map(function(result, index) {
        return '<div class="result-item" data-action="add-location-select-result" data-index="' + index + '">' +
          '<div class="result-city">' + AddLocationPage.escHtml(result.name) + '</div>' +
          '<div class="result-details">' +
          (result.admin1 ? AddLocationPage.escHtml(result.admin1) + ', ' : '') + AddLocationPage.escHtml(result.country) +
          '<span class="add-location-result-opacity">• ' + result.latitude.toFixed(4) + ', ' + result.longitude.toFixed(4) + '</span>' +
          '</div>' +
          '</div>';
      }).join('');

      container.classList.add('active');
    },

    selectSearchResult: function(index) {
      const result = AddLocationPage.searchResults[index];
      if (result) AddLocationPage.selectLocation(result);
    },

    selectLocation: function(location) {
      AddLocationPage.selectedLocation = location;
      document.getElementById('searchResults').classList.remove('active');

      document.getElementById('name').value = location.name;
      document.getElementById('displayCity').value = location.name + ', ' + (location.admin1 ? location.admin1 + ', ' : '') + location.country;
      document.getElementById('displayCoords').value = location.latitude.toFixed(6) + ', ' + location.longitude.toFixed(6);
      document.getElementById('latitude').value = location.latitude;
      document.getElementById('longitude').value = location.longitude;
      document.getElementById('timezone').value = location.timezone || '';

      document.getElementById('locationForm').classList.add('location-form-visible');
      AddLocationPage.showMessage('Location selected! Adjust the name if needed and click Save.', 'success');
    },

    lookupZipCode: function() {
      const zipCode = document.getElementById('zipCode').value.trim();

      if (!zipCode) {
        AddLocationPage.showMessage('Please enter a ZIP or postal code', 'error');
        return;
      }

      Loading.show('#zip-tab');

      fetch(AddLocationPage.getData().apiPath + '/locations/lookup/zip/' + encodeURIComponent(zipCode))
        .then(function(response) {
          return response.json().then(function(location) { return { ok: response.ok, location: location }; });
        })
        .then(function(result) {
          if (result.ok) {
            AddLocationPage.selectLocation(result.location);
          } else {
            AddLocationPage.showMessage(result.location.error || 'ZIP code not found', 'error');
          }
        })
        .catch(function() {
          AddLocationPage.showMessage('Failed to look up ZIP code', 'error');
        })
        .finally(function() {
          Loading.hide('#zip-tab');
        });
    },

    lookupCoordinates: function() {
      const lat = parseFloat(document.getElementById('manualLat').value);
      const lon = parseFloat(document.getElementById('manualLon').value);

      if (isNaN(lat) || isNaN(lon)) {
        AddLocationPage.showMessage('Please enter valid coordinates', 'error');
        return;
      }

      Loading.show('#coords-tab');

      fetch(AddLocationPage.getData().apiPath + '/locations/lookup/coords?lat=' + lat + '&lon=' + lon)
        .then(function(response) {
          return response.json().then(function(location) { return { ok: response.ok, location: location }; });
        })
        .then(function(result) {
          if (result.ok) {
            AddLocationPage.selectLocation(result.location);
          } else {
            AddLocationPage.showMessage(result.location.error || 'Location not found', 'error');
          }
        })
        .catch(function() {
          AddLocationPage.showMessage('Failed to look up coordinates', 'error');
        })
        .finally(function() {
          Loading.hide('#coords-tab');
        });
    },

    useCurrentLocation: function() {
      if (!navigator.geolocation) {
        AddLocationPage.showMessage('Geolocation is not supported by your browser', 'error');
        return;
      }

      AddLocationPage.showMessage('Getting your location...', 'info');

      navigator.geolocation.getCurrentPosition(
        function(position) {
          const lat = position.coords.latitude;
          const lon = position.coords.longitude;

          fetch(AddLocationPage.getData().apiPath + '/locations/lookup/coords?lat=' + lat + '&lon=' + lon)
            .then(function(response) {
              return response.json().then(function(location) { return { ok: response.ok, location: location }; });
            })
            .then(function(result) {
              if (result.ok) {
                AddLocationPage.selectLocation(result.location);
              } else {
                AddLocationPage.showMessage('Could not identify your location', 'error');
              }
            })
            .catch(function() {
              AddLocationPage.showMessage('Failed to look up your location', 'error');
            });
        },
        function(error) {
          AddLocationPage.showMessage('Unable to get your location: ' + error.message, 'error');
        }
      );
    },

    resetForm: function() {
      document.getElementById('locationForm').classList.remove('location-form-visible');
      document.getElementById('searchResults').classList.remove('active');
      document.getElementById('citySearch').value = '';
      document.getElementById('zipCode').value = '';
      document.getElementById('manualLat').value = '';
      document.getElementById('manualLon').value = '';
      AddLocationPage.selectedLocation = null;
      AddLocationPage.clearMessage();
    },

    handleSubmit: function(e) {
      e.preventDefault();

      const data = {
        name: document.getElementById('name').value,
        latitude: parseFloat(document.getElementById('latitude').value),
        longitude: parseFloat(document.getElementById('longitude').value),
        timezone: document.getElementById('timezone').value || ''
      };

      fetch(AddLocationPage.getData().apiPath + '/users/locations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
      })
        .then(function(response) {
          if (response.ok) {
            Toast.success('Location saved successfully!');
            setTimeout(function() {
              window.location.href = '/users/dashboard';
            }, 1500);
            return;
          }
          return response.json().then(function(error) {
            AddLocationPage.showMessage(error.error || 'Failed to save location', 'error');
          });
        })
        .catch(function() {
          AddLocationPage.showMessage('Error saving location', 'error');
        });
    },

    showMessage: function(message, type) {
      AddLocationPage.clearMessage();
      Alert.create(message, {
        type: type,
        container: '#message'
      });
    },

    clearMessage: function() {
      const msgEl = document.getElementById('message');
      if (msgEl) {
        msgEl.innerHTML = '';
      }
    },

    init: function() {
      const form = document.getElementById('locationForm');
      if (!form) return;

      const citySearch = document.getElementById('citySearch');
      if (citySearch) {
        citySearch.addEventListener('input', function() {
          AddLocationPage.searchCities(this.value);
        });
      }

      form.addEventListener('submit', function(e) {
        AddLocationPage.handleSubmit(e);
      });
    }
  };

  const ContactPage = {
    dataPayload: null,

    getData: function() {
      if (!ContactPage.dataPayload) {
        const el = document.getElementById('contact-page-data');
        ContactPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '' };
      }
      return ContactPage.dataPayload;
    },

    handleSubmit: function(event) {
      event.preventDefault();

      const form = event.target;
      const submitBtn = document.getElementById('contact-submit');
      const messageDiv = document.getElementById('contact-message');

      submitBtn.disabled = true;
      submitBtn.textContent = '📤 Sending...';

      const formData = {
        name: form.name.value,
        email: form.email.value,
        subject: form.subject.value,
        message: form.message.value
      };

      fetch(ContactPage.getData().apiPath + '/server/contact', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(formData)
      })
        .then(function(response) {
          if (response.ok) {
            Alert.create('Message sent successfully! We\'ll get back to you soon.', {
              type: 'success',
              container: '#contact-message'
            });
            form.reset();
            Toast.success('Message sent!');
            return;
          }
          return response.json().then(function(error) {
            Alert.create(error.error || 'Failed to send message. Please try again.', {
              type: 'error',
              container: '#contact-message'
            });
          });
        })
        .catch(function() {
          Alert.create('Network error. Please check your connection and try again.', {
            type: 'error',
            container: messageDiv ? '#contact-message' : undefined
          });
        })
        .finally(function() {
          submitBtn.disabled = false;
          submitBtn.textContent = '📧 Send Message';
        });
    },

    init: function() {
      const form = document.getElementById('contact-form');
      if (!form) return;

      form.addEventListener('submit', function(e) {
        ContactPage.handleSubmit(e);
      });
    }
  };

  const EarthquakeDetailPage = {
    getData: function() {
      const el = document.getElementById('earthquake-detail-data');
      return el ? JSON.parse(el.textContent) : null;
    },

    init: function() {
      const mapEl = document.getElementById('map');
      if (!mapEl || typeof L === 'undefined') return;

      const data = EarthquakeDetailPage.getData();
      if (!data) return;

      const map = L.map('map').setView([data.latitude, data.longitude], 8);

      L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '© OpenStreetMap contributors',
        maxZoom: 18
      }).addTo(map);

      const size = Math.max(10, data.magnitude * 5);
      const color = data.magnitude < 2 ? '#50fa7b' :
        data.magnitude < 4 ? '#f1fa8c' :
        data.magnitude < 5 ? '#ffb86c' :
        data.magnitude < 6 ? '#ff5555' : '#bd93f9';

      const marker = L.circleMarker([data.latitude, data.longitude], {
        radius: size,
        fillColor: color,
        color: '#f8f8f2',
        weight: 3,
        opacity: 1,
        fillOpacity: 0.8
      }).addTo(map);

      const popup = document.createElement('div');
      popup.className = 'map-popup';

      const title = document.createElement('strong');
      title.className = 'map-popup__title';
      title.textContent = 'M ' + data.magnitude.toFixed(1);
      popup.appendChild(title);

      const place = document.createElement('div');
      place.className = 'map-popup__place';
      place.textContent = data.place;
      popup.appendChild(place);

      const time = document.createElement('div');
      time.className = 'map-popup__time';
      time.textContent = data.time;
      popup.appendChild(time);

      const depth = document.createElement('div');
      depth.className = 'map-popup__meta';
      depth.textContent = 'Depth: ' + data.depth.toFixed(1) + 'km';
      popup.appendChild(depth);

      if (data.tsunami === 1) {
        const warning = document.createElement('div');
        warning.className = 'map-popup__warning';
        warning.textContent = '🌊 Tsunami Warning';
        popup.appendChild(warning);
      }

      marker.bindPopup(popup).openPopup();
    }
  };

  const EarthquakePage = {
    map: null,

    getData: function() {
      const el = document.getElementById('earthquake-page-data');
      return el ? JSON.parse(el.textContent) : null;
    },

    buildPopup: function(eq) {
      const popup = document.createElement('div');
      popup.className = 'map-popup';

      const title = document.createElement('strong');
      title.className = 'map-popup__title';
      title.textContent = 'M ' + eq.mag.toFixed(1);
      popup.appendChild(title);

      const place = document.createElement('div');
      place.className = 'map-popup__place';
      place.textContent = eq.place;
      popup.appendChild(place);

      const time = document.createElement('div');
      time.className = 'map-popup__time';
      time.textContent = eq.time;
      popup.appendChild(time);

      const depth = document.createElement('div');
      depth.className = 'map-popup__meta';
      depth.textContent = 'Depth: ' + eq.depth.toFixed(1) + 'km';
      popup.appendChild(depth);

      if (eq.tsunami) {
        const warning = document.createElement('div');
        warning.className = 'map-popup__warning';
        warning.textContent = '🌊 Tsunami Warning';
        popup.appendChild(warning);
      }

      const link = document.createElement('a');
      link.href = eq.url;
      link.target = '_blank';
      link.className = 'map-popup__link';
      link.textContent = 'View Details';
      popup.appendChild(link);

      return popup;
    },

    autoFillClosestLocation: function(data) {
      if (data.hasLocation || data.earthquakes.length === 0) return;

      let closest = data.earthquakes[0];
      let minDist = Number.MAX_VALUE;

      data.earthquakes.forEach(function(eq) {
        const dist = Math.sqrt(Math.pow(eq.lat - data.centerLat, 2) + Math.pow(eq.lng - data.centerLon, 2));
        if (dist < minDist) {
          minDist = dist;
          closest = eq;
        }
      });

      let locationName = closest.place;
      const match = locationName.match(/(?:of|near)\s+([^,]+(?:,\s*[^,]+)?)/i);
      if (match) {
        locationName = match[1].trim();
      }

      const input = document.getElementById('eq-location-input');
      if (input) {
        input.placeholder = '🔍 ' + locationName + ' (closest)';
      }
    },

    focusEarthquake: function(lat, lng) {
      if (EarthquakePage.map) {
        EarthquakePage.map.setView([lat, lng], 8);
      }
    },

    handleItemClick: function(item) {
      EarthquakePage.focusEarthquake(parseFloat(item.dataset.lat), parseFloat(item.dataset.lng));
      const details = item.querySelector('.eq-details');
      if (details) {
        details.classList.toggle('visible');
      }
    },

    init: function() {
      const mapEl = document.getElementById('map');
      if (!mapEl || typeof L === 'undefined') return;

      const data = EarthquakePage.getData();
      if (!data) return;

      EarthquakePage.autoFillClosestLocation(data);

      const map = L.map('map').setView([data.centerLat, data.centerLon], data.hasLocation ? 6 : 2);
      EarthquakePage.map = map;

      L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '© OpenStreetMap contributors',
        maxZoom: 18
      }).addTo(map);

      const markers = [];
      data.earthquakes.forEach(function(eq) {
        const size = Math.max(5, eq.mag * 4);
        const color = eq.mag < 2 ? '#50fa7b' :
          eq.mag < 4 ? '#f1fa8c' :
          eq.mag < 5 ? '#ffb86c' :
          eq.mag < 6 ? '#ff5555' : '#bd93f9';

        const marker = L.circleMarker([eq.lat, eq.lng], {
          radius: size,
          fillColor: color,
          color: '#f8f8f2',
          weight: 2,
          opacity: 0.8,
          fillOpacity: 0.6
        }).addTo(map);

        marker.bindPopup(EarthquakePage.buildPopup(eq));
        markers.push(marker);
      });

      if (data.hasLocation && markers.length > 0) {
        const group = L.featureGroup(markers);
        map.fitBounds(group.getBounds().pad(0.1));
      }

      setTimeout(function() {
        location.reload();
      }, 300000);
    }
  };

  const EditLocationPage = {
    dataPayload: null,

    getData: function() {
      if (!EditLocationPage.dataPayload) {
        const el = document.getElementById('edit-location-data');
        EditLocationPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '', locationId: 0 };
      }
      return EditLocationPage.dataPayload;
    },

    showMessage: function(text, type) {
      const messageDiv = document.getElementById('message');
      if (!messageDiv) return;
      messageDiv.className = type === 'error' ? 'error-message edit-error-message' : 'success-message edit-error-message';
      messageDiv.textContent = text;
      messageDiv.setAttribute('role', 'alert');
      messageDiv.setAttribute('aria-live', type === 'error' ? 'assertive' : 'polite');
    },

    handleSubmit: function(e) {
      e.preventDefault();

      const data = EditLocationPage.getData();
      const body = {
        name: document.getElementById('name').value,
        latitude: parseFloat(document.getElementById('latitude').value),
        longitude: parseFloat(document.getElementById('longitude').value),
        timezone: document.getElementById('timezone').value || '',
        alerts_enabled: document.getElementById('alerts_enabled').checked
      };

      fetch(data.apiPath + '/users/locations/' + data.locationId, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      }).then(function(response) {
        if (response.ok) {
          EditLocationPage.showMessage('Location updated successfully! Redirecting...', 'success');
          setTimeout(function() {
            window.location.href = '/users/dashboard';
          }, 1500);
        } else {
          response.json().then(function(error) {
            EditLocationPage.showMessage(error.error || 'Failed to update location', 'error');
          });
        }
      }).catch(function(error) {
        EditLocationPage.showMessage('Network error: ' + error.message, 'error');
      });
    },

    handleDelete: function() {
      if (!showConfirm('Are you sure you want to delete this location? This cannot be undone.')) {
        return;
      }

      const data = EditLocationPage.getData();

      fetch(data.apiPath + '/users/locations/' + data.locationId, {
        method: 'DELETE'
      }).then(function(response) {
        if (response.ok) {
          EditLocationPage.showMessage('Location deleted successfully! Redirecting...', 'success');
          setTimeout(function() {
            window.location.href = '/users/dashboard';
          }, 1500);
        } else {
          response.json().then(function(error) {
            EditLocationPage.showMessage(error.error || 'Failed to delete location', 'error');
          });
        }
      }).catch(function(error) {
        EditLocationPage.showMessage('Network error: ' + error.message, 'error');
      });
    },

    init: function() {
      const form = document.getElementById('locationForm');
      if (!form) return;
      form.addEventListener('submit', EditLocationPage.handleSubmit);
    }
  };

  const SecurityPage = {
    dataPayload: null,
    setup2FASecret: '',
    recoveryKeysList: [],

    getData: function() {
      if (!SecurityPage.dataPayload) {
        const el = document.getElementById('security-data');
        SecurityPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '' };
      }
      return SecurityPage.dataPayload;
    },

    bufferToBase64url: function(buffer) {
      const bytes = new Uint8Array(buffer);
      let binary = '';
      for (const byte of bytes) binary += String.fromCharCode(byte);
      return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
    },

    base64urlToBuffer: function(base64url) {
      const padded = (base64url + '==='.slice((base64url.length + 3) % 4)).replace(/-/g, '+').replace(/_/g, '/');
      const binary = atob(padded);
      const bytes = new Uint8Array(binary.length);
      for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
      return bytes.buffer;
    },

    normalizeCreationOptions: function(options) {
      const publicKey = options.publicKey || options;
      publicKey.challenge = SecurityPage.base64urlToBuffer(publicKey.challenge);
      publicKey.user.id = SecurityPage.base64urlToBuffer(publicKey.user.id);
      if (Array.isArray(publicKey.excludeCredentials)) {
        publicKey.excludeCredentials = publicKey.excludeCredentials.map((credential) => ({
          ...credential,
          id: SecurityPage.base64urlToBuffer(credential.id),
        }));
      }
      return publicKey;
    },

    serializeCredential: function(credential) {
      return {
        id: credential.id,
        rawId: SecurityPage.bufferToBase64url(credential.rawId),
        type: credential.type,
        response: {
          attestationObject: SecurityPage.bufferToBase64url(credential.response.attestationObject),
          clientDataJSON: SecurityPage.bufferToBase64url(credential.response.clientDataJSON),
          transports: credential.response.getTransports ? credential.response.getTransports() : [],
          publicKeyAlgorithm: credential.response.getPublicKeyAlgorithm ? credential.response.getPublicKeyAlgorithm() : undefined,
          publicKey: credential.response.getPublicKey ? SecurityPage.bufferToBase64url(credential.response.getPublicKey()) : undefined,
          authenticatorData: credential.response.getAuthenticatorData ? SecurityPage.bufferToBase64url(credential.response.getAuthenticatorData()) : undefined,
        },
        authenticatorAttachment: credential.authenticatorAttachment || undefined,
      };
    },

    enable2FA: async function() {
      try {
        const response = await fetch(SecurityPage.getData().apiPath + '/users/security/2fa/setup', {
          method: 'GET'
        });

        if (!response.ok) {
          const error = await response.json();
          throw new Error(error.error || 'Failed to start 2FA setup');
        }

        const data = await response.json();
        SecurityPage.setup2FASecret = data.secret;

        document.getElementById('qrCodeContainer').innerHTML = `<img src="${data.qr_code}" alt="QR Code">`;
        document.getElementById('manualSecret').value = data.secret;

        document.getElementById('setup2FAModal').style.display = 'flex';
        document.getElementById('setupStep1').style.display = 'block';
        document.getElementById('setupStep2').style.display = 'none';
        document.getElementById('setupStep3').style.display = 'none';
      } catch (error) {
        Toast.error('Failed to setup 2FA: ' + error.message);
      }
    },

    nextSetupStep: function() {
      document.getElementById('setupStep1').style.display = 'none';
      document.getElementById('setupStep2').style.display = 'block';
      document.getElementById('verificationCode').focus();
    },

    prevSetupStep: function() {
      document.getElementById('setupStep2').style.display = 'none';
      document.getElementById('setupStep1').style.display = 'block';
    },

    registerPasskey: async function() {
      if (!window.PublicKeyCredential || !navigator.credentials || !navigator.credentials.create) {
        Toast.error('This browser does not support passkeys');
        return;
      }

      const name = document.getElementById('newPasskeyName').value.trim();
      const password = document.getElementById('newPasskeyPassword').value;
      if (!name || !password) {
        Toast.error('Passkey name and password are required');
        return;
      }

      try {
        const startResponse = await fetch(SecurityPage.getData().apiPath + '/users/security/passkeys', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, password }),
        });
        const startPayload = await startResponse.json();
        if (!startResponse.ok || !startPayload.ok) {
          throw new Error(startPayload.error || 'Failed to start passkey registration');
        }

        const credential = await navigator.credentials.create({
          publicKey: SecurityPage.normalizeCreationOptions(startPayload.options),
        });
        if (!credential) {
          throw new Error('Passkey registration was cancelled');
        }

        const finishResponse = await fetch(SecurityPage.getData().apiPath + '/users/security/passkeys', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(SecurityPage.serializeCredential(credential)),
        });
        const finishPayload = await finishResponse.json();
        if (!finishResponse.ok || !finishPayload.ok) {
          throw new Error(finishPayload.error || 'Failed to register passkey');
        }

        if (finishPayload.recovery_keys) {
          SecurityPage.recoveryKeysList = finishPayload.recovery_keys;
          document.getElementById('recoveryKeysContainer').innerHTML = SecurityPage.recoveryKeysList.map((key) => `<div class="recovery-key">${key}</div>`).join('');
          document.getElementById('setup2FAModal').style.display = 'flex';
          document.getElementById('setupStep1').style.display = 'none';
          document.getElementById('setupStep2').style.display = 'none';
          document.getElementById('setupStep3').style.display = 'block';
        } else {
          window.location.reload();
        }
      } catch (error) {
        Toast.error('Failed to register passkey: ' + error.message);
      }
    },

    deletePasskey: async function(passkeyID, name) {
      if (!confirm(`Delete passkey "${name}"?`)) {
        return;
      }

      try {
        const response = await fetch(SecurityPage.getData().apiPath + '/users/security/passkeys/' + passkeyID, {
          method: 'DELETE',
        });
        const payload = await response.json();
        if (!response.ok || !payload.ok) {
          throw new Error(payload.error || 'Failed to delete passkey');
        }
        window.location.reload();
      } catch (error) {
        Toast.error('Failed to delete passkey: ' + error.message);
      }
    },

    displayRecoveryKeys: function(keys) {
      const container = document.getElementById('recoveryKeysContainer');
      container.innerHTML = '<h4>Your Recovery Keys:</h4>' +
        keys.map(key => `<div class="recovery-key">${key}</div>`).join('');
    },

    downloadRecoveryKeys: function() {
      const text = 'Weather - Recovery Keys\n\n' +
        'These recovery keys can be used to access your account if you lose your authenticator device.\n' +
        'Each key can only be used once. Keep them in a safe place.\n\n' +
        SecurityPage.recoveryKeysList.join('\n');

      const blob = new Blob([text], { type: 'text/plain' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'recovery-keys.txt';
      a.click();
      URL.revokeObjectURL(url);
    },

    finishSetup: function() {
      SecurityPage.closeSetupModal();
      Toast.success('Two-factor authentication enabled successfully!');
      setTimeout(() => window.location.reload(), 1000);
    },

    closeSetupModal: function() {
      document.getElementById('setup2FAModal').style.display = 'none';
      document.getElementById('verificationCode').value = '';
      SecurityPage.setup2FASecret = '';
      SecurityPage.recoveryKeysList = [];
    },

    disable2FA: function() {
      document.getElementById('disable2FAModal').style.display = 'flex';
    },

    closeDisableModal: function() {
      document.getElementById('disable2FAModal').style.display = 'none';
      document.getElementById('disablePassword').value = '';
    },

    regenerateRecoveryKeys: async function() {
      const code = prompt('Enter your authenticator code to regenerate recovery keys:');
      if (!code) return;

      try {
        const response = await fetch(SecurityPage.getData().apiPath + '/users/security/2fa/recovery/regenerate', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ code: code })
        });

        if (!response.ok) {
          const error = await response.json();
          throw new Error(error.error || 'Failed to regenerate recovery keys');
        }

        const data = await response.json();
        SecurityPage.recoveryKeysList = data.recovery_keys;

        SecurityPage.displayRecoveryKeys(SecurityPage.recoveryKeysList);
        document.getElementById('setup2FAModal').style.display = 'flex';
        document.getElementById('setupStep1').style.display = 'none';
        document.getElementById('setupStep2').style.display = 'none';
        document.getElementById('setupStep3').style.display = 'block';
      } catch (error) {
        Toast.error('Failed to regenerate recovery keys: ' + error.message);
      }
    },

    init: function() {
      const verifyForm = document.getElementById('verify2FAForm');
      if (!verifyForm) return;

      verifyForm.addEventListener('submit', async function(e) {
        e.preventDefault();

        const code = document.getElementById('verificationCode').value;

        try {
          const response = await fetch(SecurityPage.getData().apiPath + '/users/security/2fa/enable', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              secret: SecurityPage.setup2FASecret,
              code: code
            })
          });

          if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'Invalid verification code');
          }

          const data = await response.json();
          SecurityPage.recoveryKeysList = data.recovery_keys;

          SecurityPage.displayRecoveryKeys(SecurityPage.recoveryKeysList);

          document.getElementById('setupStep2').style.display = 'none';
          document.getElementById('setupStep3').style.display = 'block';
        } catch (error) {
          Toast.error('Verification failed: ' + error.message);
        }
      });

      const disableForm = document.getElementById('disable2FAForm');
      if (disableForm) {
        disableForm.addEventListener('submit', async function(e) {
          e.preventDefault();

          const password = document.getElementById('disablePassword').value;

          try {
            const response = await fetch(SecurityPage.getData().apiPath + '/users/security/2fa/disable', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ password: password })
            });

            if (!response.ok) {
              const error = await response.json();
              throw new Error(error.error || 'Failed to disable 2FA');
            }

            Toast.success('Two-factor authentication disabled');
            setTimeout(() => window.location.reload(), 1000);
          } catch (error) {
            Toast.error('Failed to disable 2FA: ' + error.message);
          }
        });
      }
    }
  };

  const SettingsTokensPage = {
    dataPayload: null,

    getData: function() {
      if (!SettingsTokensPage.dataPayload) {
        const el = document.getElementById('settings-tokens-data');
        SettingsTokensPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '' };
      }
      return SettingsTokensPage.dataPayload;
    },

    showNewModal: function() {
      document.getElementById('newTokenModal').style.display = 'flex';
    },

    closeNewModal: function() {
      document.getElementById('newTokenModal').style.display = 'none';
      document.getElementById('newTokenForm').reset();
    },

    closeCreatedModal: function() {
      document.getElementById('tokenCreatedModal').style.display = 'none';
      window.location.reload();
    },

    copyCreatedToken: function() {
      const tokenInput = document.getElementById('createdToken');
      tokenInput.select();
      document.execCommand('copy');
      Toast.success('Token copied to clipboard');
    },

    revokeToken: async function(tokenId) {
      if (!await showConfirm('Are you sure you want to revoke this token? This action cannot be undone.', 'Revoke Token')) {
        return;
      }

      try {
        const response = await fetch(SettingsTokensPage.getData().apiPath + '/users/tokens/' + tokenId, {
          method: 'DELETE'
        });

        if (!response.ok) {
          const error = await response.json();
          throw new Error(error.error || 'Failed to revoke token');
        }

        Toast.success('Token revoked');
        document.querySelector(`[data-token-id="${tokenId}"]`).remove();
      } catch (error) {
        Toast.error('Failed to revoke token: ' + error.message);
      }
    },

    init: function() {
      const newTokenForm = document.getElementById('newTokenForm');
      if (!newTokenForm) return;

      newTokenForm.addEventListener('submit', async function(e) {
        e.preventDefault();

        const data = {
          name: document.getElementById('tokenName').value,
          scopes: document.getElementById('tokenScopes').value,
          expires_in: parseInt(document.getElementById('tokenExpires').value)
        };

        try {
          const response = await fetch(SettingsTokensPage.getData().apiPath + '/users/tokens', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
          });

          if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'Failed to create token');
          }

          const result = await response.json();

          SettingsTokensPage.closeNewModal();
          document.getElementById('createdToken').value = result.token;
          document.getElementById('tokenCreatedModal').style.display = 'flex';
        } catch (error) {
          Toast.error('Failed to create token: ' + error.message);
        }
      });
    }
  };

  const SettingsPage = {
    dataPayload: null,

    getData: function() {
      if (!SettingsPage.dataPayload) {
        const el = document.getElementById('settings-data');
        SettingsPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '' };
      }
      return SettingsPage.dataPayload;
    },

    resetForm: function() {
      document.getElementById('accountSettingsForm').reset();
    },

    init: function() {
      const form = document.getElementById('accountSettingsForm');
      if (!form) return;

      const bio = document.getElementById('bio');
      bio.addEventListener('input', function() {
        document.getElementById('bioCount').textContent = this.value.length;
      });

      form.addEventListener('submit', async function(e) {
        e.preventDefault();

        const data = {
          account: {
            display_name: document.getElementById('displayName').value,
            bio: document.getElementById('bio').value,
            location: document.getElementById('location').value,
            website: document.getElementById('website').value,
            timezone: document.getElementById('timezone').value,
            language: document.getElementById('language').value,
            date_format: document.querySelector('input[name="dateFormat"]:checked').value,
            time_format: document.querySelector('input[name="timeFormat"]:checked').value
          }
        };

        try {
          const response = await fetch(SettingsPage.getData().apiPath + '/users/settings', {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
          });

          if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'Failed to save settings');
          }

          Toast.success('Settings saved successfully!');
        } catch (error) {
          Toast.error('Failed to save settings: ' + error.message);
        }
      });
    }
  };

  const ProfilePage = {
    dataPayload: null,

    getData: function() {
      if (!ProfilePage.dataPayload) {
        const el = document.getElementById('profile-data');
        ProfilePage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '' };
      }
      return ProfilePage.dataPayload;
    },

    resetForm: function() {
      document.getElementById('profileForm').reset();
    },

    updateDurationLabel: function(type, value) {
      const label = type.charAt(0).toUpperCase() + type.slice(1);
      document.getElementById('durationLabel' + label).textContent = value + 's';
    },

    loadNotificationPreferences: async function() {
      try {
        const response = await fetch(ProfilePage.getData().apiPath + '/users/notifications/preferences');

        if (!response.ok) {
          throw new Error('Failed to load preferences');
        }

        const prefs = await response.json();

        document.getElementById('enableToast').checked = prefs.enable_toast !== false;
        document.getElementById('enableBanner').checked = prefs.enable_banner !== false;
        document.getElementById('enableCenter').checked = prefs.enable_center !== false;
        document.getElementById('enableSound').checked = prefs.enable_sound === true;

        const successDuration = prefs.toast_duration_success || 5;
        const infoDuration = prefs.toast_duration_info || 5;
        const warningDuration = prefs.toast_duration_warning || 10;

        document.getElementById('toastDurationSuccess').value = successDuration;
        document.getElementById('toastDurationInfo').value = infoDuration;
        document.getElementById('toastDurationWarning').value = warningDuration;

        ProfilePage.updateDurationLabel('success', successDuration);
        ProfilePage.updateDurationLabel('info', infoDuration);
        ProfilePage.updateDurationLabel('warning', warningDuration);
      } catch (error) {
        console.error('Failed to load notification preferences:', error);
      }
    },

    init: function() {
      const profileForm = document.getElementById('profileForm');
      if (!profileForm) return;

      profileForm.addEventListener('submit', async function(e) {
        e.preventDefault();

        const formData = {
          email: document.getElementById('email').value,
          display_name: document.getElementById('displayName').value,
          phone: document.getElementById('phone').value
        };

        try {
          const response = await fetch(ProfilePage.getData().apiPath + '/users/profile', {
            method: 'PUT',
            headers: {
              'Content-Type': 'application/json'
            },
            body: JSON.stringify(formData)
          });

          if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'Failed to update profile');
          }

          Toast.success('Profile updated successfully!');

          setTimeout(() => window.location.reload(), 1000);
        } catch (error) {
          Toast.error('Failed to update profile: ' + error.message);
        }
      });

      document.getElementById('passwordForm').addEventListener('submit', async function(e) {
        e.preventDefault();

        const currentPassword = document.getElementById('currentPassword').value;
        const newPassword = document.getElementById('newPassword').value;
        const confirmPassword = document.getElementById('confirmPassword').value;

        if (newPassword !== confirmPassword) {
          Toast.error('Passwords do not match');
          return;
        }

        if (newPassword.length < 8) {
          Toast.error('Password must be at least 8 characters');
          return;
        }

        try {
          const response = await fetch(ProfilePage.getData().apiPath + '/users/password', {
            method: 'PUT',
            headers: {
              'Content-Type': 'application/json'
            },
            body: JSON.stringify({
              current_password: currentPassword,
              new_password: newPassword
            })
          });

          if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'Failed to change password');
          }

          Toast.success('Password changed successfully!');

          document.getElementById('currentPassword').value = '';
          document.getElementById('newPassword').value = '';
          document.getElementById('confirmPassword').value = '';
        } catch (error) {
          Toast.error('Failed to change password: ' + error.message);
        }
      });

      document.getElementById('toastDurationSuccess').addEventListener('input', function() {
        ProfilePage.updateDurationLabel('success', this.value);
      });
      document.getElementById('toastDurationInfo').addEventListener('input', function() {
        ProfilePage.updateDurationLabel('info', this.value);
      });
      document.getElementById('toastDurationWarning').addEventListener('input', function() {
        ProfilePage.updateDurationLabel('warning', this.value);
      });

      document.getElementById('notificationPreferencesForm').addEventListener('submit', async function(e) {
        e.preventDefault();

        const preferences = {
          enable_toast: document.getElementById('enableToast').checked,
          enable_banner: document.getElementById('enableBanner').checked,
          enable_center: document.getElementById('enableCenter').checked,
          enable_sound: document.getElementById('enableSound').checked,
          toast_duration_success: parseInt(document.getElementById('toastDurationSuccess').value),
          toast_duration_info: parseInt(document.getElementById('toastDurationInfo').value),
          toast_duration_warning: parseInt(document.getElementById('toastDurationWarning').value)
        };

        try {
          const response = await fetch(ProfilePage.getData().apiPath + '/users/notifications/preferences', {
            method: 'PATCH',
            headers: {
              'Content-Type': 'application/json'
            },
            body: JSON.stringify(preferences)
          });

          if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'Failed to update preferences');
          }

          if (typeof notificationManager !== 'undefined') {
            notificationManager.preferences = preferences;
            notificationManager.showToast({
              type: 'success',
              title: 'Preferences Saved',
              message: 'Your notification preferences have been updated successfully.',
              display: 'toast'
            });
          } else {
            Toast.success('Notification preferences updated successfully!');
          }
        } catch (error) {
          if (typeof notificationManager !== 'undefined') {
            notificationManager.showToast({
              type: 'error',
              title: 'Save Failed',
              message: 'Failed to update preferences: ' + error.message,
              display: 'toast'
            });
          } else {
            Toast.error('Failed to update preferences: ' + error.message);
          }
        }
      });

      ProfilePage.loadNotificationPreferences();
    }
  };

  const NotificationsListPage = {
    dataPayload: null,
    currentFilter: 'all',

    getData: function() {
      if (!NotificationsListPage.dataPayload) {
        const el = document.getElementById('notifications-list-data');
        NotificationsListPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '' };
      }
      return NotificationsListPage.dataPayload;
    },

    load: async function() {
      const unreadParam = NotificationsListPage.currentFilter === 'unread' ? '?unread=true' : '';
      const response = await fetch(`${NotificationsListPage.getData().apiPath}/users/notifications${unreadParam}`);
      const data = await response.json();

      const container = document.getElementById('notifications-list');

      if (!data.notifications || data.notifications.length === 0) {
        container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-state__icon">📭</div>
                    <h3 class="empty-state__title">No notifications</h3>
                    <p class="empty-state__message">You're all caught up!</p>
                </div>
            `;
        return;
      }

      container.innerHTML = data.notifications.map(n => {
        const time = Utils.escapeAttr(new Date(n.created_at).toLocaleString());
        const title = Utils.escapeAttr(n.title);
        const type = Utils.escapeAttr(n.type || 'info');
        const message = Utils.escapeAttr(n.message);
        const id = Utils.escapeAttr(n.id);
        const unreadClass = n.read ? '' : 'unread';
        // Only render the link if it is a same-origin absolute path or an
        // http(s) URL, to prevent a javascript: URI or markup injection.
        const safeLink = typeof n.link === 'string' && (/^https?:\/\//i.test(n.link) || (n.link.charAt(0) === '/' && n.link.charAt(1) !== '/' && n.link.charAt(1) !== '\\')) ? Utils.escapeAttr(n.link) : '';

        return `
                <div class="notification-item ${unreadClass}" data-id="${id}">
                    <div class="notification-header">
                        <div>
                            <span class="notification-title">${title}</span>
                            <span class="notification-type ${type}">${type}</span>
                        </div>
                        <span class="notification-time">${time}</span>
                    </div>
                    <div class="notification-message">${message}</div>
                    <div class="notification-actions">
                        ${!n.read ? `<button class="btn btn-outline btn-sm" data-action="notifications-list-mark-read" data-id="${id}">✓ Mark as Read</button>` : ''}
                        ${safeLink ? `<a href="${safeLink}" class="btn btn-outline btn-sm">View</a>` : ''}
                        <button class="btn btn-danger btn-sm" data-action="notifications-list-delete" data-id="${id}">🗑️ Delete</button>
                    </div>
                </div>
            `;
      }).join('');
    },

    markAsRead: async function(id) {
      await fetch(`${NotificationsListPage.getData().apiPath}/users/notifications/${id}/read`, { method: 'PATCH' });
      NotificationsListPage.load();
    },

    deleteNotification: async function(id) {
      if (!await showConfirm('Delete this notification?')) return;
      await fetch(`${NotificationsListPage.getData().apiPath}/users/notifications/${id}`, { method: 'DELETE' });
      NotificationsListPage.load();
    },

    init: function() {
      const list = document.getElementById('notifications-list');
      if (!list) return;

      document.getElementById('mark-all-read').addEventListener('click', async () => {
        await fetch(NotificationsListPage.getData().apiPath + '/users/notifications/read', { method: 'PATCH' });
        NotificationsListPage.load();
      });

      document.querySelectorAll('.filter-tab').forEach(tab => {
        tab.addEventListener('click', () => {
          document.querySelectorAll('.filter-tab').forEach(t => t.classList.remove('active'));
          tab.classList.add('active');
          NotificationsListPage.currentFilter = tab.dataset.filter;
          NotificationsListPage.load();
        });
      });

      NotificationsListPage.load();
      setInterval(NotificationsListPage.load, 30000);

      if (typeof Notifications !== 'undefined') {
        Notifications.startPolling(30000);
      }
    }
  };

  const AdminWebPage = {
    dataPayload: null,

    getData: function() {
      if (!AdminWebPage.dataPayload) {
        const el = document.getElementById('admin-web-data');
        AdminWebPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '', adminApiPath: '' };
      }
      return AdminWebPage.dataPayload;
    },

    loadSettings: function() {
      const adminApiPath = AdminWebPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/settings/all')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (!data.settings) return;
          for (const key in data.settings) {
            const input = document.querySelector('[name="' + key + '"]');
            if (input) input.value = data.settings[key].value || '';
          }
        })
        .catch(function(error) {
          console.error('Failed to load settings:', error);
          Toast.error('Failed to load settings');
        });
    },

    saveFormSettings: function(form, successMessage) {
      const formData = new FormData(form);
      const settings = {};
      for (const entry of formData.entries()) {
        settings[entry[0]] = entry[1];
      }

      const adminApiPath = AdminWebPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/settings/bulk', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ settings: settings })
      }).then(function(response) {
        return response.json().then(function(data) {
          if (!response.ok) {
            throw new Error((data.error && data.error.message) || 'Failed to save settings');
          }
          Toast.success(successMessage);
        });
      }).catch(function(error) {
        console.error('Save error:', error);
        Toast.error(error.message);
      });
    },

    previewRobotsTxt: function() {
      window.open('/robots.txt', '_blank');
    },

    previewSecurityTxt: function() {
      window.open('/.well-known/security.txt', '_blank');
    },

    resetRobotsTxt: function() {
      showConfirm('Reset robots.txt to default configuration?', 'Reset Robots.txt').then(function(confirmed) {
        if (!confirmed) return;
        const robotsTxt = document.getElementById('robots_txt');
        if (robotsTxt) {
          robotsTxt.value = 'User-agent: *\nAllow: /\nDisallow: /admin/\nDisallow: /api/\nSitemap: {app_url}/sitemap.xml';
        }
        Toast.info('robots.txt reset to defaults (not saved yet)');
      });
    },

    init: function() {
      const brandingForm = document.getElementById('branding-form');
      if (!brandingForm) return;

      AdminWebPage.loadSettings();

      brandingForm.addEventListener('submit', function(e) {
        e.preventDefault();
        AdminWebPage.saveFormSettings(brandingForm, 'Branding settings saved successfully');
      });

      const seoForm = document.getElementById('seo-form');
      if (seoForm) {
        seoForm.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminWebPage.saveFormSettings(seoForm, 'SEO settings saved successfully');
        });
      }

      const robotsForm = document.getElementById('robots-form');
      if (robotsForm) {
        robotsForm.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminWebPage.saveFormSettings(robotsForm, 'robots.txt saved successfully');
        });
      }

      const securityTxtForm = document.getElementById('security-txt-form');
      if (securityTxtForm) {
        securityTxtForm.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminWebPage.saveFormSettings(securityTxtForm, 'security.txt saved successfully');
        });
      }
    }
  };

  const HealthzPage = {
    init: function() {
      const countdown = document.getElementById('health-countdown');
      if (!countdown) return;

      let remaining = 30;
      setInterval(function() {
        remaining -= 1;
        if (remaining <= 0) {
          window.location.reload();
          return;
        }
        countdown.textContent = String(remaining);
      }, 1000);
    }
  };

  const LoadingPage = {
    countdown: 30,
    countdownInterval: null,
    healthCheckInterval: null,
    firstCheckDone: false,

    updateCountdown: function() {
      const el = document.getElementById('countdown');
      if (el) el.textContent = String(Math.max(LoadingPage.countdown, 0));
      LoadingPage.countdown -= 1;

      if (LoadingPage.countdown < 0) {
        clearInterval(LoadingPage.countdownInterval);
        LoadingPage.retryConnection();
      }
    },

    startCountdown: function(seconds) {
      if (LoadingPage.countdownInterval) {
        clearInterval(LoadingPage.countdownInterval);
      }
      LoadingPage.countdown = seconds;
      LoadingPage.countdownInterval = setInterval(LoadingPage.updateCountdown, 1000);
    },

    redirectToTarget: function() {
      const urlParams = new URLSearchParams(window.location.search);
      const raw = urlParams.get('redirect') || '/';
      // Only allow a same-origin, absolute-path target (single leading slash,
      // not `//`/`/\` which browsers treat as protocol-relative) to prevent
      // an open redirect via the `redirect` query parameter.
      const safe = (typeof raw === 'string' && raw.charAt(0) === '/' && raw.charAt(1) !== '/' && raw.charAt(1) !== '\\') ? raw : '/';
      window.location.href = safe;
    },

    checkHealth: function() {
      fetch('/server/healthz')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (data.ready === true) {
            if (LoadingPage.countdownInterval) clearInterval(LoadingPage.countdownInterval);
            if (LoadingPage.healthCheckInterval) clearInterval(LoadingPage.healthCheckInterval);
            LoadingPage.redirectToTarget();
          } else if (!LoadingPage.firstCheckDone) {
            LoadingPage.firstCheckDone = true;
            if (LoadingPage.healthCheckInterval) clearInterval(LoadingPage.healthCheckInterval);
            LoadingPage.healthCheckInterval = setInterval(LoadingPage.checkHealth, 15000);
            LoadingPage.startCountdown(15);
          }
        })
        .catch(function() {
          if (!LoadingPage.firstCheckDone) {
            LoadingPage.firstCheckDone = true;
            if (LoadingPage.healthCheckInterval) clearInterval(LoadingPage.healthCheckInterval);
            LoadingPage.healthCheckInterval = setInterval(LoadingPage.checkHealth, 15000);
            LoadingPage.startCountdown(15);
          }
        });
    },

    retryConnection: function() {
      if (LoadingPage.countdownInterval) {
        clearInterval(LoadingPage.countdownInterval);
      }

      const retryBtn = document.getElementById('retry-btn');
      if (retryBtn) {
        retryBtn.textContent = '☕ Checking...';
        retryBtn.disabled = true;
      }

      fetch('/server/healthz')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (data.ready === true) {
            if (LoadingPage.healthCheckInterval) clearInterval(LoadingPage.healthCheckInterval);
            LoadingPage.redirectToTarget();
          } else {
            throw new Error('Service not ready');
          }
        })
        .catch(function() {
          if (retryBtn) {
            retryBtn.textContent = '🔄 Try Again Now';
            retryBtn.disabled = false;
          }
          LoadingPage.startCountdown(15);
        });
    },

    init: function() {
      if (!document.getElementById('retry-btn')) return;

      LoadingPage.startCountdown(30);
      setTimeout(LoadingPage.checkHealth, 30000);
    }
  };

  const AdminTorPage = {
    dataPayload: null,
    vanityPolling: null,

    getData: function() {
      if (!AdminTorPage.dataPayload) {
        const el = document.getElementById('admin-tor-data');
        AdminTorPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '', adminApiPath: '' };
      }
      return AdminTorPage.dataPayload;
    },

    enableTor: function() {
      const adminApiPath = AdminTorPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/tor/enable', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
      }).then(function(response) {
        return response.json().then(function(data) {
          if (!response.ok) {
            throw new Error((data.error && data.error.message) || 'Unknown error');
          }
          Toast.success('Tor enabled successfully!');
          location.reload();
        });
      }).catch(function(error) {
        Toast.error('Failed to enable Tor: ' + error.message);
      });
    },

    disableTor: function() {
      showConfirm('Are you sure you want to disable Tor?', 'Disable Tor').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminTorPage.getData().adminApiPath;
        fetch(adminApiPath + '/server/tor/disable', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' }
        }).then(function(response) {
          return response.json().then(function(data) {
            if (!response.ok) {
              throw new Error((data.error && data.error.message) || 'Unknown error');
            }
            Toast.success('Tor disabled successfully!');
            location.reload();
          });
        }).catch(function(error) {
          Toast.error('Failed to disable Tor: ' + error.message);
        });
      });
    },

    showRegenerateModal: function() {
      const modal = document.getElementById('regenerate-modal');
      if (modal) modal.classList.add('active');
    },

    closeRegenerateModal: function() {
      const modal = document.getElementById('regenerate-modal');
      if (modal) modal.classList.remove('active');
    },

    confirmRegenerate: function() {
      AdminTorPage.closeRegenerateModal();
      const adminApiPath = AdminTorPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/tor/regenerate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
      }).then(function(response) {
        return response.json().then(function(data) {
          if (!response.ok) {
            throw new Error((data.error && data.error.message) || 'Unknown error');
          }
          Toast.success('Address regenerated successfully!');
          location.reload();
        });
      }).catch(function(error) {
        Toast.error('Failed to regenerate: ' + error.message);
      });
    },

    generateVanity: function() {
      const prefixInput = document.getElementById('vanity-prefix');
      const prefix = prefixInput ? prefixInput.value.toLowerCase() : '';
      if (!prefix || prefix.length < 1 || prefix.length > 6) {
        Toast.error('Please enter a prefix between 1-6 characters');
        return;
      }
      if (!/^[a-z2-7]+$/.test(prefix)) {
        Toast.error('Please use only lowercase letters (a-z) and numbers (2-7)');
        return;
      }

      const adminApiPath = AdminTorPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/tor/vanity/generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prefix: prefix })
      }).then(function(response) {
        return response.json().then(function(data) {
          if (!response.ok) {
            throw new Error((data.error && data.error.message) || 'Unknown error');
          }
          const display = document.getElementById('vanity-prefix-display');
          if (display) display.textContent = prefix;
          const progress = document.getElementById('vanity-progress');
          if (progress) progress.classList.remove('d-none');
          const generateBtn = document.getElementById('generate-vanity-btn');
          if (generateBtn) generateBtn.disabled = true;
          AdminTorPage.startVanityPolling();
        });
      }).catch(function(error) {
        Toast.error('Failed to start generation: ' + error.message);
      });
    },

    startVanityPolling: function() {
      AdminTorPage.vanityPolling = setInterval(AdminTorPage.updateVanityStatus, 1000);
    },

    updateVanityStatus: function() {
      const adminApiPath = AdminTorPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/tor/vanity/status')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (data.running) {
            const attempts = document.getElementById('attempts-count');
            if (attempts) attempts.textContent = data.attempts.toLocaleString();
            const elapsed = Math.floor((Date.now() - new Date(data.start_time)) / 1000);
            const elapsedEl = document.getElementById('elapsed-time');
            if (elapsedEl) elapsedEl.textContent = AdminTorPage.formatDuration(elapsed);
            const estimated = document.getElementById('estimated-time');
            if (estimated) estimated.textContent = data.estimated_time || 'Calculating...';
          } else if (data.address) {
            clearInterval(AdminTorPage.vanityPolling);
            const success = document.getElementById('vanity-success');
            if (success) success.classList.remove('d-none');
            const result = document.getElementById('vanity-address-result');
            if (result) result.textContent = data.address;
          }
        })
        .catch(function(error) {
          console.error('Error polling status:', error);
        });
    },

    formatDuration: function(seconds) {
      if (seconds < 60) return seconds + 's';
      const minutes = Math.floor(seconds / 60);
      if (minutes < 60) return minutes + 'm ' + (seconds % 60) + 's';
      const hours = Math.floor(minutes / 60);
      return hours + 'h ' + (minutes % 60) + 'm';
    },

    cancelVanity: function() {
      const adminApiPath = AdminTorPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/tor/vanity/cancel', { method: 'POST' })
        .then(function() {
          clearInterval(AdminTorPage.vanityPolling);
          const progress = document.getElementById('vanity-progress');
          if (progress) progress.classList.add('d-none');
          const generateBtn = document.getElementById('generate-vanity-btn');
          if (generateBtn) generateBtn.disabled = false;
        })
        .catch(function(error) {
          Toast.error('Error: ' + error.message);
        });
    },

    applyVanity: function() {
      showConfirm('Apply this vanity address? Your current address will be replaced.', 'Apply Vanity Address').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminTorPage.getData().adminApiPath;
        fetch(adminApiPath + '/server/tor/vanity/apply', { method: 'POST' })
          .then(function(response) {
            return response.json().then(function(data) {
              if (!response.ok) {
                throw new Error((data.error && data.error.message) || 'Unknown error');
              }
              Toast.success('Vanity address applied successfully!');
              location.reload();
            });
          })
          .catch(function(error) {
            Toast.error('Failed to apply: ' + error.message);
          });
      });
    },

    importKeys: function(e) {
      e.preventDefault();
      const formData = new FormData(e.target);
      const adminApiPath = AdminTorPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/tor/keys/import', {
        method: 'POST',
        body: formData
      }).then(function(response) {
        return response.json().then(function(data) {
          if (!response.ok) {
            throw new Error((data.error && data.error.message) || 'Unknown error');
          }
          Toast.success('Keys imported successfully!');
          location.reload();
        });
      }).catch(function(error) {
        Toast.error('Failed to import: ' + error.message);
      });
    },

    exportKeys: function() {
      const adminApiPath = AdminTorPage.getData().adminApiPath;
      window.location.href = adminApiPath + '/server/tor/keys/export';
    },

    copyAddress: function() {
      const addressEl = document.getElementById('onion-address');
      if (!addressEl) return;
      navigator.clipboard.writeText(addressEl.textContent).then(function() {
        Toast.success('Address copied to clipboard!');
      }).catch(function(error) {
        Toast.error('Failed to copy: ' + error.message);
      });
    },

    init: function() {
      const importForm = document.getElementById('import-form');
      if (importForm) {
        importForm.addEventListener('submit', AdminTorPage.importKeys);
      }
    }
  };

  // ============================================
  // ADMIN DASHBOARD - legacy tabbed panel (AI.md PART 17)
  // ============================================

  const AdminDashboardPage = {
    dataPayload: null,
    changedSettings: new Set(),

    getData: function() {
      if (!AdminDashboardPage.dataPayload) {
        const el = document.getElementById('admin-dashboard-data');
        AdminDashboardPage.dataPayload = el ? JSON.parse(el.textContent) : { apiPath: '', adminApiPath: '' };
      }
      return AdminDashboardPage.dataPayload;
    },

    switchTab: function(tabId) {
      const tabs = document.querySelectorAll('.tab');
      const tabContents = document.querySelectorAll('.tab-content');
      tabs.forEach(function(t) { t.classList.remove('active'); });
      tabContents.forEach(function(tc) { tc.classList.remove('active'); });

      const activeTab = document.querySelector('.tab[data-tab="' + tabId + '"]');
      if (activeTab) activeTab.classList.add('active');
      const activeContent = document.getElementById(tabId);
      if (activeContent) activeContent.classList.add('active');

      if (tabId === 'users') AdminDashboardPage.loadUsers();
      if (tabId === 'settings') AdminDashboardPage.loadSettings();
      if (tabId === 'tokens') AdminDashboardPage.loadTokens();
      if (tabId === 'logs') AdminDashboardPage.loadLogs();
      if (tabId === 'tasks') AdminDashboardPage.loadScheduledTasks();
      if (tabId === 'backup') AdminDashboardPage.loadBackupList();
    },

    // User Management

    loadUsers: function() {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/users')
        .then(function(response) { return response.json(); })
        .then(function(users) {
          const html = '<table class="admin-table-full">' +
            '<thead><tr class="admin-table-head-row">' +
            '<th class="admin-table-head-cell">ID</th>' +
            '<th class="admin-table-head-cell">Email</th>' +
            '<th class="admin-table-head-cell">Role</th>' +
            '<th class="admin-table-head-cell">Created</th>' +
            '<th class="admin-table-head-cell-right">Actions</th>' +
            '</tr></thead><tbody>' +
            users.map(function(user) {
              const isFirstAdmin = user.id === 1;
              const actionButtons = isFirstAdmin
                ? '<button data-action="admin-dashboard-edit-admin-user" data-user-id="' + user.id + '" data-user-email="' + user.email + '" class="btn-edit">Edit</button>'
                : '<button data-action="admin-dashboard-delete-user" data-user-id="' + user.id + '" class="btn-delete">Delete</button>';

              return '<tr class="admin-table-body-row">' +
                '<td class="admin-table-body-cell">' + user.id + (isFirstAdmin ? ' <span class="admin-user-primary">(Primary)</span>' : '') + '</td>' +
                '<td class="admin-table-body-cell">' + user.email + '</td>' +
                '<td class="admin-table-body-cell"><span class="' + (user.role === 'admin' ? 'admin-role-badge-admin' : 'admin-role-badge') + '">' + user.role + '</span></td>' +
                '<td class="admin-table-body-cell">' + new Date(user.created_at).toLocaleDateString() + '</td>' +
                '<td class="admin-table-body-cell-right">' + actionButtons + '</td>' +
                '</tr>';
            }).join('') +
            '</tbody></table>';
          document.getElementById('users-list').innerHTML = html;
        })
        .catch(function() {
          document.getElementById('users-list').innerHTML = '<p class="admin-error-text">Error loading users</p>';
        });
    },

    deleteUser: function(id) {
      showConfirm('Are you sure you want to delete this user? This action cannot be undone.', 'Delete User').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/server/users/' + id, { method: 'DELETE' })
          .then(function() {
            Toast.success('User deleted successfully');
            AdminDashboardPage.loadUsers();
          })
          .catch(function() {
            Toast.error('Failed to delete user');
          });
      });
    },

    editAdminUser: function(userId, currentEmail) {
      const modalId = 'edit-admin-modal-' + Date.now();

      Modal.create({
        id: modalId,
        title: 'Edit Primary Admin Account',
        body: '<form id="editAdminForm">' +
          '<div class="form-group">' +
          '<label for="editAdminEmail">Email Address</label>' +
          '<input type="email" id="editAdminEmail" class="form-input" required value="' + currentEmail + '" placeholder="admin@example.com">' +
          '<small class="text-comment">Update the email address for this account</small>' +
          '</div>' +
          '<div class="form-group">' +
          '<label for="editAdminPassword">New Password (optional)</label>' +
          '<input type="password" id="editAdminPassword" class="form-input" minlength="8" placeholder="Leave blank to keep current password">' +
          '<small class="text-comment">Minimum 8 characters (leave blank to keep current password)</small>' +
          '</div>' +
          '<div class="form-group">' +
          '<label for="editAdminPasswordConfirm">Confirm New Password</label>' +
          '<input type="password" id="editAdminPasswordConfirm" class="form-input" minlength="8" placeholder="Re-enter new password">' +
          '<small class="text-comment">Must match the new password above</small>' +
          '</div>' +
          '<div class="admin-settings-note">' +
          '<p class="admin-settings-note-text">ℹ️ <strong>Note:</strong> This is the primary admin account and cannot be deleted. You can update the email and/or password.</p>' +
          '</div>' +
          '</form>',
        footer: '<button class="btn btn-secondary" data-action="modal-cancel">Cancel</button>' +
          '<button class="btn btn-primary" data-action="admin-dashboard-update-admin-user" data-user-id="' + userId + '" data-modal-id="' + modalId + '">Update Account</button>',
        size: 'md'
      });
    },

    updateAdminUser: function(userId, modalId) {
      const email = document.getElementById('editAdminEmail').value;
      const password = document.getElementById('editAdminPassword').value;
      const passwordConfirm = document.getElementById('editAdminPasswordConfirm').value;

      if (!email) {
        Toast.error('Please enter an email address');
        return;
      }

      if (password || passwordConfirm) {
        if (password !== passwordConfirm) {
          Toast.error('Passwords do not match');
          return;
        }
        if (password.length < 8) {
          Toast.error('Password must be at least 8 characters');
          return;
        }
      }

      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/users/' + userId, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email })
      })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to update user');
          if (!password) return null;
          return fetch(adminApiPath + '/server/users/' + userId + '/password', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ password: password })
          }).then(function(pwResponse) {
            if (!pwResponse.ok) throw new Error('Failed to update password');
          });
        })
        .then(function() {
          Modal.close(modalId);
          Toast.success('Admin account updated successfully');
          AdminDashboardPage.loadUsers();
        })
        .catch(function(error) {
          Toast.error('Failed to update admin account: ' + error.message);
        });
    },

    showAddUserModal: function() {
      const modalId = 'add-user-modal-' + Date.now();

      Modal.create({
        id: modalId,
        title: 'Add New User',
        body: '<form id="addUserForm">' +
          '<div class="form-group">' +
          '<label for="newUsername">Username</label>' +
          '<input type="text" id="newUsername" class="form-input" required placeholder="username" pattern="[a-zA-Z0-9_]+" minlength="3" maxlength="50">' +
          '<small class="text-comment">3-50 characters, letters, numbers, and underscores only</small>' +
          '</div>' +
          '<div class="form-group">' +
          '<label for="newUserEmail">Email Address</label>' +
          '<input type="email" id="newUserEmail" class="form-input" required placeholder="user@example.com">' +
          '</div>' +
          '<div class="form-group">' +
          '<label for="newUserPassword">Password</label>' +
          '<input type="password" id="newUserPassword" class="form-input" required minlength="8" placeholder="Minimum 8 characters">' +
          '<small class="text-comment">Minimum 8 characters</small>' +
          '</div>' +
          '<div class="form-group">' +
          '<label for="newUserRole">Role</label>' +
          '<select id="newUserRole" class="form-input">' +
          '<option value="user">User</option>' +
          '<option value="admin">Admin</option>' +
          '</select>' +
          '</div>' +
          '</form>',
        footer: '<button class="btn btn-secondary" data-action="modal-cancel">Cancel</button>' +
          '<button class="btn btn-primary" data-action="admin-dashboard-create-user-from-modal" data-modal-id="' + modalId + '">Create User</button>',
        size: 'md'
      });
    },

    createUserFromModal: function(modalId) {
      const username = document.getElementById('newUsername').value;
      const email = document.getElementById('newUserEmail').value;
      const password = document.getElementById('newUserPassword').value;
      const role = document.getElementById('newUserRole').value;

      if (!username || !email || !password) {
        Toast.error('Please fill in all fields');
        return;
      }

      if (password.length < 8) {
        Toast.error('Password must be at least 8 characters');
        return;
      }

      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: username, email: email, password: password, role: role })
      })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to create user');
          Modal.close(modalId);
          Toast.success('User created successfully');
          AdminDashboardPage.loadUsers();
        })
        .catch(function(error) {
          Toast.error('Failed to create user: ' + error.message);
        });
    },

    // Settings Management

    formatSettingLabel: function(key) {
      const labelMap = {
        'smtp.test_recipient': 'Test Email Recipient',
        'smtp.from_address': 'From Email Address',
        'smtp.from_name': 'From Name',
        'smtp.use_tls': 'Use TLS/SSL',
        'smtp.password': 'SMTP Password',
        'auth.session_timeout': 'Session Timeout (seconds)',
        'rate_limit.anonymous': 'Anonymous Rate Limit',
        'rate_limit.window': 'Rate Limit Window (seconds)',
        'log.format': 'Log Format',
        'log.level': 'Log Level',
        'backup.enabled': 'Enable Backups',
        'backup.interval': 'Backup Interval (seconds)',
        'alerts.enabled': 'Enable Weather Alerts',
        'alerts.check_interval': 'Alert Check Interval (seconds)',
        'weather.refresh_interval': 'Weather Cache Refresh (seconds)',
        'notifications.enabled': 'Enable Notifications',
        'notifications.retry_max': 'Max Retry Attempts',
        'notifications.queue_workers': 'Queue Workers',
        'notifications.retry_backoff': 'Retry Backoff Strategy'
      };

      if (labelMap[key]) return labelMap[key];

      const parts = key.split('.');
      const lastPart = parts[parts.length - 1];
      return lastPart
        .replace(/_/g, ' ')
        .split(' ')
        .map(function(word) { return word.charAt(0).toUpperCase() + word.slice(1); })
        .join(' ');
    },

    loadSettings: function() {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/settings')
        .then(function(response) { return response.json(); })
        .then(function(settings) {
          const grouped = {};
          settings.forEach(function(setting) {
            const category = setting.key.split('.')[0];
            if (!grouped[category]) grouped[category] = [];
            grouped[category].push(setting);
          });

          const categoryNames = {
            'server': '🌐 Server',
            'auth': '🔐 Authentication',
            'rate_limit': '⏱️ Rate Limiting',
            'smtp': '📧 Email (SMTP)',
            'notifications': '🔔 Notifications',
            'weather': '🌤️ Weather',
            'alerts': '⚠️ Alerts',
            'backup': '💾 Backup',
            'log': '📝 Logging',
            'audit': '📋 Audit',
            'security': '🔒 Security',
            'features': '✨ Features'
          };

          const enumFields = {
            'log.level': ['info', 'debug', 'warn', 'error'],
            'log.format': ['apache', 'json'],
            'notifications.retry_backoff': ['linear', 'exponential']
          };

          let html = '';
          Object.keys(grouped).forEach(function(category) {
            const categorySettings = grouped[category];
            const displayName = categoryNames[category] || category;
            html += '<div class="settings-category">' +
              '<h3 class="settings-category-title">' + displayName + '</h3>' +
              '<div class="settings-grid">' +
              categorySettings.map(function(setting) {
                const name = AdminDashboardPage.formatSettingLabel(setting.key);
                const isBool = setting.value === 'true' || setting.value === 'false';
                const isNumber = !isNaN(setting.value) && setting.value !== '' && setting.value !== null;
                const isPassword = setting.key.indexOf('password') !== -1;

                let inputHtml = '';

                if (isBool) {
                  const checked = setting.value === 'true' ? 'checked' : '';
                  inputHtml = '<label class="custom-checkbox">' +
                    '<input type="checkbox" data-key="' + setting.key + '" ' + checked + '>' +
                    '<span class="checkbox-checkmark"></span>' +
                    '<span class="checkbox-label">' + (checked ? 'Enabled' : 'Disabled') + '</span>' +
                    '</label>';
                } else if (enumFields[setting.key]) {
                  const options = enumFields[setting.key].map(function(opt) {
                    return '<option value="' + opt + '" ' + (setting.value === opt ? 'selected' : '') + '>' + (opt.charAt(0).toUpperCase() + opt.slice(1)) + '</option>';
                  }).join('');
                  inputHtml = '<select class="form-input" data-key="' + setting.key + '">' + options + '</select>';
                } else if (isPassword) {
                  inputHtml = '<input type="password" class="form-input" data-key="' + setting.key + '" value="' + setting.value + '" placeholder="••••••••">';
                } else if (isNumber) {
                  inputHtml = '<input type="number" class="form-input" data-key="' + setting.key + '" value="' + setting.value + '" min="0">';
                } else {
                  inputHtml = '<input type="text" class="form-input" data-key="' + setting.key + '" value="' + setting.value + '">';
                }

                return '<div class="setting-item">' +
                  '<div class="setting-info">' +
                  '<div class="setting-name">' + name + '</div>' +
                  '<div class="setting-key">' + setting.key + '</div>' +
                  (setting.description ? '<div class="setting-description">' + setting.description + '</div>' : '') +
                  '</div>' +
                  '<div class="setting-value">' + inputHtml + '</div>' +
                  '</div>';
              }).join('') +
              '</div></div>';
          });

          document.getElementById('settings-list').innerHTML = html;
        })
        .catch(function(error) {
          document.getElementById('settings-list').innerHTML = '<p class="text-error text-center">Error loading settings: ' + error.message + '</p>';
        });
    },

    markSettingChanged: function(key) {
      AdminDashboardPage.changedSettings.add(key);
      const applyBtn = document.getElementById('apply-settings-btn');
      if (AdminDashboardPage.changedSettings.size > 0) {
        applyBtn.textContent = '✓ Apply ' + AdminDashboardPage.changedSettings.size + ' Change(s) & Reload';
        applyBtn.classList.add('btn-warning');
      }
    },

    applyAllSettings: function() {
      if (AdminDashboardPage.changedSettings.size === 0) {
        showAlert('No changes to apply', 'Info');
        return;
      }

      showConfirm(
        'Apply ' + AdminDashboardPage.changedSettings.size + ' setting(s) and reload server configuration?',
        'Apply Settings'
      ).then(function(confirmApply) {
        if (!confirmApply) return;

        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        const applyBtn = document.getElementById('apply-settings-btn');
        applyBtn.disabled = true;
        applyBtn.textContent = '⏳ Applying...';

        const updates = {};
        const inputs = document.querySelectorAll('[data-key]');
        inputs.forEach(function(input) {
          if (AdminDashboardPage.changedSettings.has(input.dataset.key)) {
            updates[input.dataset.key] = input.value;
          }
        });

        fetch(adminApiPath + '/settings/bulk', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ settings: updates })
        })
          .then(function() {
            return fetch(adminApiPath + '/reload', { method: 'POST' });
          })
          .then(function() {
            Modal.create({
              title: '✅ Settings Applied',
              body: '<p class="modal-body-text-center">' + AdminDashboardPage.changedSettings.size + ' setting(s) applied successfully.<br>Server configuration reloaded.</p>',
              size: 'sm',
              autoClose: 3,
              closeable: true
            });

            AdminDashboardPage.changedSettings.clear();
            applyBtn.textContent = '✓ Apply All & Reload';
            applyBtn.classList.remove('btn-warning');
            applyBtn.disabled = false;

            setTimeout(function() { AdminDashboardPage.loadSettings(); }, 500);
          })
          .catch(function(error) {
            applyBtn.disabled = false;
            applyBtn.textContent = '✓ Apply All & Reload';
            showAlert('Failed to apply settings: ' + error.message, 'Error');
          });
      });
    },

    // Token Management

    loadTokens: function() {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/tokens')
        .then(function(response) {
          if (!response.ok) throw new Error('HTTP ' + response.status + ': ' + response.statusText);
          return response.json();
        })
        .then(function(tokens) {
          if (!Array.isArray(tokens) || tokens.length === 0) {
            document.getElementById('tokens-list').innerHTML = '<p class="admin-no-data-text">No API tokens found. Generate one to get started.</p>';
            return;
          }

          const html = '<table class="admin-table-full">' +
            '<thead><tr class="admin-table-head-row">' +
            '<th class="admin-table-head-cell">ID</th>' +
            '<th class="admin-table-head-cell">User</th>' +
            '<th class="admin-table-head-cell">Name</th>' +
            '<th class="admin-table-head-cell">Created</th>' +
            '<th class="admin-table-head-cell">Last Used</th>' +
            '<th class="admin-table-head-cell-right">Actions</th>' +
            '</tr></thead><tbody>' +
            tokens.map(function(token) {
              return '<tr class="admin-table-body-row">' +
                '<td class="admin-table-body-cell">' + token.id + '</td>' +
                '<td class="admin-table-body-cell">' + (token.user_email || 'N/A') + '</td>' +
                '<td class="admin-table-body-cell">' + token.name + '</td>' +
                '<td class="admin-table-body-cell">' + new Date(token.created_at).toLocaleDateString() + '</td>' +
                '<td class="admin-table-body-cell">' + (token.last_used_at ? new Date(token.last_used_at).toLocaleDateString() : 'Never') + '</td>' +
                '<td class="admin-table-body-cell-right">' +
                '<button data-action="admin-dashboard-revoke-token" data-token-id="' + token.id + '" class="admin-token-button">Revoke</button>' +
                '</td></tr>';
            }).join('') +
            '</tbody></table>';
          document.getElementById('tokens-list').innerHTML = html;
        })
        .catch(function(error) {
          document.getElementById('tokens-list').innerHTML = '<p class="admin-error-text">Error loading tokens: ' + error.message + '</p>';
        });
    },

    revokeToken: function(id) {
      showConfirm('Are you sure you want to revoke this token? Applications using this token will no longer have access.', 'Revoke Token').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/server/security/tokens/' + id, { method: 'DELETE' })
          .then(function() {
            Toast.success('Token revoked successfully');
            AdminDashboardPage.loadTokens();
          })
          .catch(function() {
            Toast.error('Failed to revoke token');
          });
      });
    },

    showGenerateTokenModal: function() {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/users')
        .then(function(response) { return response.json(); })
        .then(function(users) {
          const userOptions = users.map(function(u) {
            return '<option value="' + u.id + '">' + u.email + '</option>';
          }).join('');

          const modalId = 'generate-token-modal-' + Date.now();

          Modal.create({
            id: modalId,
            title: 'Generate API Token',
            body: '<form id="generateTokenForm">' +
              '<div class="form-group">' +
              '<label for="tokenUser">Select User</label>' +
              '<select id="tokenUser" class="form-input" required>' +
              '<option value="">-- Select User --</option>' + userOptions +
              '</select>' +
              '</div>' +
              '<div class="form-group">' +
              '<label for="tokenName">Token Name</label>' +
              '<input type="text" id="tokenName" class="form-input" required placeholder="e.g., Production API Key">' +
              '<small class="text-comment">A descriptive name to identify this token</small>' +
              '</div>' +
              '</form>',
            footer: '<button class="btn btn-secondary" data-action="modal-cancel">Cancel</button>' +
              '<button class="btn btn-primary" data-action="admin-dashboard-generate-token-from-modal" data-modal-id="' + modalId + '">Generate Token</button>',
            size: 'md'
          });
        });
    },

    generateTokenFromModal: function(modalId) {
      const userId = document.getElementById('tokenUser').value;
      const tokenName = document.getElementById('tokenName').value;

      if (!userId || !tokenName) {
        Toast.error('Please fill in all fields');
        return;
      }

      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/tokens', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id: parseInt(userId, 10), name: tokenName })
      })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to generate token');
          return response.json();
        })
        .then(function(result) {
          Modal.close(modalId);
          AdminDashboardPage.showTokenModal(result.token);
          AdminDashboardPage.loadTokens();
        })
        .catch(function(error) {
          Toast.error('Failed to generate token: ' + error.message);
        });
    },

    showTokenModal: function(token) {
      const modalId = 'token-display-modal-' + Date.now();

      Modal.create({
        id: modalId,
        title: '✅ Token Generated Successfully',
        body: '<div class="margin-bottom-1">' +
          '<p class="text-orange margin-bottom-1">⚠️ <strong>Important:</strong> Copy this token now. For security reasons, it will not be shown again!</p>' +
          '<div class="admin-settings-note">' +
          '<code class="font-monospace word-break-all text-green font-size-09">' + token + '</code>' +
          '</div></div>',
        footer: '<button class="btn btn-primary" data-action="admin-dashboard-copy-token" data-token="' + token + '">📋 Copy to Clipboard</button>' +
          '<button class="btn btn-secondary" data-action="modal-close">Close</button>',
        size: 'lg'
      });
    },

    copyToken: function(token) {
      navigator.clipboard.writeText(token).then(function() {
        Toast.success('Token copied to clipboard!');
      }).catch(function() {
        const textarea = document.createElement('textarea');
        textarea.value = token;
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        Toast.success('Token copied to clipboard!');
      });
    },

    // Logs Management

    loadLogs: function() {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/logs?limit=50')
        .then(function(response) {
          if (!response.ok) throw new Error('HTTP ' + response.status + ': ' + response.statusText);
          return response.json();
        })
        .then(function(logs) {
          if (!Array.isArray(logs) || logs.length === 0) {
            document.getElementById('logs-list').innerHTML = '<p class="admin-loading-text">No audit logs found</p>';
            return;
          }

          const html = '<table class="admin-logs-table">' +
            '<thead><tr class="admin-table-head-row">' +
            '<th class="admin-table-head-cell">Time</th>' +
            '<th class="admin-table-head-cell">User</th>' +
            '<th class="admin-table-head-cell">Action</th>' +
            '<th class="admin-table-head-cell">Resource</th>' +
            '<th class="admin-table-head-cell">IP</th>' +
            '</tr></thead><tbody>' +
            logs.map(function(log) {
              return '<tr class="admin-table-body-row">' +
                '<td class="admin-logs-cell-mono">' + new Date(log.created_at).toLocaleString() + '</td>' +
                '<td class="admin-logs-cell">' + (log.user_email || 'Anonymous') + '</td>' +
                '<td class="admin-logs-cell-cyan">' + log.action + '</td>' +
                '<td class="admin-logs-cell-comment">' + (log.resource || '-') + '</td>' +
                '<td class="admin-logs-cell-mono">' + (log.ip_address || '-') + '</td>' +
                '</tr>';
            }).join('') +
            '</tbody></table>';
          document.getElementById('logs-list').innerHTML = html;
        })
        .catch(function(error) {
          document.getElementById('logs-list').innerHTML = '<p class="admin-error-text">Error loading logs: ' + error.message + '</p>';
        });
    },

    clearAuditLogs: function() {
      const modalId = 'clear-logs-modal-' + Date.now();

      Modal.create({
        id: modalId,
        title: 'Clear Audit Logs',
        body: '<form id="clearLogsForm">' +
          '<div class="form-group">' +
          '<label for="logDays">Delete logs older than (days)</label>' +
          '<input type="number" id="logDays" class="form-input" value="30" min="1" required>' +
          '<small class="text-comment">Logs older than this many days will be permanently deleted</small>' +
          '</div>' +
          '<div class="admin-warning-box">' +
          '<p class="text-orange margin-0">⚠️ <strong>Warning:</strong> This action cannot be undone. Deleted logs cannot be recovered.</p>' +
          '</div>' +
          '</form>',
        footer: '<button class="btn btn-secondary" data-action="modal-cancel">Cancel</button>' +
          '<button class="btn btn-danger" data-action="admin-dashboard-confirm-clear-logs" data-modal-id="' + modalId + '">Delete Logs</button>',
        size: 'md'
      });
    },

    confirmClearLogs: function(modalId) {
      const days = document.getElementById('logDays').value;

      if (!days || days < 1) {
        Toast.error('Please enter a valid number of days');
        return;
      }

      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/logs?days=' + days, { method: 'DELETE' })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to clear logs');
          Modal.close(modalId);
          Toast.success('Audit logs older than ' + days + ' days have been deleted');
          AdminDashboardPage.loadLogs();
        })
        .catch(function(error) {
          Toast.error('Failed to clear logs: ' + error.message);
        });
    },

    // Formatting helpers

    formatRelativeTime: function(dateStr) {
      if (!dateStr) return 'N/A';
      const now = new Date();
      const target = new Date(dateStr);
      const diff = target - now;

      if (diff < 0) return 'Overdue';

      const mins = Math.floor(diff / 60000);
      const hours = Math.floor(mins / 60);
      const days = Math.floor(hours / 24);

      if (days > 0) return 'in ' + days + 'd ' + (hours % 24) + 'h';
      if (hours > 0) return 'in ' + hours + 'h ' + (mins % 60) + 'm';
      return 'in ' + mins + 'm';
    },

    formatInterval: function(seconds) {
      if (!seconds || seconds === 0) return 'N/A';

      const minutes = Math.floor(seconds / 60);
      const hours = Math.floor(minutes / 60);
      const days = Math.floor(hours / 24);
      const weeks = Math.floor(days / 7);

      if (weeks > 0) return weeks + ' week' + (weeks > 1 ? 's' : '');
      if (days > 0) return days + ' day' + (days > 1 ? 's' : '');
      if (hours > 0) return hours + ' hour' + (hours > 1 ? 's' : '');
      if (minutes > 0) return minutes + ' minute' + (minutes > 1 ? 's' : '');
      return seconds + ' second' + (seconds > 1 ? 's' : '');
    },

    formatBytes: function(bytes) {
      if (bytes === 0) return '0 Bytes';
      const k = 1024;
      const sizes = ['Bytes', 'KB', 'MB', 'GB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
    },

    // Scheduled Tasks Management

    loadScheduledTasks: function() {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/tasks')
        .then(function(response) {
          if (!response.ok) throw new Error('HTTP ' + response.status + ': ' + response.statusText);
          return response.json();
        })
        .then(function(data) {
          const tasks = data.tasks || [];

          if (!Array.isArray(tasks) || tasks.length === 0) {
            document.getElementById('tasks-list').innerHTML = '<p class="admin-loading-text">No scheduled tasks found</p>';
            return;
          }

          const html = '<table class="admin-table-full">' +
            '<thead><tr class="admin-table-head-row">' +
            '<th class="admin-table-head-cell">Task Name</th>' +
            '<th class="admin-table-head-cell">Interval</th>' +
            '<th class="admin-table-head-cell">Enabled</th>' +
            '<th class="admin-table-head-cell">Status</th>' +
            '<th class="admin-table-head-cell">Last Run</th>' +
            '<th class="admin-table-head-cell">Next Run</th>' +
            '<th class="admin-table-head-cell">Stats</th>' +
            '<th class="admin-table-head-cell">Actions</th>' +
            '</tr></thead><tbody>' +
            tasks.map(function(task) {
              const lastRunInfo = task.last_run
                ? new Date(task.last_run.start_time).toLocaleString() + '<br>' +
                  '<small class="' + (task.last_run.status === 'success' ? 'status-success' : 'status-error') + '">' +
                  (task.last_run.status === 'success' ? '✓' : '✗') + ' ' + task.last_run.duration_ms + 'ms</small>'
                : 'Never';
              const nextRun = task.next_run ? new Date(task.next_run).toLocaleString() : 'N/A';
              const countdown = task.next_run ? AdminDashboardPage.formatRelativeTime(task.next_run) : '';

              return '<tr class="admin-table-body-row">' +
                '<td class="admin-task-name">' + task.name + '</td>' +
                '<td class="admin-table-body-cell">' + task.interval + '</td>' +
                '<td class="admin-table-body-cell"><span class="' + (task.enabled ? 'status-enabled' : 'status-disabled') + '">' +
                (task.enabled ? '✓ Enabled' : '○ Disabled') + '</span></td>' +
                '<td class="admin-table-body-cell"><span class="' + (task.running ? 'admin-task-status-running' : 'admin-task-status-idle') + '">' +
                (task.running ? 'Running' : 'Idle') + '</span></td>' +
                '<td class="admin-table-body-cell admin-task-next-run">' + lastRunInfo + '</td>' +
                '<td class="admin-table-body-cell"><div class="admin-task-next-run">' + nextRun + '</div>' +
                (countdown ? '<div class="admin-task-countdown">' + countdown + '</div>' : '') + '</td>' +
                '<td class="admin-table-body-cell">' +
                '<small>' + task.success_count + '✓ / ' + task.error_count + '✗</small><br>' +
                '<small>' + task.run_count + ' total runs</small></td>' +
                '<td class="admin-table-body-cell">' +
                '<button data-action="admin-dashboard-trigger-task" data-task-name="' + task.name + '" class="btn-primary btn-task-action">▶ Run Now</button>' +
                (task.enabled
                  ? '<button data-action="admin-dashboard-disable-task" data-task-name="' + task.name + '" class="btn-danger btn-task-action">⏸ Disable</button>'
                  : '<button data-action="admin-dashboard-enable-task" data-task-name="' + task.name + '" class="btn-success btn-task-action">▶ Enable</button>') +
                '<button data-action="admin-dashboard-view-task-history" data-task-name="' + task.name + '" class="btn-secondary btn-task-action">📊 History</button>' +
                '</td></tr>';
            }).join('') +
            '</tbody></table>';
          document.getElementById('tasks-list').innerHTML = html;
        })
        .catch(function(error) {
          document.getElementById('tasks-list').innerHTML = '<p class="admin-error-text">Error loading scheduled tasks: ' + error.message + '</p>';
        });
    },

    enableTask: function(taskName) {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/scheduler/' + taskName + '/enable', { method: 'POST' })
        .then(function(response) {
          if (!response.ok) return response.text().then(function(text) { throw new Error(text); });
          return AdminDashboardPage.loadScheduledTasks();
        })
        .then(function() {
          Toast.success('Task "' + taskName + '" enabled successfully');
        })
        .catch(function(error) {
          Toast.error('Error enabling task: ' + error.message);
        });
    },

    disableTask: function(taskName) {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/scheduler/' + taskName + '/disable', { method: 'POST' })
        .then(function(response) {
          if (!response.ok) return response.text().then(function(text) { throw new Error(text); });
          return AdminDashboardPage.loadScheduledTasks();
        })
        .then(function() {
          Toast.success('Task "' + taskName + '" disabled successfully');
        })
        .catch(function(error) {
          Toast.error('Error disabling task: ' + error.message);
        });
    },

    triggerTask: function(taskName) {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/scheduler/' + taskName + '/run', { method: 'POST' })
        .then(function(response) {
          if (!response.ok) return response.text().then(function(text) { throw new Error(text); });
          Toast.success('Task "' + taskName + '" triggered successfully');
          setTimeout(function() { AdminDashboardPage.loadScheduledTasks(); }, 2000);
        })
        .catch(function(error) {
          Toast.error('Error triggering task: ' + error.message);
        });
    },

    viewTaskHistory: function(taskName) {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/scheduler/' + taskName + '?limit=20')
        .then(function(response) {
          if (!response.ok) return response.text().then(function(text) { throw new Error(text); });
          return response.json();
        })
        .then(function(data) {
          const history = data.history || [];

          if (history.length === 0) {
            Toast.info('No execution history found for task "' + taskName + '"');
            return;
          }

          const historyHTML = '<div class="task-history-scroll">' +
            '<table class="admin-table-full"><thead><tr class="admin-table-head-row">' +
            '<th>Start Time</th><th>Duration</th><th>Status</th><th>Error</th>' +
            '</tr></thead><tbody>' +
            history.map(function(run) {
              return '<tr class="admin-table-body-row">' +
                '<td>' + new Date(run.start_time).toLocaleString() + '</td>' +
                '<td>' + run.duration_ms + 'ms</td>' +
                '<td class="' + (run.status === 'success' ? 'status-success' : 'status-error') + '">' +
                (run.status === 'success' ? '✓ Success' : '✗ Error') + '</td>' +
                '<td>' + (run.error || '-') + '</td></tr>';
            }).join('') +
            '</tbody></table></div>';

          Modal.create({
            title: 'Task History: ' + taskName,
            body: historyHTML,
            size: 'lg'
          });
        })
        .catch(function(error) {
          Toast.error('Error loading task history: ' + error.message);
        });
    },

    // Web Settings

    saveWebSettings: function() {
      const settings = {
        title: document.getElementById('web-title').value,
        tagline: document.getElementById('web-tagline').value,
        footer: document.getElementById('web-footer').value,
        theme: document.getElementById('web-theme').value,
        page_size: parseInt(document.getElementById('web-page-size').value, 10),
        analytics: document.getElementById('web-analytics').checked
      };

      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/settings/web', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings)
      })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to save settings');
          Toast.success('Web settings saved successfully!');
        })
        .catch(function(error) {
          Toast.error('Error saving web settings: ' + error.message);
        });
    },

    // Security Settings

    saveSecuritySettings: function() {
      const settings = {
        rate_limit_enabled: document.getElementById('sec-ratelimit-enabled').checked,
        global_limit: parseInt(document.getElementById('sec-global-limit').value, 10),
        api_limit: parseInt(document.getElementById('sec-api-limit').value, 10),
        admin_limit: parseInt(document.getElementById('sec-admin-limit').value, 10),
        hsts_enabled: document.getElementById('sec-hsts').checked,
        csp: document.getElementById('sec-csp').value,
        blocked_ips: document.getElementById('sec-blocked-ips').value.split('\n').filter(function(ip) { return ip.trim(); }),
        require_https: document.getElementById('sec-require-https').checked
      };

      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/settings/security', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings)
      })
        .then(function(response) {
          if (!response.ok) throw new Error('Failed to save settings');
          Toast.success('Security settings saved successfully!');
        })
        .catch(function(error) {
          Toast.error('Error saving security settings: ' + error.message);
        });
    },

    // Database

    testDatabaseConnection: function() {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/database/test')
        .then(function(response) { return response.json(); })
        .then(function(result) {
          if (result.success) {
            Toast.success('Database connection successful!');
          } else {
            Toast.error('Database connection failed: ' + result.error);
          }
        })
        .catch(function(error) {
          Toast.error('Error testing database: ' + error.message);
        });
    },

    optimizeDatabase: function() {
      showConfirm('Optimize database? This may take a few moments.').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/database/optimize', { method: 'POST' })
          .then(function(response) { return response.json(); })
          .then(function(result) {
            if (result.success) {
              Toast.success('Database optimized successfully!');
            } else {
              Toast.error('Optimization failed: ' + result.error);
            }
          })
          .catch(function(error) {
            Toast.error('Error optimizing database: ' + error.message);
          });
      });
    },

    clearCache: function() {
      showConfirm('Clear all cache? This will temporarily slow down requests.').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/cache/clear', { method: 'POST' })
          .then(function(response) { return response.json(); })
          .then(function(result) {
            if (result.success) {
              Toast.success('Cache cleared successfully!');
            } else {
              Toast.error('Failed to clear cache: ' + result.error);
            }
          })
          .catch(function(error) {
            Toast.error('Error clearing cache: ' + error.message);
          });
      });
    },

    vacuumDatabase: function() {
      showConfirm('Vacuum database? This will compact the database file and may take several minutes.').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/database/vacuum', { method: 'POST' })
          .then(function(response) { return response.json(); })
          .then(function(result) {
            if (result.success) {
              Toast.success('Database vacuumed successfully!');
            } else {
              Toast.error('Vacuum failed: ' + result.error);
            }
          })
          .catch(function(error) {
            Toast.error('Error vacuuming database: ' + error.message);
          });
      });
    },

    // SSL/TLS

    testSSLCertificate: function() {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/ssl/verify')
        .then(function(response) { return response.json(); })
        .then(function(result) {
          if (result.valid) {
            Toast.success('Certificate valid! Expires: ' + new Date(result.expires).toLocaleDateString());
          } else {
            Toast.error('Certificate invalid or expired');
          }
        })
        .catch(function(error) {
          Toast.error('Error verifying certificate: ' + error.message);
        });
    },

    obtainCertificate: function() {
      const domain = document.getElementById('ssl-domain').value;
      const email = document.getElementById('ssl-email').value;

      if (!domain || !email) {
        Toast.error('Please enter domain and email');
        return;
      }

      showConfirm('Obtain SSL certificate for ' + domain + '? This requires port 80/443 to be accessible.').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/ssl/obtain', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ domain: domain, email: email, provider: document.getElementById('ssl-provider').value })
        })
          .then(function(response) { return response.json(); })
          .then(function(result) {
            if (result.success) {
              Toast.success('Certificate obtained successfully!');
            } else {
              Toast.error('Failed to obtain certificate: ' + result.error);
            }
          })
          .catch(function(error) {
            Toast.error('Error obtaining certificate: ' + error.message);
          });
      });
    },

    renewCertificate: function() {
      showConfirm('Renew SSL certificate?').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/ssl/renew', { method: 'POST' })
          .then(function(response) { return response.json(); })
          .then(function(result) {
            if (result.success) {
              Toast.success('Certificate renewed successfully!');
            } else {
              Toast.error('Renewal failed: ' + result.error);
            }
          })
          .catch(function(error) {
            Toast.error('Error renewing certificate: ' + error.message);
          });
      });
    },

    // Backup

    createBackup: function() {
      showConfirm('Create backup now? This may take a few moments.').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/server/backup', { method: 'POST' })
          .then(function(response) { return response.json(); })
          .then(function(result) {
            if (result.success) {
              Toast.success('Backup created: ' + result.filename);
              AdminDashboardPage.loadBackupList();
            } else {
              Toast.error('Backup failed: ' + result.error);
            }
          })
          .catch(function(error) {
            Toast.error('Error creating backup: ' + error.message);
          });
      });
    },

    restoreBackup: function() {
      const fileInput = document.getElementById('backup-upload');

      if (!fileInput.files.length) {
        Toast.error('Please select a backup file');
        return;
      }

      showConfirm('⚠️ WARNING: This will replace all current data and restart the server. Continue?', 'Restore Backup').then(function(confirmed) {
        if (!confirmed) return;

        const formData = new FormData();
        formData.append('backup', fileInput.files[0]);

        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/backup/restore', {
          method: 'POST',
          body: formData
        })
          .then(function(response) { return response.json(); })
          .then(function(result) {
            if (result.success) {
              Toast.success('Backup restored! Server restarting...');
              setTimeout(function() { window.location.reload(); }, 3000);
            } else {
              Toast.error('Restore failed: ' + result.error);
            }
          })
          .catch(function(error) {
            Toast.error('Error restoring backup: ' + error.message);
          });
      });
    },

    loadBackupList: function() {
      const adminApiPath = AdminDashboardPage.getData().adminApiPath;
      fetch(adminApiPath + '/server/backup')
        .then(function(response) { return response.json(); })
        .then(function(backups) {
          const html = backups.length
            ? '<table class="admin-table-full"><thead><tr class="admin-table-head-row">' +
              '<th class="admin-table-head-cell">Filename</th>' +
              '<th class="admin-table-head-cell">Size</th>' +
              '<th class="admin-table-head-cell">Date</th>' +
              '<th class="admin-table-head-cell-right">Actions</th>' +
              '</tr></thead><tbody>' +
              backups.map(function(backup) {
                const name = Utils.escapeAttr(backup.filename);
                const encodedName = encodeURIComponent(backup.filename);
                return '<tr class="admin-table-body-row">' +
                  '<td class="admin-table-body-cell admin-logs-cell-mono">' + name + '</td>' +
                  '<td class="admin-table-body-cell">' + AdminDashboardPage.formatBytes(backup.size) + '</td>' +
                  '<td class="admin-table-body-cell">' + new Date(backup.created).toLocaleString() + '</td>' +
                  '<td class="admin-table-body-cell-right">' +
                  '<a href="' + adminApiPath + '/server/backup/' + encodedName + '/download" class="btn btn-sm btn-primary">Download</a>' +
                  '<button data-action="admin-dashboard-delete-backup" data-filename="' + name + '" class="btn btn-sm btn-danger">Delete</button>' +
                  '</td></tr>';
              }).join('') +
              '</tbody></table>'
            : '<p class="text-comment admin-loading-text">No backups available</p>';

          document.getElementById('backup-list').innerHTML = html;
        })
        .catch(function() {
          document.getElementById('backup-list').innerHTML = '<p class="admin-error-text">Error loading backups</p>';
        });
    },

    deleteBackup: function(filename) {
      showConfirm('Delete backup ' + filename + '?', 'Delete Backup').then(function(confirmed) {
        if (!confirmed) return;
        const adminApiPath = AdminDashboardPage.getData().adminApiPath;
        fetch(adminApiPath + '/server/backup/' + filename, { method: 'DELETE' })
          .then(function(response) { return response.json(); })
          .then(function(result) {
            if (result.success) {
              Toast.success('Backup deleted');
              AdminDashboardPage.loadBackupList();
            } else {
              Toast.error('Failed to delete backup: ' + result.error);
            }
          })
          .catch(function(error) {
            Toast.error('Error deleting backup: ' + error.message);
          });
      });
    },

    init: function() {
      if (!document.getElementById('admin-dashboard-data')) return;

      AdminDashboardPage.loadUsers();

      const settingsList = document.getElementById('settings-list');
      if (settingsList) {
        settingsList.addEventListener('change', function(e) {
          const input = e.target.closest('[data-key]');
          if (!input) return;
          if (input.type === 'checkbox') {
            const label = input.closest('.custom-checkbox');
            const labelSpan = label ? label.querySelector('.checkbox-label') : null;
            if (labelSpan) labelSpan.textContent = input.checked ? 'Enabled' : 'Disabled';
          }
          AdminDashboardPage.markSettingChanged(input.dataset.key);
        });
      }
    }
  };

  document.addEventListener('DOMContentLoaded', function() {
    AdminSettingsPage.init();
    AdminDatabasePage.init();
    AdminBackupPage.init();
    AdminEmailEditorPage.init();
    AdminEmailPage.init();
    AdminLogsPage.init();
    AdminMetricsPage.init();
    AdminNotificationsPage.init();
    AdminPasskeyLoginPage.init();
    AdminSchedulerPage.init();
    AdminSecurityPage.init();
    AdminSslPage.init();
    AdminSystemPage.init();
    AdminTasksPage.init();
    AddLocationPage.init();
    ContactPage.init();
    EarthquakeDetailPage.init();
    EarthquakePage.init();
    EditLocationPage.init();
    HealthzPage.init();
    AdminWebPage.init();
    AdminTorPage.init();
    AdminDashboardPage.init();
    LoadingPage.init();
    SecurityPage.init();
    SettingsTokensPage.init();
    SettingsPage.init();
    ProfilePage.init();
    NotificationsListPage.init();

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
        case 'switch-settings-tab':
          AdminSettingsPage.switchTab(btn, btn.dataset.tab);
          break;
        case 'navigate':
          window.location.href = btn.dataset.href;
          break;
        case 'stop-propagation':
          break;
        case 'earthquake-item':
          EarthquakePage.handleItemClick(btn);
          break;
        case 'edit-location-delete':
          EditLocationPage.handleDelete();
          break;
        case 'admin-web-preview-robots':
          AdminWebPage.previewRobotsTxt();
          break;
        case 'admin-web-reset-robots':
          AdminWebPage.resetRobotsTxt();
          break;
        case 'admin-web-preview-security':
          AdminWebPage.previewSecurityTxt();
          break;
        case 'admin-tor-copy-address':
          AdminTorPage.copyAddress();
          break;
        case 'admin-tor-disable':
          AdminTorPage.disableTor();
          break;
        case 'admin-tor-show-regenerate-modal':
          AdminTorPage.showRegenerateModal();
          break;
        case 'admin-tor-enable':
          AdminTorPage.enableTor();
          break;
        case 'admin-tor-generate-vanity':
          AdminTorPage.generateVanity();
          break;
        case 'admin-tor-cancel-vanity':
          AdminTorPage.cancelVanity();
          break;
        case 'admin-tor-apply-vanity':
          AdminTorPage.applyVanity();
          break;
        case 'admin-tor-export-keys':
          AdminTorPage.exportKeys();
          break;
        case 'admin-tor-confirm-regenerate':
          AdminTorPage.confirmRegenerate();
          break;
        case 'admin-tor-close-regenerate-modal':
          AdminTorPage.closeRegenerateModal();
          break;
        case 'delete-admin-passkey':
          AdminSecurityPage.deleteAdminPasskey(btn.dataset.passkeyId, btn.dataset.passkeyName);
          break;
        case 'refresh-system-info':
          AdminSystemPage.refreshSystemInfo();
          break;
        case 'trigger-gc':
          AdminSystemPage.triggerGC();
          break;
        case 'refresh-resource-usage':
          AdminSystemPage.refreshResourceUsage();
          break;
        case 'reload-config':
          AdminSystemPage.reloadConfig();
          break;
        case 'view-all-routes':
          AdminSystemPage.viewAllRoutes();
          break;
        case 'export-system-info':
          AdminSystemPage.exportSystemInfo();
          break;
        case 'trigger-task':
          AdminTasksPage.triggerTask(btn.dataset.name);
          break;
        case 'toggle-task':
          AdminTasksPage.toggleTask(btn.dataset.name, btn.dataset.enabled === 'true');
          break;
        case 'reset-admin-notification-prefs':
          AdminNotificationsPage.loadPreferences();
          break;
        case 'db-refresh-stats':
          AdminDatabasePage.refreshStats();
          break;
        case 'db-optimize':
          AdminDatabasePage.optimizeDatabase();
          break;
        case 'db-vacuum':
          AdminDatabasePage.vacuumDatabase();
          break;
        case 'db-test-connection':
          AdminDatabasePage.testConnection();
          break;
        case 'db-clear-cache':
          AdminDatabasePage.clearCache();
          break;
        case 'db-refresh-cache-stats':
          AdminDatabasePage.refreshCacheStats();
          break;
        case 'db-toggle-password':
          AdminDatabasePage.togglePasswordVisibility();
          break;
        case 'db-test-connection-config':
          AdminDatabasePage.testDatabaseConnectionConfig();
          break;
        case 'db-cancel-config':
          AdminDatabasePage.cancelDatabaseConfig();
          break;
        case 'modal-close':
        case 'modal-cancel': {
          const overlay = btn.closest('.modal-overlay');
          if (overlay) Modal.close(overlay.id);
          break;
        }
        case 'add-location-use-current':
          AddLocationPage.useCurrentLocation();
          break;
        case 'add-location-switch-tab':
          AddLocationPage.switchTab(btn.dataset.tab, btn);
          break;
        case 'add-location-lookup-zip':
          AddLocationPage.lookupZipCode();
          break;
        case 'add-location-lookup-coords':
          AddLocationPage.lookupCoordinates();
          break;
        case 'add-location-reset':
          AddLocationPage.resetForm();
          break;
        case 'add-location-select-result':
          AddLocationPage.selectSearchResult(parseInt(btn.dataset.index, 10));
          break;
        case 'copy-command': {
          const text = btn.dataset.text;
          navigator.clipboard.writeText(text).then(function() {
            const originalText = btn.textContent;
            btn.textContent = '✓ Copied!';
            btn.classList.add('copied');
            setTimeout(function() {
              btn.textContent = originalText;
              btn.classList.remove('copied');
            }, 2000);
          }).catch(function(err) {
            console.error('Failed to copy:', err);
          });
          break;
        }
        case 'copy-to-clipboard': {
          const text = btn.dataset.text;
          navigator.clipboard.writeText(text).then(function() {
            Toast.success('Copied to clipboard!');
          }).catch(function() {
            Toast.error('Failed to copy');
          });
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
        case 'admin-dashboard-switch-tab':
          AdminDashboardPage.switchTab(btn.dataset.tab);
          break;
        case 'admin-dashboard-show-add-user-modal':
          AdminDashboardPage.showAddUserModal();
          break;
        case 'admin-dashboard-edit-admin-user':
          AdminDashboardPage.editAdminUser(btn.dataset.userId, btn.dataset.userEmail);
          break;
        case 'admin-dashboard-delete-user':
          AdminDashboardPage.deleteUser(btn.dataset.userId);
          break;
        case 'admin-dashboard-update-admin-user':
          AdminDashboardPage.updateAdminUser(btn.dataset.userId, btn.dataset.modalId);
          break;
        case 'admin-dashboard-create-user-from-modal':
          AdminDashboardPage.createUserFromModal(btn.dataset.modalId);
          break;
        case 'admin-dashboard-apply-all-settings':
          AdminDashboardPage.applyAllSettings();
          break;
        case 'admin-dashboard-revoke-token':
          AdminDashboardPage.revokeToken(btn.dataset.tokenId);
          break;
        case 'admin-dashboard-show-generate-token-modal':
          AdminDashboardPage.showGenerateTokenModal();
          break;
        case 'admin-dashboard-generate-token-from-modal':
          AdminDashboardPage.generateTokenFromModal(btn.dataset.modalId);
          break;
        case 'admin-dashboard-copy-token':
          AdminDashboardPage.copyToken(btn.dataset.token);
          break;
        case 'admin-dashboard-clear-audit-logs':
          AdminDashboardPage.clearAuditLogs();
          break;
        case 'admin-dashboard-confirm-clear-logs':
          AdminDashboardPage.confirmClearLogs(btn.dataset.modalId);
          break;
        case 'admin-dashboard-trigger-task':
          AdminDashboardPage.triggerTask(btn.dataset.taskName);
          break;
        case 'admin-dashboard-enable-task':
          AdminDashboardPage.enableTask(btn.dataset.taskName);
          break;
        case 'admin-dashboard-disable-task':
          AdminDashboardPage.disableTask(btn.dataset.taskName);
          break;
        case 'admin-dashboard-view-task-history':
          AdminDashboardPage.viewTaskHistory(btn.dataset.taskName);
          break;
        case 'admin-dashboard-save-web-settings':
          AdminDashboardPage.saveWebSettings();
          break;
        case 'admin-dashboard-save-security-settings':
          AdminDashboardPage.saveSecuritySettings();
          break;
        case 'admin-dashboard-test-database-connection':
          AdminDashboardPage.testDatabaseConnection();
          break;
        case 'admin-dashboard-optimize-database':
          AdminDashboardPage.optimizeDatabase();
          break;
        case 'admin-dashboard-clear-cache':
          AdminDashboardPage.clearCache();
          break;
        case 'admin-dashboard-vacuum-database':
          AdminDashboardPage.vacuumDatabase();
          break;
        case 'admin-dashboard-test-ssl-certificate':
          AdminDashboardPage.testSSLCertificate();
          break;
        case 'admin-dashboard-obtain-certificate':
          AdminDashboardPage.obtainCertificate();
          break;
        case 'admin-dashboard-renew-certificate':
          AdminDashboardPage.renewCertificate();
          break;
        case 'admin-dashboard-create-backup':
          AdminDashboardPage.createBackup();
          break;
        case 'admin-dashboard-restore-backup':
          AdminDashboardPage.restoreBackup();
          break;
        case 'admin-dashboard-delete-backup':
          AdminDashboardPage.deleteBackup(btn.dataset.filename);
          break;
        case 'loading-retry':
          LoadingPage.retryConnection();
          break;
        case 'nav-dropdown-toggle':
          Dropdown.toggle(btn.dataset.dropdownId, btn.dataset.triggerId);
          break;
        case 'nav-notifications-mark-all-read':
          Notifications.markAllRead();
          break;
        case 'nav-theme-set':
          Theme.set(btn.dataset.theme);
          break;
        case 'toggle-alert-details':
          (function() {
            const details = btn.querySelector('.alert-details');
            const expand = btn.querySelector('.alert-expand');
            if (details && expand) {
              if (details.classList.contains('display-none')) {
                details.classList.remove('display-none');
                expand.textContent = '▲ Click to collapse';
              } else {
                details.classList.add('display-none');
                expand.textContent = '▼ Click to expand';
              }
            }
          })();
          break;
        case 'regenerate-recovery-keys':
          SecurityPage.regenerateRecoveryKeys();
          break;
        case 'disable-2fa':
          SecurityPage.disable2FA();
          break;
        case 'enable-2fa':
          SecurityPage.enable2FA();
          break;
        case 'register-passkey':
          SecurityPage.registerPasskey();
          break;
        case 'delete-passkey':
          SecurityPage.deletePasskey(btn.dataset.passkeyId, btn.dataset.passkeyName);
          break;
        case 'close-setup-2fa-modal':
          SecurityPage.closeSetupModal();
          break;
        case 'setup-2fa-next-step':
          SecurityPage.nextSetupStep();
          break;
        case 'setup-2fa-prev-step':
          SecurityPage.prevSetupStep();
          break;
        case 'download-recovery-keys':
          SecurityPage.downloadRecoveryKeys();
          break;
        case 'finish-2fa-setup':
          SecurityPage.finishSetup();
          break;
        case 'close-disable-2fa-modal':
          SecurityPage.closeDisableModal();
          break;
        case 'tokens-show-new-modal':
          SettingsTokensPage.showNewModal();
          break;
        case 'tokens-close-new-modal':
          SettingsTokensPage.closeNewModal();
          break;
        case 'tokens-close-created-modal':
          SettingsTokensPage.closeCreatedModal();
          break;
        case 'tokens-copy-created':
          SettingsTokensPage.copyCreatedToken();
          break;
        case 'tokens-revoke':
          SettingsTokensPage.revokeToken(btn.dataset.tokenId);
          break;
        case 'settings-reset-form':
          SettingsPage.resetForm();
          break;
        case 'profile-reset-form':
          ProfilePage.resetForm();
          break;
        case 'profile-load-notification-prefs':
          ProfilePage.loadNotificationPreferences();
          break;
        case 'notifications-list-mark-read':
          NotificationsListPage.markAsRead(btn.dataset.id);
          break;
        case 'notifications-list-delete':
          NotificationsListPage.deleteNotification(btn.dataset.id);
          break;
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
