/**
 * Weather Service - Application JavaScript
 * Provides utilities for modals, alerts, toasts, and interactive components
 */

(function() {
  'use strict';

  // ============================================
  // API PATHS
  // ============================================

  // Rendered by partial/head.tmpl as a JSON data island; CSP forbids inline executable script.
  (function readApiPaths() {
    const el = document.getElementById('api-paths-data');
    if (!el) return;
    try {
      window.API_PATHS = JSON.parse(el.textContent);
    } catch (err) {
      console.error('Failed to parse api-paths-data:', err);
    }
  })();

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
     * Escape a string for safe interpolation into an HTML fragment
     */
    escapeHtml: function(text) {
      const div = document.createElement('div');
      div.textContent = text == null ? '' : String(text);
      return div.innerHTML;
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
    },

    setErrorMessage: function(container, className, message) {
      if (!container) return;
      const paragraph = document.createElement('p');
      paragraph.className = className;
      paragraph.textContent = message;
      container.replaceChildren(paragraph);
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
      const safeSize = ['sm', 'md', 'lg', 'xl'].includes(size) ? size : 'md';

      const modalId = Utils.generateId();
      const autoCloseHtml = autoClose > 0
        ? `<div class="modal-auto-close">Auto-closing in <span id="${modalId}-countdown">${autoClose}</span>s</div>`
        : '';

      const modalHTML = `
        <div id="${modalId}" class="modal-overlay">
          <div class="modal modal-${safeSize}">
            <div class="modal-header">
              <h3 class="modal-title">${Utils.escapeHtml(title)}</h3>
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
      const safeType = ['success', 'error', 'warning', 'info'].includes(type) ? type : 'info';

      const icons = {
        success: '✓',
        error: '✗',
        warning: '⚠',
        info: 'ℹ'
      };

      const toastId = Utils.generateId();
      const toastHTML = `
        <div id="${toastId}" class="toast toast-${safeType}">
          <div class="toast-icon">${icons[safeType]}</div>
          <div class="toast-content">
            ${title ? `<div class="toast-title">${Utils.escapeHtml(title)}</div>` : ''}
            <div class="toast-message">${Utils.escapeHtml(message)}</div>
          </div>
          ${dismissible ? '<button class="toast-close" data-action="toast-dismiss" aria-label="Dismiss">&times;</button>' : ''}
        </div>
      `;

      this.container.insertAdjacentHTML('beforeend', toastHTML);

      if (duration > 0) {
        setTimeout(() => this.dismiss(toastId), duration);
      }

      Utils.dispatchEvent('toast:shown', { toastId, type: safeType, message });
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
      const safeType = ['success', 'error', 'warning', 'info'].includes(type) ? type : 'info';

      const icons = {
        success: '✓',
        error: '✗',
        warning: '⚠',
        info: 'ℹ'
      };

      const alertId = Utils.generateId();
      const alertHTML = `
        <div id="${alertId}" class="alert alert-${safeType} ${dismissible ? 'alert-dismissible' : ''}">
          <div class="alert-icon">${icons[safeType]}</div>
          <div class="alert-content">
            ${title ? `<div class="alert-title">${Utils.escapeHtml(title)}</div>` : ''}
            <div class="alert-message">${Utils.escapeHtml(message)}</div>
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

  // Locale: strings from /locales/{lang}.json (AI.md PART 31), English fallback when a key or the file is missing
  const Locale = {
    messages: null,
    pending: null,

    lang: function() {
      return (document.documentElement.lang || 'en').split('-')[0];
    },

    load: function() {
      if (Locale.messages) {
        return Promise.resolve(Locale.messages);
      }
      if (!Locale.pending) {
        Locale.pending = fetch('/locales/' + encodeURIComponent(Locale.lang()) + '.json')
          .then(function(response) { return response.ok ? response.json() : {}; })
          .catch(function() { return {}; })
          .then(function(messages) { Locale.messages = messages || {}; return Locale.messages; });
      }
      return Locale.pending;
    },

    t: function(key, fallback) {
      const value = Locale.messages && Locale.messages[key];
      return typeof value === 'string' && value !== '' ? value : fallback;
    },

    ago: function(seconds) {
      try {
        const formatter = new Intl.RelativeTimeFormat(Locale.lang(), { numeric: 'auto' });
        if (seconds < 60) return formatter.format(0, 'second');
        if (seconds < 3600) return formatter.format(-Math.floor(seconds / 60), 'minute');
        if (seconds < 86400) return formatter.format(-Math.floor(seconds / 3600), 'hour');
        return formatter.format(-Math.floor(seconds / 86400), 'day');
      } catch (e) {
        return '';
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
        await Locale.load();
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
        list.innerHTML = '<div class="notification-empty">' + Utils.escapeHtml(Locale.t('nav.no_notifications', 'No notifications')) + '</div>';
        return;
      }

      list.innerHTML = this.notifications.map(function(n) {
        var icon = n.type === 'error' ? '❌' : n.type === 'warning' ? '⚠️' : n.type === 'success' ? '✅' : 'ℹ️';
        var unreadClass = n.read ? '' : ' unread';
        var timeAgo = Notifications.formatTimeAgo(n.created_at);
        var link = typeof n.link === 'string' && (/^https?:\/\//i.test(n.link) || (n.link.charAt(0) === '/' && n.link.charAt(1) !== '/' && n.link.charAt(1) !== '\\'))
          ? n.link
          : '/users/notifications';
        return '<a href="' + Utils.escapeAttr(link) + '" class="notification-item' + unreadClass + '" data-id="' + Utils.escapeAttr(n.id) + '">' +
          '<span class="notification-dot"></span>' +
          '<span class="notification-icon">' + icon + '</span>' +
          '<div class="notification-content">' +
            '<span class="notification-title">' + Utils.escapeHtml(n.title || Locale.t('notifications.default_title', 'Notification')) + '</span>' +
            '<span class="notification-message">' + Utils.escapeHtml(n.message || '') + '</span>' +
            '<span class="notification-time">' + Utils.escapeHtml(timeAgo) + '</span>' +
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

      return Locale.ago(diff);
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
          Toast.show(Locale.t('notifications.all_marked_read', 'All notifications marked as read'), 'success');
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
    // A checkbox that shows/hides another element, optionally toggling `required`
    // on the fields inside it. data-toggle-invert means "checked hides the target".
    function applyCheckboxToggle(box) {
      const target = document.querySelector(box.dataset.toggleTarget);
      if (!target) return;
      const invert = box.dataset.toggleInvert === 'true';
      const visible = invert ? !box.checked : box.checked;
      target.classList.toggle('hidden', !visible);
      if (box.dataset.toggleRequired === 'true') {
        target.querySelectorAll('input, select, textarea').forEach(function(field) {
          if (visible) {
            field.setAttribute('required', 'required');
          } else {
            field.removeAttribute('required');
          }
        });
      }
    }
    document.querySelectorAll('input[type="checkbox"][data-toggle-target]').forEach(applyCheckboxToggle);

    // A checkbox that gates another control (e.g. "I saved my token" -> enable Continue)
    function applyCheckboxGate(box) {
      const target = document.querySelector(box.dataset.enables);
      if (target) target.disabled = !box.checked;
    }
    document.querySelectorAll('input[type="checkbox"][data-enables]').forEach(applyCheckboxGate);

    document.addEventListener('change', function(e) {
      const box = e.target.closest('input[type="checkbox"]');
      if (!box) return;
      if (box.dataset.toggleTarget) applyCheckboxToggle(box);
      if (box.dataset.enables) applyCheckboxGate(box);
    });

    // Confirmation fields validate against their partner natively so the browser
    // blocks submission and :user-invalid styling applies - no alert() needed.
    document.addEventListener('input', function(e) {
      const field = e.target.closest('input[data-match-with]');
      if (!field) return;
      const partner = document.querySelector(field.dataset.matchWith);
      if (!partner) return;
      field.setCustomValidity(field.value === partner.value ? '' : (field.dataset.mismatchMessage || 'Values do not match'));
    });

    // PART 16: submit buttons disable immediately and show a loading label
    document.addEventListener('submit', function(e) {
      const form = e.target;
      if (e.defaultPrevented) return;
      const btn = form.querySelector('button[type="submit"][data-loading-label]');
      if (!btn) return;
      btn.disabled = true;
      btn.textContent = btn.dataset.loadingLabel;
    });

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
        body: `<p class="modal-body-text">${Utils.escapeHtml(message)}</p>`,
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
        body: `<p class="modal-body-text">${Utils.escapeHtml(message)}</p>`,
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
          <p class="modal-body-text-spacing">${Utils.escapeHtml(message)}</p>
          <input type="text" id="${inputId}" class="modal-input-full" value="${Utils.escapeAttr(defaultValue)}"
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

  // Pages whose data goes stale opt in with <body data-auto-refresh-seconds="N">
  (function autoRefreshPage() {
    const seconds = parseInt(document.body.dataset.autoRefreshSeconds, 10);
    if (!seconds || seconds <= 0) return;
    setTimeout(function() { window.location.reload(); }, seconds * 1000);
  })();

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
      return Array.from(rows).filter(function(row) {
        const remove = row.querySelector('[name="oidc_provider_remove"]');
        if (remove && remove.checked) return false;
        return Array.from(row.querySelectorAll('input')).some(function(input) {
          return input.name !== 'oidc_provider_remove' && input.value.trim() !== '';
        });
      }).map(function(row) {
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

    saveCacheConfig: async function(form) {
      const field = function(name) {
        return form.elements.namedItem(name);
      };
      const value = function(name) {
        const input = field(name);
        return input ? input.value.trim() : '';
      };
      const checked = function(name) {
        const input = field(name);
        return Boolean(input && input.checked);
      };
      const clearUrl = checked('clear_url');
      const clearUsername = checked('clear_username');
      const clearPassword = checked('clear_password');
      const body = {
        type: value('type'),
        url: clearUrl ? '' : value('url'),
        host: value('host'),
        port: Number(value('port')),
        username: clearUsername ? '' : value('username'),
        password: clearPassword ? '' : value('password'),
        db: Number(value('db')),
        tls: checked('tls'),
        tls_skip_verify: checked('tls_skip_verify'),
        pool_size: Number(value('pool_size')),
        min_idle: Number(value('min_idle')),
        timeout: value('timeout'),
        prefix: value('prefix'),
        ttl: value('ttl'),
        cluster: checked('cluster'),
        cluster_nodes: value('cluster_nodes'),
        clear_url: clearUrl,
        clear_username: clearUsername,
        clear_password: clearPassword
      };
      const result = form.querySelector('[data-cache-config-result]');
      const submit = form.querySelector('button[type="submit"]');
      if (submit) submit.disabled = true;
      if (result) {
        result.classList.add('hidden');
        result.textContent = '';
      }

      try {
        const headers = { 'Content-Type': 'application/json' };
        const csrf = field('csrf_token');
        if (csrf && csrf.value) headers['X-CSRF-Token'] = csrf.value;
        const response = await fetch(AdminSettingsPage.getData().adminApiPath + '/settings/cache', {
          method: 'PUT',
          headers: headers,
          body: JSON.stringify(body)
        });
        const payload = await response.json().catch(function() { return {}; });
        if (!response.ok || payload.ok === false) throw new Error('cache configuration request failed');

        const data = payload.data || payload;
        const restart = data.requires_restart !== undefined ? data.requires_restart : data.restart_required;
        const restartRequired = Array.isArray(restart) ? restart.length > 0 : Boolean(restart);
        const message = restartRequired
          ? form.dataset.saveSuccess + ' ' + form.dataset.restartRequired
          : form.dataset.saveSuccess;
        if (result) {
          result.textContent = message;
          result.classList.remove('hidden');
        }
        if (restartRequired) Toast.info(message);
        else Toast.success(message);
      } catch {
        const message = form.dataset.saveError;
        if (result) {
          result.textContent = message;
          result.classList.remove('hidden');
        }
        Toast.error(message);
      } finally {
        if (submit) submit.disabled = false;
      }
    },

    saveRateLimitConfig: async function(form) {
      const body = {};
      for (const element of form.elements) {
        if (!element.name || element.name === 'csrf_token') continue;
        body[element.name] = element.type === 'checkbox' ? element.checked : Number(element.value);
      }
      const headers = { 'Content-Type': 'application/json' };
      const csrf = form.elements.namedItem('csrf_token');
      if (csrf && csrf.value) headers['X-CSRF-Token'] = csrf.value;
      const submit = form.querySelector('button[type="submit"]');
      if (submit) submit.disabled = true;
      try {
        const response = await fetch(AdminSettingsPage.getData().adminApiPath + '/settings/rate-limit', {
          method: 'PUT',
          headers: headers,
          body: JSON.stringify(body)
        });
        const payload = await response.json().catch(function() { return {}; });
        if (!response.ok || payload.ok === false) throw new Error('rate limit update failed');
        Toast.success(form.dataset.saveSuccess);
      } catch {
        Toast.error(form.dataset.saveError);
      } finally {
        if (submit) submit.disabled = false;
      }
    },

    init: function() {
      document.querySelectorAll('form[data-settings-category]').forEach(function(form) {
        form.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminSettingsPage.saveSettings(form, form.dataset.settingsCategory);
        });
      });
      document.querySelectorAll('form[data-cache-config-form]').forEach(function(form) {
        form.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminSettingsPage.saveCacheConfig(form);
        });
      });
      document.querySelectorAll('form[data-rate-limit-config-form]').forEach(function(form) {
        form.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminSettingsPage.saveRateLimitConfig(form);
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
      const btn = document.getElementById('vacuum-btn');
      const queuedMessage = document.getElementById('database-vacuum-queued-message');
      const failedMessage = document.getElementById('database-vacuum-failed-message');
      const form = btn.closest('form');
      btn.disabled = true;
      const originalLabel = btn.textContent;
      btn.textContent = '⏳ ' + originalLabel;
      fetch(form.action, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams(new FormData(form))
      })
        .then(function(response) {
          if (!response.ok) throw new Error(failedMessage ? failedMessage.textContent.trim() : '');
          AdminDatabasePage.showOperationResult('✅ ' + (queuedMessage ? queuedMessage.textContent.trim() : ''), 'success');
        })
        .catch(function(error) {
          AdminDatabasePage.showOperationResult('❌ ' + error.message, 'error');
        })
        .finally(function() {
          btn.disabled = false;
          btn.textContent = originalLabel;
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
      return AdminBackupPage.getData().adminApiPath + '/backup';
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
          if (schedule.restore) {
            document.getElementById('restoreMaxArchiveSize').value = schedule.restore.max_archive_size;
            document.getElementById('restoreMaxExpandedSize').value = schedule.restore.max_expanded_size;
            document.getElementById('restoreMaxFiles').value = schedule.restore.max_files;
            document.getElementById('restoreMaxFileSize').value = schedule.restore.max_file_size;
            document.getElementById('restoreMaxRatio').value = schedule.restore.max_ratio;
          }
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
        retention: parseInt(document.getElementById('retention').value, 10),
        restore_max_archive_size: document.getElementById('restoreMaxArchiveSize').value,
        restore_max_expanded_size: document.getElementById('restoreMaxExpandedSize').value,
        restore_max_files: parseInt(document.getElementById('restoreMaxFiles').value, 10),
        restore_max_file_size: document.getElementById('restoreMaxFileSize').value,
        restore_max_ratio: parseInt(document.getElementById('restoreMaxRatio').value, 10)
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

      const data = AdminEmailEditorPage.getData();
      document.getElementById('previewSubject').textContent = previewSubject || data.subjectPlaceholder;
      document.getElementById('previewBody').textContent = previewBody || data.bodyPlaceholder;
    },

    reloadTemplate: function() {
      const data = AdminEmailEditorPage.getData();
      showConfirm(data.reloadConfirm, data.reloadTitle)
        .then(function(confirmed) {
          if (!confirmed) return;
          window.location.reload();
        });
    },

    init: function() {
      if (!document.getElementById('admin-email-editor-data')) return;

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

      const reloadBtn = document.getElementById('reloadBtn');
      if (reloadBtn) {
        reloadBtn.addEventListener('click', AdminEmailEditorPage.reloadTemplate);
      }

      AdminEmailEditorPage.updatePreview();
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
      result.classList.remove('d-none');

      setTimeout(function() {
        result.classList.add('d-none');
      }, 5000);
    },

    init: function() {
      if (!document.getElementById('admin-email-data')) return;

      AdminEmailPage.loadSettings();
      AdminEmailPage.toggleSMTPFields();

      document.getElementById('smtp_enabled').addEventListener('change', AdminEmailPage.toggleSMTPFields);

      document.getElementById('smtp-form').addEventListener('submit', function(e) {
        if (e.submitter && e.submitter.id === 'test-email-btn') {
          e.preventDefault();
          AdminEmailPage.sendTestEmail();
          return;
        }
        if (e.submitter && e.submitter.id === 'autodetect-btn') {
          e.preventDefault();
          AdminEmailPage.autodetectSMTP();
          return;
        }
        e.preventDefault();
        AdminEmailPage.submitSMTPForm(e.target);
      });

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

        const level = String(log.level || 'INFO');
        const safeLevel = ['TRACE', 'DEBUG', 'INFO', 'WARN', 'WARNING', 'ERROR', 'FATAL'].includes(level.toUpperCase()) ? level.toLowerCase() : 'info';
        html += '<div class="log-entry">' +
          '<span class="log-timestamp">' + AdminLogsPage.escapeHtml(log.timestamp) + '</span>' +
          '<span class="log-level log-level-' + safeLevel + '">' + AdminLogsPage.escapeHtml(level) + '</span>' +
          '<span class="log-message">' + message + '</span>' +
          '</div>';
      });

      container.innerHTML = html;
      container.scrollTop = container.scrollHeight;
    },

    showNoLogs: function(message) {
      const container = document.getElementById('logsContainer');
      const paragraph = document.createElement('p');
      paragraph.textContent = message;
      const emptyState = document.createElement('div');
      emptyState.className = 'no-logs';
      emptyState.appendChild(paragraph);
      container.replaceChildren(emptyState);
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

    /**
     * Base path of the admin metrics API, derived from the rendered payload.
     */
    apiBase: function() {
      return AdminMetricsPage.getData().adminApiPath + '/server/metrics';
    },

    /**
     * Populate the live registry counters from the /stats endpoint.
     */
    loadStats: function() {
      fetch(AdminMetricsPage.apiBase() + '/stats')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          document.getElementById('totalMetrics').textContent = data.families || 0;
          document.getElementById('seriesMetrics').textContent = data.series || 0;
        })
        .catch(function() {
          AdminMetricsPage.showError(AdminMetricsPage.getData().msgLoadError);
        });
    },

    /**
     * Load the stored metrics settings into the form. Configured tokens come
     * back masked, so submitting the mask unchanged preserves the stored value.
     */
    loadSettings: function() {
      fetch(AdminMetricsPage.apiBase() + '/config')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          const tokens = data.tokens || {};

          document.getElementById('metricsEnabled').checked = !!data.enabled;
          document.getElementById('rootAliasEnabled').checked = !!data.rootAliasEnabled;
          document.getElementById('allowUnauthenticated').checked = !!data.allowUnauthenticated;
          document.getElementById('includeSystem').checked = !!data.includeSystem;
          document.getElementById('includeRuntime').checked = !!data.includeRuntime;
          document.getElementById('lokiMaxEntries').value = data.lokiMaxEntries || 0;
          document.getElementById('lokiMaxAge').value = data.lokiMaxAge || '';
          document.getElementById('tokenPrometheus').value = tokens.prometheus || '';
          document.getElementById('tokenGrafana').value = tokens.grafana || '';
          document.getElementById('tokenLoki').value = tokens.loki || '';

          AdminMetricsPage.renderIntegration(!!data.rootAliasEnabled);
        })
        .catch(function() {
          AdminMetricsPage.showError(AdminMetricsPage.getData().msgLoadError);
        });
    },

    /**
     * Show the scrape target using the host the admin is already browsing,
     * never a hardcoded loopback or placeholder address.
     */
    renderIntegration: function(rootAliasEnabled) {
      const path = rootAliasEnabled ? '/metrics' : AdminMetricsPage.getData().apiPath + '/server/metrics';
      const endpoint = window.location.origin + path;

      document.getElementById('metricsEndpointDisplay').textContent = endpoint;
      document.getElementById('scrapeConfigDisplay').textContent =
        'scrape_configs:\n' +
        '  - job_name: wthr\n' +
        '    metrics_path: ' + path + '\n' +
        '    scheme: ' + window.location.protocol.replace(':', '') + '\n' +
        '    static_configs:\n' +
        '      - targets: [' + window.location.host + ']';
    },

    /**
     * Render the live registry contents into the registered-metrics table.
     */
    loadRegistered: function() {
      fetch(AdminMetricsPage.apiBase() + '/list')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          const body = document.getElementById('registeredMetricsBody');
          const metrics = data.metrics || [];

          body.textContent = '';

          if (metrics.length === 0) {
            const row = document.createElement('tr');
            const cell = document.createElement('td');
            cell.colSpan = 4;
            cell.className = 'text-center text-muted p-3';
            cell.textContent = AdminMetricsPage.getData().registeredEmpty;
            row.appendChild(cell);
            body.appendChild(row);
            return;
          }

          metrics.forEach(function(metric) {
            const row = document.createElement('tr');
            [metric.name, metric.type, metric.series, metric.help].forEach(function(value) {
              const cell = document.createElement('td');
              cell.textContent = value === undefined || value === null ? '' : String(value);
              row.appendChild(cell);
            });
            body.appendChild(row);
          });
        })
        .catch(function() {
          AdminMetricsPage.showError(AdminMetricsPage.getData().msgLoadError);
        });
    },

    /**
     * Persist the metrics settings. Token fields left at their masked value
     * are sent back unchanged and the server keeps the stored secret.
     */
    saveConfig: function(e) {
      e.preventDefault();

      const form = e.target;
      const submitBtn = form.querySelector('button[type="submit"]');
      const savingLabel = submitBtn ? submitBtn.dataset.labelSaving : '';
      const savedLabel = submitBtn ? submitBtn.dataset.labelSave : '';

      if (submitBtn) {
        submitBtn.disabled = true;
        if (savingLabel) submitBtn.textContent = savingLabel;
      }

      const settings = {
        enabled: document.getElementById('metricsEnabled').checked,
        rootAliasEnabled: document.getElementById('rootAliasEnabled').checked,
        allowUnauthenticated: document.getElementById('allowUnauthenticated').checked,
        includeSystem: document.getElementById('includeSystem').checked,
        includeRuntime: document.getElementById('includeRuntime').checked,
        lokiMaxEntries: parseInt(document.getElementById('lokiMaxEntries').value, 10) || 0,
        lokiMaxAge: document.getElementById('lokiMaxAge').value,
        tokens: {
          prometheus: document.getElementById('tokenPrometheus').value,
          grafana: document.getElementById('tokenGrafana').value,
          loki: document.getElementById('tokenLoki').value
        }
      };

      fetch(AdminMetricsPage.apiBase() + '/config', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': AdminMetricsPage.getData().csrfToken
        },
        body: JSON.stringify(settings)
      })
        .then(function(response) {
          if (!response.ok) throw new Error('save failed');
          AdminMetricsPage.showSuccess(form.dataset.msgSaved);
          AdminMetricsPage.loadSettings();
        })
        .catch(function() {
          AdminMetricsPage.showError(form.dataset.msgError);
        })
        .finally(function() {
          if (submitBtn) {
            submitBtn.disabled = false;
            if (savedLabel) submitBtn.textContent = savedLabel;
          }
        });
    },

    /**
     * Verify the admin export endpoint responds, which exercises the same
     * gatherer the public exposition endpoint uses without needing a token.
     */
    testEndpoint: function(e) {
      const button = e.target;

      fetch(AdminMetricsPage.apiBase() + '/export')
        .then(function(response) {
          if (response.ok) {
            AdminMetricsPage.showSuccess(button.dataset.msgSuccess);
          } else {
            AdminMetricsPage.showError(button.dataset.msgError);
          }
        })
        .catch(function() {
          AdminMetricsPage.showError(button.dataset.msgError);
        });
    },

    /**
     * Fetch one export format into the preview pane.
     */
    exportFormat: function(format) {
      const query = format ? '?format=' + encodeURIComponent(format) : '';

      fetch(AdminMetricsPage.apiBase() + '/export' + query)
        .then(function(response) {
          if (!response.ok) throw new Error('export failed');
          return response.text();
        })
        .then(function(text) {
          document.getElementById('exportPreview').textContent = text;
        })
        .catch(function() {
          AdminMetricsPage.showError(AdminMetricsPage.getData().msgExportError);
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
      document.getElementById('exportPrometheusBtn').addEventListener('click', function() {
        AdminMetricsPage.exportFormat('');
      });
      document.getElementById('exportJsonBtn').addEventListener('click', function() {
        AdminMetricsPage.exportFormat('json');
      });
      document.getElementById('exportOpenMetricsBtn').addEventListener('click', function() {
        AdminMetricsPage.exportFormat('openmetrics');
      });

      AdminMetricsPage.loadStats();
      AdminMetricsPage.loadSettings();
      AdminMetricsPage.loadRegistered();
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
      if (e.submitter && e.submitter.name === 'reset') return;
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
      box.textContent = msg;
    },

    run: function() {
      const pendingToken = AdminPasskeyLoginPage.getData().pendingToken;
      const adminPath = AdminPasskeyLoginPage.getData().adminPath;

      if (!pendingToken) {
        AdminPasskeyLoginPage.setStatus('Session expired or invalid. Please log in again at /server/auth/login.', 'status-error');
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
          AdminPasskeyLoginPage.setStatus('Touch your security key or use your device passkey to continue…', 'status-info');
          return navigator.credentials.get({
            publicKey: AdminPasskeyLoginPage.normalizeOptions(challengeData.options),
          });
        })
        .then(function(credential) {
          if (!credential) {
            throw new Error('Passkey authentication was cancelled');
          }
          AdminPasskeyLoginPage.setStatus('Verifying credential…', 'status-info');
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
            window.location.href = '/' + adminPath;
          });
        })
        .catch(function(err) {
          AdminPasskeyLoginPage.setStatus('Passkey authentication failed: ' + (err.message || String(err)) +
            ' Try again at /server/auth/login.', 'status-error');
        });
    },

    init: function() {
      if (!document.getElementById('admin-passkey-login-data')) return;
      AdminPasskeyLoginPage.run();
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
                'data-passkey-id="' + Utils.escapeAttr(id) + '" ' +
                'data-passkey-name="' + Utils.escapeAttr(name) + '" ' +
                'aria-label="Delete passkey ' + Utils.escapeAttr(name) + '"' +
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

    message: function(key) {
      return AdminSslPage.getData().messages[key] || '';
    },

    api: function(path, options) {
      const data = AdminSslPage.getData();
      const settings = options || {};
      const headers = Object.assign({ 'Accept': 'application/json', 'X-CSRF-Token': data.csrfToken }, settings.headers || {});
      if (settings.body) headers['Content-Type'] = 'application/json';
      return fetch(data.adminApiPath + '/ssl' + path, Object.assign({}, settings, { headers: headers, credentials: 'same-origin' }));
    },

    failureText: function(body) {
      let text = (body && body.message) || AdminSslPage.message('request_failed');
      const variables = body && body.details && body.details.variables;
      if (variables && variables.length) {
        text += ' ' + AdminSslPage.message('check_variables') + ' ' + variables.join(', ');
      }
      return text;
    },

    loadCertStatus: function() {
      AdminSslPage.api('/')
        .then(function(response) { return response.json(); })
        .then(AdminSslPage.renderCertStatus)
        .catch(function() {
          Toast.show(AdminSslPage.message('load_failed'), 'error');
        });
    },

    renderCertStatus: function(data) {
      const container = document.getElementById('certStatus');
      container.textContent = '';

      if (!data.certificate) {
        const empty = document.createElement('p');
        empty.className = 'text-center p-4 text-comment';
        empty.textContent = AdminSslPage.message('no_certificate');
        container.appendChild(empty);
        return;
      }

      const cert = data.certificate;
      const rows = [
        [AdminSslPage.message('subject'), cert.subject || '-'],
        [AdminSslPage.message('issuer'), cert.issuer || '-'],
        [AdminSslPage.message('valid_until'), new Date(cert.notAfter).toLocaleDateString()],
        [AdminSslPage.message('days_remaining'), String(cert.daysRemaining)]
      ];
      const grid = document.createElement('div');
      grid.className = 'info-grid';
      rows.forEach(function(row) {
        const line = document.createElement('div');
        line.className = 'info-row';
        const label = document.createElement('span');
        label.className = 'info-label';
        label.textContent = row[0];
        const value = document.createElement('span');
        value.className = 'info-value';
        value.textContent = row[1];
        line.appendChild(label);
        line.appendChild(value);
        grid.appendChild(line);
      });
      container.appendChild(grid);
    },

    submitLetsEncryptForm: function(e) {
      e.preventDefault();
      const obtainBtn = document.getElementById('obtainBtn');
      obtainBtn.disabled = true;

      const body = {
        domain: document.getElementById('domain').value.trim(),
        email: document.getElementById('email').value.trim(),
        altNames: document.getElementById('altNames').value.split('\n').map(function(n) { return n.trim(); }).filter(Boolean),
        challengeType: document.getElementById('challengeType').value,
        staging: document.getElementById('staging').checked
      };

      AdminSslPage.api('/obtain', { method: 'POST', body: JSON.stringify(body) })
        .then(function(response) {
          return response.json().then(function(json) { return { ok: response.ok, json: json }; });
        })
        .then(function(result) {
          if (!result.ok) throw new Error(AdminSslPage.failureText(result.json));
          Toast.show(AdminSslPage.message('obtained'), 'success');
          AdminSslPage.loadCertStatus();
          AdminSslPage.loadDNS();
        })
        .catch(function(error) {
          Toast.show(error.message, 'error');
        })
        .finally(function() {
          obtainBtn.disabled = false;
        });
    },

    renewCert: function() {
      showConfirm(AdminSslPage.message('confirm_renew')).then(function(confirmed) {
        if (!confirmed) return;
        const renewBtn = document.getElementById('renewBtn');
        renewBtn.disabled = true;

        const body = {
          domain: document.getElementById('domain').value.trim(),
          challengeType: document.getElementById('challengeType').value,
          force: true
        };
        AdminSslPage.api('/renew', { method: 'POST', body: JSON.stringify(body) })
          .then(function(response) {
            return response.json().then(function(json) { return { ok: response.ok, json: json }; });
          })
          .then(function(result) {
            if (!result.ok) throw new Error(AdminSslPage.failureText(result.json));
            Toast.show(AdminSslPage.message('renewed'), 'success');
            AdminSslPage.loadCertStatus();
            AdminSslPage.loadDNS();
          })
          .catch(function(error) {
            Toast.show(error.message, 'error');
          })
          .finally(function() {
            renewBtn.disabled = false;
          });
      });
    },

    verifyCert: function() {
      AdminSslPage.api('/verify', { method: 'POST' })
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (data.valid) {
            Toast.show(AdminSslPage.message('valid'), 'success');
          } else {
            Toast.show(data.error || AdminSslPage.message('request_failed'), 'error');
          }
        })
        .catch(function() {
          Toast.show(AdminSslPage.message('request_failed'), 'error');
        });
    },

    addDNSVariable: function(name, stored) {
      const holder = document.getElementById('dnsVariables');
      const row = document.createElement('div');
      row.className = 'form-group dns-variable';

      const nameInput = document.createElement('input');
      nameInput.type = 'text';
      nameInput.className = 'form-input dns-variable-name';
      nameInput.value = name || '';
      nameInput.setAttribute('aria-label', AdminSslPage.dnsLabels.name);
      nameInput.setAttribute('placeholder', AdminSslPage.dnsLabels.name);
      nameInput.setAttribute('autocomplete', 'off');
      nameInput.setAttribute('spellcheck', 'false');

      const valueInput = document.createElement('input');
      valueInput.type = 'password';
      valueInput.className = 'form-input dns-variable-value';
      valueInput.value = '';
      valueInput.setAttribute('aria-label', AdminSslPage.dnsLabels.value);
      valueInput.setAttribute('placeholder', stored ? '********' : AdminSslPage.dnsLabels.value);
      valueInput.setAttribute('autocomplete', 'new-password');

      row.appendChild(nameInput);
      row.appendChild(valueInput);
      holder.appendChild(row);
    },

    collectDNS: function() {
      const credentials = {};
      document.querySelectorAll('#dnsVariables .dns-variable').forEach(function(row) {
        const name = row.querySelector('.dns-variable-name').value.trim();
        const value = row.querySelector('.dns-variable-value').value;
        if (name) credentials[name] = value === '' ? '********' : value;
      });
      return { provider: document.getElementById('dnsProvider').value, credentials: credentials };
    },

    loadDNS: function() {
      AdminSslPage.api('/dns')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          const select = document.getElementById('dnsProvider');
          if (select.options.length <= 1) {
            (data.providers || []).forEach(function(name) {
              const option = document.createElement('option');
              option.value = name;
              option.textContent = name;
              select.appendChild(option);
            });
          }
          select.value = data.provider || '';
          document.getElementById('dnsVariables').textContent = '';
          Object.keys(data.credentials || {}).sort().forEach(function(name) {
            AdminSslPage.addDNSVariable(name, true);
          });
          document.getElementById('dnsValidated').textContent = data.validatedAt
            ? AdminSslPage.dnsLabels.validated + ' ' + new Date(data.validatedAt).toLocaleString()
            : '';
        })
        .catch(function() {
          Toast.show(AdminSslPage.message('load_failed'), 'error');
        });
    },

    sendDNS: function(path, method, successToast) {
      const button = document.getElementById(path === '/dns/check' ? 'dnsCheckBtn' : 'dnsSaveBtn');
      button.disabled = true;
      AdminSslPage.api(path, { method: method, body: JSON.stringify(AdminSslPage.collectDNS()) })
        .then(function(response) {
          return response.json().then(function(json) { return { ok: response.ok, json: json }; });
        })
        .then(function(result) {
          if (!result.ok) throw new Error(AdminSslPage.failureText(result.json));
          Toast.show(result.json.message || successToast, 'success');
          if (path === '/dns') AdminSslPage.loadDNS();
        })
        .catch(function(error) {
          Toast.show(error.message, 'error');
        })
        .finally(function() {
          button.disabled = false;
        });
    },

    init: function() {
      if (!document.getElementById('admin-ssl-data')) return;

      AdminSslPage.dnsLabels = {
        name: document.getElementById('dnsVariableAdd').dataset.nameLabel || 'Variable',
        value: document.getElementById('dnsVariableAdd').dataset.valueLabel || 'Value',
        validated: document.getElementById('dnsValidated').dataset.label || ''
      };

      document.getElementById('letsEncryptForm').addEventListener('submit', AdminSslPage.submitLetsEncryptForm);
      document.getElementById('renewBtn').addEventListener('click', AdminSslPage.renewCert);
      document.getElementById('refreshBtn').addEventListener('click', AdminSslPage.loadCertStatus);
      document.getElementById('verifyBtn').addEventListener('click', AdminSslPage.verifyCert);
      document.getElementById('dnsVariableAdd').addEventListener('click', function() { AdminSslPage.addDNSVariable('', false); });
      document.getElementById('dnsCheckBtn').addEventListener('click', function() { AdminSslPage.sendDNS('/dns/check', 'POST', ''); });
      document.getElementById('dnsForm').addEventListener('submit', function(e) {
        e.preventDefault();
        AdminSslPage.sendDNS('/dns', 'PUT', '');
      });

      AdminSslPage.loadCertStatus();
      AdminSslPage.loadDNS();
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
      const validResults = Array.isArray(results) ? results.filter(function(result) {
        return result && typeof result.name === 'string' && typeof result.country === 'string' &&
          Number.isFinite(result.latitude) && Number.isFinite(result.longitude);
      }) : [];
      AddLocationPage.searchResults = validResults;

      if (validResults.length === 0) {
        container.innerHTML = '<div class="result-item">No results found</div>';
        container.classList.add('active');
        return;
      }

      container.innerHTML = validResults.map(function(result, index) {
        return '<div class="result-item" data-action="add-location-select-result" data-index="' + index + '">' +
          '<div class="result-city">' + AddLocationPage.escHtml(result.name) + '</div>' +
          '<div class="result-details">' +
          (typeof result.admin1 === 'string' && result.admin1 ? AddLocationPage.escHtml(result.admin1) + ', ' : '') + AddLocationPage.escHtml(result.country) +
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

  const EarthquakeDetailPage = {
    getData: function() {
      const el = document.getElementById('map');
      if (!el) return null;
      return {
        latitude: Number(el.dataset.latitude),
        longitude: Number(el.dataset.longitude),
        magnitude: Number(el.dataset.magnitude),
        place: el.dataset.place,
        time: el.dataset.time,
        depth: Number(el.dataset.depth),
        tsunami: Number(el.dataset.tsunami)
      };
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
        if (typeof data.qr_code !== 'string' || !/^data:image\/png;base64,[A-Za-z0-9+/]+=*$/.test(data.qr_code)) {
          throw new Error('Invalid QR code response');
        }
        const qrImage = document.createElement('img');
        qrImage.src = data.qr_code;
        qrImage.alt = 'QR Code';
        document.getElementById('qrCodeContainer').replaceChildren(qrImage);
        document.getElementById('manualSecret').value = data.secret;

        document.getElementById('setup2FAModal').classList.add('active');
        document.getElementById('setupStep1').classList.remove('d-none');
        document.getElementById('setupStep2').classList.add('d-none');
        document.getElementById('setupStep3').classList.add('d-none');
      } catch (error) {
        Toast.error('Failed to setup 2FA: ' + error.message);
      }
    },

    nextSetupStep: function() {
      document.getElementById('setupStep1').classList.add('d-none');
      document.getElementById('setupStep2').classList.remove('d-none');
      document.getElementById('verificationCode').focus();
    },

    prevSetupStep: function() {
      document.getElementById('setupStep2').classList.add('d-none');
      document.getElementById('setupStep1').classList.remove('d-none');
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
          SecurityPage.displayRecoveryKeys(SecurityPage.recoveryKeysList);
          document.getElementById('setup2FAModal').classList.add('active');
          document.getElementById('setupStep1').classList.add('d-none');
          document.getElementById('setupStep2').classList.add('d-none');
          document.getElementById('setupStep3').classList.remove('d-none');
        } else {
          window.location.reload();
        }
      } catch (error) {
        Toast.error('Failed to register passkey: ' + error.message);
      }
    },

    deletePasskey: async function(passkeyID, name) {
      const confirmed = await showConfirm(`Delete passkey "${name}"?`, 'Delete Passkey');
      if (!confirmed) {
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
      const heading = document.createElement('h4');
      heading.textContent = 'Your Recovery Keys:';
      const keyElements = (Array.isArray(keys) ? keys : []).map(function(key) {
        const element = document.createElement('div');
        element.className = 'recovery-key';
        element.textContent = key;
        return element;
      });
      container.replaceChildren(heading, ...keyElements);
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
      document.getElementById('setup2FAModal').classList.remove('active');
      document.getElementById('verificationCode').value = '';
      SecurityPage.setup2FASecret = '';
      SecurityPage.recoveryKeysList = [];
    },

    disable2FA: function() {
      document.getElementById('disable2FAModal').classList.add('active');
    },

    closeDisableModal: function() {
      document.getElementById('disable2FAModal').classList.remove('active');
      document.getElementById('disablePassword').value = '';
    },

    regenerateRecoveryKeys: async function() {
      const code = await showPrompt('Enter your authenticator code to regenerate recovery keys:', '', 'Regenerate Recovery Keys');
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
        document.getElementById('setup2FAModal').classList.add('active');
        document.getElementById('setupStep1').classList.add('d-none');
        document.getElementById('setupStep2').classList.add('d-none');
        document.getElementById('setupStep3').classList.remove('d-none');
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

          document.getElementById('setupStep2').classList.add('d-none');
          document.getElementById('setupStep3').classList.remove('d-none');
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
      document.getElementById('newTokenModal').classList.add('active');
    },

    closeNewModal: function() {
      document.getElementById('newTokenModal').classList.remove('active');
      document.getElementById('newTokenForm').reset();
    },

    closeCreatedModal: function() {
      document.getElementById('tokenCreatedModal').classList.remove('active');
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
        document.querySelectorAll('[data-token-id]').forEach(function(element) {
          if (element.dataset.tokenId === String(tokenId)) element.remove();
        });
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
          document.getElementById('tokenCreatedModal').classList.add('active');
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
        const el = document.getElementById('notifications-list');
        NotificationsListPage.dataPayload = { apiPath: el ? el.dataset.apiPath || '' : '' };
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

    previewSecurityTxt: function() {
      window.open('/.well-known/security.txt', '_blank');
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

      const securityTxtForm = document.getElementById('security-txt-form');
      if (securityTxtForm) {
        securityTxtForm.addEventListener('submit', function(e) {
          e.preventDefault();
          AdminWebPage.saveFormSettings(securityTxtForm, 'security.txt saved successfully');
        });
      }

      const mcpForm = document.getElementById('mcp-form');
      if (mcpForm) {
        mcpForm.addEventListener('submit', function(e) {
          e.preventDefault();
          const value = mcpForm.querySelector('#mcp_enabled');
          const adminApiPath = AdminWebPage.getData().adminApiPath;
          fetch(adminApiPath + '/web', {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mcp_enabled: value ? value.value === 'true' : false })
          }).then(function(response) {
            return response.json().then(function(data) {
              if (!response.ok) throw new Error(data.message || 'Failed to save MCP settings');
              Toast.success('MCP settings saved successfully');
            });
          }).catch(function(error) {
            Toast.error(error.message);
          });
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
              const idAttr = Utils.escapeAttr(user.id);
              const actionButtons = isFirstAdmin
                ? '<button data-action="admin-dashboard-edit-admin-user" data-user-id="' + idAttr + '" data-user-email="' + Utils.escapeAttr(user.email) + '" class="btn-edit">Edit</button>'
                : '<button data-action="admin-dashboard-delete-user" data-user-id="' + idAttr + '" class="btn-delete">Delete</button>';
              const role = user.role === 'admin' ? 'admin' : 'user';

              return '<tr class="admin-table-body-row">' +
                '<td class="admin-table-body-cell">' + Utils.escapeHtml(user.id) + (isFirstAdmin ? ' <span class="admin-user-primary">(Primary)</span>' : '') + '</td>' +
                '<td class="admin-table-body-cell">' + Utils.escapeHtml(user.email) + '</td>' +
                '<td class="admin-table-body-cell"><span class="' + (role === 'admin' ? 'admin-role-badge-admin' : 'admin-role-badge') + '">' + role + '</span></td>' +
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
          '<input type="email" id="editAdminEmail" class="form-input" required value="' + Utils.escapeAttr(currentEmail) + '" placeholder="admin@example.com">' +
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
              '<h3 class="settings-category-title">' + Utils.escapeHtml(displayName) + '</h3>' +
              '<div class="settings-grid">' +
              categorySettings.map(function(setting) {
                const name = AdminDashboardPage.formatSettingLabel(setting.key);
                const key = String(setting.key || '');
                const value = setting.value === null || setting.value === undefined ? '' : String(setting.value);
                const escapedKey = Utils.escapeAttr(key);
                const escapedValue = Utils.escapeAttr(value);
                const isBool = value === 'true' || value === 'false';
                const isNumber = value !== '' && !isNaN(value);
                const isPassword = key.indexOf('password') !== -1;

                let inputHtml = '';

                if (isBool) {
                  const checked = setting.value === 'true' ? 'checked' : '';
                  inputHtml = '<label class="custom-checkbox">' +
                    '<input type="checkbox" data-key="' + escapedKey + '" ' + checked + '>' +
                    '<span class="checkbox-checkmark"></span>' +
                    '<span class="checkbox-label">' + (checked ? 'Enabled' : 'Disabled') + '</span>' +
                    '</label>';
                } else if (enumFields[key]) {
                  const options = enumFields[key].map(function(opt) {
                    return '<option value="' + opt + '" ' + (setting.value === opt ? 'selected' : '') + '>' + (opt.charAt(0).toUpperCase() + opt.slice(1)) + '</option>';
                  }).join('');
                  inputHtml = '<select class="form-input" data-key="' + escapedKey + '">' + options + '</select>';
                } else if (isPassword) {
                  inputHtml = '<input type="password" class="form-input" data-key="' + escapedKey + '" value="' + escapedValue + '" placeholder="••••••••">';
                } else if (isNumber) {
                  inputHtml = '<input type="number" class="form-input" data-key="' + escapedKey + '" value="' + escapedValue + '" min="0">';
                } else {
                  inputHtml = '<input type="text" class="form-input" data-key="' + escapedKey + '" value="' + escapedValue + '">';
                }

                return '<div class="setting-item">' +
                  '<div class="setting-info">' +
                  '<div class="setting-name">' + Utils.escapeHtml(name) + '</div>' +
                  '<div class="setting-key">' + Utils.escapeHtml(key) + '</div>' +
                  (setting.description ? '<div class="setting-description">' + Utils.escapeHtml(setting.description) + '</div>' : '') +
                  '</div>' +
                  '<div class="setting-value">' + inputHtml + '</div>' +
                  '</div>';
              }).join('') +
              '</div></div>';
          });

          document.getElementById('settings-list').innerHTML = html;
        })
        .catch(function() {
          Utils.setErrorMessage(document.getElementById('settings-list'), 'text-error text-center', 'Error loading settings.');
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
              const id = Utils.escapeAttr(token.id);
              return '<tr class="admin-table-body-row">' +
                '<td class="admin-table-body-cell">' + Utils.escapeHtml(token.id) + '</td>' +
                '<td class="admin-table-body-cell">' + Utils.escapeHtml(token.user_email || 'N/A') + '</td>' +
                '<td class="admin-table-body-cell">' + Utils.escapeHtml(token.name) + '</td>' +
                '<td class="admin-table-body-cell">' + new Date(token.created_at).toLocaleDateString() + '</td>' +
                '<td class="admin-table-body-cell">' + (token.last_used_at ? new Date(token.last_used_at).toLocaleDateString() : 'Never') + '</td>' +
                '<td class="admin-table-body-cell-right">' +
                '<button data-action="admin-dashboard-revoke-token" data-token-id="' + id + '" class="admin-token-button">Revoke</button>' +
                '</td></tr>';
            }).join('') +
            '</tbody></table>';
          document.getElementById('tokens-list').innerHTML = html;
        })
        .catch(function() {
          Utils.setErrorMessage(document.getElementById('tokens-list'), 'admin-error-text', 'Error loading tokens.');
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
            return '<option value="' + Utils.escapeAttr(u.id) + '">' + Utils.escapeHtml(u.email) + '</option>';
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
          '<code class="font-monospace word-break-all text-green font-size-09">' + Utils.escapeHtml(token) + '</code>' +
          '</div></div>',
        footer: '<button class="btn btn-primary" data-action="admin-dashboard-copy-token" data-token="' + Utils.escapeAttr(token) + '">📋 Copy to Clipboard</button>' +
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
                '<td class="admin-logs-cell-mono">' + Utils.escapeHtml(new Date(log.created_at).toLocaleString()) + '</td>' +
                '<td class="admin-logs-cell">' + Utils.escapeHtml(log.user_email || 'Anonymous') + '</td>' +
                '<td class="admin-logs-cell-cyan">' + Utils.escapeHtml(log.action) + '</td>' +
                '<td class="admin-logs-cell-comment">' + Utils.escapeHtml(log.resource || '-') + '</td>' +
                '<td class="admin-logs-cell-mono">' + Utils.escapeHtml(log.ip_address || '-') + '</td>' +
                '</tr>';
            }).join('') +
            '</tbody></table>';
          document.getElementById('logs-list').innerHTML = html;
        })
        .catch(function() {
          Utils.setErrorMessage(document.getElementById('logs-list'), 'admin-error-text', 'Error loading logs.');
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

  const SetupTokenPage = {
    strings: null,

    getStrings: function() {
      if (!SetupTokenPage.strings) {
        const el = document.getElementById('setup-token-data');
        SetupTokenPage.strings = el ? JSON.parse(el.textContent) : {};
      }
      return SetupTokenPage.strings;
    },

    showError: function(errorMsg, text) {
      errorMsg.textContent = text;
      errorMsg.classList.add('edit-error-message');
    },

    submit: function(form) {
      const strings = SetupTokenPage.getStrings();
      const submitBtn = form.querySelector('#submit-btn');
      const errorMsg = document.getElementById('error-message');
      const token = form.querySelector('#setup_token').value.trim().toLowerCase();

      errorMsg.classList.remove('edit-error-message');

      if (!/^[a-f0-9]{32}$/.test(token)) {
        SetupTokenPage.showError(errorMsg, strings.invalidFormat);
        return;
      }

      submitBtn.disabled = true;
      submitBtn.textContent = strings.verifying;

      const restore = function(text) {
        SetupTokenPage.showError(errorMsg, text);
        submitBtn.disabled = false;
        submitBtn.textContent = strings.continueLabel;
      };

      fetch(window.location.pathname.replace(/\/$/, '') + '/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ setup_token: token })
      })
        .then(function(response) {
          return response.json().then(function(result) {
            if (response.ok) {
              window.location.href = result.redirect;
            } else {
              restore(result.error || strings.invalidToken);
            }
          });
        })
        .catch(function() {
          restore(strings.networkError);
        });
    },

    init: function() {
      const form = document.getElementById('token-form');
      if (!form || !document.getElementById('setup-token-data')) return;
      form.addEventListener('submit', function(e) {
        e.preventDefault();
        SetupTokenPage.submit(form);
      });
    }
  };

  const LoginPage = {
    strings: null,

    getStrings: function() {
      if (!LoginPage.strings) {
        const el = document.getElementById('login-data');
        LoginPage.strings = el ? JSON.parse(el.textContent) : {};
      }
      return LoginPage.strings;
    },

    init: function() {
      const toggleLink = document.getElementById('toggleRecoveryKey');
      const input = document.getElementById('two_factor_code');
      const label = document.getElementById('twoFactorLabel');
      const flag = document.getElementById('use_recovery_key');
      if (!toggleLink || !input || !label || !flag) return;

      const strings = LoginPage.getStrings();
      let usingRecoveryKey = false;

      toggleLink.addEventListener('click', function(e) {
        e.preventDefault();
        usingRecoveryKey = !usingRecoveryKey;

        if (usingRecoveryKey) {
          label.textContent = strings.recoveryKeyLabel;
          input.placeholder = strings.recoveryKeyPlaceholder;
          input.maxLength = 19;
          toggleLink.textContent = strings.useAuthenticatorCodeLink;
          flag.value = 'true';
        } else {
          label.textContent = strings.twoFactorCodeLabel;
          input.placeholder = strings.twoFactorCodePlaceholder;
          input.maxLength = 6;
          toggleLink.textContent = strings.useRecoveryKeyLink;
          flag.value = 'false';
        }

        input.value = '';
        input.focus();
      });
    }
  };

  const IndexPage = {
    strings: null,

    unitsParam: function() {
      const units = document.getElementById('units-select').value;
      return units !== 'imperial' ? '?units=' + encodeURIComponent(units) : '';
    },

    showError: function(node) {
      const form = document.querySelector('form');
      form.parentNode.insertBefore(node, form.nextSibling);
      setTimeout(function() { node.remove(); }, 6000);
    },

    getPosition: function(options) {
      return new Promise(function(resolve, reject) {
        navigator.geolocation.getCurrentPosition(resolve, reject, options);
      });
    },

    locate: function() {
      const strings = IndexPage.strings;
      const button = document.getElementById('location-btn');
      if (!button) return;

      const originalText = button.textContent;
      button.textContent = '📍 ' + strings.getting;
      button.disabled = true;

      if (!navigator.geolocation) {
        IndexPage.ipFallback(button, originalText);
        return;
      }

      const forceRefresh = button.dataset.forceRefresh === 'true';
      button.dataset.forceRefresh = 'false';
      button.textContent = '📍 ' + strings.gps;

      IndexPage.getPosition({
        timeout: forceRefresh ? 30000 : 20000,
        enableHighAccuracy: true,
        maximumAge: forceRefresh ? 0 : 60000
      })
        .then(function(position) {
          if (position.coords.accuracy > 1000) {
            button.textContent = '📍 ' + strings.lowAccuracy;
          }
          return position;
        })
        .catch(function() {
          button.textContent = '📍 ' + strings.locating;
          return IndexPage.getPosition({
            timeout: 12000,
            enableHighAccuracy: false,
            maximumAge: 600000
          });
        })
        .then(function(position) {
          const lat = position.coords.latitude.toFixed(4);
          const lon = position.coords.longitude.toFixed(4);
          button.textContent = '📍 ' + strings.found;
          const target = window.location.origin + '/' + lat + ',' + lon + IndexPage.unitsParam();
          setTimeout(function() { window.location.href = target; }, 500);
        })
        .catch(function() {
          IndexPage.ipFallback(button, originalText);
        });
    },

    ipFallback: function(button, originalText) {
      const strings = IndexPage.strings;
      button.textContent = '📍 ' + strings.ipLocation;

      fetch('/api/v1/weather/locations')
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (!data.location || !data.location.shortName) throw new Error(strings.locationUnavailable);
          const target = window.location.origin + '/' +
            data.location.shortName.replace(/ /g, '+') + IndexPage.unitsParam();
          button.textContent = '📍 ' + strings.found;
          setTimeout(function() { window.location.href = target; }, 800);
        })
        .catch(function() {
          button.textContent = originalText;
          button.disabled = false;

          const input = document.querySelector('input[name="location"]');
          const current = (input && input.value) || strings.approximateLocation;
          const notice = document.createElement('div');
          notice.className = 'js-error-display-orange';
          notice.innerHTML = '📍 ' + Utils.escapeHtml(strings.locationNotAvailable) + '<br>' +
            Utils.escapeHtml(strings.locationPrefilled) + ' <strong>' + Utils.escapeHtml(current) + '</strong>';
          IndexPage.showError(notice);
        });
    },

    hideAutocomplete: function() {
      document.getElementById('autocomplete-dropdown').classList.remove('show');
    },

    showAutocomplete: function(results) {
      const dropdown = document.getElementById('autocomplete-dropdown');
      dropdown.innerHTML = '';

      results.forEach(function(result) {
        const item = document.createElement('div');
        item.className = 'autocomplete-item';
        item.innerHTML =
          '<div class="city-name">' + Utils.escapeHtml(result.shortName || result.name) + '</div>' +
          '<div class="location-details">' + Utils.escapeHtml(result.fullName) + '</div>';

        item.addEventListener('click', function() {
          document.getElementById('location-input').value = result.shortName || result.name;
          IndexPage.hideAutocomplete();
          const coords = result.latitude.toFixed(4) + ',' + result.longitude.toFixed(4);
          window.location.href = window.location.origin + '/' + coords + IndexPage.unitsParam();
        });

        dropdown.appendChild(item);
      });

      dropdown.classList.add('show');
    },

    searchLocations: function(query) {
      const base = (window.API_PATHS && window.API_PATHS.base) || '/api/v1';
      fetch(base + '/search?q=' + encodeURIComponent(query))
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (data.results && data.results.length > 0) {
            IndexPage.showAutocomplete(data.results);
          } else {
            IndexPage.hideAutocomplete();
          }
        })
        .catch(function() {
          IndexPage.hideAutocomplete();
        });
    },

    bindForm: function(locationInput, weatherForm) {
      weatherForm.addEventListener('submit', function(e) {
        e.preventDefault();
        const location = locationInput.value.trim().replace(/ /g, '+');
        window.location.href = window.location.origin + '/' + location + IndexPage.unitsParam();
      });

      let autocompleteTimeout;
      locationInput.addEventListener('input', function() {
        const query = this.value.trim();
        clearTimeout(autocompleteTimeout);
        if (query.length < 3) {
          IndexPage.hideAutocomplete();
          return;
        }
        autocompleteTimeout = setTimeout(function() {
          IndexPage.searchLocations(query);
        }, 300);
      });

      document.addEventListener('click', function(e) {
        if (!e.target.closest('#location-input') && !e.target.closest('#autocomplete-dropdown')) {
          IndexPage.hideAutocomplete();
        }
      });
    },

    bindLocationButton: function(locationBtn) {
      let lastTap = 0;
      let tapTimeout;

      locationBtn.addEventListener('click', function(e) {
        e.preventDefault();
        const now = Date.now();
        const tapLength = now - lastTap;
        clearTimeout(tapTimeout);

        if (tapLength < 500 && tapLength > 0) {
          this.dataset.forceRefresh = 'true';
          this.textContent = '🔄 ' + IndexPage.strings.forceRefresh;
          IndexPage.locate();
        } else {
          tapTimeout = setTimeout(IndexPage.locate, 250);
        }

        lastTap = now;
      });

      locationBtn.title = IndexPage.strings.locationBtnTooltip;
    },

    init: function() {
      const data = document.getElementById('index-data');
      const weatherForm = document.getElementById('weather-form');
      const locationInput = document.getElementById('location-input');
      if (!data || !weatherForm || !locationInput) return;

      IndexPage.strings = JSON.parse(data.textContent);

      if (!locationInput.value) locationInput.focus();
      IndexPage.bindForm(locationInput, weatherForm);

      const locationBtn = document.getElementById('location-btn');
      if (locationBtn) IndexPage.bindLocationButton(locationBtn);
    }
  };

  const MoonPage = {
    strings: null,

    unitsParam: function() {
      const units = document.getElementById('moon-units-select').value;
      return units !== 'imperial' ? '?units=' + encodeURIComponent(units) : '';
    },

    navigate: function(path) {
      window.location.href = window.location.origin + '/moon/' + path + MoonPage.unitsParam();
    },

    hideAutocomplete: function() {
      const dropdown = document.getElementById('moon-autocomplete-dropdown');
      if (!dropdown) return;
      dropdown.classList.remove('autocomplete-dropdown-visible');
      dropdown.innerHTML = '';
    },

    showAutocomplete: function(results) {
      const dropdown = document.getElementById('moon-autocomplete-dropdown');
      dropdown.innerHTML = '';

      results.forEach(function(result) {
        const item = document.createElement('div');
        item.className = 'autocomplete-item';
        const region = result.admin1 ? result.admin1 + ', ' : '';
        item.innerHTML =
          '<span class="autocomplete-item city-name">' + Utils.escapeHtml(result.name) + '</span>' +
          '<span class="autocomplete-item location-details">' +
          Utils.escapeHtml(region + result.countryCode) + '</span>';

        item.addEventListener('click', function() {
          MoonPage.hideAutocomplete();
          MoonPage.navigate(result.latitude.toFixed(4) + ',' + result.longitude.toFixed(4));
        });

        dropdown.appendChild(item);
      });

      dropdown.classList.add('autocomplete-dropdown-visible');
    },

    searchLocations: function(query) {
      fetch(MoonPage.strings.apiPath + '/search?q=' + encodeURIComponent(query))
        .then(function(response) { return response.json(); })
        .then(function(data) {
          if (data.results && data.results.length > 0) {
            MoonPage.showAutocomplete(data.results);
          } else {
            MoonPage.hideAutocomplete();
          }
        })
        .catch(function() {
          MoonPage.hideAutocomplete();
        });
    },

    locate: function(button) {
      const strings = MoonPage.strings;
      const originalText = button.textContent;
      button.disabled = true;
      button.textContent = '📍 ' + strings.gettingLocation;

      new Promise(function(resolve, reject) {
        if (!navigator.geolocation) {
          reject(new Error('unsupported'));
          return;
        }
        navigator.geolocation.getCurrentPosition(resolve, reject, {
          timeout: 10000,
          enableHighAccuracy: true
        });
      })
        .then(function(position) {
          button.textContent = '📍 ' + strings.foundLocation;
          const coords = position.coords.latitude.toFixed(4) + ',' + position.coords.longitude.toFixed(4);
          setTimeout(function() { MoonPage.navigate(coords); }, 500);
        })
        .catch(function() {
          fetch('/api/v1/weather/locations')
            .then(function(response) { return response.json(); })
            .then(function(data) {
              if (!data.location || !data.location.shortName) throw new Error('no location');
              button.textContent = '📍 ' + strings.foundLocation;
              const detected = data.location.shortName.replace(/ /g, '+');
              setTimeout(function() { MoonPage.navigate(detected); }, 500);
            })
            .catch(function() {
              button.textContent = originalText;
              button.disabled = false;
              showAlert(strings.locationError);
            });
        });
    },

    init: function() {
      const data = document.getElementById('moon-data');
      if (!data) return;

      MoonPage.strings = JSON.parse(data.textContent);

      const moonForm = document.getElementById('moon-form');
      const locationInput = document.getElementById('moon-location-input');
      const locationBtn = document.getElementById('moon-location-btn');

      if (moonForm) {
        moonForm.addEventListener('submit', function(e) {
          e.preventDefault();
          const location = locationInput.value.trim();
          if (!location) {
            window.location.href = window.location.origin + '/moon';
            return;
          }
          MoonPage.navigate(location.replace(/ /g, '+'));
        });
      }

      if (locationBtn) {
        locationBtn.addEventListener('click', function() {
          MoonPage.locate(this);
        });
      }

      if (locationInput) {
        let autocompleteTimeout;
        locationInput.addEventListener('input', function(e) {
          clearTimeout(autocompleteTimeout);
          const query = e.target.value.trim();
          if (query.length < 2) {
            MoonPage.hideAutocomplete();
            return;
          }
          autocompleteTimeout = setTimeout(function() {
            MoonPage.searchLocations(query);
          }, 300);
        });

        document.addEventListener('click', function(e) {
          if (!e.target.closest('#moon-location-input') && !e.target.closest('#moon-autocomplete-dropdown')) {
            MoonPage.hideAutocomplete();
          }
        });
      }
    }
  };

  const PasskeyLoginPage = {
    normalizeRequestOptions: function(options) {
      const publicKey = options.publicKey || options;
      publicKey.challenge = AdminPasskeyLoginPage.base64urlToBuffer(publicKey.challenge);
      if (Array.isArray(publicKey.allowCredentials)) {
        publicKey.allowCredentials = publicKey.allowCredentials.map(function(credential) {
          return Object.assign({}, credential, {
            id: AdminPasskeyLoginPage.base64urlToBuffer(credential.id)
          });
        });
      }
      return publicKey;
    },

    serializeAssertion: function(assertion) {
      const encode = AdminPasskeyLoginPage.bufferToBase64url;
      return {
        id: assertion.id,
        rawId: encode(assertion.rawId),
        type: assertion.type,
        response: {
          authenticatorData: encode(assertion.response.authenticatorData),
          clientDataJSON: encode(assertion.response.clientDataJSON),
          signature: encode(assertion.response.signature),
          userHandle: assertion.response.userHandle ? encode(assertion.response.userHandle) : null
        }
      };
    },

    start: function(status, strings) {
      if (!window.PublicKeyCredential || !navigator.credentials || !navigator.credentials.get) {
        status.className = 'alert alert-error';
        status.textContent = strings.unsupportedBrowser;
        return;
      }

      fetch('/api/v1/server/auth/passkey/challenge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}'
      })
        .then(function(response) {
          return response.json().then(function(payload) {
            if (!response.ok || !payload.ok) throw new Error(payload.error || strings.failedStart);
            status.textContent = strings.touchPrompt;
            return navigator.credentials.get({
              publicKey: PasskeyLoginPage.normalizeRequestOptions(payload.options)
            });
          });
        })
        .then(function(credential) {
          if (!credential) throw new Error(strings.cancelled);
          return fetch('/api/v1/server/auth/passkey/verify', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(PasskeyLoginPage.serializeAssertion(credential))
          });
        })
        .then(function(response) {
          return response.json().then(function(payload) {
            if (!response.ok || !payload.ok) throw new Error(payload.error || strings.failedVerify);
            status.className = 'alert alert-success';
            status.textContent = strings.successRedirecting;
            window.location.href = '/users/dashboard';
          });
        })
        .catch(function(error) {
          status.className = 'alert alert-error';
          status.textContent = error.message || strings.failedGeneric;
        });
    },

    init: function() {
      const status = document.getElementById('passkeyStatus');
      const data = document.getElementById('passkey-login-data');
      if (!status || !data) return;
      PasskeyLoginPage.start(status, JSON.parse(data.textContent));
    }
  };

  const RegisterPage = {
    init: function() {
      const password = document.getElementById('password');
      const bar = document.getElementById('strengthBar');
      const indicator = document.getElementById('password-strength-indicator');
      const data = document.getElementById('register-data');
      if (!password || !bar || !indicator || !data) return;

      const strings = JSON.parse(data.textContent);

      password.addEventListener('input', function() {
        const value = password.value;
        let strength = 0;
        if (value.length >= 8) strength++;
        if (/[a-z]/.test(value) && /[A-Z]/.test(value)) strength++;
        if (/[0-9]/.test(value)) strength++;
        if (/[^a-zA-Z0-9]/.test(value)) strength++;

        const level = strength <= 1 ? 'weak' : (strength <= 3 ? 'medium' : 'strong');
        bar.className = 'password-strength-bar ' + level;
        bar.setAttribute('aria-valuenow', strength);
        indicator.textContent = strings[level];
      });
    }
  };

  document.addEventListener('DOMContentLoaded', function() {
    IndexPage.init();
    MoonPage.init();
    RegisterPage.init();
    LoginPage.init();
    PasskeyLoginPage.init();
    SetupTokenPage.init();
    AdminSettingsPage.init();
    AdminDatabasePage.init();
    AdminBackupPage.init();
    AdminEmailEditorPage.init();
    AdminEmailPage.init();
    AdminLogsPage.init();
    AdminMetricsPage.init();
    AdminNotificationsPage.init();
    AdminPasskeyLoginPage.init();
    AdminSecurityPage.init();
    AdminSslPage.init();
    AdminSystemPage.init();
    AddLocationPage.init();
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
        case 'toggle-nav-section':
          AdminNavPage.toggleSection(btn);
          break;
        case 'stop-propagation':
          break;
        case 'earthquake-item':
          EarthquakePage.handleItemClick(btn);
          break;
        case 'edit-location-delete':
          EditLocationPage.handleDelete();
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
        // Server-rendered flash markup carries no id, so remove the node directly.
        case 'dismiss-flash': {
          const flashEl = btn.closest('.flash-alert');
          if (flashEl) flashEl.remove();
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
      body: `<p class="modal-body-text">${Utils.escapeHtml(message)}</p>`,
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
   * Persists which admin sidebar sections are collapsed. The markup renders
   * the server-known state on every load, so navigation works with JavaScript
   * disabled; this only records the admin's later choices. The stored value
   * is a UI preference, never a session token.
   */
  const AdminNavPage = {
    /**
     * Persist the sidebar's collapsed sections to the server after a toggle.
     * The cookie stays HttpOnly, so the list is POSTed to the session-auth
     * admin route and the next server render reads it back. Failure to save
     * only costs the preference; the section already toggled correctly.
     */
    toggleSection: function(btn) {
      const nav = document.getElementById('admin-nav');
      if (!nav) return;

      const url = nav.dataset.navStateUrl;
      if (!url) return;

      const collapsed = [];
      nav.querySelectorAll('[data-nav-section]').forEach(section => {
        if (!section.open) collapsed.push(section.dataset.navSection);
      });

      fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': nav.dataset.navStateCsrf || ''
        },
        body: JSON.stringify({ collapsed: collapsed })
      }).catch(function() {
        return;
      });
    }
  };

  // Expose AdminPanel
  window.AdminPanel = AdminPanel;

  // Initialize admin panel if on admin page
  if (document.querySelector('.admin-sidebar')) {
    AdminPanel.init();
  }

  // ============================================
  // THEME SYSTEM (AI.md PART 16)
  // Dark theme is DEFAULT per AI.md spec
  // ============================================

  // Cycle order per AI.md PART 16: dark -> light -> auto -> dark
  const THEME_CYCLE = ['dark', 'light', 'auto'];

  const Theme = {
    // Available themes: dark (default), light, auto
    THEMES: THEME_CYCLE,
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
      if (document.documentElement.dataset.gpcOptOut === 'true') {
        return;
      }
      var secure = window.location.protocol === 'https:' ? '; Secure' : '';
      document.cookie = this.COOKIE_NAME + '=' + encodeURIComponent(theme) +
        '; path=/; max-age=31536000; SameSite=Strict' + secure;
    },

    /**
     * Get the LIVE theme from the theme-* class the server rendered on
     * <html> (AI.md PART 16). The class is the single source of truth
     * because it also reflects switches this script already applied in
     * the current page view - the cookie and a form's hidden value are
     * both stale after the first JS-driven switch, which is exactly what
     * makes a hardcoded next-target stick after one click.
     */
    get: function() {
      var match = document.documentElement.className.match(/theme-(dark|light|auto)/);
      return match ? match[1] : 'dark';
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
      // theme-auto is pure CSS via prefers-color-scheme (AI.md PART 16),
      // so the class is written verbatim and never resolved here; only the
      // theme-color meta tag needs the effective light/dark value.
      var effectiveTheme = theme;
      if (theme === 'auto') {
        effectiveTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
      }
      var root = document.documentElement;
      THEME_CYCLE.forEach(function(name) {
        root.classList.remove('theme-' + name);
      });
      root.classList.add('theme-' + theme);
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
      document.querySelectorAll('.theme-btn, .theme-toggle-inline .theme-button').forEach(function(btn) {
        btn.classList.remove('active');
      });
      // Add active class to the button representing the current theme:
      // data-theme on the dropdown buttons, value on the guest 3-button form
      var activeBtn = document.querySelector('.theme-btn[data-theme="' + theme + '"]') ||
        document.querySelector('.theme-toggle-inline .theme-button[value="' + theme + '"]');
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
      // The server already rendered the correct theme-* class on <html>;
      // JS never re-resolves it, it only syncs the button states to it.
      this.updateActiveButton(this.get());

      // Instant preview: the form still submits normally (no preventDefault)
      // so the server persists the value and re-renders the next target.
      document.querySelectorAll('.theme-toggle-form, .theme-toggle-row').forEach(function(form) {
        form.addEventListener('submit', function() {
          var next = THEME_CYCLE[(THEME_CYCLE.indexOf(Theme.get()) + 1) % THEME_CYCLE.length];
          Theme.apply(next);
        });
      });

      // Guest 3-button form: preview the exact mode that was submitted
      document.querySelectorAll('.theme-toggle-inline .theme-button').forEach(function(btn) {
        btn.addEventListener('click', function() {
          if (THEME_CYCLE.includes(btn.value)) {
            Theme.apply(btn.value);
          }
        });
      });

      // theme-color meta only: auto follows the OS through CSS on its own
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function() {
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

  // PWA: service worker registration, update prompt, and boot watchdog (AI.md PART 16)
  const Pwa = {
    recoveredFlag: 'wthr_recovered',

    // True once the user pressed "Update now"; only then may controllerchange reload the page
    updateAccepted: false,

    // True after the single reload so the page never reloads twice
    reloading: false,

    reloadOnce() {
      if (Pwa.reloading) {
        return;
      }
      Pwa.reloading = true;
      window.location.reload();
    },

    strings: {
      'pwa.update_available': 'A new version is available',
      'pwa.update_now': 'Update now',
      'pwa.later': 'Later'
    },

    async loadStrings() {
      const lang = (document.documentElement.lang || 'en').split('-')[0];
      try {
        const controller = new AbortController();
        const timer = setTimeout(function() { controller.abort(); }, 30000);
        const response = await fetch('/locales/' + encodeURIComponent(lang) + '.json', { signal: controller.signal });
        clearTimeout(timer);
        if (response.ok) {
          const messages = await response.json();
          Object.keys(Pwa.strings).forEach(function(key) {
            if (typeof messages[key] === 'string' && messages[key] !== '') {
              Pwa.strings[key] = messages[key];
            }
          });
        }
      } catch (e) {
        // English fallback strings stay in place
      }
    },

    cacheOfflineStrings() {
      const data = document.getElementById('offline-strings-data');
      if (!data || !window.caches) {
        return;
      }
      if (!data.dataset.title || !data.dataset.heading || !data.dataset.message) {
        return;
      }
      const strings = {
        lang: data.dataset.lang || document.documentElement.lang,
        dir: data.dataset.dir || 'ltr',
        title: data.dataset.title,
        heading: data.dataset.heading,
        message: data.dataset.message
      };
      caches.open('wthr-offline-strings')
        .then(function(cache) {
          return cache.put('/offline-strings.json', new Response(JSON.stringify(strings), {
            headers: { 'Content-Type': 'application/json' }
          }));
        })
        .catch(function() {
          // Offline support remains an enhancement if Cache Storage is unavailable.
        });
    },

    async showUpdate() {
      if (document.querySelector('.update-banner')) {
        return;
      }
      await Pwa.loadStrings();
      const banner = document.createElement('div');
      banner.className = 'update-banner';
      banner.setAttribute('role', 'status');
      banner.setAttribute('aria-live', 'polite');
      const label = document.createElement('span');
      label.textContent = Pwa.strings['pwa.update_available'];
      const update = document.createElement('button');
      update.type = 'button';
      update.className = 'btn btn-primary';
      update.textContent = Pwa.strings['pwa.update_now'];
      update.addEventListener('click', function() {
        Pwa.updateAccepted = true;
        navigator.serviceWorker.ready.then(function(registration) {
          if (registration.waiting) {
            registration.waiting.postMessage({ type: 'SKIP_WAITING' });
            // The takeover normally fires controllerchange within a moment; if it does not, reload anyway
            setTimeout(Pwa.reloadOnce, 5000);
          } else {
            Pwa.reloadOnce();
          }
        });
      });
      const later = document.createElement('button');
      later.type = 'button';
      later.className = 'btn';
      later.textContent = Pwa.strings['pwa.later'];
      later.addEventListener('click', function() { banner.remove(); });
      banner.append(label, update, later);
      document.body.appendChild(banner);
    },

    register() {
      if (!('serviceWorker' in navigator)) {
        return;
      }
      // Registered once at page load, never inside the click handler: reloads only after the user accepted the update
      navigator.serviceWorker.addEventListener('controllerchange', function() {
        if (Pwa.updateAccepted) {
          Pwa.reloadOnce();
        }
      });
      window.addEventListener('load', async function() {
        try {
          const registration = await navigator.serviceWorker.register('/sw.js', { scope: '/' });
          registration.addEventListener('updatefound', function() {
            const worker = registration.installing;
            if (!worker) {
              return;
            }
            worker.addEventListener('statechange', function() {
              if (worker.state === 'installed' && navigator.serviceWorker.controller) {
                Pwa.showUpdate();
              }
            });
          });
          setInterval(function() { registration.update(); }, 60 * 60 * 1000);
        } catch (e) {
          // The site is fully usable without a service worker
        }
      });
    },

    // Boot watchdog: if the first render never completes, unregister every
    // service worker, drop every cache and reload once per tab; a second
    // expiry leaves the page as is instead of looping.
    watchdog() {
      const timer = setTimeout(async function() {
        try {
          if (sessionStorage.getItem(Pwa.recoveredFlag)) {
            return;
          }
          sessionStorage.setItem(Pwa.recoveredFlag, '1');
        } catch (e) {
          return;
        }
        try {
          if ('serviceWorker' in navigator) {
            const registrations = await navigator.serviceWorker.getRegistrations();
            await Promise.all(registrations.map(function(r) { return r.unregister(); }));
          }
          if (window.caches) {
            const keys = await caches.keys();
            await Promise.all(keys.map(function(k) { return caches.delete(k); }));
          }
        } finally {
          window.location.reload();
        }
      }, 10000);
      const done = function() { clearTimeout(timer); };
      if (document.readyState === 'complete') {
        done();
      } else {
        window.addEventListener('load', done);
      }
    }
  };

  Pwa.watchdog();
  Pwa.cacheOfflineStrings();
  Pwa.register();

})();

// Severe weather page: location form, filters and autocomplete (runs only when its markup is present)
(function () {
  'use strict';

  const dataElement = document.getElementById('severe-weather-page-data');
  const form = document.getElementById('severe-weather-form');
  const locationInput = document.getElementById('location-input');
  const typeSelect = document.getElementById('type-select');
  const distanceSelect = document.getElementById('distance-select');
  const locationButton = document.getElementById('location-btn');
  if (!dataElement || !form || !locationInput || !typeSelect || !distanceSelect) return;

  const pageData = JSON.parse(dataElement.textContent);

  function updateLocation() {
    const location = locationInput.value.trim();
    const type = typeSelect.value;
    const distance = distanceSelect.value;
    if (!location) {
      window.location.href = `${window.location.origin}/severe-weather`;
      return;
    }

    const query = new URLSearchParams();
    if (type !== 'all') query.set('type', type);
    if (distance !== '50') query.set('distance', distance);
    const suffix = query.size ? `?${query}` : '';
    window.location.href = `${window.location.origin}/severe-weather/${encodeURIComponent(location).replace(/%20/g, '+')}${suffix}`;
  }

  function hideAutocomplete() {
    const dropdown = document.getElementById('severe-autocomplete-dropdown');
    if (!dropdown) return;
    dropdown.classList.remove('autocomplete-dropdown-visible');
    dropdown.replaceChildren();
  }

  function showAutocomplete(results) {
    const dropdown = document.getElementById('severe-autocomplete-dropdown');
    if (!dropdown) return;
    dropdown.replaceChildren();

    results.forEach((result) => {
      if (typeof result.name !== 'string' || !Number.isFinite(result.latitude) || !Number.isFinite(result.longitude)) return;
      const item = document.createElement('div');
      item.className = 'autocomplete-item';

      const name = document.createElement('span');
      name.className = 'autocomplete-item city-name';
      name.textContent = result.name;
      const details = document.createElement('span');
      details.className = 'autocomplete-item location-details';
      details.textContent = [result.admin1, result.countryCode].filter((part) => typeof part === 'string' && part).join(', ');
      item.append(name, details);

      item.addEventListener('click', () => {
        locationInput.value = `${result.latitude.toFixed(4)},${result.longitude.toFixed(4)}`;
        hideAutocomplete();
        updateLocation();
      });
      dropdown.appendChild(item);
    });
    dropdown.classList.add('autocomplete-dropdown-visible');
  }

  async function searchLocations(query, generation) {
    try {
      const response = await fetch(`${pageData.apiPath}/search?q=${encodeURIComponent(query)}`);
      if (!response.ok) throw new Error('Location search failed');
      const result = await response.json();
      if (generation !== autocompleteGeneration) return;
      if (Array.isArray(result.results) && result.results.length > 0) showAutocomplete(result.results);
      else hideAutocomplete();
    } catch {
      if (generation === autocompleteGeneration) hideAutocomplete();
    }
  }

  form.addEventListener('submit', (event) => {
    event.preventDefault();
    updateLocation();
  });
  typeSelect.addEventListener('change', updateLocation);
  distanceSelect.addEventListener('change', updateLocation);

  if (locationButton) {
    locationButton.addEventListener('click', async function () {
      const originalText = this.textContent;
      this.disabled = true;
      this.textContent = `📍 ${pageData.gettingLocation}`;
      try {
        const position = await new Promise((resolve, reject) => {
          if (!navigator.geolocation) {
            reject(new Error('Geolocation unavailable'));
            return;
          }
          navigator.geolocation.getCurrentPosition(resolve, reject, { timeout: 10000, enableHighAccuracy: true });
        });
        locationInput.value = `${position.coords.latitude.toFixed(4)},${position.coords.longitude.toFixed(4)}`;
        this.textContent = `📍 ${pageData.foundLocation}`;
        setTimeout(updateLocation, 500);
      } catch {
        try {
          const response = await fetch('/api/v1/weather/locations');
          if (!response.ok) throw new Error('IP location lookup failed');
          const result = await response.json();
          if (typeof result.location?.shortName !== 'string' || !result.location.shortName) throw new Error('IP location unavailable');
          locationInput.value = result.location.shortName;
          this.textContent = `📍 ${pageData.foundLocation}`;
          setTimeout(updateLocation, 500);
        } catch {
          this.textContent = originalText;
          this.disabled = false;
          showAlert(pageData.locationError);
        }
      }
    });
  }

  let autocompleteTimeout;
  let autocompleteGeneration = 0;
  locationInput.addEventListener('input', (event) => {
    clearTimeout(autocompleteTimeout);
    const generation = ++autocompleteGeneration;
    const query = event.target.value.trim();
    if (query.length < 2) {
      hideAutocomplete();
      return;
    }
    autocompleteTimeout = setTimeout(() => searchLocations(query, generation), 300);
  });
  document.addEventListener('click', (event) => {
    if (!event.target.closest('#location-input') && !event.target.closest('#severe-autocomplete-dropdown')) hideAutocomplete();
  });

})();

// Boot watchdog (AI.md PART 9 "Version-Change Purge"): if the first render has not completed
// within 10 seconds, drop every service worker and Cache API cache and reload, at most once
// per tab. A second expiry shows a static error line instead of looping.
(function() {
  const recoveredKey = 'wthr_recovered';
  const timer = setTimeout(function() {
    let alreadyRecovered = false;
    try {
      alreadyRecovered = sessionStorage.getItem(recoveredKey) === '1';
      sessionStorage.setItem(recoveredKey, '1');
    } catch (e) {
      alreadyRecovered = true;
    }
    if (alreadyRecovered) {
      const meta = document.querySelector('meta[name="wthr-boot-error"]');
      const notice = document.createElement('p');
      notice.setAttribute('role', 'alert');
      notice.textContent = meta ? meta.content : '';
      document.body.prepend(notice);
      return;
    }
    const jobs = [];
    if (navigator.serviceWorker && navigator.serviceWorker.getRegistrations) {
      jobs.push(navigator.serviceWorker.getRegistrations().then(function(registrations) {
        return Promise.all(registrations.map(function(registration) { return registration.unregister(); }));
      }));
    }
    if (window.caches && caches.keys) {
      jobs.push(caches.keys().then(function(keys) {
        return Promise.all(keys.map(function(key) { return caches.delete(key); }));
      }));
    }
    Promise.all(jobs).catch(function() {}).then(function() { location.reload(); });
  }, 10000);
  window.addEventListener('load', function() { clearTimeout(timer); try { sessionStorage.removeItem(recoveredKey); } catch (e) {} });
})();


// GraphiQL and Swagger initialize from escaped page data with no inline scripts.
(function initializeGraphiQL() {
  const container = document.getElementById('graphiql');
  if (!container || !window.GraphiQL || !window.ReactDOM) return;

  const endpoint = container.dataset.endpoint || '/api/graphql';
  const url = location.protocol + '//' + location.host + endpoint;
  const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const subscriptionUrl = wsProto + '//' + location.host + endpoint;
  const fetcher = window.GraphiQL.createFetcher({ url: url, subscriptionUrl: subscriptionUrl });
  window.ReactDOM.render(
    window.React.createElement(window.GraphiQL, {
      fetcher: fetcher,
      isHeadersEditorEnabled: true,
      shouldPersistHeaders: true
    }),
    container
  );
})();

(function initializeSwaggerUI() {
  const root = document.getElementById('swagger-ui');
  if (!root || typeof window.SwaggerUIBundle !== 'function') return;

  const config = {
    url: root.dataset.specUrl,
    dom_id: '#swagger-ui',
    deepLinking: root.dataset.deepLinking === 'true',
    docExpansion: root.dataset.docExpansion,
    persistAuthorization: root.dataset.persistAuthorization === 'true',
    defaultModelsExpandDepth: Number(root.dataset.modelsDepth),
    presets: [window.SwaggerUIBundle.presets.apis, window.SwaggerUIStandalonePreset],
    plugins: [window.SwaggerUIBundle.plugins.DownloadUrl],
    layout: 'StandaloneLayout',
    oauth2RedirectUrl: new URL('oauth2-redirect.html', document.baseURI).href
  };
  window.ui = window.SwaggerUIBundle(config);
})();

/* Bundled Leaflet 1.9.4; copyright and license notice retained from upstream. */
/* @preserve
 * Leaflet 1.9.4, a JS library for interactive maps. https://leafletjs.com
 * (c) 2010-2023 Vladimir Agafonkin, (c) 2010-2011 CloudMade
 */
!function(t,e){"object"==typeof exports&&"undefined"!=typeof module?e(exports):"function"==typeof define&&define.amd?define(["exports"],e):e((t="undefined"!=typeof globalThis?globalThis:t||self).leaflet={})}(this,function(t){"use strict";function l(t){for(var e,i,n=1,o=arguments.length;n<o;n++)for(e in i=arguments[n])t[e]=i[e];return t}var R=Object.create||function(t){return N.prototype=t,new N};function N(){}function a(t,e){var i,n=Array.prototype.slice;return t.bind?t.bind.apply(t,n.call(arguments,1)):(i=n.call(arguments,2),function(){return t.apply(e,i.length?i.concat(n.call(arguments)):arguments)})}var D=0;function h(t){return"_leaflet_id"in t||(t._leaflet_id=++D),t._leaflet_id}function j(t,e,i){var n,o,s=function(){n=!1,o&&(r.apply(i,o),o=!1)},r=function(){n?o=arguments:(t.apply(i,arguments),setTimeout(s,e),n=!0)};return r}function H(t,e,i){var n=e[1],e=e[0],o=n-e;return t===n&&i?t:((t-e)%o+o)%o+e}function u(){return!1}function i(t,e){return!1===e?t:(e=Math.pow(10,void 0===e?6:e),Math.round(t*e)/e)}function W(t){return t.trim?t.trim():t.replace(/^\s+|\s+$/g,"")}function F(t){return W(t).split(/\s+/)}function c(t,e){for(var i in Object.prototype.hasOwnProperty.call(t,"options")||(t.options=t.options?R(t.options):{}),e)t.options[i]=e[i];return t.options}function U(t,e,i){var n,o=[];for(n in t)o.push(encodeURIComponent(i?n.toUpperCase():n)+"="+encodeURIComponent(t[n]));return(e&&-1!==e.indexOf("?")?"&":"?")+o.join("&")}var V=/\{ *([\w_ -]+) *\}/g;function q(t,i){return t.replace(V,function(t,e){e=i[e];if(void 0===e)throw new Error("No value provided for variable "+t);return e="function"==typeof e?e(i):e})}var d=Array.isArray||function(t){return"[object Array]"===Object.prototype.toString.call(t)};function G(t,e){for(var i=0;i<t.length;i++)if(t[i]===e)return i;return-1}var K="data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=";function Y(t){return window["webkit"+t]||window["moz"+t]||window["ms"+t]}var X=0;function J(t){var e=+new Date,i=Math.max(0,16-(e-X));return X=e+i,window.setTimeout(t,i)}var $=window.requestAnimationFrame||Y("RequestAnimationFrame")||J,Q=window.cancelAnimationFrame||Y("CancelAnimationFrame")||Y("CancelRequestAnimationFrame")||function(t){window.clearTimeout(t)};function x(t,e,i){if(!i||$!==J)return $.call(window,a(t,e));t.call(e)}function r(t){t&&Q.call(window,t)}var tt={__proto__:null,extend:l,create:R,bind:a,get lastId(){return D},stamp:h,throttle:j,wrapNum:H,falseFn:u,formatNum:i,trim:W,splitWords:F,setOptions:c,getParamString:U,template:q,isArray:d,indexOf:G,emptyImageUrl:K,requestFn:$,cancelFn:Q,requestAnimFrame:x,cancelAnimFrame:r};function et(){}et.extend=function(t){function e(){c(this),this.initialize&&this.initialize.apply(this,arguments),this.callInitHooks()}var i,n=e.__super__=this.prototype,o=R(n);for(i in(o.constructor=e).prototype=o,this)Object.prototype.hasOwnProperty.call(this,i)&&"prototype"!==i&&"__super__"!==i&&(e[i]=this[i]);if(t.statics&&l(e,t.statics),t.includes){var s=t.includes;if("undefined"!=typeof L&&L&&L.Mixin){s=d(s)?s:[s];for(var r=0;r<s.length;r++)s[r]===L.Mixin.Events&&console.warn("Deprecated include of L.Mixin.Events: this property will be removed in future releases, please inherit from L.Evented instead.",(new Error).stack)}l.apply(null,[o].concat(t.includes))}return l(o,t),delete o.statics,delete o.includes,o.options&&(o.options=n.options?R(n.options):{},l(o.options,t.options)),o._initHooks=[],o.callInitHooks=function(){if(!this._initHooksCalled){n.callInitHooks&&n.callInitHooks.call(this),this._initHooksCalled=!0;for(var t=0,e=o._initHooks.length;t<e;t++)o._initHooks[t].call(this)}},e},et.include=function(t){var e=this.prototype.options;return l(this.prototype,t),t.options&&(this.prototype.options=e,this.mergeOptions(t.options)),this},et.mergeOptions=function(t){return l(this.prototype.options,t),this},et.addInitHook=function(t){var e=Array.prototype.slice.call(arguments,1),i="function"==typeof t?t:function(){this[t].apply(this,e)};return this.prototype._initHooks=this.prototype._initHooks||[],this.prototype._initHooks.push(i),this};var e={on:function(t,e,i){if("object"==typeof t)for(var n in t)this._on(n,t[n],e);else for(var o=0,s=(t=F(t)).length;o<s;o++)this._on(t[o],e,i);return this},off:function(t,e,i){if(arguments.length)if("object"==typeof t)for(var n in t)this._off(n,t[n],e);else{t=F(t);for(var o=1===arguments.length,s=0,r=t.length;s<r;s++)o?this._off(t[s]):this._off(t[s],e,i)}else delete this._events;return this},_on:function(t,e,i,n){"function"!=typeof e?console.warn("wrong listener type: "+typeof e):!1===this._listens(t,e,i)&&(e={fn:e,ctx:i=i===this?void 0:i},n&&(e.once=!0),this._events=this._events||{},this._events[t]=this._events[t]||[],this._events[t].push(e))},_off:function(t,e,i){var n,o,s;if(this._events&&(n=this._events[t]))if(1===arguments.length){if(this._firingCount)for(o=0,s=n.length;o<s;o++)n[o].fn=u;delete this._events[t]}else"function"!=typeof e?console.warn("wrong listener type: "+typeof e):!1!==(e=this._listens(t,e,i))&&(i=n[e],this._firingCount&&(i.fn=u,this._events[t]=n=n.slice()),n.splice(e,1))},fire:function(t,e,i){if(this.listens(t,i)){var n=l({},e,{type:t,target:this,sourceTarget:e&&e.sourceTarget||this});if(this._events){var o=this._events[t];if(o){this._firingCount=this._firingCount+1||1;for(var s=0,r=o.length;s<r;s++){var a=o[s],h=a.fn;a.once&&this.off(t,h,a.ctx),h.call(a.ctx||this,n)}this._firingCount--}}i&&this._propagateEvent(n)}return this},listens:function(t,e,i,n){"string"!=typeof t&&console.warn('"string" type argument expected');var o=e,s=("function"!=typeof e&&(n=!!e,i=o=void 0),this._events&&this._events[t]);if(s&&s.length&&!1!==this._listens(t,o,i))return!0;if(n)for(var r in this._eventParents)if(this._eventParents[r].listens(t,e,i,n))return!0;return!1},_listens:function(t,e,i){if(this._events){var n=this._events[t]||[];if(!e)return!!n.length;i===this&&(i=void 0);for(var o=0,s=n.length;o<s;o++)if(n[o].fn===e&&n[o].ctx===i)return o}return!1},once:function(t,e,i){if("object"==typeof t)for(var n in t)this._on(n,t[n],e,!0);else for(var o=0,s=(t=F(t)).length;o<s;o++)this._on(t[o],e,i,!0);return this},addEventParent:function(t){return this._eventParents=this._eventParents||{},this._eventParents[h(t)]=t,this},removeEventParent:function(t){return this._eventParents&&delete this._eventParents[h(t)],this},_propagateEvent:function(t){for(var e in this._eventParents)this._eventParents[e].fire(t.type,l({layer:t.target,propagatedFrom:t.target},t),!0)}},it=(e.addEventListener=e.on,e.removeEventListener=e.clearAllEventListeners=e.off,e.addOneTimeEventListener=e.once,e.fireEvent=e.fire,e.hasEventListeners=e.listens,et.extend(e));function p(t,e,i){this.x=i?Math.round(t):t,this.y=i?Math.round(e):e}var nt=Math.trunc||function(t){return 0<t?Math.floor(t):Math.ceil(t)};function m(t,e,i){return t instanceof p?t:d(t)?new p(t[0],t[1]):null==t?t:"object"==typeof t&&"x"in t&&"y"in t?new p(t.x,t.y):new p(t,e,i)}function f(t,e){if(t)for(var i=e?[t,e]:t,n=0,o=i.length;n<o;n++)this.extend(i[n])}function _(t,e){return!t||t instanceof f?t:new f(t,e)}function s(t,e){if(t)for(var i=e?[t,e]:t,n=0,o=i.length;n<o;n++)this.extend(i[n])}function g(t,e){return t instanceof s?t:new s(t,e)}function v(t,e,i){if(isNaN(t)||isNaN(e))throw new Error("Invalid LatLng object: ("+t+", "+e+")");this.lat=+t,this.lng=+e,void 0!==i&&(this.alt=+i)}function w(t,e,i){return t instanceof v?t:d(t)&&"object"!=typeof t[0]?3===t.length?new v(t[0],t[1],t[2]):2===t.length?new v(t[0],t[1]):null:null==t?t:"object"==typeof t&&"lat"in t?new v(t.lat,"lng"in t?t.lng:t.lon,t.alt):void 0===e?null:new v(t,e,i)}p.prototype={clone:function(){return new p(this.x,this.y)},add:function(t){return this.clone()._add(m(t))},_add:function(t){return this.x+=t.x,this.y+=t.y,this},subtract:function(t){return this.clone()._subtract(m(t))},_subtract:function(t){return this.x-=t.x,this.y-=t.y,this},divideBy:function(t){return this.clone()._divideBy(t)},_divideBy:function(t){return this.x/=t,this.y/=t,this},multiplyBy:function(t){return this.clone()._multiplyBy(t)},_multiplyBy:function(t){return this.x*=t,this.y*=t,this},scaleBy:function(t){return new p(this.x*t.x,this.y*t.y)},unscaleBy:function(t){return new p(this.x/t.x,this.y/t.y)},round:function(){return this.clone()._round()},_round:function(){return this.x=Math.round(this.x),this.y=Math.round(this.y),this},floor:function(){return this.clone()._floor()},_floor:function(){return this.x=Math.floor(this.x),this.y=Math.floor(this.y),this},ceil:function(){return this.clone()._ceil()},_ceil:function(){return this.x=Math.ceil(this.x),this.y=Math.ceil(this.y),this},trunc:function(){return this.clone()._trunc()},_trunc:function(){return this.x=nt(this.x),this.y=nt(this.y),this},distanceTo:function(t){var e=(t=m(t)).x-this.x,t=t.y-this.y;return Math.sqrt(e*e+t*t)},equals:function(t){return(t=m(t)).x===this.x&&t.y===this.y},contains:function(t){return t=m(t),Math.abs(t.x)<=Math.abs(this.x)&&Math.abs(t.y)<=Math.abs(this.y)},toString:function(){return"Point("+i(this.x)+", "+i(this.y)+")"}},f.prototype={extend:function(t){var e,i;if(t){if(t instanceof p||"number"==typeof t[0]||"x"in t)e=i=m(t);else if(e=(t=_(t)).min,i=t.max,!e||!i)return this;this.min||this.max?(this.min.x=Math.min(e.x,this.min.x),this.max.x=Math.max(i.x,this.max.x),this.min.y=Math.min(e.y,this.min.y),this.max.y=Math.max(i.y,this.max.y)):(this.min=e.clone(),this.max=i.clone())}return this},getCenter:function(t){return m((this.min.x+this.max.x)/2,(this.min.y+this.max.y)/2,t)},getBottomLeft:function(){return m(this.min.x,this.max.y)},getTopRight:function(){return m(this.max.x,this.min.y)},getTopLeft:function(){return this.min},getBottomRight:function(){return this.max},getSize:function(){return this.max.subtract(this.min)},contains:function(t){var e,i;return(t=("number"==typeof t[0]||t instanceof p?m:_)(t))instanceof f?(e=t.min,i=t.max):e=i=t,e.x>=this.min.x&&i.x<=this.max.x&&e.y>=this.min.y&&i.y<=this.max.y},intersects:function(t){t=_(t);var e=this.min,i=this.max,n=t.min,t=t.max,o=t.x>=e.x&&n.x<=i.x,t=t.y>=e.y&&n.y<=i.y;return o&&t},overlaps:function(t){t=_(t);var e=this.min,i=this.max,n=t.min,t=t.max,o=t.x>e.x&&n.x<i.x,t=t.y>e.y&&n.y<i.y;return o&&t},isValid:function(){return!(!this.min||!this.max)},pad:function(t){var e=this.min,i=this.max,n=Math.abs(e.x-i.x)*t,t=Math.abs(e.y-i.y)*t;return _(m(e.x-n,e.y-t),m(i.x+n,i.y+t))},equals:function(t){return!!t&&(t=_(t),this.min.equals(t.getTopLeft())&&this.max.equals(t.getBottomRight()))}},s.prototype={extend:function(t){var e,i,n=this._southWest,o=this._northEast;if(t instanceof v)i=e=t;else{if(!(t instanceof s))return t?this.extend(w(t)||g(t)):this;if(e=t._southWest,i=t._northEast,!e||!i)return this}return n||o?(n.lat=Math.min(e.lat,n.lat),n.lng=Math.min(e.lng,n.lng),o.lat=Math.max(i.lat,o.lat),o.lng=Math.max(i.lng,o.lng)):(this._southWest=new v(e.lat,e.lng),this._northEast=new v(i.lat,i.lng)),this},pad:function(t){var e=this._southWest,i=this._northEast,n=Math.abs(e.lat-i.lat)*t,t=Math.abs(e.lng-i.lng)*t;return new s(new v(e.lat-n,e.lng-t),new v(i.lat+n,i.lng+t))},getCenter:function(){return new v((this._southWest.lat+this._northEast.lat)/2,(this._southWest.lng+this._northEast.lng)/2)},getSouthWest:function(){return this._southWest},getNorthEast:function(){return this._northEast},getNorthWest:function(){return new v(this.getNorth(),this.getWest())},getSouthEast:function(){return new v(this.getSouth(),this.getEast())},getWest:function(){return this._southWest.lng},getSouth:function(){return this._southWest.lat},getEast:function(){return this._northEast.lng},getNorth:function(){return this._northEast.lat},contains:function(t){t=("number"==typeof t[0]||t instanceof v||"lat"in t?w:g)(t);var e,i,n=this._southWest,o=this._northEast;return t instanceof s?(e=t.getSouthWest(),i=t.getNorthEast()):e=i=t,e.lat>=n.lat&&i.lat<=o.lat&&e.lng>=n.lng&&i.lng<=o.lng},intersects:function(t){t=g(t);var e=this._southWest,i=this._northEast,n=t.getSouthWest(),t=t.getNorthEast(),o=t.lat>=e.lat&&n.lat<=i.lat,t=t.lng>=e.lng&&n.lng<=i.lng;return o&&t},overlaps:function(t){t=g(t);var e=this._southWest,i=this._northEast,n=t.getSouthWest(),t=t.getNorthEast(),o=t.lat>e.lat&&n.lat<i.lat,t=t.lng>e.lng&&n.lng<i.lng;return o&&t},toBBoxString:function(){return[this.getWest(),this.getSouth(),this.getEast(),this.getNorth()].join(",")},equals:function(t,e){return!!t&&(t=g(t),this._southWest.equals(t.getSouthWest(),e)&&this._northEast.equals(t.getNorthEast(),e))},isValid:function(){return!(!this._southWest||!this._northEast)}};var ot={latLngToPoint:function(t,e){t=this.projection.project(t),e=this.scale(e);return this.transformation._transform(t,e)},pointToLatLng:function(t,e){e=this.scale(e),t=this.transformation.untransform(t,e);return this.projection.unproject(t)},project:function(t){return this.projection.project(t)},unproject:function(t){return this.projection.unproject(t)},scale:function(t){return 256*Math.pow(2,t)},zoom:function(t){return Math.log(t/256)/Math.LN2},getProjectedBounds:function(t){var e;return this.infinite?null:(e=this.projection.bounds,t=this.scale(t),new f(this.transformation.transform(e.min,t),this.transformation.transform(e.max,t)))},infinite:!(v.prototype={equals:function(t,e){return!!t&&(t=w(t),Math.max(Math.abs(this.lat-t.lat),Math.abs(this.lng-t.lng))<=(void 0===e?1e-9:e))},toString:function(t){return"LatLng("+i(this.lat,t)+", "+i(this.lng,t)+")"},distanceTo:function(t){return st.distance(this,w(t))},wrap:function(){return st.wrapLatLng(this)},toBounds:function(t){var t=180*t/40075017,e=t/Math.cos(Math.PI/180*this.lat);return g([this.lat-t,this.lng-e],[this.lat+t,this.lng+e])},clone:function(){return new v(this.lat,this.lng,this.alt)}}),wrapLatLng:function(t){var e=this.wrapLng?H(t.lng,this.wrapLng,!0):t.lng;return new v(this.wrapLat?H(t.lat,this.wrapLat,!0):t.lat,e,t.alt)},wrapLatLngBounds:function(t){var e=t.getCenter(),i=this.wrapLatLng(e),n=e.lat-i.lat,e=e.lng-i.lng;return 0==n&&0==e?t:(i=t.getSouthWest(),t=t.getNorthEast(),new s(new v(i.lat-n,i.lng-e),new v(t.lat-n,t.lng-e)))}},st=l({},ot,{wrapLng:[-180,180],R:6371e3,distance:function(t,e){var i=Math.PI/180,n=t.lat*i,o=e.lat*i,s=Math.sin((e.lat-t.lat)*i/2),e=Math.sin((e.lng-t.lng)*i/2),t=s*s+Math.cos(n)*Math.cos(o)*e*e,i=2*Math.atan2(Math.sqrt(t),Math.sqrt(1-t));return this.R*i}}),rt=6378137,rt={R:rt,MAX_LATITUDE:85.0511287798,project:function(t){var e=Math.PI/180,i=this.MAX_LATITUDE,i=Math.max(Math.min(i,t.lat),-i),i=Math.sin(i*e);return new p(this.R*t.lng*e,this.R*Math.log((1+i)/(1-i))/2)},unproject:function(t){var e=180/Math.PI;return new v((2*Math.atan(Math.exp(t.y/this.R))-Math.PI/2)*e,t.x*e/this.R)},bounds:new f([-(rt=rt*Math.PI),-rt],[rt,rt])};function at(t,e,i,n){d(t)?(this._a=t[0],this._b=t[1],this._c=t[2],this._d=t[3]):(this._a=t,this._b=e,this._c=i,this._d=n)}function ht(t,e,i,n){return new at(t,e,i,n)}at.prototype={transform:function(t,e){return this._transform(t.clone(),e)},_transform:function(t,e){return t.x=(e=e||1)*(this._a*t.x+this._b),t.y=e*(this._c*t.y+this._d),t},untransform:function(t,e){return new p((t.x/(e=e||1)-this._b)/this._a,(t.y/e-this._d)/this._c)}};var lt=l({},st,{code:"EPSG:3857",projection:rt,transformation:ht(lt=.5/(Math.PI*rt.R),.5,-lt,.5)}),ut=l({},lt,{code:"EPSG:900913"});function ct(t){return document.createElementNS("http://www.w3.org/2000/svg",t)}function dt(t,e){for(var i,n,o,s,r="",a=0,h=t.length;a<h;a++){for(i=0,n=(o=t[a]).length;i<n;i++)r+=(i?"L":"M")+(s=o[i]).x+" "+s.y;r+=e?b.svg?"z":"x":""}return r||"M0 0"}var _t=document.documentElement.style,pt="ActiveXObject"in window,mt=pt&&!document.addEventListener,n="msLaunchUri"in navigator&&!("documentMode"in document),ft=y("webkit"),gt=y("android"),vt=y("android 2")||y("android 3"),yt=parseInt(/WebKit\/([0-9]+)|$/.exec(navigator.userAgent)[1],10),yt=gt&&y("Google")&&yt<537&&!("AudioNode"in window),xt=!!window.opera,wt=!n&&y("chrome"),bt=y("gecko")&&!ft&&!xt&&!pt,Pt=!wt&&y("safari"),Lt=y("phantom"),o="OTransition"in _t,Tt=0===navigator.platform.indexOf("Win"),Mt=pt&&"transition"in _t,zt="WebKitCSSMatrix"in window&&"m11"in new window.WebKitCSSMatrix&&!vt,_t="MozPerspective"in _t,Ct=!window.L_DISABLE_3D&&(Mt||zt||_t)&&!o&&!Lt,Zt="undefined"!=typeof orientation||y("mobile"),St=Zt&&ft,Et=Zt&&zt,kt=!window.PointerEvent&&window.MSPointerEvent,Ot=!(!window.PointerEvent&&!kt),At="ontouchstart"in window||!!window.TouchEvent,Bt=!window.L_NO_TOUCH&&(At||Ot),It=Zt&&xt,Rt=Zt&&bt,Nt=1<(window.devicePixelRatio||window.screen.deviceXDPI/window.screen.logicalXDPI),Dt=function(){var t=!1;try{var e=Object.defineProperty({},"passive",{get:function(){t=!0}});window.addEventListener("testPassiveEventSupport",u,e),window.removeEventListener("testPassiveEventSupport",u,e)}catch(t){}return t}(),jt=!!document.createElement("canvas").getContext,Ht=!(!document.createElementNS||!ct("svg").createSVGRect),Wt=!!Ht&&((Wt=document.createElement("div")).innerHTML="<svg/>","http://www.w3.org/2000/svg"===(Wt.firstChild&&Wt.firstChild.namespaceURI));function y(t){return 0<=navigator.userAgent.toLowerCase().indexOf(t)}var b={ie:pt,ielt9:mt,edge:n,webkit:ft,android:gt,android23:vt,androidStock:yt,opera:xt,chrome:wt,gecko:bt,safari:Pt,phantom:Lt,opera12:o,win:Tt,ie3d:Mt,webkit3d:zt,gecko3d:_t,any3d:Ct,mobile:Zt,mobileWebkit:St,mobileWebkit3d:Et,msPointer:kt,pointer:Ot,touch:Bt,touchNative:At,mobileOpera:It,mobileGecko:Rt,retina:Nt,passiveEvents:Dt,canvas:jt,svg:Ht,vml:!Ht&&function(){try{var t=document.createElement("div"),e=(t.innerHTML='<v:shape adj="1"/>',t.firstChild);return e.style.behavior="url(#default#VML)",e&&"object"==typeof e.adj}catch(t){return!1}}(),inlineSvg:Wt,mac:0===navigator.platform.indexOf("Mac"),linux:0===navigator.platform.indexOf("Linux")},Ft=b.msPointer?"MSPointerDown":"pointerdown",Ut=b.msPointer?"MSPointerMove":"pointermove",Vt=b.msPointer?"MSPointerUp":"pointerup",qt=b.msPointer?"MSPointerCancel":"pointercancel",Gt={touchstart:Ft,touchmove:Ut,touchend:Vt,touchcancel:qt},Kt={touchstart:function(t,e){e.MSPOINTER_TYPE_TOUCH&&e.pointerType===e.MSPOINTER_TYPE_TOUCH&&O(e);ee(t,e)},touchmove:ee,touchend:ee,touchcancel:ee},Yt={},Xt=!1;function Jt(t,e,i){return"touchstart"!==e||Xt||(document.addEventListener(Ft,$t,!0),document.addEventListener(Ut,Qt,!0),document.addEventListener(Vt,te,!0),document.addEventListener(qt,te,!0),Xt=!0),Kt[e]?(i=Kt[e].bind(this,i),t.addEventListener(Gt[e],i,!1),i):(console.warn("wrong event specified:",e),u)}function $t(t){Yt[t.pointerId]=t}function Qt(t){Yt[t.pointerId]&&(Yt[t.pointerId]=t)}function te(t){delete Yt[t.pointerId]}function ee(t,e){if(e.pointerType!==(e.MSPOINTER_TYPE_MOUSE||"mouse")){for(var i in e.touches=[],Yt)e.touches.push(Yt[i]);e.changedTouches=[e],t(e)}}var ie=200;function ne(t,i){t.addEventListener("dblclick",i);var n,o=0;function e(t){var e;1!==t.detail?n=t.detail:"mouse"===t.pointerType||t.sourceCapabilities&&!t.sourceCapabilities.firesTouchEvents||((e=Ne(t)).some(function(t){return t instanceof HTMLLabelElement&&t.attributes.for})&&!e.some(function(t){return t instanceof HTMLInputElement||t instanceof HTMLSelectElement})||((e=Date.now())-o<=ie?2===++n&&i(function(t){var e,i,n={};for(i in t)e=t[i],n[i]=e&&e.bind?e.bind(t):e;return(t=n).type="dblclick",n.detail=2,n.isTrusted=!1,n._simulated=!0,n}(t)):n=1,o=e))}return t.addEventListener("click",e),{dblclick:i,simDblclick:e}}var oe,se,re,ae,he,le,ue=we(["transform","webkitTransform","OTransform","MozTransform","msTransform"]),ce=we(["webkitTransition","transition","OTransition","MozTransition","msTransition"]),de="webkitTransition"===ce||"OTransition"===ce?ce+"End":"transitionend";function _e(t){return"string"==typeof t?document.getElementById(t):t}function pe(t,e){var i=t.style[e]||t.currentStyle&&t.currentStyle[e];return"auto"===(i=i&&"auto"!==i||!document.defaultView?i:(t=document.defaultView.getComputedStyle(t,null))?t[e]:null)?null:i}function P(t,e,i){t=document.createElement(t);return t.className=e||"",i&&i.appendChild(t),t}function T(t){var e=t.parentNode;e&&e.removeChild(t)}function me(t){for(;t.firstChild;)t.removeChild(t.firstChild)}function fe(t){var e=t.parentNode;e&&e.lastChild!==t&&e.appendChild(t)}function ge(t){var e=t.parentNode;e&&e.firstChild!==t&&e.insertBefore(t,e.firstChild)}function ve(t,e){return void 0!==t.classList?t.classList.contains(e):0<(t=xe(t)).length&&new RegExp("(^|\\s)"+e+"(\\s|$)").test(t)}function M(t,e){var i;if(void 0!==t.classList)for(var n=F(e),o=0,s=n.length;o<s;o++)t.classList.add(n[o]);else ve(t,e)||ye(t,((i=xe(t))?i+" ":"")+e)}function z(t,e){void 0!==t.classList?t.classList.remove(e):ye(t,W((" "+xe(t)+" ").replace(" "+e+" "," ")))}function ye(t,e){void 0===t.className.baseVal?t.className=e:t.className.baseVal=e}function xe(t){return void 0===(t=t.correspondingElement?t.correspondingElement:t).className.baseVal?t.className:t.className.baseVal}function C(t,e){if("opacity"in t.style)t.style.opacity=e;else if("filter"in t.style){var i=!1,n="DXImageTransform.Microsoft.Alpha";try{i=t.filters.item(n)}catch(t){if(1===e)return}e=Math.round(100*e),i?(i.Enabled=100!==e,i.Opacity=e):t.style.filter+=" progid:"+n+"(opacity="+e+")"}}function we(t){for(var e=document.documentElement.style,i=0;i<t.length;i++)if(t[i]in e)return t[i];return!1}function be(t,e,i){e=e||new p(0,0);t.style[ue]=(b.ie3d?"translate("+e.x+"px,"+e.y+"px)":"translate3d("+e.x+"px,"+e.y+"px,0)")+(i?" scale("+i+")":"")}function Z(t,e){t._leaflet_pos=e,b.any3d?be(t,e):(t.style.left=e.x+"px",t.style.top=e.y+"px")}function Pe(t){return t._leaflet_pos||new p(0,0)}function Le(){S(window,"dragstart",O)}function Te(){k(window,"dragstart",O)}function Me(t){for(;-1===t.tabIndex;)t=t.parentNode;t.style&&(ze(),le=(he=t).style.outlineStyle,t.style.outlineStyle="none",S(window,"keydown",ze))}function ze(){he&&(he.style.outlineStyle=le,le=he=void 0,k(window,"keydown",ze))}function Ce(t){for(;!((t=t.parentNode).offsetWidth&&t.offsetHeight||t===document.body););return t}function Ze(t){var e=t.getBoundingClientRect();return{x:e.width/t.offsetWidth||1,y:e.height/t.offsetHeight||1,boundingClientRect:e}}ae="onselectstart"in document?(re=function(){S(window,"selectstart",O)},function(){k(window,"selectstart",O)}):(se=we(["userSelect","WebkitUserSelect","OUserSelect","MozUserSelect","msUserSelect"]),re=function(){var t;se&&(t=document.documentElement.style,oe=t[se],t[se]="none")},function(){se&&(document.documentElement.style[se]=oe,oe=void 0)});pt={__proto__:null,TRANSFORM:ue,TRANSITION:ce,TRANSITION_END:de,get:_e,getStyle:pe,create:P,remove:T,empty:me,toFront:fe,toBack:ge,hasClass:ve,addClass:M,removeClass:z,setClass:ye,getClass:xe,setOpacity:C,testProp:we,setTransform:be,setPosition:Z,getPosition:Pe,get disableTextSelection(){return re},get enableTextSelection(){return ae},disableImageDrag:Le,enableImageDrag:Te,preventOutline:Me,restoreOutline:ze,getSizedParentNode:Ce,getScale:Ze};function S(t,e,i,n){if(e&&"object"==typeof e)for(var o in e)ke(t,o,e[o],i);else for(var s=0,r=(e=F(e)).length;s<r;s++)ke(t,e[s],i,n);return this}var E="_leaflet_events";function k(t,e,i,n){if(1===arguments.length)Se(t),delete t[E];else if(e&&"object"==typeof e)for(var o in e)Oe(t,o,e[o],i);else if(e=F(e),2===arguments.length)Se(t,function(t){return-1!==G(e,t)});else for(var s=0,r=e.length;s<r;s++)Oe(t,e[s],i,n);return this}function Se(t,e){for(var i in t[E]){var n=i.split(/\d/)[0];e&&!e(n)||Oe(t,n,null,null,i)}}var Ee={mouseenter:"mouseover",mouseleave:"mouseout",wheel:!("onwheel"in window)&&"mousewheel"};function ke(e,t,i,n){var o,s,r=t+h(i)+(n?"_"+h(n):"");e[E]&&e[E][r]||(s=o=function(t){return i.call(n||e,t||window.event)},!b.touchNative&&b.pointer&&0===t.indexOf("touch")?o=Jt(e,t,o):b.touch&&"dblclick"===t?o=ne(e,o):"addEventListener"in e?"touchstart"===t||"touchmove"===t||"wheel"===t||"mousewheel"===t?e.addEventListener(Ee[t]||t,o,!!b.passiveEvents&&{passive:!1}):"mouseenter"===t||"mouseleave"===t?e.addEventListener(Ee[t],o=function(t){t=t||window.event,We(e,t)&&s(t)},!1):e.addEventListener(t,s,!1):e.attachEvent("on"+t,o),e[E]=e[E]||{},e[E][r]=o)}function Oe(t,e,i,n,o){o=o||e+h(i)+(n?"_"+h(n):"");var s,r,i=t[E]&&t[E][o];i&&(!b.touchNative&&b.pointer&&0===e.indexOf("touch")?(n=t,r=i,Gt[s=e]?n.removeEventListener(Gt[s],r,!1):console.warn("wrong event specified:",s)):b.touch&&"dblclick"===e?(n=i,(r=t).removeEventListener("dblclick",n.dblclick),r.removeEventListener("click",n.simDblclick)):"removeEventListener"in t?t.removeEventListener(Ee[e]||e,i,!1):t.detachEvent("on"+e,i),t[E][o]=null)}function Ae(t){return t.stopPropagation?t.stopPropagation():t.originalEvent?t.originalEvent._stopped=!0:t.cancelBubble=!0,this}function Be(t){return ke(t,"wheel",Ae),this}function Ie(t){return S(t,"mousedown touchstart dblclick contextmenu",Ae),t._leaflet_disable_click=!0,this}function O(t){return t.preventDefault?t.preventDefault():t.returnValue=!1,this}function Re(t){return O(t),Ae(t),this}function Ne(t){if(t.composedPath)return t.composedPath();for(var e=[],i=t.target;i;)e.push(i),i=i.parentNode;return e}function De(t,e){var i,n;return e?(n=(i=Ze(e)).boundingClientRect,new p((t.clientX-n.left)/i.x-e.clientLeft,(t.clientY-n.top)/i.y-e.clientTop)):new p(t.clientX,t.clientY)}var je=b.linux&&b.chrome?window.devicePixelRatio:b.mac?3*window.devicePixelRatio:0<window.devicePixelRatio?2*window.devicePixelRatio:1;function He(t){return b.edge?t.wheelDeltaY/2:t.deltaY&&0===t.deltaMode?-t.deltaY/je:t.deltaY&&1===t.deltaMode?20*-t.deltaY:t.deltaY&&2===t.deltaMode?60*-t.deltaY:t.deltaX||t.deltaZ?0:t.wheelDelta?(t.wheelDeltaY||t.wheelDelta)/2:t.detail&&Math.abs(t.detail)<32765?20*-t.detail:t.detail?t.detail/-32765*60:0}function We(t,e){var i=e.relatedTarget;if(!i)return!0;try{for(;i&&i!==t;)i=i.parentNode}catch(t){return!1}return i!==t}var mt={__proto__:null,on:S,off:k,stopPropagation:Ae,disableScrollPropagation:Be,disableClickPropagation:Ie,preventDefault:O,stop:Re,getPropagationPath:Ne,getMousePosition:De,getWheelDelta:He,isExternalTarget:We,addListener:S,removeListener:k},Fe=it.extend({run:function(t,e,i,n){this.stop(),this._el=t,this._inProgress=!0,this._duration=i||.25,this._easeOutPower=1/Math.max(n||.5,.2),this._startPos=Pe(t),this._offset=e.subtract(this._startPos),this._startTime=+new Date,this.fire("start"),this._animate()},stop:function(){this._inProgress&&(this._step(!0),this._complete())},_animate:function(){this._animId=x(this._animate,this),this._step()},_step:function(t){var e=+new Date-this._startTime,i=1e3*this._duration;e<i?this._runFrame(this._easeOut(e/i),t):(this._runFrame(1),this._complete())},_runFrame:function(t,e){t=this._startPos.add(this._offset.multiplyBy(t));e&&t._round(),Z(this._el,t),this.fire("step")},_complete:function(){r(this._animId),this._inProgress=!1,this.fire("end")},_easeOut:function(t){return 1-Math.pow(1-t,this._easeOutPower)}}),A=it.extend({options:{crs:lt,center:void 0,zoom:void 0,minZoom:void 0,maxZoom:void 0,layers:[],maxBounds:void 0,renderer:void 0,zoomAnimation:!0,zoomAnimationThreshold:4,fadeAnimation:!0,markerZoomAnimation:!0,transform3DLimit:8388608,zoomSnap:1,zoomDelta:1,trackResize:!0},initialize:function(t,e){e=c(this,e),this._handlers=[],this._layers={},this._zoomBoundLayers={},this._sizeChanged=!0,this._initContainer(t),this._initLayout(),this._onResize=a(this._onResize,this),this._initEvents(),e.maxBounds&&this.setMaxBounds(e.maxBounds),void 0!==e.zoom&&(this._zoom=this._limitZoom(e.zoom)),e.center&&void 0!==e.zoom&&this.setView(w(e.center),e.zoom,{reset:!0}),this.callInitHooks(),this._zoomAnimated=ce&&b.any3d&&!b.mobileOpera&&this.options.zoomAnimation,this._zoomAnimated&&(this._createAnimProxy(),S(this._proxy,de,this._catchTransitionEnd,this)),this._addLayers(this.options.layers)},setView:function(t,e,i){if((e=void 0===e?this._zoom:this._limitZoom(e),t=this._limitCenter(w(t),e,this.options.maxBounds),i=i||{},this._stop(),this._loaded&&!i.reset&&!0!==i)&&(void 0!==i.animate&&(i.zoom=l({animate:i.animate},i.zoom),i.pan=l({animate:i.animate,duration:i.duration},i.pan)),this._zoom!==e?this._tryAnimatedZoom&&this._tryAnimatedZoom(t,e,i.zoom):this._tryAnimatedPan(t,i.pan)))return clearTimeout(this._sizeTimer),this;return this._resetView(t,e,i.pan&&i.pan.noMoveStart),this},setZoom:function(t,e){return this._loaded?this.setView(this.getCenter(),t,{zoom:e}):(this._zoom=t,this)},zoomIn:function(t,e){return t=t||(b.any3d?this.options.zoomDelta:1),this.setZoom(this._zoom+t,e)},zoomOut:function(t,e){return t=t||(b.any3d?this.options.zoomDelta:1),this.setZoom(this._zoom-t,e)},setZoomAround:function(t,e,i){var n=this.getZoomScale(e),o=this.getSize().divideBy(2),t=(t instanceof p?t:this.latLngToContainerPoint(t)).subtract(o).multiplyBy(1-1/n),n=this.containerPointToLatLng(o.add(t));return this.setView(n,e,{zoom:i})},_getBoundsCenterZoom:function(t,e){e=e||{},t=t.getBounds?t.getBounds():g(t);var i=m(e.paddingTopLeft||e.padding||[0,0]),n=m(e.paddingBottomRight||e.padding||[0,0]),o=this.getBoundsZoom(t,!1,i.add(n));return(o="number"==typeof e.maxZoom?Math.min(e.maxZoom,o):o)===1/0?{center:t.getCenter(),zoom:o}:(e=n.subtract(i).divideBy(2),n=this.project(t.getSouthWest(),o),i=this.project(t.getNorthEast(),o),{center:this.unproject(n.add(i).divideBy(2).add(e),o),zoom:o})},fitBounds:function(t,e){if((t=g(t)).isValid())return t=this._getBoundsCenterZoom(t,e),this.setView(t.center,t.zoom,e);throw new Error("Bounds are not valid.")},fitWorld:function(t){return this.fitBounds([[-90,-180],[90,180]],t)},panTo:function(t,e){return this.setView(t,this._zoom,{pan:e})},panBy:function(t,e){var i;return e=e||{},(t=m(t).round()).x||t.y?(!0===e.animate||this.getSize().contains(t)?(this._panAnim||(this._panAnim=new Fe,this._panAnim.on({step:this._onPanTransitionStep,end:this._onPanTransitionEnd},this)),e.noMoveStart||this.fire("movestart"),!1!==e.animate?(M(this._mapPane,"leaflet-pan-anim"),i=this._getMapPanePos().subtract(t).round(),this._panAnim.run(this._mapPane,i,e.duration||.25,e.easeLinearity)):(this._rawPanBy(t),this.fire("move").fire("moveend"))):this._resetView(this.unproject(this.project(this.getCenter()).add(t)),this.getZoom()),this):this.fire("moveend")},flyTo:function(n,o,t){if(!1===(t=t||{}).animate||!b.any3d)return this.setView(n,o,t);this._stop();var s=this.project(this.getCenter()),r=this.project(n),e=this.getSize(),a=this._zoom,h=(n=w(n),o=void 0===o?a:o,Math.max(e.x,e.y)),i=h*this.getZoomScale(a,o),l=r.distanceTo(s)||1,u=1.42,c=u*u;function d(t){t=(i*i-h*h+(t?-1:1)*c*c*l*l)/(2*(t?i:h)*c*l),t=Math.sqrt(t*t+1)-t;return t<1e-9?-18:Math.log(t)}function _(t){return(Math.exp(t)-Math.exp(-t))/2}function p(t){return(Math.exp(t)+Math.exp(-t))/2}var m=d(0);function f(t){return h*(p(m)*(_(t=m+u*t)/p(t))-_(m))/c}var g=Date.now(),v=(d(1)-m)/u,y=t.duration?1e3*t.duration:1e3*v*.8;return this._moveStart(!0,t.noMoveStart),function t(){var e=(Date.now()-g)/y,i=(1-Math.pow(1-e,1.5))*v;e<=1?(this._flyToFrame=x(t,this),this._move(this.unproject(s.add(r.subtract(s).multiplyBy(f(i)/l)),a),this.getScaleZoom(h/(e=i,h*(p(m)/p(m+u*e))),a),{flyTo:!0})):this._move(n,o)._moveEnd(!0)}.call(this),this},flyToBounds:function(t,e){t=this._getBoundsCenterZoom(t,e);return this.flyTo(t.center,t.zoom,e)},setMaxBounds:function(t){return t=g(t),this.listens("moveend",this._panInsideMaxBounds)&&this.off("moveend",this._panInsideMaxBounds),t.isValid()?(this.options.maxBounds=t,this._loaded&&this._panInsideMaxBounds(),this.on("moveend",this._panInsideMaxBounds)):(this.options.maxBounds=null,this)},setMinZoom:function(t){var e=this.options.minZoom;return this.options.minZoom=t,this._loaded&&e!==t&&(this.fire("zoomlevelschange"),this.getZoom()<this.options.minZoom)?this.setZoom(t):this},setMaxZoom:function(t){var e=this.options.maxZoom;return this.options.maxZoom=t,this._loaded&&e!==t&&(this.fire("zoomlevelschange"),this.getZoom()>this.options.maxZoom)?this.setZoom(t):this},panInsideBounds:function(t,e){this._enforcingBounds=!0;var i=this.getCenter(),t=this._limitCenter(i,this._zoom,g(t));return i.equals(t)||this.panTo(t,e),this._enforcingBounds=!1,this},panInside:function(t,e){var i=m((e=e||{}).paddingTopLeft||e.padding||[0,0]),n=m(e.paddingBottomRight||e.padding||[0,0]),o=this.project(this.getCenter()),t=this.project(t),s=this.getPixelBounds(),i=_([s.min.add(i),s.max.subtract(n)]),s=i.getSize();return i.contains(t)||(this._enforcingBounds=!0,n=t.subtract(i.getCenter()),i=i.extend(t).getSize().subtract(s),o.x+=n.x<0?-i.x:i.x,o.y+=n.y<0?-i.y:i.y,this.panTo(this.unproject(o),e),this._enforcingBounds=!1),this},invalidateSize:function(t){if(!this._loaded)return this;t=l({animate:!1,pan:!0},!0===t?{animate:!0}:t);var e=this.getSize(),i=(this._sizeChanged=!0,this._lastCenter=null,this.getSize()),n=e.divideBy(2).round(),o=i.divideBy(2).round(),n=n.subtract(o);return n.x||n.y?(t.animate&&t.pan?this.panBy(n):(t.pan&&this._rawPanBy(n),this.fire("move"),t.debounceMoveend?(clearTimeout(this._sizeTimer),this._sizeTimer=setTimeout(a(this.fire,this,"moveend"),200)):this.fire("moveend")),this.fire("resize",{oldSize:e,newSize:i})):this},stop:function(){return this.setZoom(this._limitZoom(this._zoom)),this.options.zoomSnap||this.fire("viewreset"),this._stop()},locate:function(t){var e,i;return t=this._locateOptions=l({timeout:1e4,watch:!1},t),"geolocation"in navigator?(e=a(this._handleGeolocationResponse,this),i=a(this._handleGeolocationError,this),t.watch?this._locationWatchId=navigator.geolocation.watchPosition(e,i,t):navigator.geolocation.getCurrentPosition(e,i,t)):this._handleGeolocationError({code:0,message:"Geolocation not supported."}),this},stopLocate:function(){return navigator.geolocation&&navigator.geolocation.clearWatch&&navigator.geolocation.clearWatch(this._locationWatchId),this._locateOptions&&(this._locateOptions.setView=!1),this},_handleGeolocationError:function(t){var e;this._container._leaflet_id&&(e=t.code,t=t.message||(1===e?"permission denied":2===e?"position unavailable":"timeout"),this._locateOptions.setView&&!this._loaded&&this.fitWorld(),this.fire("locationerror",{code:e,message:"Geolocation error: "+t+"."}))},_handleGeolocationResponse:function(t){if(this._container._leaflet_id){var e,i,n=new v(t.coords.latitude,t.coords.longitude),o=n.toBounds(2*t.coords.accuracy),s=this._locateOptions,r=(s.setView&&(e=this.getBoundsZoom(o),this.setView(n,s.maxZoom?Math.min(e,s.maxZoom):e)),{latlng:n,bounds:o,timestamp:t.timestamp});for(i in t.coords)"number"==typeof t.coords[i]&&(r[i]=t.coords[i]);this.fire("locationfound",r)}},addHandler:function(t,e){return e&&(e=this[t]=new e(this),this._handlers.push(e),this.options[t]&&e.enable()),this},remove:function(){if(this._initEvents(!0),this.options.maxBounds&&this.off("moveend",this._panInsideMaxBounds),this._containerId!==this._container._leaflet_id)throw new Error("Map container is being reused by another instance");try{delete this._container._leaflet_id,delete this._containerId}catch(t){this._container._leaflet_id=void 0,this._containerId=void 0}for(var t in void 0!==this._locationWatchId&&this.stopLocate(),this._stop(),T(this._mapPane),this._clearControlPos&&this._clearControlPos(),this._resizeRequest&&(r(this._resizeRequest),this._resizeRequest=null),this._clearHandlers(),this._loaded&&this.fire("unload"),this._layers)this._layers[t].remove();for(t in this._panes)T(this._panes[t]);return this._layers=[],this._panes=[],delete this._mapPane,delete this._renderer,this},createPane:function(t,e){e=P("div","leaflet-pane"+(t?" leaflet-"+t.replace("Pane","")+"-pane":""),e||this._mapPane);return t&&(this._panes[t]=e),e},getCenter:function(){return this._checkIfLoaded(),this._lastCenter&&!this._moved()?this._lastCenter.clone():this.layerPointToLatLng(this._getCenterLayerPoint())},getZoom:function(){return this._zoom},getBounds:function(){var t=this.getPixelBounds();return new s(this.unproject(t.getBottomLeft()),this.unproject(t.getTopRight()))},getMinZoom:function(){return void 0===this.options.minZoom?this._layersMinZoom||0:this.options.minZoom},getMaxZoom:function(){return void 0===this.options.maxZoom?void 0===this._layersMaxZoom?1/0:this._layersMaxZoom:this.options.maxZoom},getBoundsZoom:function(t,e,i){t=g(t),i=m(i||[0,0]);var n=this.getZoom()||0,o=this.getMinZoom(),s=this.getMaxZoom(),r=t.getNorthWest(),t=t.getSouthEast(),i=this.getSize().subtract(i),t=_(this.project(t,n),this.project(r,n)).getSize(),r=b.any3d?this.options.zoomSnap:1,a=i.x/t.x,i=i.y/t.y,t=e?Math.max(a,i):Math.min(a,i),n=this.getScaleZoom(t,n);return r&&(n=Math.round(n/(r/100))*(r/100),n=e?Math.ceil(n/r)*r:Math.floor(n/r)*r),Math.max(o,Math.min(s,n))},getSize:function(){return this._size&&!this._sizeChanged||(this._size=new p(this._container.clientWidth||0,this._container.clientHeight||0),this._sizeChanged=!1),this._size.clone()},getPixelBounds:function(t,e){t=this._getTopLeftPoint(t,e);return new f(t,t.add(this.getSize()))},getPixelOrigin:function(){return this._checkIfLoaded(),this._pixelOrigin},getPixelWorldBounds:function(t){return this.options.crs.getProjectedBounds(void 0===t?this.getZoom():t)},getPane:function(t){return"string"==typeof t?this._panes[t]:t},getPanes:function(){return this._panes},getContainer:function(){return this._container},getZoomScale:function(t,e){var i=this.options.crs;return e=void 0===e?this._zoom:e,i.scale(t)/i.scale(e)},getScaleZoom:function(t,e){var i=this.options.crs,t=(e=void 0===e?this._zoom:e,i.zoom(t*i.scale(e)));return isNaN(t)?1/0:t},project:function(t,e){return e=void 0===e?this._zoom:e,this.options.crs.latLngToPoint(w(t),e)},unproject:function(t,e){return e=void 0===e?this._zoom:e,this.options.crs.pointToLatLng(m(t),e)},layerPointToLatLng:function(t){t=m(t).add(this.getPixelOrigin());return this.unproject(t)},latLngToLayerPoint:function(t){return this.project(w(t))._round()._subtract(this.getPixelOrigin())},wrapLatLng:function(t){return this.options.crs.wrapLatLng(w(t))},wrapLatLngBounds:function(t){return this.options.crs.wrapLatLngBounds(g(t))},distance:function(t,e){return this.options.crs.distance(w(t),w(e))},containerPointToLayerPoint:function(t){return m(t).subtract(this._getMapPanePos())},layerPointToContainerPoint:function(t){return m(t).add(this._getMapPanePos())},containerPointToLatLng:function(t){t=this.containerPointToLayerPoint(m(t));return this.layerPointToLatLng(t)},latLngToContainerPoint:function(t){return this.layerPointToContainerPoint(this.latLngToLayerPoint(w(t)))},mouseEventToContainerPoint:function(t){return De(t,this._container)},mouseEventToLayerPoint:function(t){return this.containerPointToLayerPoint(this.mouseEventToContainerPoint(t))},mouseEventToLatLng:function(t){return this.layerPointToLatLng(this.mouseEventToLayerPoint(t))},_initContainer:function(t){t=this._container=_e(t);if(!t)throw new Error("Map container not found.");if(t._leaflet_id)throw new Error("Map container is already initialized.");S(t,"scroll",this._onScroll,this),this._containerId=h(t)},_initLayout:function(){var t=this._container,e=(this._fadeAnimated=this.options.fadeAnimation&&b.any3d,M(t,"leaflet-container"+(b.touch?" leaflet-touch":"")+(b.retina?" leaflet-retina":"")+(b.ielt9?" leaflet-oldie":"")+(b.safari?" leaflet-safari":"")+(this._fadeAnimated?" leaflet-fade-anim":"")),pe(t,"position"));"absolute"!==e&&"relative"!==e&&"fixed"!==e&&"sticky"!==e&&(t.style.position="relative"),this._initPanes(),this._initControlPos&&this._initControlPos()},_initPanes:function(){var t=this._panes={};this._paneRenderers={},this._mapPane=this.createPane("mapPane",this._container),Z(this._mapPane,new p(0,0)),this.createPane("tilePane"),this.createPane("overlayPane"),this.createPane("shadowPane"),this.createPane("markerPane"),this.createPane("tooltipPane"),this.createPane("popupPane"),this.options.markerZoomAnimation||(M(t.markerPane,"leaflet-zoom-hide"),M(t.shadowPane,"leaflet-zoom-hide"))},_resetView:function(t,e,i){Z(this._mapPane,new p(0,0));var n=!this._loaded,o=(this._loaded=!0,e=this._limitZoom(e),this.fire("viewprereset"),this._zoom!==e);this._moveStart(o,i)._move(t,e)._moveEnd(o),this.fire("viewreset"),n&&this.fire("load")},_moveStart:function(t,e){return t&&this.fire("zoomstart"),e||this.fire("movestart"),this},_move:function(t,e,i,n){void 0===e&&(e=this._zoom);var o=this._zoom!==e;return this._zoom=e,this._lastCenter=t,this._pixelOrigin=this._getNewPixelOrigin(t),n?i&&i.pinch&&this.fire("zoom",i):((o||i&&i.pinch)&&this.fire("zoom",i),this.fire("move",i)),this},_moveEnd:function(t){return t&&this.fire("zoomend"),this.fire("moveend")},_stop:function(){return r(this._flyToFrame),this._panAnim&&this._panAnim.stop(),this},_rawPanBy:function(t){Z(this._mapPane,this._getMapPanePos().subtract(t))},_getZoomSpan:function(){return this.getMaxZoom()-this.getMinZoom()},_panInsideMaxBounds:function(){this._enforcingBounds||this.panInsideBounds(this.options.maxBounds)},_checkIfLoaded:function(){if(!this._loaded)throw new Error("Set map center and zoom first.")},_initEvents:function(t){this._targets={};var e=t?k:S;e((this._targets[h(this._container)]=this)._container,"click dblclick mousedown mouseup mouseover mouseout mousemove contextmenu keypress keydown keyup",this._handleDOMEvent,this),this.options.trackResize&&e(window,"resize",this._onResize,this),b.any3d&&this.options.transform3DLimit&&(t?this.off:this.on).call(this,"moveend",this._onMoveEnd)},_onResize:function(){r(this._resizeRequest),this._resizeRequest=x(function(){this.invalidateSize({debounceMoveend:!0})},this)},_onScroll:function(){this._container.scrollTop=0,this._container.scrollLeft=0},_onMoveEnd:function(){var t=this._getMapPanePos();Math.max(Math.abs(t.x),Math.abs(t.y))>=this.options.transform3DLimit&&this._resetView(this.getCenter(),this.getZoom())},_findEventTargets:function(t,e){for(var i,n=[],o="mouseout"===e||"mouseover"===e,s=t.target||t.srcElement,r=!1;s;){if((i=this._targets[h(s)])&&("click"===e||"preclick"===e)&&this._draggableMoved(i)){r=!0;break}if(i&&i.listens(e,!0)){if(o&&!We(s,t))break;if(n.push(i),o)break}if(s===this._container)break;s=s.parentNode}return n=n.length||r||o||!this.listens(e,!0)?n:[this]},_isClickDisabled:function(t){for(;t&&t!==this._container;){if(t._leaflet_disable_click)return!0;t=t.parentNode}},_handleDOMEvent:function(t){var e,i=t.target||t.srcElement;!this._loaded||i._leaflet_disable_events||"click"===t.type&&this._isClickDisabled(i)||("mousedown"===(e=t.type)&&Me(i),this._fireDOMEvent(t,e))},_mouseEvents:["click","dblclick","mouseover","mouseout","contextmenu"],_fireDOMEvent:function(t,e,i){"click"===t.type&&((a=l({},t)).type="preclick",this._fireDOMEvent(a,a.type,i));var n=this._findEventTargets(t,e);if(i){for(var o=[],s=0;s<i.length;s++)i[s].listens(e,!0)&&o.push(i[s]);n=o.concat(n)}if(n.length){"contextmenu"===e&&O(t);var r,a=n[0],h={originalEvent:t};for("keypress"!==t.type&&"keydown"!==t.type&&"keyup"!==t.type&&(r=a.getLatLng&&(!a._radius||a._radius<=10),h.containerPoint=r?this.latLngToContainerPoint(a.getLatLng()):this.mouseEventToContainerPoint(t),h.layerPoint=this.containerPointToLayerPoint(h.containerPoint),h.latlng=r?a.getLatLng():this.layerPointToLatLng(h.layerPoint)),s=0;s<n.length;s++)if(n[s].fire(e,h,!0),h.originalEvent._stopped||!1===n[s].options.bubblingMouseEvents&&-1!==G(this._mouseEvents,e))return}},_draggableMoved:function(t){return(t=t.dragging&&t.dragging.enabled()?t:this).dragging&&t.dragging.moved()||this.boxZoom&&this.boxZoom.moved()},_clearHandlers:function(){for(var t=0,e=this._handlers.length;t<e;t++)this._handlers[t].disable()},whenReady:function(t,e){return this._loaded?t.call(e||this,{target:this}):this.on("load",t,e),this},_getMapPanePos:function(){return Pe(this._mapPane)||new p(0,0)},_moved:function(){var t=this._getMapPanePos();return t&&!t.equals([0,0])},_getTopLeftPoint:function(t,e){return(t&&void 0!==e?this._getNewPixelOrigin(t,e):this.getPixelOrigin()).subtract(this._getMapPanePos())},_getNewPixelOrigin:function(t,e){var i=this.getSize()._divideBy(2);return this.project(t,e)._subtract(i)._add(this._getMapPanePos())._round()},_latLngToNewLayerPoint:function(t,e,i){i=this._getNewPixelOrigin(i,e);return this.project(t,e)._subtract(i)},_latLngBoundsToNewLayerBounds:function(t,e,i){i=this._getNewPixelOrigin(i,e);return _([this.project(t.getSouthWest(),e)._subtract(i),this.project(t.getNorthWest(),e)._subtract(i),this.project(t.getSouthEast(),e)._subtract(i),this.project(t.getNorthEast(),e)._subtract(i)])},_getCenterLayerPoint:function(){return this.containerPointToLayerPoint(this.getSize()._divideBy(2))},_getCenterOffset:function(t){return this.latLngToLayerPoint(t).subtract(this._getCenterLayerPoint())},_limitCenter:function(t,e,i){var n,o;return!i||(n=this.project(t,e),o=this.getSize().divideBy(2),o=new f(n.subtract(o),n.add(o)),o=this._getBoundsOffset(o,i,e),Math.abs(o.x)<=1&&Math.abs(o.y)<=1)?t:this.unproject(n.add(o),e)},_limitOffset:function(t,e){var i;return e?(i=new f((i=this.getPixelBounds()).min.add(t),i.max.add(t)),t.add(this._getBoundsOffset(i,e))):t},_getBoundsOffset:function(t,e,i){e=_(this.project(e.getNorthEast(),i),this.project(e.getSouthWest(),i)),i=e.min.subtract(t.min),e=e.max.subtract(t.max);return new p(this._rebound(i.x,-e.x),this._rebound(i.y,-e.y))},_rebound:function(t,e){return 0<t+e?Math.round(t-e)/2:Math.max(0,Math.ceil(t))-Math.max(0,Math.floor(e))},_limitZoom:function(t){var e=this.getMinZoom(),i=this.getMaxZoom(),n=b.any3d?this.options.zoomSnap:1;return n&&(t=Math.round(t/n)*n),Math.max(e,Math.min(i,t))},_onPanTransitionStep:function(){this.fire("move")},_onPanTransitionEnd:function(){z(this._mapPane,"leaflet-pan-anim"),this.fire("moveend")},_tryAnimatedPan:function(t,e){t=this._getCenterOffset(t)._trunc();return!(!0!==(e&&e.animate)&&!this.getSize().contains(t))&&(this.panBy(t,e),!0)},_createAnimProxy:function(){var t=this._proxy=P("div","leaflet-proxy leaflet-zoom-animated");this._panes.mapPane.appendChild(t),this.on("zoomanim",function(t){var e=ue,i=this._proxy.style[e];be(this._proxy,this.project(t.center,t.zoom),this.getZoomScale(t.zoom,1)),i===this._proxy.style[e]&&this._animatingZoom&&this._onZoomTransitionEnd()},this),this.on("load moveend",this._animMoveEnd,this),this._on("unload",this._destroyAnimProxy,this)},_destroyAnimProxy:function(){T(this._proxy),this.off("load moveend",this._animMoveEnd,this),delete this._proxy},_animMoveEnd:function(){var t=this.getCenter(),e=this.getZoom();be(this._proxy,this.project(t,e),this.getZoomScale(e,1))},_catchTransitionEnd:function(t){this._animatingZoom&&0<=t.propertyName.indexOf("transform")&&this._onZoomTransitionEnd()},_nothingToAnimate:function(){return!this._container.getElementsByClassName("leaflet-zoom-animated").length},_tryAnimatedZoom:function(t,e,i){if(!this._animatingZoom){if(i=i||{},!this._zoomAnimated||!1===i.animate||this._nothingToAnimate()||Math.abs(e-this._zoom)>this.options.zoomAnimationThreshold)return!1;var n=this.getZoomScale(e),n=this._getCenterOffset(t)._divideBy(1-1/n);if(!0!==i.animate&&!this.getSize().contains(n))return!1;x(function(){this._moveStart(!0,i.noMoveStart||!1)._animateZoom(t,e,!0)},this)}return!0},_animateZoom:function(t,e,i,n){this._mapPane&&(i&&(this._animatingZoom=!0,this._animateToCenter=t,this._animateToZoom=e,M(this._mapPane,"leaflet-zoom-anim")),this.fire("zoomanim",{center:t,zoom:e,noUpdate:n}),this._tempFireZoomEvent||(this._tempFireZoomEvent=this._zoom!==this._animateToZoom),this._move(this._animateToCenter,this._animateToZoom,void 0,!0),setTimeout(a(this._onZoomTransitionEnd,this),250))},_onZoomTransitionEnd:function(){this._animatingZoom&&(this._mapPane&&z(this._mapPane,"leaflet-zoom-anim"),this._animatingZoom=!1,this._move(this._animateToCenter,this._animateToZoom,void 0,!0),this._tempFireZoomEvent&&this.fire("zoom"),delete this._tempFireZoomEvent,this.fire("move"),this._moveEnd(!0))}});function Ue(t){return new B(t)}var B=et.extend({options:{position:"topright"},initialize:function(t){c(this,t)},getPosition:function(){return this.options.position},setPosition:function(t){var e=this._map;return e&&e.removeControl(this),this.options.position=t,e&&e.addControl(this),this},getContainer:function(){return this._container},addTo:function(t){this.remove(),this._map=t;var e=this._container=this.onAdd(t),i=this.getPosition(),t=t._controlCorners[i];return M(e,"leaflet-control"),-1!==i.indexOf("bottom")?t.insertBefore(e,t.firstChild):t.appendChild(e),this._map.on("unload",this.remove,this),this},remove:function(){return this._map&&(T(this._container),this.onRemove&&this.onRemove(this._map),this._map.off("unload",this.remove,this),this._map=null),this},_refocusOnMap:function(t){this._map&&t&&0<t.screenX&&0<t.screenY&&this._map.getContainer().focus()}}),Ve=(A.include({addControl:function(t){return t.addTo(this),this},removeControl:function(t){return t.remove(),this},_initControlPos:function(){var i=this._controlCorners={},n="leaflet-",o=this._controlContainer=P("div",n+"control-container",this._container);function t(t,e){i[t+e]=P("div",n+t+" "+n+e,o)}t("top","left"),t("top","right"),t("bottom","left"),t("bottom","right")},_clearControlPos:function(){for(var t in this._controlCorners)T(this._controlCorners[t]);T(this._controlContainer),delete this._controlCorners,delete this._controlContainer}}),B.extend({options:{collapsed:!0,position:"topright",autoZIndex:!0,hideSingleBase:!1,sortLayers:!1,sortFunction:function(t,e,i,n){return i<n?-1:n<i?1:0}},initialize:function(t,e,i){for(var n in c(this,i),this._layerControlInputs=[],this._layers=[],this._lastZIndex=0,this._handlingClick=!1,this._preventClick=!1,t)this._addLayer(t[n],n);for(n in e)this._addLayer(e[n],n,!0)},onAdd:function(t){this._initLayout(),this._update(),(this._map=t).on("zoomend",this._checkDisabledLayers,this);for(var e=0;e<this._layers.length;e++)this._layers[e].layer.on("add remove",this._onLayerChange,this);return this._container},addTo:function(t){return B.prototype.addTo.call(this,t),this._expandIfNotCollapsed()},onRemove:function(){this._map.off("zoomend",this._checkDisabledLayers,this);for(var t=0;t<this._layers.length;t++)this._layers[t].layer.off("add remove",this._onLayerChange,this)},addBaseLayer:function(t,e){return this._addLayer(t,e),this._map?this._update():this},addOverlay:function(t,e){return this._addLayer(t,e,!0),this._map?this._update():this},removeLayer:function(t){t.off("add remove",this._onLayerChange,this);t=this._getLayer(h(t));return t&&this._layers.splice(this._layers.indexOf(t),1),this._map?this._update():this},expand:function(){M(this._container,"leaflet-control-layers-expanded"),this._section.style.height=null;var t=this._map.getSize().y-(this._container.offsetTop+50);return t<this._section.clientHeight?(M(this._section,"leaflet-control-layers-scrollbar"),this._section.style.height=t+"px"):z(this._section,"leaflet-control-layers-scrollbar"),this._checkDisabledLayers(),this},collapse:function(){return z(this._container,"leaflet-control-layers-expanded"),this},_initLayout:function(){var t="leaflet-control-layers",e=this._container=P("div",t),i=this.options.collapsed,n=(e.setAttribute("aria-haspopup",!0),Ie(e),Be(e),this._section=P("section",t+"-list")),o=(i&&(this._map.on("click",this.collapse,this),S(e,{mouseenter:this._expandSafely,mouseleave:this.collapse},this)),this._layersLink=P("a",t+"-toggle",e));o.href="#",o.title="Layers",o.setAttribute("role","button"),S(o,{keydown:function(t){13===t.keyCode&&this._expandSafely()},click:function(t){O(t),this._expandSafely()}},this),i||this.expand(),this._baseLayersList=P("div",t+"-base",n),this._separator=P("div",t+"-separator",n),this._overlaysList=P("div",t+"-overlays",n),e.appendChild(n)},_getLayer:function(t){for(var e=0;e<this._layers.length;e++)if(this._layers[e]&&h(this._layers[e].layer)===t)return this._layers[e]},_addLayer:function(t,e,i){this._map&&t.on("add remove",this._onLayerChange,this),this._layers.push({layer:t,name:e,overlay:i}),this.options.sortLayers&&this._layers.sort(a(function(t,e){return this.options.sortFunction(t.layer,e.layer,t.name,e.name)},this)),this.options.autoZIndex&&t.setZIndex&&(this._lastZIndex++,t.setZIndex(this._lastZIndex)),this._expandIfNotCollapsed()},_update:function(){if(this._container){me(this._baseLayersList),me(this._overlaysList),this._layerControlInputs=[];for(var t,e,i,n=0,o=0;o<this._layers.length;o++)i=this._layers[o],this._addItem(i),e=e||i.overlay,t=t||!i.overlay,n+=i.overlay?0:1;this.options.hideSingleBase&&(this._baseLayersList.style.display=(t=t&&1<n)?"":"none"),this._separator.style.display=e&&t?"":"none"}return this},_onLayerChange:function(t){this._handlingClick||this._update();var e=this._getLayer(h(t.target)),t=e.overlay?"add"===t.type?"overlayadd":"overlayremove":"add"===t.type?"baselayerchange":null;t&&this._map.fire(t,e)},_createRadioElement:function(t,e){t='<input type="radio" class="leaflet-control-layers-selector" name="'+t+'"'+(e?' checked="checked"':"")+"/>",e=document.createElement("div");return e.innerHTML=t,e.firstChild},_addItem:function(t){var e,i=document.createElement("label"),n=this._map.hasLayer(t.layer),n=(t.overlay?((e=document.createElement("input")).type="checkbox",e.className="leaflet-control-layers-selector",e.defaultChecked=n):e=this._createRadioElement("leaflet-base-layers_"+h(this),n),this._layerControlInputs.push(e),e.layerId=h(t.layer),S(e,"click",this._onInputClick,this),document.createElement("span")),o=(n.innerHTML=" "+t.name,document.createElement("span"));return i.appendChild(o),o.appendChild(e),o.appendChild(n),(t.overlay?this._overlaysList:this._baseLayersList).appendChild(i),this._checkDisabledLayers(),i},_onInputClick:function(){if(!this._preventClick){var t,e,i=this._layerControlInputs,n=[],o=[];this._handlingClick=!0;for(var s=i.length-1;0<=s;s--)t=i[s],e=this._getLayer(t.layerId).layer,t.checked?n.push(e):t.checked||o.push(e);for(s=0;s<o.length;s++)this._map.hasLayer(o[s])&&this._map.removeLayer(o[s]);for(s=0;s<n.length;s++)this._map.hasLayer(n[s])||this._map.addLayer(n[s]);this._handlingClick=!1,this._refocusOnMap()}},_checkDisabledLayers:function(){for(var t,e,i=this._layerControlInputs,n=this._map.getZoom(),o=i.length-1;0<=o;o--)t=i[o],e=this._getLayer(t.layerId).layer,t.disabled=void 0!==e.options.minZoom&&n<e.options.minZoom||void 0!==e.options.maxZoom&&n>e.options.maxZoom},_expandIfNotCollapsed:function(){return this._map&&!this.options.collapsed&&this.expand(),this},_expandSafely:function(){var t=this._section,e=(this._preventClick=!0,S(t,"click",O),this.expand(),this);setTimeout(function(){k(t,"click",O),e._preventClick=!1})}})),qe=B.extend({options:{position:"topleft",zoomInText:'<span aria-hidden="true">+</span>',zoomInTitle:"Zoom in",zoomOutText:'<span aria-hidden="true">&#x2212;</span>',zoomOutTitle:"Zoom out"},onAdd:function(t){var e="leaflet-control-zoom",i=P("div",e+" leaflet-bar"),n=this.options;return this._zoomInButton=this._createButton(n.zoomInText,n.zoomInTitle,e+"-in",i,this._zoomIn),this._zoomOutButton=this._createButton(n.zoomOutText,n.zoomOutTitle,e+"-out",i,this._zoomOut),this._updateDisabled(),t.on("zoomend zoomlevelschange",this._updateDisabled,this),i},onRemove:function(t){t.off("zoomend zoomlevelschange",this._updateDisabled,this)},disable:function(){return this._disabled=!0,this._updateDisabled(),this},enable:function(){return this._disabled=!1,this._updateDisabled(),this},_zoomIn:function(t){!this._disabled&&this._map._zoom<this._map.getMaxZoom()&&this._map.zoomIn(this._map.options.zoomDelta*(t.shiftKey?3:1))},_zoomOut:function(t){!this._disabled&&this._map._zoom>this._map.getMinZoom()&&this._map.zoomOut(this._map.options.zoomDelta*(t.shiftKey?3:1))},_createButton:function(t,e,i,n,o){i=P("a",i,n);return i.innerHTML=t,i.href="#",i.title=e,i.setAttribute("role","button"),i.setAttribute("aria-label",e),Ie(i),S(i,"click",Re),S(i,"click",o,this),S(i,"click",this._refocusOnMap,this),i},_updateDisabled:function(){var t=this._map,e="leaflet-disabled";z(this._zoomInButton,e),z(this._zoomOutButton,e),this._zoomInButton.setAttribute("aria-disabled","false"),this._zoomOutButton.setAttribute("aria-disabled","false"),!this._disabled&&t._zoom!==t.getMinZoom()||(M(this._zoomOutButton,e),this._zoomOutButton.setAttribute("aria-disabled","true")),!this._disabled&&t._zoom!==t.getMaxZoom()||(M(this._zoomInButton,e),this._zoomInButton.setAttribute("aria-disabled","true"))}}),Ge=(A.mergeOptions({zoomControl:!0}),A.addInitHook(function(){this.options.zoomControl&&(this.zoomControl=new qe,this.addControl(this.zoomControl))}),B.extend({options:{position:"bottomleft",maxWidth:100,metric:!0,imperial:!0},onAdd:function(t){var e="leaflet-control-scale",i=P("div",e),n=this.options;return this._addScales(n,e+"-line",i),t.on(n.updateWhenIdle?"moveend":"move",this._update,this),t.whenReady(this._update,this),i},onRemove:function(t){t.off(this.options.updateWhenIdle?"moveend":"move",this._update,this)},_addScales:function(t,e,i){t.metric&&(this._mScale=P("div",e,i)),t.imperial&&(this._iScale=P("div",e,i))},_update:function(){var t=this._map,e=t.getSize().y/2,t=t.distance(t.containerPointToLatLng([0,e]),t.containerPointToLatLng([this.options.maxWidth,e]));this._updateScales(t)},_updateScales:function(t){this.options.metric&&t&&this._updateMetric(t),this.options.imperial&&t&&this._updateImperial(t)},_updateMetric:function(t){var e=this._getRoundNum(t);this._updateScale(this._mScale,e<1e3?e+" m":e/1e3+" km",e/t)},_updateImperial:function(t){var e,i,t=3.2808399*t;5280<t?(i=this._getRoundNum(e=t/5280),this._updateScale(this._iScale,i+" mi",i/e)):(i=this._getRoundNum(t),this._updateScale(this._iScale,i+" ft",i/t))},_updateScale:function(t,e,i){t.style.width=Math.round(this.options.maxWidth*i)+"px",t.innerHTML=e},_getRoundNum:function(t){var e=Math.pow(10,(Math.floor(t)+"").length-1),t=t/e;return e*(t=10<=t?10:5<=t?5:3<=t?3:2<=t?2:1)}})),Ke=B.extend({options:{position:"bottomright",prefix:'<a href="https://leafletjs.com" title="A JavaScript library for interactive maps">'+(b.inlineSvg?'<svg aria-hidden="true" xmlns="http://www.w3.org/2000/svg" width="12" height="8" viewBox="0 0 12 8" class="leaflet-attribution-flag"><path fill="#4C7BE1" d="M0 0h12v4H0z"/><path fill="#FFD500" d="M0 4h12v3H0z"/><path fill="#E0BC00" d="M0 7h12v1H0z"/></svg> ':"")+"Leaflet</a>"},initialize:function(t){c(this,t),this._attributions={}},onAdd:function(t){for(var e in(t.attributionControl=this)._container=P("div","leaflet-control-attribution"),Ie(this._container),t._layers)t._layers[e].getAttribution&&this.addAttribution(t._layers[e].getAttribution());return this._update(),t.on("layeradd",this._addAttribution,this),this._container},onRemove:function(t){t.off("layeradd",this._addAttribution,this)},_addAttribution:function(t){t.layer.getAttribution&&(this.addAttribution(t.layer.getAttribution()),t.layer.once("remove",function(){this.removeAttribution(t.layer.getAttribution())},this))},setPrefix:function(t){return this.options.prefix=t,this._update(),this},addAttribution:function(t){return t&&(this._attributions[t]||(this._attributions[t]=0),this._attributions[t]++,this._update()),this},removeAttribution:function(t){return t&&this._attributions[t]&&(this._attributions[t]--,this._update()),this},_update:function(){if(this._map){var t,e=[];for(t in this._attributions)this._attributions[t]&&e.push(t);var i=[];this.options.prefix&&i.push(this.options.prefix),e.length&&i.push(e.join(", ")),this._container.innerHTML=i.join(' <span aria-hidden="true">|</span> ')}}}),n=(A.mergeOptions({attributionControl:!0}),A.addInitHook(function(){this.options.attributionControl&&(new Ke).addTo(this)}),B.Layers=Ve,B.Zoom=qe,B.Scale=Ge,B.Attribution=Ke,Ue.layers=function(t,e,i){return new Ve(t,e,i)},Ue.zoom=function(t){return new qe(t)},Ue.scale=function(t){return new Ge(t)},Ue.attribution=function(t){return new Ke(t)},et.extend({initialize:function(t){this._map=t},enable:function(){return this._enabled||(this._enabled=!0,this.addHooks()),this},disable:function(){return this._enabled&&(this._enabled=!1,this.removeHooks()),this},enabled:function(){return!!this._enabled}})),ft=(n.addTo=function(t,e){return t.addHandler(e,this),this},{Events:e}),Ye=b.touch?"touchstart mousedown":"mousedown",Xe=it.extend({options:{clickTolerance:3},initialize:function(t,e,i,n){c(this,n),this._element=t,this._dragStartTarget=e||t,this._preventOutline=i},enable:function(){this._enabled||(S(this._dragStartTarget,Ye,this._onDown,this),this._enabled=!0)},disable:function(){this._enabled&&(Xe._dragging===this&&this.finishDrag(!0),k(this._dragStartTarget,Ye,this._onDown,this),this._enabled=!1,this._moved=!1)},_onDown:function(t){var e,i;this._enabled&&(this._moved=!1,ve(this._element,"leaflet-zoom-anim")||(t.touches&&1!==t.touches.length?Xe._dragging===this&&this.finishDrag():Xe._dragging||t.shiftKey||1!==t.which&&1!==t.button&&!t.touches||((Xe._dragging=this)._preventOutline&&Me(this._element),Le(),re(),this._moving||(this.fire("down"),i=t.touches?t.touches[0]:t,e=Ce(this._element),this._startPoint=new p(i.clientX,i.clientY),this._startPos=Pe(this._element),this._parentScale=Ze(e),i="mousedown"===t.type,S(document,i?"mousemove":"touchmove",this._onMove,this),S(document,i?"mouseup":"touchend touchcancel",this._onUp,this)))))},_onMove:function(t){var e;this._enabled&&(t.touches&&1<t.touches.length?this._moved=!0:!(e=new p((e=t.touches&&1===t.touches.length?t.touches[0]:t).clientX,e.clientY)._subtract(this._startPoint)).x&&!e.y||Math.abs(e.x)+Math.abs(e.y)<this.options.clickTolerance||(e.x/=this._parentScale.x,e.y/=this._parentScale.y,O(t),this._moved||(this.fire("dragstart"),this._moved=!0,M(document.body,"leaflet-dragging"),this._lastTarget=t.target||t.srcElement,window.SVGElementInstance&&this._lastTarget instanceof window.SVGElementInstance&&(this._lastTarget=this._lastTarget.correspondingUseElement),M(this._lastTarget,"leaflet-drag-target")),this._newPos=this._startPos.add(e),this._moving=!0,this._lastEvent=t,this._updatePosition()))},_updatePosition:function(){var t={originalEvent:this._lastEvent};this.fire("predrag",t),Z(this._element,this._newPos),this.fire("drag",t)},_onUp:function(){this._enabled&&this.finishDrag()},finishDrag:function(t){z(document.body,"leaflet-dragging"),this._lastTarget&&(z(this._lastTarget,"leaflet-drag-target"),this._lastTarget=null),k(document,"mousemove touchmove",this._onMove,this),k(document,"mouseup touchend touchcancel",this._onUp,this),Te(),ae();var e=this._moved&&this._moving;this._moving=!1,Xe._dragging=!1,e&&this.fire("dragend",{noInertia:t,distance:this._newPos.distanceTo(this._startPos)})}});function Je(t,e,i){for(var n,o,s,r,a,h,l,u=[1,4,2,8],c=0,d=t.length;c<d;c++)t[c]._code=si(t[c],e);for(s=0;s<4;s++){for(h=u[s],n=[],c=0,o=(d=t.length)-1;c<d;o=c++)r=t[c],a=t[o],r._code&h?a._code&h||((l=oi(a,r,h,e,i))._code=si(l,e),n.push(l)):(a._code&h&&((l=oi(a,r,h,e,i))._code=si(l,e),n.push(l)),n.push(r));t=n}return t}function $e(t,e){var i,n,o,s,r,a,h;if(!t||0===t.length)throw new Error("latlngs not passed");I(t)||(console.warn("latlngs are not flat! Only the first ring will be used"),t=t[0]);for(var l=w([0,0]),u=g(t),c=(u.getNorthWest().distanceTo(u.getSouthWest())*u.getNorthEast().distanceTo(u.getNorthWest())<1700&&(l=Qe(t)),t.length),d=[],_=0;_<c;_++){var p=w(t[_]);d.push(e.project(w([p.lat-l.lat,p.lng-l.lng])))}for(_=r=a=h=0,i=c-1;_<c;i=_++)n=d[_],o=d[i],s=n.y*o.x-o.y*n.x,a+=(n.x+o.x)*s,h+=(n.y+o.y)*s,r+=3*s;u=0===r?d[0]:[a/r,h/r],u=e.unproject(m(u));return w([u.lat+l.lat,u.lng+l.lng])}function Qe(t){for(var e=0,i=0,n=0,o=0;o<t.length;o++){var s=w(t[o]);e+=s.lat,i+=s.lng,n++}return w([e/n,i/n])}var ti,gt={__proto__:null,clipPolygon:Je,polygonCenter:$e,centroid:Qe};function ei(t,e){if(e&&t.length){var i=t=function(t,e){for(var i=[t[0]],n=1,o=0,s=t.length;n<s;n++)(function(t,e){var i=e.x-t.x,e=e.y-t.y;return i*i+e*e})(t[n],t[o])>e&&(i.push(t[n]),o=n);o<s-1&&i.push(t[s-1]);return i}(t,e=e*e),n=i.length,o=new(typeof Uint8Array!=void 0+""?Uint8Array:Array)(n);o[0]=o[n-1]=1,function t(e,i,n,o,s){var r,a,h,l=0;for(a=o+1;a<=s-1;a++)h=ri(e[a],e[o],e[s],!0),l<h&&(r=a,l=h);n<l&&(i[r]=1,t(e,i,n,o,r),t(e,i,n,r,s))}(i,o,e,0,n-1);var s,r=[];for(s=0;s<n;s++)o[s]&&r.push(i[s]);return r}return t.slice()}function ii(t,e,i){return Math.sqrt(ri(t,e,i,!0))}function ni(t,e,i,n,o){var s,r,a,h=n?ti:si(t,i),l=si(e,i);for(ti=l;;){if(!(h|l))return[t,e];if(h&l)return!1;a=si(r=oi(t,e,s=h||l,i,o),i),s===h?(t=r,h=a):(e=r,l=a)}}function oi(t,e,i,n,o){var s,r,a=e.x-t.x,e=e.y-t.y,h=n.min,n=n.max;return 8&i?(s=t.x+a*(n.y-t.y)/e,r=n.y):4&i?(s=t.x+a*(h.y-t.y)/e,r=h.y):2&i?(s=n.x,r=t.y+e*(n.x-t.x)/a):1&i&&(s=h.x,r=t.y+e*(h.x-t.x)/a),new p(s,r,o)}function si(t,e){var i=0;return t.x<e.min.x?i|=1:t.x>e.max.x&&(i|=2),t.y<e.min.y?i|=4:t.y>e.max.y&&(i|=8),i}function ri(t,e,i,n){var o=e.x,e=e.y,s=i.x-o,r=i.y-e,a=s*s+r*r;return 0<a&&(1<(a=((t.x-o)*s+(t.y-e)*r)/a)?(o=i.x,e=i.y):0<a&&(o+=s*a,e+=r*a)),s=t.x-o,r=t.y-e,n?s*s+r*r:new p(o,e)}function I(t){return!d(t[0])||"object"!=typeof t[0][0]&&void 0!==t[0][0]}function ai(t){return console.warn("Deprecated use of _flat, please use L.LineUtil.isFlat instead."),I(t)}function hi(t,e){var i,n,o,s,r,a;if(!t||0===t.length)throw new Error("latlngs not passed");I(t)||(console.warn("latlngs are not flat! Only the first ring will be used"),t=t[0]);for(var h=w([0,0]),l=g(t),u=(l.getNorthWest().distanceTo(l.getSouthWest())*l.getNorthEast().distanceTo(l.getNorthWest())<1700&&(h=Qe(t)),t.length),c=[],d=0;d<u;d++){var _=w(t[d]);c.push(e.project(w([_.lat-h.lat,_.lng-h.lng])))}for(i=d=0;d<u-1;d++)i+=c[d].distanceTo(c[d+1])/2;if(0===i)a=c[0];else for(n=d=0;d<u-1;d++)if(o=c[d],s=c[d+1],i<(n+=r=o.distanceTo(s))){a=[s.x-(r=(n-i)/r)*(s.x-o.x),s.y-r*(s.y-o.y)];break}l=e.unproject(m(a));return w([l.lat+h.lat,l.lng+h.lng])}var vt={__proto__:null,simplify:ei,pointToSegmentDistance:ii,closestPointOnSegment:function(t,e,i){return ri(t,e,i)},clipSegment:ni,_getEdgeIntersection:oi,_getBitCode:si,_sqClosestPointOnSegment:ri,isFlat:I,_flat:ai,polylineCenter:hi},yt={project:function(t){return new p(t.lng,t.lat)},unproject:function(t){return new v(t.y,t.x)},bounds:new f([-180,-90],[180,90])},xt={R:6378137,R_MINOR:6356752.314245179,bounds:new f([-20037508.34279,-15496570.73972],[20037508.34279,18764656.23138]),project:function(t){var e=Math.PI/180,i=this.R,n=t.lat*e,o=this.R_MINOR/i,o=Math.sqrt(1-o*o),s=o*Math.sin(n),s=Math.tan(Math.PI/4-n/2)/Math.pow((1-s)/(1+s),o/2),n=-i*Math.log(Math.max(s,1e-10));return new p(t.lng*e*i,n)},unproject:function(t){for(var e,i=180/Math.PI,n=this.R,o=this.R_MINOR/n,s=Math.sqrt(1-o*o),r=Math.exp(-t.y/n),a=Math.PI/2-2*Math.atan(r),h=0,l=.1;h<15&&1e-7<Math.abs(l);h++)e=s*Math.sin(a),e=Math.pow((1-e)/(1+e),s/2),a+=l=Math.PI/2-2*Math.atan(r*e)-a;return new v(a*i,t.x*i/n)}},wt={__proto__:null,LonLat:yt,Mercator:xt,SphericalMercator:rt},Pt=l({},st,{code:"EPSG:3395",projection:xt,transformation:ht(bt=.5/(Math.PI*xt.R),.5,-bt,.5)}),li=l({},st,{code:"EPSG:4326",projection:yt,transformation:ht(1/180,1,-1/180,.5)}),Lt=l({},ot,{projection:yt,transformation:ht(1,0,-1,0),scale:function(t){return Math.pow(2,t)},zoom:function(t){return Math.log(t)/Math.LN2},distance:function(t,e){var i=e.lng-t.lng,e=e.lat-t.lat;return Math.sqrt(i*i+e*e)},infinite:!0}),o=(ot.Earth=st,ot.EPSG3395=Pt,ot.EPSG3857=lt,ot.EPSG900913=ut,ot.EPSG4326=li,ot.Simple=Lt,it.extend({options:{pane:"overlayPane",attribution:null,bubblingMouseEvents:!0},addTo:function(t){return t.addLayer(this),this},remove:function(){return this.removeFrom(this._map||this._mapToAdd)},removeFrom:function(t){return t&&t.removeLayer(this),this},getPane:function(t){return this._map.getPane(t?this.options[t]||t:this.options.pane)},addInteractiveTarget:function(t){return this._map._targets[h(t)]=this},removeInteractiveTarget:function(t){return delete this._map._targets[h(t)],this},getAttribution:function(){return this.options.attribution},_layerAdd:function(t){var e,i=t.target;i.hasLayer(this)&&(this._map=i,this._zoomAnimated=i._zoomAnimated,this.getEvents&&(e=this.getEvents(),i.on(e,this),this.once("remove",function(){i.off(e,this)},this)),this.onAdd(i),this.fire("add"),i.fire("layeradd",{layer:this}))}})),ui=(A.include({addLayer:function(t){var e;if(t._layerAdd)return e=h(t),this._layers[e]||((this._layers[e]=t)._mapToAdd=this,t.beforeAdd&&t.beforeAdd(this),this.whenReady(t._layerAdd,t)),this;throw new Error("The provided object is not a Layer.")},removeLayer:function(t){var e=h(t);return this._layers[e]&&(this._loaded&&t.onRemove(this),delete this._layers[e],this._loaded&&(this.fire("layerremove",{layer:t}),t.fire("remove")),t._map=t._mapToAdd=null),this},hasLayer:function(t){return h(t)in this._layers},eachLayer:function(t,e){for(var i in this._layers)t.call(e,this._layers[i]);return this},_addLayers:function(t){for(var e=0,i=(t=t?d(t)?t:[t]:[]).length;e<i;e++)this.addLayer(t[e])},_addZoomLimit:function(t){isNaN(t.options.maxZoom)&&isNaN(t.options.minZoom)||(this._zoomBoundLayers[h(t)]=t,this._updateZoomLevels())},_removeZoomLimit:function(t){t=h(t);this._zoomBoundLayers[t]&&(delete this._zoomBoundLayers[t],this._updateZoomLevels())},_updateZoomLevels:function(){var t,e=1/0,i=-1/0,n=this._getZoomSpan();for(t in this._zoomBoundLayers)var o=this._zoomBoundLayers[t].options,e=void 0===o.minZoom?e:Math.min(e,o.minZoom),i=void 0===o.maxZoom?i:Math.max(i,o.maxZoom);this._layersMaxZoom=i===-1/0?void 0:i,this._layersMinZoom=e===1/0?void 0:e,n!==this._getZoomSpan()&&this.fire("zoomlevelschange"),void 0===this.options.maxZoom&&this._layersMaxZoom&&this.getZoom()>this._layersMaxZoom&&this.setZoom(this._layersMaxZoom),void 0===this.options.minZoom&&this._layersMinZoom&&this.getZoom()<this._layersMinZoom&&this.setZoom(this._layersMinZoom)}}),o.extend({initialize:function(t,e){var i,n;if(c(this,e),this._layers={},t)for(i=0,n=t.length;i<n;i++)this.addLayer(t[i])},addLayer:function(t){var e=this.getLayerId(t);return this._layers[e]=t,this._map&&this._map.addLayer(t),this},removeLayer:function(t){t=t in this._layers?t:this.getLayerId(t);return this._map&&this._layers[t]&&this._map.removeLayer(this._layers[t]),delete this._layers[t],this},hasLayer:function(t){return("number"==typeof t?t:this.getLayerId(t))in this._layers},clearLayers:function(){return this.eachLayer(this.removeLayer,this)},invoke:function(t){var e,i,n=Array.prototype.slice.call(arguments,1);for(e in this._layers)(i=this._layers[e])[t]&&i[t].apply(i,n);return this},onAdd:function(t){this.eachLayer(t.addLayer,t)},onRemove:function(t){this.eachLayer(t.removeLayer,t)},eachLayer:function(t,e){for(var i in this._layers)t.call(e,this._layers[i]);return this},getLayer:function(t){return this._layers[t]},getLayers:function(){var t=[];return this.eachLayer(t.push,t),t},setZIndex:function(t){return this.invoke("setZIndex",t)},getLayerId:h})),ci=ui.extend({addLayer:function(t){return this.hasLayer(t)?this:(t.addEventParent(this),ui.prototype.addLayer.call(this,t),this.fire("layeradd",{layer:t}))},removeLayer:function(t){return this.hasLayer(t)?((t=t in this._layers?this._layers[t]:t).removeEventParent(this),ui.prototype.removeLayer.call(this,t),this.fire("layerremove",{layer:t})):this},setStyle:function(t){return this.invoke("setStyle",t)},bringToFront:function(){return this.invoke("bringToFront")},bringToBack:function(){return this.invoke("bringToBack")},getBounds:function(){var t,e=new s;for(t in this._layers){var i=this._layers[t];e.extend(i.getBounds?i.getBounds():i.getLatLng())}return e}}),di=et.extend({options:{popupAnchor:[0,0],tooltipAnchor:[0,0],crossOrigin:!1},initialize:function(t){c(this,t)},createIcon:function(t){return this._createIcon("icon",t)},createShadow:function(t){return this._createIcon("shadow",t)},_createIcon:function(t,e){var i=this._getIconUrl(t);if(i)return i=this._createImg(i,e&&"IMG"===e.tagName?e:null),this._setIconStyles(i,t),!this.options.crossOrigin&&""!==this.options.crossOrigin||(i.crossOrigin=!0===this.options.crossOrigin?"":this.options.crossOrigin),i;if("icon"===t)throw new Error("iconUrl not set in Icon options (see the docs).");return null},_setIconStyles:function(t,e){var i=this.options,n=i[e+"Size"],n=m(n="number"==typeof n?[n,n]:n),o=m("shadow"===e&&i.shadowAnchor||i.iconAnchor||n&&n.divideBy(2,!0));t.className="leaflet-marker-"+e+" "+(i.className||""),o&&(t.style.marginLeft=-o.x+"px",t.style.marginTop=-o.y+"px"),n&&(t.style.width=n.x+"px",t.style.height=n.y+"px")},_createImg:function(t,e){return(e=e||document.createElement("img")).src=t,e},_getIconUrl:function(t){return b.retina&&this.options[t+"RetinaUrl"]||this.options[t+"Url"]}});var _i=di.extend({options:{iconUrl:"marker-icon.png",iconRetinaUrl:"marker-icon-2x.png",shadowUrl:"marker-shadow.png",iconSize:[25,41],iconAnchor:[12,41],popupAnchor:[1,-34],tooltipAnchor:[16,-28],shadowSize:[41,41]},_getIconUrl:function(t){return"string"!=typeof _i.imagePath&&(_i.imagePath=this._detectIconPath()),(this.options.imagePath||_i.imagePath)+di.prototype._getIconUrl.call(this,t)},_stripUrl:function(t){function e(t,e,i){return(e=e.exec(t))&&e[i]}return(t=e(t,/^url\((['"])?(.+)\1\)$/,2))&&e(t,/^(.*)marker-icon\.png$/,1)},_detectIconPath:function(){var t=P("div","leaflet-default-icon-path",document.body),e=pe(t,"background-image")||pe(t,"backgroundImage");return document.body.removeChild(t),(e=this._stripUrl(e))?e:(t=document.querySelector('link[href$="leaflet.css"]'))?t.href.substring(0,t.href.length-"leaflet.css".length-1):""}}),pi=n.extend({initialize:function(t){this._marker=t},addHooks:function(){var t=this._marker._icon;this._draggable||(this._draggable=new Xe(t,t,!0)),this._draggable.on({dragstart:this._onDragStart,predrag:this._onPreDrag,drag:this._onDrag,dragend:this._onDragEnd},this).enable(),M(t,"leaflet-marker-draggable")},removeHooks:function(){this._draggable.off({dragstart:this._onDragStart,predrag:this._onPreDrag,drag:this._onDrag,dragend:this._onDragEnd},this).disable(),this._marker._icon&&z(this._marker._icon,"leaflet-marker-draggable")},moved:function(){return this._draggable&&this._draggable._moved},_adjustPan:function(t){var e=this._marker,i=e._map,n=this._marker.options.autoPanSpeed,o=this._marker.options.autoPanPadding,s=Pe(e._icon),r=i.getPixelBounds(),a=i.getPixelOrigin(),a=_(r.min._subtract(a).add(o),r.max._subtract(a).subtract(o));a.contains(s)||(o=m((Math.max(a.max.x,s.x)-a.max.x)/(r.max.x-a.max.x)-(Math.min(a.min.x,s.x)-a.min.x)/(r.min.x-a.min.x),(Math.max(a.max.y,s.y)-a.max.y)/(r.max.y-a.max.y)-(Math.min(a.min.y,s.y)-a.min.y)/(r.min.y-a.min.y)).multiplyBy(n),i.panBy(o,{animate:!1}),this._draggable._newPos._add(o),this._draggable._startPos._add(o),Z(e._icon,this._draggable._newPos),this._onDrag(t),this._panRequest=x(this._adjustPan.bind(this,t)))},_onDragStart:function(){this._oldLatLng=this._marker.getLatLng(),this._marker.closePopup&&this._marker.closePopup(),this._marker.fire("movestart").fire("dragstart")},_onPreDrag:function(t){this._marker.options.autoPan&&(r(this._panRequest),this._panRequest=x(this._adjustPan.bind(this,t)))},_onDrag:function(t){var e=this._marker,i=e._shadow,n=Pe(e._icon),o=e._map.layerPointToLatLng(n);i&&Z(i,n),e._latlng=o,t.latlng=o,t.oldLatLng=this._oldLatLng,e.fire("move",t).fire("drag",t)},_onDragEnd:function(t){r(this._panRequest),delete this._oldLatLng,this._marker.fire("moveend").fire("dragend",t)}}),mi=o.extend({options:{icon:new _i,interactive:!0,keyboard:!0,title:"",alt:"Marker",zIndexOffset:0,opacity:1,riseOnHover:!1,riseOffset:250,pane:"markerPane",shadowPane:"shadowPane",bubblingMouseEvents:!1,autoPanOnFocus:!0,draggable:!1,autoPan:!1,autoPanPadding:[50,50],autoPanSpeed:10},initialize:function(t,e){c(this,e),this._latlng=w(t)},onAdd:function(t){this._zoomAnimated=this._zoomAnimated&&t.options.markerZoomAnimation,this._zoomAnimated&&t.on("zoomanim",this._animateZoom,this),this._initIcon(),this.update()},onRemove:function(t){this.dragging&&this.dragging.enabled()&&(this.options.draggable=!0,this.dragging.removeHooks()),delete this.dragging,this._zoomAnimated&&t.off("zoomanim",this._animateZoom,this),this._removeIcon(),this._removeShadow()},getEvents:function(){return{zoom:this.update,viewreset:this.update}},getLatLng:function(){return this._latlng},setLatLng:function(t){var e=this._latlng;return this._latlng=w(t),this.update(),this.fire("move",{oldLatLng:e,latlng:this._latlng})},setZIndexOffset:function(t){return this.options.zIndexOffset=t,this.update()},getIcon:function(){return this.options.icon},setIcon:function(t){return this.options.icon=t,this._map&&(this._initIcon(),this.update()),this._popup&&this.bindPopup(this._popup,this._popup.options),this},getElement:function(){return this._icon},update:function(){var t;return this._icon&&this._map&&(t=this._map.latLngToLayerPoint(this._latlng).round(),this._setPos(t)),this},_initIcon:function(){var t=this.options,e="leaflet-zoom-"+(this._zoomAnimated?"animated":"hide"),i=t.icon.createIcon(this._icon),n=!1,i=(i!==this._icon&&(this._icon&&this._removeIcon(),n=!0,t.title&&(i.title=t.title),"IMG"===i.tagName&&(i.alt=t.alt||"")),M(i,e),t.keyboard&&(i.tabIndex="0",i.setAttribute("role","button")),this._icon=i,t.riseOnHover&&this.on({mouseover:this._bringToFront,mouseout:this._resetZIndex}),this.options.autoPanOnFocus&&S(i,"focus",this._panOnFocus,this),t.icon.createShadow(this._shadow)),o=!1;i!==this._shadow&&(this._removeShadow(),o=!0),i&&(M(i,e),i.alt=""),this._shadow=i,t.opacity<1&&this._updateOpacity(),n&&this.getPane().appendChild(this._icon),this._initInteraction(),i&&o&&this.getPane(t.shadowPane).appendChild(this._shadow)},_removeIcon:function(){this.options.riseOnHover&&this.off({mouseover:this._bringToFront,mouseout:this._resetZIndex}),this.options.autoPanOnFocus&&k(this._icon,"focus",this._panOnFocus,this),T(this._icon),this.removeInteractiveTarget(this._icon),this._icon=null},_removeShadow:function(){this._shadow&&T(this._shadow),this._shadow=null},_setPos:function(t){this._icon&&Z(this._icon,t),this._shadow&&Z(this._shadow,t),this._zIndex=t.y+this.options.zIndexOffset,this._resetZIndex()},_updateZIndex:function(t){this._icon&&(this._icon.style.zIndex=this._zIndex+t)},_animateZoom:function(t){t=this._map._latLngToNewLayerPoint(this._latlng,t.zoom,t.center).round();this._setPos(t)},_initInteraction:function(){var t;this.options.interactive&&(M(this._icon,"leaflet-interactive"),this.addInteractiveTarget(this._icon),pi&&(t=this.options.draggable,this.dragging&&(t=this.dragging.enabled(),this.dragging.disable()),this.dragging=new pi(this),t&&this.dragging.enable()))},setOpacity:function(t){return this.options.opacity=t,this._map&&this._updateOpacity(),this},_updateOpacity:function(){var t=this.options.opacity;this._icon&&C(this._icon,t),this._shadow&&C(this._shadow,t)},_bringToFront:function(){this._updateZIndex(this.options.riseOffset)},_resetZIndex:function(){this._updateZIndex(0)},_panOnFocus:function(){var t,e,i=this._map;i&&(t=(e=this.options.icon.options).iconSize?m(e.iconSize):m(0,0),e=e.iconAnchor?m(e.iconAnchor):m(0,0),i.panInside(this._latlng,{paddingTopLeft:e,paddingBottomRight:t.subtract(e)}))},_getPopupAnchor:function(){return this.options.icon.options.popupAnchor},_getTooltipAnchor:function(){return this.options.icon.options.tooltipAnchor}});var fi=o.extend({options:{stroke:!0,color:"#3388ff",weight:3,opacity:1,lineCap:"round",lineJoin:"round",dashArray:null,dashOffset:null,fill:!1,fillColor:null,fillOpacity:.2,fillRule:"evenodd",interactive:!0,bubblingMouseEvents:!0},beforeAdd:function(t){this._renderer=t.getRenderer(this)},onAdd:function(){this._renderer._initPath(this),this._reset(),this._renderer._addPath(this)},onRemove:function(){this._renderer._removePath(this)},redraw:function(){return this._map&&this._renderer._updatePath(this),this},setStyle:function(t){return c(this,t),this._renderer&&(this._renderer._updateStyle(this),this.options.stroke&&t&&Object.prototype.hasOwnProperty.call(t,"weight")&&this._updateBounds()),this},bringToFront:function(){return this._renderer&&this._renderer._bringToFront(this),this},bringToBack:function(){return this._renderer&&this._renderer._bringToBack(this),this},getElement:function(){return this._path},_reset:function(){this._project(),this._update()},_clickTolerance:function(){return(this.options.stroke?this.options.weight/2:0)+(this._renderer.options.tolerance||0)}}),gi=fi.extend({options:{fill:!0,radius:10},initialize:function(t,e){c(this,e),this._latlng=w(t),this._radius=this.options.radius},setLatLng:function(t){var e=this._latlng;return this._latlng=w(t),this.redraw(),this.fire("move",{oldLatLng:e,latlng:this._latlng})},getLatLng:function(){return this._latlng},setRadius:function(t){return this.options.radius=this._radius=t,this.redraw()},getRadius:function(){return this._radius},setStyle:function(t){var e=t&&t.radius||this._radius;return fi.prototype.setStyle.call(this,t),this.setRadius(e),this},_project:function(){this._point=this._map.latLngToLayerPoint(this._latlng),this._updateBounds()},_updateBounds:function(){var t=this._radius,e=this._radiusY||t,i=this._clickTolerance(),t=[t+i,e+i];this._pxBounds=new f(this._point.subtract(t),this._point.add(t))},_update:function(){this._map&&this._updatePath()},_updatePath:function(){this._renderer._updateCircle(this)},_empty:function(){return this._radius&&!this._renderer._bounds.intersects(this._pxBounds)},_containsPoint:function(t){return t.distanceTo(this._point)<=this._radius+this._clickTolerance()}});var vi=gi.extend({initialize:function(t,e,i){if(c(this,e="number"==typeof e?l({},i,{radius:e}):e),this._latlng=w(t),isNaN(this.options.radius))throw new Error("Circle radius cannot be NaN");this._mRadius=this.options.radius},setRadius:function(t){return this._mRadius=t,this.redraw()},getRadius:function(){return this._mRadius},getBounds:function(){var t=[this._radius,this._radiusY||this._radius];return new s(this._map.layerPointToLatLng(this._point.subtract(t)),this._map.layerPointToLatLng(this._point.add(t)))},setStyle:fi.prototype.setStyle,_project:function(){var t,e,i,n,o,s=this._latlng.lng,r=this._latlng.lat,a=this._map,h=a.options.crs;h.distance===st.distance?(n=Math.PI/180,o=this._mRadius/st.R/n,t=a.project([r+o,s]),e=a.project([r-o,s]),e=t.add(e).divideBy(2),i=a.unproject(e).lat,n=Math.acos((Math.cos(o*n)-Math.sin(r*n)*Math.sin(i*n))/(Math.cos(r*n)*Math.cos(i*n)))/n,!isNaN(n)&&0!==n||(n=o/Math.cos(Math.PI/180*r)),this._point=e.subtract(a.getPixelOrigin()),this._radius=isNaN(n)?0:e.x-a.project([i,s-n]).x,this._radiusY=e.y-t.y):(o=h.unproject(h.project(this._latlng).subtract([this._mRadius,0])),this._point=a.latLngToLayerPoint(this._latlng),this._radius=this._point.x-a.latLngToLayerPoint(o).x),this._updateBounds()}});var yi=fi.extend({options:{smoothFactor:1,noClip:!1},initialize:function(t,e){c(this,e),this._setLatLngs(t)},getLatLngs:function(){return this._latlngs},setLatLngs:function(t){return this._setLatLngs(t),this.redraw()},isEmpty:function(){return!this._latlngs.length},closestLayerPoint:function(t){for(var e=1/0,i=null,n=ri,o=0,s=this._parts.length;o<s;o++)for(var r=this._parts[o],a=1,h=r.length;a<h;a++){var l,u,c=n(t,l=r[a-1],u=r[a],!0);c<e&&(e=c,i=n(t,l,u))}return i&&(i.distance=Math.sqrt(e)),i},getCenter:function(){if(this._map)return hi(this._defaultShape(),this._map.options.crs);throw new Error("Must add layer to map before using getCenter()")},getBounds:function(){return this._bounds},addLatLng:function(t,e){return e=e||this._defaultShape(),t=w(t),e.push(t),this._bounds.extend(t),this.redraw()},_setLatLngs:function(t){this._bounds=new s,this._latlngs=this._convertLatLngs(t)},_defaultShape:function(){return I(this._latlngs)?this._latlngs:this._latlngs[0]},_convertLatLngs:function(t){for(var e=[],i=I(t),n=0,o=t.length;n<o;n++)i?(e[n]=w(t[n]),this._bounds.extend(e[n])):e[n]=this._convertLatLngs(t[n]);return e},_project:function(){var t=new f;this._rings=[],this._projectLatlngs(this._latlngs,this._rings,t),this._bounds.isValid()&&t.isValid()&&(this._rawPxBounds=t,this._updateBounds())},_updateBounds:function(){var t=this._clickTolerance(),t=new p(t,t);this._rawPxBounds&&(this._pxBounds=new f([this._rawPxBounds.min.subtract(t),this._rawPxBounds.max.add(t)]))},_projectLatlngs:function(t,e,i){var n,o,s=t[0]instanceof v,r=t.length;if(s){for(o=[],n=0;n<r;n++)o[n]=this._map.latLngToLayerPoint(t[n]),i.extend(o[n]);e.push(o)}else for(n=0;n<r;n++)this._projectLatlngs(t[n],e,i)},_clipPoints:function(){var t=this._renderer._bounds;if(this._parts=[],this._pxBounds&&this._pxBounds.intersects(t))if(this.options.noClip)this._parts=this._rings;else for(var e,i,n,o,s=this._parts,r=0,a=0,h=this._rings.length;r<h;r++)for(e=0,i=(o=this._rings[r]).length;e<i-1;e++)(n=ni(o[e],o[e+1],t,e,!0))&&(s[a]=s[a]||[],s[a].push(n[0]),n[1]===o[e+1]&&e!==i-2||(s[a].push(n[1]),a++))},_simplifyPoints:function(){for(var t=this._parts,e=this.options.smoothFactor,i=0,n=t.length;i<n;i++)t[i]=ei(t[i],e)},_update:function(){this._map&&(this._clipPoints(),this._simplifyPoints(),this._updatePath())},_updatePath:function(){this._renderer._updatePoly(this)},_containsPoint:function(t,e){var i,n,o,s,r,a,h=this._clickTolerance();if(this._pxBounds&&this._pxBounds.contains(t))for(i=0,s=this._parts.length;i<s;i++)for(n=0,o=(r=(a=this._parts[i]).length)-1;n<r;o=n++)if((e||0!==n)&&ii(t,a[o],a[n])<=h)return!0;return!1}});yi._flat=ai;var xi=yi.extend({options:{fill:!0},isEmpty:function(){return!this._latlngs.length||!this._latlngs[0].length},getCenter:function(){if(this._map)return $e(this._defaultShape(),this._map.options.crs);throw new Error("Must add layer to map before using getCenter()")},_convertLatLngs:function(t){var t=yi.prototype._convertLatLngs.call(this,t),e=t.length;return 2<=e&&t[0]instanceof v&&t[0].equals(t[e-1])&&t.pop(),t},_setLatLngs:function(t){yi.prototype._setLatLngs.call(this,t),I(this._latlngs)&&(this._latlngs=[this._latlngs])},_defaultShape:function(){return(I(this._latlngs[0])?this._latlngs:this._latlngs[0])[0]},_clipPoints:function(){var t=this._renderer._bounds,e=this.options.weight,e=new p(e,e),t=new f(t.min.subtract(e),t.max.add(e));if(this._parts=[],this._pxBounds&&this._pxBounds.intersects(t))if(this.options.noClip)this._parts=this._rings;else for(var i,n=0,o=this._rings.length;n<o;n++)(i=Je(this._rings[n],t,!0)).length&&this._parts.push(i)},_updatePath:function(){this._renderer._updatePoly(this,!0)},_containsPoint:function(t){var e,i,n,o,s,r,a,h,l=!1;if(!this._pxBounds||!this._pxBounds.contains(t))return!1;for(o=0,a=this._parts.length;o<a;o++)for(s=0,r=(h=(e=this._parts[o]).length)-1;s<h;r=s++)i=e[s],n=e[r],i.y>t.y!=n.y>t.y&&t.x<(n.x-i.x)*(t.y-i.y)/(n.y-i.y)+i.x&&(l=!l);return l||yi.prototype._containsPoint.call(this,t,!0)}});var wi=ci.extend({initialize:function(t,e){c(this,e),this._layers={},t&&this.addData(t)},addData:function(t){var e,i,n,o=d(t)?t:t.features;if(o){for(e=0,i=o.length;e<i;e++)((n=o[e]).geometries||n.geometry||n.features||n.coordinates)&&this.addData(n);return this}var s,r=this.options;return(!r.filter||r.filter(t))&&(s=bi(t,r))?(s.feature=Zi(t),s.defaultOptions=s.options,this.resetStyle(s),r.onEachFeature&&r.onEachFeature(t,s),this.addLayer(s)):this},resetStyle:function(t){return void 0===t?this.eachLayer(this.resetStyle,this):(t.options=l({},t.defaultOptions),this._setLayerStyle(t,this.options.style),this)},setStyle:function(e){return this.eachLayer(function(t){this._setLayerStyle(t,e)},this)},_setLayerStyle:function(t,e){t.setStyle&&("function"==typeof e&&(e=e(t.feature)),t.setStyle(e))}});function bi(t,e){var i,n,o,s,r="Feature"===t.type?t.geometry:t,a=r?r.coordinates:null,h=[],l=e&&e.pointToLayer,u=e&&e.coordsToLatLng||Li;if(!a&&!r)return null;switch(r.type){case"Point":return Pi(l,t,i=u(a),e);case"MultiPoint":for(o=0,s=a.length;o<s;o++)i=u(a[o]),h.push(Pi(l,t,i,e));return new ci(h);case"LineString":case"MultiLineString":return n=Ti(a,"LineString"===r.type?0:1,u),new yi(n,e);case"Polygon":case"MultiPolygon":return n=Ti(a,"Polygon"===r.type?1:2,u),new xi(n,e);case"GeometryCollection":for(o=0,s=r.geometries.length;o<s;o++){var c=bi({geometry:r.geometries[o],type:"Feature",properties:t.properties},e);c&&h.push(c)}return new ci(h);case"FeatureCollection":for(o=0,s=r.features.length;o<s;o++){var d=bi(r.features[o],e);d&&h.push(d)}return new ci(h);default:throw new Error("Invalid GeoJSON object.")}}function Pi(t,e,i,n){return t?t(e,i):new mi(i,n&&n.markersInheritOptions&&n)}function Li(t){return new v(t[1],t[0],t[2])}function Ti(t,e,i){for(var n,o=[],s=0,r=t.length;s<r;s++)n=e?Ti(t[s],e-1,i):(i||Li)(t[s]),o.push(n);return o}function Mi(t,e){return void 0!==(t=w(t)).alt?[i(t.lng,e),i(t.lat,e),i(t.alt,e)]:[i(t.lng,e),i(t.lat,e)]}function zi(t,e,i,n){for(var o=[],s=0,r=t.length;s<r;s++)o.push(e?zi(t[s],I(t[s])?0:e-1,i,n):Mi(t[s],n));return!e&&i&&0<o.length&&o.push(o[0].slice()),o}function Ci(t,e){return t.feature?l({},t.feature,{geometry:e}):Zi(e)}function Zi(t){return"Feature"===t.type||"FeatureCollection"===t.type?t:{type:"Feature",properties:{},geometry:t}}Tt={toGeoJSON:function(t){return Ci(this,{type:"Point",coordinates:Mi(this.getLatLng(),t)})}};function Si(t,e){return new wi(t,e)}mi.include(Tt),vi.include(Tt),gi.include(Tt),yi.include({toGeoJSON:function(t){var e=!I(this._latlngs);return Ci(this,{type:(e?"Multi":"")+"LineString",coordinates:zi(this._latlngs,e?1:0,!1,t)})}}),xi.include({toGeoJSON:function(t){var e=!I(this._latlngs),i=e&&!I(this._latlngs[0]),t=zi(this._latlngs,i?2:e?1:0,!0,t);return Ci(this,{type:(i?"Multi":"")+"Polygon",coordinates:t=e?t:[t]})}}),ui.include({toMultiPoint:function(e){var i=[];return this.eachLayer(function(t){i.push(t.toGeoJSON(e).geometry.coordinates)}),Ci(this,{type:"MultiPoint",coordinates:i})},toGeoJSON:function(e){var i,n,t=this.feature&&this.feature.geometry&&this.feature.geometry.type;return"MultiPoint"===t?this.toMultiPoint(e):(i="GeometryCollection"===t,n=[],this.eachLayer(function(t){t.toGeoJSON&&(t=t.toGeoJSON(e),i?n.push(t.geometry):"FeatureCollection"===(t=Zi(t)).type?n.push.apply(n,t.features):n.push(t))}),i?Ci(this,{geometries:n,type:"GeometryCollection"}):{type:"FeatureCollection",features:n})}});var Mt=Si,Ei=o.extend({options:{opacity:1,alt:"",interactive:!1,crossOrigin:!1,errorOverlayUrl:"",zIndex:1,className:""},initialize:function(t,e,i){this._url=t,this._bounds=g(e),c(this,i)},onAdd:function(){this._image||(this._initImage(),this.options.opacity<1&&this._updateOpacity()),this.options.interactive&&(M(this._image,"leaflet-interactive"),this.addInteractiveTarget(this._image)),this.getPane().appendChild(this._image),this._reset()},onRemove:function(){T(this._image),this.options.interactive&&this.removeInteractiveTarget(this._image)},setOpacity:function(t){return this.options.opacity=t,this._image&&this._updateOpacity(),this},setStyle:function(t){return t.opacity&&this.setOpacity(t.opacity),this},bringToFront:function(){return this._map&&fe(this._image),this},bringToBack:function(){return this._map&&ge(this._image),this},setUrl:function(t){return this._url=t,this._image&&(this._image.src=t),this},setBounds:function(t){return this._bounds=g(t),this._map&&this._reset(),this},getEvents:function(){var t={zoom:this._reset,viewreset:this._reset};return this._zoomAnimated&&(t.zoomanim=this._animateZoom),t},setZIndex:function(t){return this.options.zIndex=t,this._updateZIndex(),this},getBounds:function(){return this._bounds},getElement:function(){return this._image},_initImage:function(){var t="IMG"===this._url.tagName,e=this._image=t?this._url:P("img");M(e,"leaflet-image-layer"),this._zoomAnimated&&M(e,"leaflet-zoom-animated"),this.options.className&&M(e,this.options.className),e.onselectstart=u,e.onmousemove=u,e.onload=a(this.fire,this,"load"),e.onerror=a(this._overlayOnError,this,"error"),!this.options.crossOrigin&&""!==this.options.crossOrigin||(e.crossOrigin=!0===this.options.crossOrigin?"":this.options.crossOrigin),this.options.zIndex&&this._updateZIndex(),t?this._url=e.src:(e.src=this._url,e.alt=this.options.alt)},_animateZoom:function(t){var e=this._map.getZoomScale(t.zoom),t=this._map._latLngBoundsToNewLayerBounds(this._bounds,t.zoom,t.center).min;be(this._image,t,e)},_reset:function(){var t=this._image,e=new f(this._map.latLngToLayerPoint(this._bounds.getNorthWest()),this._map.latLngToLayerPoint(this._bounds.getSouthEast())),i=e.getSize();Z(t,e.min),t.style.width=i.x+"px",t.style.height=i.y+"px"},_updateOpacity:function(){C(this._image,this.options.opacity)},_updateZIndex:function(){this._image&&void 0!==this.options.zIndex&&null!==this.options.zIndex&&(this._image.style.zIndex=this.options.zIndex)},_overlayOnError:function(){this.fire("error");var t=this.options.errorOverlayUrl;t&&this._url!==t&&(this._url=t,this._image.src=t)},getCenter:function(){return this._bounds.getCenter()}}),ki=Ei.extend({options:{autoplay:!0,loop:!0,keepAspectRatio:!0,muted:!1,playsInline:!0},_initImage:function(){var t="VIDEO"===this._url.tagName,e=this._image=t?this._url:P("video");if(M(e,"leaflet-image-layer"),this._zoomAnimated&&M(e,"leaflet-zoom-animated"),this.options.className&&M(e,this.options.className),e.onselectstart=u,e.onmousemove=u,e.onloadeddata=a(this.fire,this,"load"),t){for(var i=e.getElementsByTagName("source"),n=[],o=0;o<i.length;o++)n.push(i[o].src);this._url=0<i.length?n:[e.src]}else{d(this._url)||(this._url=[this._url]),!this.options.keepAspectRatio&&Object.prototype.hasOwnProperty.call(e.style,"objectFit")&&(e.style.objectFit="fill"),e.autoplay=!!this.options.autoplay,e.loop=!!this.options.loop,e.muted=!!this.options.muted,e.playsInline=!!this.options.playsInline;for(var s=0;s<this._url.length;s++){var r=P("source");r.src=this._url[s],e.appendChild(r)}}}});var Oi=Ei.extend({_initImage:function(){var t=this._image=this._url;M(t,"leaflet-image-layer"),this._zoomAnimated&&M(t,"leaflet-zoom-animated"),this.options.className&&M(t,this.options.className),t.onselectstart=u,t.onmousemove=u}});var Ai=o.extend({options:{interactive:!1,offset:[0,0],className:"",pane:void 0,content:""},initialize:function(t,e){t&&(t instanceof v||d(t))?(this._latlng=w(t),c(this,e)):(c(this,t),this._source=e),this.options.content&&(this._content=this.options.content)},openOn:function(t){return(t=arguments.length?t:this._source._map).hasLayer(this)||t.addLayer(this),this},close:function(){return this._map&&this._map.removeLayer(this),this},toggle:function(t){return this._map?this.close():(arguments.length?this._source=t:t=this._source,this._prepareOpen(),this.openOn(t._map)),this},onAdd:function(t){this._zoomAnimated=t._zoomAnimated,this._container||this._initLayout(),t._fadeAnimated&&C(this._container,0),clearTimeout(this._removeTimeout),this.getPane().appendChild(this._container),this.update(),t._fadeAnimated&&C(this._container,1),this.bringToFront(),this.options.interactive&&(M(this._container,"leaflet-interactive"),this.addInteractiveTarget(this._container))},onRemove:function(t){t._fadeAnimated?(C(this._container,0),this._removeTimeout=setTimeout(a(T,void 0,this._container),200)):T(this._container),this.options.interactive&&(z(this._container,"leaflet-interactive"),this.removeInteractiveTarget(this._container))},getLatLng:function(){return this._latlng},setLatLng:function(t){return this._latlng=w(t),this._map&&(this._updatePosition(),this._adjustPan()),this},getContent:function(){return this._content},setContent:function(t){return this._content=t,this.update(),this},getElement:function(){return this._container},update:function(){this._map&&(this._container.style.visibility="hidden",this._updateContent(),this._updateLayout(),this._updatePosition(),this._container.style.visibility="",this._adjustPan())},getEvents:function(){var t={zoom:this._updatePosition,viewreset:this._updatePosition};return this._zoomAnimated&&(t.zoomanim=this._animateZoom),t},isOpen:function(){return!!this._map&&this._map.hasLayer(this)},bringToFront:function(){return this._map&&fe(this._container),this},bringToBack:function(){return this._map&&ge(this._container),this},_prepareOpen:function(t){if(!(i=this._source)._map)return!1;if(i instanceof ci){var e,i=null,n=this._source._layers;for(e in n)if(n[e]._map){i=n[e];break}if(!i)return!1;this._source=i}if(!t)if(i.getCenter)t=i.getCenter();else if(i.getLatLng)t=i.getLatLng();else{if(!i.getBounds)throw new Error("Unable to get source layer LatLng.");t=i.getBounds().getCenter()}return this.setLatLng(t),this._map&&this.update(),!0},_updateContent:function(){if(this._content){var t=this._contentNode,e="function"==typeof this._content?this._content(this._source||this):this._content;if("string"==typeof e)t.innerHTML=e;else{for(;t.hasChildNodes();)t.removeChild(t.firstChild);t.appendChild(e)}this.fire("contentupdate")}},_updatePosition:function(){var t,e,i;this._map&&(e=this._map.latLngToLayerPoint(this._latlng),t=m(this.options.offset),i=this._getAnchor(),this._zoomAnimated?Z(this._container,e.add(i)):t=t.add(e).add(i),e=this._containerBottom=-t.y,i=this._containerLeft=-Math.round(this._containerWidth/2)+t.x,this._container.style.bottom=e+"px",this._container.style.left=i+"px")},_getAnchor:function(){return[0,0]}}),Bi=(A.include({_initOverlay:function(t,e,i,n){var o=e;return o instanceof t||(o=new t(n).setContent(e)),i&&o.setLatLng(i),o}}),o.include({_initOverlay:function(t,e,i,n){var o=i;return o instanceof t?(c(o,n),o._source=this):(o=e&&!n?e:new t(n,this)).setContent(i),o}}),Ai.extend({options:{pane:"popupPane",offset:[0,7],maxWidth:300,minWidth:50,maxHeight:null,autoPan:!0,autoPanPaddingTopLeft:null,autoPanPaddingBottomRight:null,autoPanPadding:[5,5],keepInView:!1,closeButton:!0,autoClose:!0,closeOnEscapeKey:!0,className:""},openOn:function(t){return!(t=arguments.length?t:this._source._map).hasLayer(this)&&t._popup&&t._popup.options.autoClose&&t.removeLayer(t._popup),t._popup=this,Ai.prototype.openOn.call(this,t)},onAdd:function(t){Ai.prototype.onAdd.call(this,t),t.fire("popupopen",{popup:this}),this._source&&(this._source.fire("popupopen",{popup:this},!0),this._source instanceof fi||this._source.on("preclick",Ae))},onRemove:function(t){Ai.prototype.onRemove.call(this,t),t.fire("popupclose",{popup:this}),this._source&&(this._source.fire("popupclose",{popup:this},!0),this._source instanceof fi||this._source.off("preclick",Ae))},getEvents:function(){var t=Ai.prototype.getEvents.call(this);return(void 0!==this.options.closeOnClick?this.options.closeOnClick:this._map.options.closePopupOnClick)&&(t.preclick=this.close),this.options.keepInView&&(t.moveend=this._adjustPan),t},_initLayout:function(){var t="leaflet-popup",e=this._container=P("div",t+" "+(this.options.className||"")+" leaflet-zoom-animated"),i=this._wrapper=P("div",t+"-content-wrapper",e);this._contentNode=P("div",t+"-content",i),Ie(e),Be(this._contentNode),S(e,"contextmenu",Ae),this._tipContainer=P("div",t+"-tip-container",e),this._tip=P("div",t+"-tip",this._tipContainer),this.options.closeButton&&((i=this._closeButton=P("a",t+"-close-button",e)).setAttribute("role","button"),i.setAttribute("aria-label","Close popup"),i.href="#close",i.innerHTML='<span aria-hidden="true">&#215;</span>',S(i,"click",function(t){O(t),this.close()},this))},_updateLayout:function(){var t=this._contentNode,e=t.style,i=(e.width="",e.whiteSpace="nowrap",t.offsetWidth),i=Math.min(i,this.options.maxWidth),i=(i=Math.max(i,this.options.minWidth),e.width=i+1+"px",e.whiteSpace="",e.height="",t.offsetHeight),n=this.options.maxHeight,o="leaflet-popup-scrolled";(n&&n<i?(e.height=n+"px",M):z)(t,o),this._containerWidth=this._container.offsetWidth},_animateZoom:function(t){var t=this._map._latLngToNewLayerPoint(this._latlng,t.zoom,t.center),e=this._getAnchor();Z(this._container,t.add(e))},_adjustPan:function(){var t,e,i,n,o,s,r,a;this.options.autoPan&&(this._map._panAnim&&this._map._panAnim.stop(),this._autopanning?this._autopanning=!1:(t=this._map,e=parseInt(pe(this._container,"marginBottom"),10)||0,e=this._container.offsetHeight+e,a=this._containerWidth,(i=new p(this._containerLeft,-e-this._containerBottom))._add(Pe(this._container)),i=t.layerPointToContainerPoint(i),o=m(this.options.autoPanPadding),n=m(this.options.autoPanPaddingTopLeft||o),o=m(this.options.autoPanPaddingBottomRight||o),s=t.getSize(),r=0,i.x+a+o.x>s.x&&(r=i.x+a-s.x+o.x),i.x-r-n.x<(a=0)&&(r=i.x-n.x),i.y+e+o.y>s.y&&(a=i.y+e-s.y+o.y),i.y-a-n.y<0&&(a=i.y-n.y),(r||a)&&(this.options.keepInView&&(this._autopanning=!0),t.fire("autopanstart").panBy([r,a]))))},_getAnchor:function(){return m(this._source&&this._source._getPopupAnchor?this._source._getPopupAnchor():[0,0])}})),Ii=(A.mergeOptions({closePopupOnClick:!0}),A.include({openPopup:function(t,e,i){return this._initOverlay(Bi,t,e,i).openOn(this),this},closePopup:function(t){return(t=arguments.length?t:this._popup)&&t.close(),this}}),o.include({bindPopup:function(t,e){return this._popup=this._initOverlay(Bi,this._popup,t,e),this._popupHandlersAdded||(this.on({click:this._openPopup,keypress:this._onKeyPress,remove:this.closePopup,move:this._movePopup}),this._popupHandlersAdded=!0),this},unbindPopup:function(){return this._popup&&(this.off({click:this._openPopup,keypress:this._onKeyPress,remove:this.closePopup,move:this._movePopup}),this._popupHandlersAdded=!1,this._popup=null),this},openPopup:function(t){return this._popup&&(this instanceof ci||(this._popup._source=this),this._popup._prepareOpen(t||this._latlng)&&this._popup.openOn(this._map)),this},closePopup:function(){return this._popup&&this._popup.close(),this},togglePopup:function(){return this._popup&&this._popup.toggle(this),this},isPopupOpen:function(){return!!this._popup&&this._popup.isOpen()},setPopupContent:function(t){return this._popup&&this._popup.setContent(t),this},getPopup:function(){return this._popup},_openPopup:function(t){var e;this._popup&&this._map&&(Re(t),e=t.layer||t.target,this._popup._source!==e||e instanceof fi?(this._popup._source=e,this.openPopup(t.latlng)):this._map.hasLayer(this._popup)?this.closePopup():this.openPopup(t.latlng))},_movePopup:function(t){this._popup.setLatLng(t.latlng)},_onKeyPress:function(t){13===t.originalEvent.keyCode&&this._openPopup(t)}}),Ai.extend({options:{pane:"tooltipPane",offset:[0,0],direction:"auto",permanent:!1,sticky:!1,opacity:.9},onAdd:function(t){Ai.prototype.onAdd.call(this,t),this.setOpacity(this.options.opacity),t.fire("tooltipopen",{tooltip:this}),this._source&&(this.addEventParent(this._source),this._source.fire("tooltipopen",{tooltip:this},!0))},onRemove:function(t){Ai.prototype.onRemove.call(this,t),t.fire("tooltipclose",{tooltip:this}),this._source&&(this.removeEventParent(this._source),this._source.fire("tooltipclose",{tooltip:this},!0))},getEvents:function(){var t=Ai.prototype.getEvents.call(this);return this.options.permanent||(t.preclick=this.close),t},_initLayout:function(){var t="leaflet-tooltip "+(this.options.className||"")+" leaflet-zoom-"+(this._zoomAnimated?"animated":"hide");this._contentNode=this._container=P("div",t),this._container.setAttribute("role","tooltip"),this._container.setAttribute("id","leaflet-tooltip-"+h(this))},_updateLayout:function(){},_adjustPan:function(){},_setPosition:function(t){var e,i=this._map,n=this._container,o=i.latLngToContainerPoint(i.getCenter()),i=i.layerPointToContainerPoint(t),s=this.options.direction,r=n.offsetWidth,a=n.offsetHeight,h=m(this.options.offset),l=this._getAnchor(),i="top"===s?(e=r/2,a):"bottom"===s?(e=r/2,0):(e="center"===s?r/2:"right"===s?0:"left"===s?r:i.x<o.x?(s="right",0):(s="left",r+2*(h.x+l.x)),a/2);t=t.subtract(m(e,i,!0)).add(h).add(l),z(n,"leaflet-tooltip-right"),z(n,"leaflet-tooltip-left"),z(n,"leaflet-tooltip-top"),z(n,"leaflet-tooltip-bottom"),M(n,"leaflet-tooltip-"+s),Z(n,t)},_updatePosition:function(){var t=this._map.latLngToLayerPoint(this._latlng);this._setPosition(t)},setOpacity:function(t){this.options.opacity=t,this._container&&C(this._container,t)},_animateZoom:function(t){t=this._map._latLngToNewLayerPoint(this._latlng,t.zoom,t.center);this._setPosition(t)},_getAnchor:function(){return m(this._source&&this._source._getTooltipAnchor&&!this.options.sticky?this._source._getTooltipAnchor():[0,0])}})),Ri=(A.include({openTooltip:function(t,e,i){return this._initOverlay(Ii,t,e,i).openOn(this),this},closeTooltip:function(t){return t.close(),this}}),o.include({bindTooltip:function(t,e){return this._tooltip&&this.isTooltipOpen()&&this.unbindTooltip(),this._tooltip=this._initOverlay(Ii,this._tooltip,t,e),this._initTooltipInteractions(),this._tooltip.options.permanent&&this._map&&this._map.hasLayer(this)&&this.openTooltip(),this},unbindTooltip:function(){return this._tooltip&&(this._initTooltipInteractions(!0),this.closeTooltip(),this._tooltip=null),this},_initTooltipInteractions:function(t){var e,i;!t&&this._tooltipHandlersAdded||(e=t?"off":"on",i={remove:this.closeTooltip,move:this._moveTooltip},this._tooltip.options.permanent?i.add=this._openTooltip:(i.mouseover=this._openTooltip,i.mouseout=this.closeTooltip,i.click=this._openTooltip,this._map?this._addFocusListeners():i.add=this._addFocusListeners),this._tooltip.options.sticky&&(i.mousemove=this._moveTooltip),this[e](i),this._tooltipHandlersAdded=!t)},openTooltip:function(t){return this._tooltip&&(this instanceof ci||(this._tooltip._source=this),this._tooltip._prepareOpen(t)&&(this._tooltip.openOn(this._map),this.getElement?this._setAriaDescribedByOnLayer(this):this.eachLayer&&this.eachLayer(this._setAriaDescribedByOnLayer,this))),this},closeTooltip:function(){if(this._tooltip)return this._tooltip.close()},toggleTooltip:function(){return this._tooltip&&this._tooltip.toggle(this),this},isTooltipOpen:function(){return this._tooltip.isOpen()},setTooltipContent:function(t){return this._tooltip&&this._tooltip.setContent(t),this},getTooltip:function(){return this._tooltip},_addFocusListeners:function(){this.getElement?this._addFocusListenersOnLayer(this):this.eachLayer&&this.eachLayer(this._addFocusListenersOnLayer,this)},_addFocusListenersOnLayer:function(t){var e="function"==typeof t.getElement&&t.getElement();e&&(S(e,"focus",function(){this._tooltip._source=t,this.openTooltip()},this),S(e,"blur",this.closeTooltip,this))},_setAriaDescribedByOnLayer:function(t){t="function"==typeof t.getElement&&t.getElement();t&&t.setAttribute("aria-describedby",this._tooltip._container.id)},_openTooltip:function(t){var e;this._tooltip&&this._map&&(this._map.dragging&&this._map.dragging.moving()&&!this._openOnceFlag?(this._openOnceFlag=!0,(e=this)._map.once("moveend",function(){e._openOnceFlag=!1,e._openTooltip(t)})):(this._tooltip._source=t.layer||t.target,this.openTooltip(this._tooltip.options.sticky?t.latlng:void 0)))},_moveTooltip:function(t){var e=t.latlng;this._tooltip.options.sticky&&t.originalEvent&&(t=this._map.mouseEventToContainerPoint(t.originalEvent),t=this._map.containerPointToLayerPoint(t),e=this._map.layerPointToLatLng(t)),this._tooltip.setLatLng(e)}}),di.extend({options:{iconSize:[12,12],html:!1,bgPos:null,className:"leaflet-div-icon"},createIcon:function(t){var t=t&&"DIV"===t.tagName?t:document.createElement("div"),e=this.options;return e.html instanceof Element?(me(t),t.appendChild(e.html)):t.innerHTML=!1!==e.html?e.html:"",e.bgPos&&(e=m(e.bgPos),t.style.backgroundPosition=-e.x+"px "+-e.y+"px"),this._setIconStyles(t,"icon"),t},createShadow:function(){return null}}));di.Default=_i;var Ni=o.extend({options:{tileSize:256,opacity:1,updateWhenIdle:b.mobile,updateWhenZooming:!0,updateInterval:200,zIndex:1,bounds:null,minZoom:0,maxZoom:void 0,maxNativeZoom:void 0,minNativeZoom:void 0,noWrap:!1,pane:"tilePane",className:"",keepBuffer:2},initialize:function(t){c(this,t)},onAdd:function(){this._initContainer(),this._levels={},this._tiles={},this._resetView()},beforeAdd:function(t){t._addZoomLimit(this)},onRemove:function(t){this._removeAllTiles(),T(this._container),t._removeZoomLimit(this),this._container=null,this._tileZoom=void 0},bringToFront:function(){return this._map&&(fe(this._container),this._setAutoZIndex(Math.max)),this},bringToBack:function(){return this._map&&(ge(this._container),this._setAutoZIndex(Math.min)),this},getContainer:function(){return this._container},setOpacity:function(t){return this.options.opacity=t,this._updateOpacity(),this},setZIndex:function(t){return this.options.zIndex=t,this._updateZIndex(),this},isLoading:function(){return this._loading},redraw:function(){var t;return this._map&&(this._removeAllTiles(),(t=this._clampZoom(this._map.getZoom()))!==this._tileZoom&&(this._tileZoom=t,this._updateLevels()),this._update()),this},getEvents:function(){var t={viewprereset:this._invalidateAll,viewreset:this._resetView,zoom:this._resetView,moveend:this._onMoveEnd};return this.options.updateWhenIdle||(this._onMove||(this._onMove=j(this._onMoveEnd,this.options.updateInterval,this)),t.move=this._onMove),this._zoomAnimated&&(t.zoomanim=this._animateZoom),t},createTile:function(){return document.createElement("div")},getTileSize:function(){var t=this.options.tileSize;return t instanceof p?t:new p(t,t)},_updateZIndex:function(){this._container&&void 0!==this.options.zIndex&&null!==this.options.zIndex&&(this._container.style.zIndex=this.options.zIndex)},_setAutoZIndex:function(t){for(var e,i=this.getPane().children,n=-t(-1/0,1/0),o=0,s=i.length;o<s;o++)e=i[o].style.zIndex,i[o]!==this._container&&e&&(n=t(n,+e));isFinite(n)&&(this.options.zIndex=n+t(-1,1),this._updateZIndex())},_updateOpacity:function(){if(this._map&&!b.ielt9){C(this._container,this.options.opacity);var t,e=+new Date,i=!1,n=!1;for(t in this._tiles){var o,s=this._tiles[t];s.current&&s.loaded&&(o=Math.min(1,(e-s.loaded)/200),C(s.el,o),o<1?i=!0:(s.active?n=!0:this._onOpaqueTile(s),s.active=!0))}n&&!this._noPrune&&this._pruneTiles(),i&&(r(this._fadeFrame),this._fadeFrame=x(this._updateOpacity,this))}},_onOpaqueTile:u,_initContainer:function(){this._container||(this._container=P("div","leaflet-layer "+(this.options.className||"")),this._updateZIndex(),this.options.opacity<1&&this._updateOpacity(),this.getPane().appendChild(this._container))},_updateLevels:function(){var t=this._tileZoom,e=this.options.maxZoom;if(void 0!==t){for(var i in this._levels)i=Number(i),this._levels[i].el.children.length||i===t?(this._levels[i].el.style.zIndex=e-Math.abs(t-i),this._onUpdateLevel(i)):(T(this._levels[i].el),this._removeTilesAtZoom(i),this._onRemoveLevel(i),delete this._levels[i]);var n=this._levels[t],o=this._map;return n||((n=this._levels[t]={}).el=P("div","leaflet-tile-container leaflet-zoom-animated",this._container),n.el.style.zIndex=e,n.origin=o.project(o.unproject(o.getPixelOrigin()),t).round(),n.zoom=t,this._setZoomTransform(n,o.getCenter(),o.getZoom()),u(n.el.offsetWidth),this._onCreateLevel(n)),this._level=n}},_onUpdateLevel:u,_onRemoveLevel:u,_onCreateLevel:u,_pruneTiles:function(){if(this._map){var t,e,i,n=this._map.getZoom();if(n>this.options.maxZoom||n<this.options.minZoom)this._removeAllTiles();else{for(t in this._tiles)(i=this._tiles[t]).retain=i.current;for(t in this._tiles)(i=this._tiles[t]).current&&!i.active&&(e=i.coords,this._retainParent(e.x,e.y,e.z,e.z-5)||this._retainChildren(e.x,e.y,e.z,e.z+2));for(t in this._tiles)this._tiles[t].retain||this._removeTile(t)}}},_removeTilesAtZoom:function(t){for(var e in this._tiles)this._tiles[e].coords.z===t&&this._removeTile(e)},_removeAllTiles:function(){for(var t in this._tiles)this._removeTile(t)},_invalidateAll:function(){for(var t in this._levels)T(this._levels[t].el),this._onRemoveLevel(Number(t)),delete this._levels[t];this._removeAllTiles(),this._tileZoom=void 0},_retainParent:function(t,e,i,n){var t=Math.floor(t/2),e=Math.floor(e/2),i=i-1,o=new p(+t,+e),o=(o.z=i,this._tileCoordsToKey(o)),o=this._tiles[o];return o&&o.active?o.retain=!0:(o&&o.loaded&&(o.retain=!0),n<i&&this._retainParent(t,e,i,n))},_retainChildren:function(t,e,i,n){for(var o=2*t;o<2*t+2;o++)for(var s=2*e;s<2*e+2;s++){var r=new p(o,s),r=(r.z=i+1,this._tileCoordsToKey(r)),r=this._tiles[r];r&&r.active?r.retain=!0:(r&&r.loaded&&(r.retain=!0),i+1<n&&this._retainChildren(o,s,i+1,n))}},_resetView:function(t){t=t&&(t.pinch||t.flyTo);this._setView(this._map.getCenter(),this._map.getZoom(),t,t)},_animateZoom:function(t){this._setView(t.center,t.zoom,!0,t.noUpdate)},_clampZoom:function(t){var e=this.options;return void 0!==e.minNativeZoom&&t<e.minNativeZoom?e.minNativeZoom:void 0!==e.maxNativeZoom&&e.maxNativeZoom<t?e.maxNativeZoom:t},_setView:function(t,e,i,n){var o=Math.round(e),o=void 0!==this.options.maxZoom&&o>this.options.maxZoom||void 0!==this.options.minZoom&&o<this.options.minZoom?void 0:this._clampZoom(o),s=this.options.updateWhenZooming&&o!==this._tileZoom;n&&!s||(this._tileZoom=o,this._abortLoading&&this._abortLoading(),this._updateLevels(),this._resetGrid(),void 0!==o&&this._update(t),i||this._pruneTiles(),this._noPrune=!!i),this._setZoomTransforms(t,e)},_setZoomTransforms:function(t,e){for(var i in this._levels)this._setZoomTransform(this._levels[i],t,e)},_setZoomTransform:function(t,e,i){var n=this._map.getZoomScale(i,t.zoom),e=t.origin.multiplyBy(n).subtract(this._map._getNewPixelOrigin(e,i)).round();b.any3d?be(t.el,e,n):Z(t.el,e)},_resetGrid:function(){var t=this._map,e=t.options.crs,i=this._tileSize=this.getTileSize(),n=this._tileZoom,o=this._map.getPixelWorldBounds(this._tileZoom);o&&(this._globalTileRange=this._pxBoundsToTileRange(o)),this._wrapX=e.wrapLng&&!this.options.noWrap&&[Math.floor(t.project([0,e.wrapLng[0]],n).x/i.x),Math.ceil(t.project([0,e.wrapLng[1]],n).x/i.y)],this._wrapY=e.wrapLat&&!this.options.noWrap&&[Math.floor(t.project([e.wrapLat[0],0],n).y/i.x),Math.ceil(t.project([e.wrapLat[1],0],n).y/i.y)]},_onMoveEnd:function(){this._map&&!this._map._animatingZoom&&this._update()},_getTiledPixelBounds:function(t){var e=this._map,i=e._animatingZoom?Math.max(e._animateToZoom,e.getZoom()):e.getZoom(),i=e.getZoomScale(i,this._tileZoom),t=e.project(t,this._tileZoom).floor(),e=e.getSize().divideBy(2*i);return new f(t.subtract(e),t.add(e))},_update:function(t){var e=this._map;if(e){var i=this._clampZoom(e.getZoom());if(void 0===t&&(t=e.getCenter()),void 0!==this._tileZoom){var n,e=this._getTiledPixelBounds(t),o=this._pxBoundsToTileRange(e),s=o.getCenter(),r=[],e=this.options.keepBuffer,a=new f(o.getBottomLeft().subtract([e,-e]),o.getTopRight().add([e,-e]));if(!(isFinite(o.min.x)&&isFinite(o.min.y)&&isFinite(o.max.x)&&isFinite(o.max.y)))throw new Error("Attempted to load an infinite number of tiles");for(n in this._tiles){var h=this._tiles[n].coords;h.z===this._tileZoom&&a.contains(new p(h.x,h.y))||(this._tiles[n].current=!1)}if(1<Math.abs(i-this._tileZoom))this._setView(t,i);else{for(var l=o.min.y;l<=o.max.y;l++)for(var u=o.min.x;u<=o.max.x;u++){var c,d=new p(u,l);d.z=this._tileZoom,this._isValidTile(d)&&((c=this._tiles[this._tileCoordsToKey(d)])?c.current=!0:r.push(d))}if(r.sort(function(t,e){return t.distanceTo(s)-e.distanceTo(s)}),0!==r.length){this._loading||(this._loading=!0,this.fire("loading"));for(var _=document.createDocumentFragment(),u=0;u<r.length;u++)this._addTile(r[u],_);this._level.el.appendChild(_)}}}}},_isValidTile:function(t){var e=this._map.options.crs;if(!e.infinite){var i=this._globalTileRange;if(!e.wrapLng&&(t.x<i.min.x||t.x>i.max.x)||!e.wrapLat&&(t.y<i.min.y||t.y>i.max.y))return!1}return!this.options.bounds||(e=this._tileCoordsToBounds(t),g(this.options.bounds).overlaps(e))},_keyToBounds:function(t){return this._tileCoordsToBounds(this._keyToTileCoords(t))},_tileCoordsToNwSe:function(t){var e=this._map,i=this.getTileSize(),n=t.scaleBy(i),i=n.add(i);return[e.unproject(n,t.z),e.unproject(i,t.z)]},_tileCoordsToBounds:function(t){t=this._tileCoordsToNwSe(t),t=new s(t[0],t[1]);return t=this.options.noWrap?t:this._map.wrapLatLngBounds(t)},_tileCoordsToKey:function(t){return t.x+":"+t.y+":"+t.z},_keyToTileCoords:function(t){var t=t.split(":"),e=new p(+t[0],+t[1]);return e.z=+t[2],e},_removeTile:function(t){var e=this._tiles[t];e&&(T(e.el),delete this._tiles[t],this.fire("tileunload",{tile:e.el,coords:this._keyToTileCoords(t)}))},_initTile:function(t){M(t,"leaflet-tile");var e=this.getTileSize();t.style.width=e.x+"px",t.style.height=e.y+"px",t.onselectstart=u,t.onmousemove=u,b.ielt9&&this.options.opacity<1&&C(t,this.options.opacity)},_addTile:function(t,e){var i=this._getTilePos(t),n=this._tileCoordsToKey(t),o=this.createTile(this._wrapCoords(t),a(this._tileReady,this,t));this._initTile(o),this.createTile.length<2&&x(a(this._tileReady,this,t,null,o)),Z(o,i),this._tiles[n]={el:o,coords:t,current:!0},e.appendChild(o),this.fire("tileloadstart",{tile:o,coords:t})},_tileReady:function(t,e,i){e&&this.fire("tileerror",{error:e,tile:i,coords:t});var n=this._tileCoordsToKey(t);(i=this._tiles[n])&&(i.loaded=+new Date,this._map._fadeAnimated?(C(i.el,0),r(this._fadeFrame),this._fadeFrame=x(this._updateOpacity,this)):(i.active=!0,this._pruneTiles()),e||(M(i.el,"leaflet-tile-loaded"),this.fire("tileload",{tile:i.el,coords:t})),this._noTilesToLoad()&&(this._loading=!1,this.fire("load"),b.ielt9||!this._map._fadeAnimated?x(this._pruneTiles,this):setTimeout(a(this._pruneTiles,this),250)))},_getTilePos:function(t){return t.scaleBy(this.getTileSize()).subtract(this._level.origin)},_wrapCoords:function(t){var e=new p(this._wrapX?H(t.x,this._wrapX):t.x,this._wrapY?H(t.y,this._wrapY):t.y);return e.z=t.z,e},_pxBoundsToTileRange:function(t){var e=this.getTileSize();return new f(t.min.unscaleBy(e).floor(),t.max.unscaleBy(e).ceil().subtract([1,1]))},_noTilesToLoad:function(){for(var t in this._tiles)if(!this._tiles[t].loaded)return!1;return!0}});var Di=Ni.extend({options:{minZoom:0,maxZoom:18,subdomains:"abc",errorTileUrl:"",zoomOffset:0,tms:!1,zoomReverse:!1,detectRetina:!1,crossOrigin:!1,referrerPolicy:!1},initialize:function(t,e){this._url=t,(e=c(this,e)).detectRetina&&b.retina&&0<e.maxZoom?(e.tileSize=Math.floor(e.tileSize/2),e.zoomReverse?(e.zoomOffset--,e.minZoom=Math.min(e.maxZoom,e.minZoom+1)):(e.zoomOffset++,e.maxZoom=Math.max(e.minZoom,e.maxZoom-1)),e.minZoom=Math.max(0,e.minZoom)):e.zoomReverse?e.minZoom=Math.min(e.maxZoom,e.minZoom):e.maxZoom=Math.max(e.minZoom,e.maxZoom),"string"==typeof e.subdomains&&(e.subdomains=e.subdomains.split("")),this.on("tileunload",this._onTileRemove)},setUrl:function(t,e){return this._url===t&&void 0===e&&(e=!0),this._url=t,e||this.redraw(),this},createTile:function(t,e){var i=document.createElement("img");return S(i,"load",a(this._tileOnLoad,this,e,i)),S(i,"error",a(this._tileOnError,this,e,i)),!this.options.crossOrigin&&""!==this.options.crossOrigin||(i.crossOrigin=!0===this.options.crossOrigin?"":this.options.crossOrigin),"string"==typeof this.options.referrerPolicy&&(i.referrerPolicy=this.options.referrerPolicy),i.alt="",i.src=this.getTileUrl(t),i},getTileUrl:function(t){var e={r:b.retina?"@2x":"",s:this._getSubdomain(t),x:t.x,y:t.y,z:this._getZoomForUrl()};return this._map&&!this._map.options.crs.infinite&&(t=this._globalTileRange.max.y-t.y,this.options.tms&&(e.y=t),e["-y"]=t),q(this._url,l(e,this.options))},_tileOnLoad:function(t,e){b.ielt9?setTimeout(a(t,this,null,e),0):t(null,e)},_tileOnError:function(t,e,i){var n=this.options.errorTileUrl;n&&e.getAttribute("src")!==n&&(e.src=n),t(i,e)},_onTileRemove:function(t){t.tile.onload=null},_getZoomForUrl:function(){var t=this._tileZoom,e=this.options.maxZoom;return(t=this.options.zoomReverse?e-t:t)+this.options.zoomOffset},_getSubdomain:function(t){t=Math.abs(t.x+t.y)%this.options.subdomains.length;return this.options.subdomains[t]},_abortLoading:function(){var t,e,i;for(t in this._tiles)this._tiles[t].coords.z!==this._tileZoom&&((i=this._tiles[t].el).onload=u,i.onerror=u,i.complete||(i.src=K,e=this._tiles[t].coords,T(i),delete this._tiles[t],this.fire("tileabort",{tile:i,coords:e})))},_removeTile:function(t){var e=this._tiles[t];if(e)return e.el.setAttribute("src",K),Ni.prototype._removeTile.call(this,t)},_tileReady:function(t,e,i){if(this._map&&(!i||i.getAttribute("src")!==K))return Ni.prototype._tileReady.call(this,t,e,i)}});function ji(t,e){return new Di(t,e)}var Hi=Di.extend({defaultWmsParams:{service:"WMS",request:"GetMap",layers:"",styles:"",format:"image/jpeg",transparent:!1,version:"1.1.1"},options:{crs:null,uppercase:!1},initialize:function(t,e){this._url=t;var i,n=l({},this.defaultWmsParams);for(i in e)i in this.options||(n[i]=e[i]);var t=(e=c(this,e)).detectRetina&&b.retina?2:1,o=this.getTileSize();n.width=o.x*t,n.height=o.y*t,this.wmsParams=n},onAdd:function(t){this._crs=this.options.crs||t.options.crs,this._wmsVersion=parseFloat(this.wmsParams.version);var e=1.3<=this._wmsVersion?"crs":"srs";this.wmsParams[e]=this._crs.code,Di.prototype.onAdd.call(this,t)},getTileUrl:function(t){var e=this._tileCoordsToNwSe(t),i=this._crs,i=_(i.project(e[0]),i.project(e[1])),e=i.min,i=i.max,e=(1.3<=this._wmsVersion&&this._crs===li?[e.y,e.x,i.y,i.x]:[e.x,e.y,i.x,i.y]).join(","),i=Di.prototype.getTileUrl.call(this,t);return i+U(this.wmsParams,i,this.options.uppercase)+(this.options.uppercase?"&BBOX=":"&bbox=")+e},setParams:function(t,e){return l(this.wmsParams,t),e||this.redraw(),this}});Di.WMS=Hi,ji.wms=function(t,e){return new Hi(t,e)};var Wi=o.extend({options:{padding:.1},initialize:function(t){c(this,t),h(this),this._layers=this._layers||{}},onAdd:function(){this._container||(this._initContainer(),M(this._container,"leaflet-zoom-animated")),this.getPane().appendChild(this._container),this._update(),this.on("update",this._updatePaths,this)},onRemove:function(){this.off("update",this._updatePaths,this),this._destroyContainer()},getEvents:function(){var t={viewreset:this._reset,zoom:this._onZoom,moveend:this._update,zoomend:this._onZoomEnd};return this._zoomAnimated&&(t.zoomanim=this._onAnimZoom),t},_onAnimZoom:function(t){this._updateTransform(t.center,t.zoom)},_onZoom:function(){this._updateTransform(this._map.getCenter(),this._map.getZoom())},_updateTransform:function(t,e){var i=this._map.getZoomScale(e,this._zoom),n=this._map.getSize().multiplyBy(.5+this.options.padding),o=this._map.project(this._center,e),n=n.multiplyBy(-i).add(o).subtract(this._map._getNewPixelOrigin(t,e));b.any3d?be(this._container,n,i):Z(this._container,n)},_reset:function(){for(var t in this._update(),this._updateTransform(this._center,this._zoom),this._layers)this._layers[t]._reset()},_onZoomEnd:function(){for(var t in this._layers)this._layers[t]._project()},_updatePaths:function(){for(var t in this._layers)this._layers[t]._update()},_update:function(){var t=this.options.padding,e=this._map.getSize(),i=this._map.containerPointToLayerPoint(e.multiplyBy(-t)).round();this._bounds=new f(i,i.add(e.multiplyBy(1+2*t)).round()),this._center=this._map.getCenter(),this._zoom=this._map.getZoom()}}),Fi=Wi.extend({options:{tolerance:0},getEvents:function(){var t=Wi.prototype.getEvents.call(this);return t.viewprereset=this._onViewPreReset,t},_onViewPreReset:function(){this._postponeUpdatePaths=!0},onAdd:function(){Wi.prototype.onAdd.call(this),this._draw()},_initContainer:function(){var t=this._container=document.createElement("canvas");S(t,"mousemove",this._onMouseMove,this),S(t,"click dblclick mousedown mouseup contextmenu",this._onClick,this),S(t,"mouseout",this._handleMouseOut,this),t._leaflet_disable_events=!0,this._ctx=t.getContext("2d")},_destroyContainer:function(){r(this._redrawRequest),delete this._ctx,T(this._container),k(this._container),delete this._container},_updatePaths:function(){if(!this._postponeUpdatePaths){for(var t in this._redrawBounds=null,this._layers)this._layers[t]._update();this._redraw()}},_update:function(){var t,e,i,n;this._map._animatingZoom&&this._bounds||(Wi.prototype._update.call(this),t=this._bounds,e=this._container,i=t.getSize(),n=b.retina?2:1,Z(e,t.min),e.width=n*i.x,e.height=n*i.y,e.style.width=i.x+"px",e.style.height=i.y+"px",b.retina&&this._ctx.scale(2,2),this._ctx.translate(-t.min.x,-t.min.y),this.fire("update"))},_reset:function(){Wi.prototype._reset.call(this),this._postponeUpdatePaths&&(this._postponeUpdatePaths=!1,this._updatePaths())},_initPath:function(t){this._updateDashArray(t);t=(this._layers[h(t)]=t)._order={layer:t,prev:this._drawLast,next:null};this._drawLast&&(this._drawLast.next=t),this._drawLast=t,this._drawFirst=this._drawFirst||this._drawLast},_addPath:function(t){this._requestRedraw(t)},_removePath:function(t){var e=t._order,i=e.next,e=e.prev;i?i.prev=e:this._drawLast=e,e?e.next=i:this._drawFirst=i,delete t._order,delete this._layers[h(t)],this._requestRedraw(t)},_updatePath:function(t){this._extendRedrawBounds(t),t._project(),t._update(),this._requestRedraw(t)},_updateStyle:function(t){this._updateDashArray(t),this._requestRedraw(t)},_updateDashArray:function(t){if("string"==typeof t.options.dashArray){for(var e,i=t.options.dashArray.split(/[, ]+/),n=[],o=0;o<i.length;o++){if(e=Number(i[o]),isNaN(e))return;n.push(e)}t.options._dashArray=n}else t.options._dashArray=t.options.dashArray},_requestRedraw:function(t){this._map&&(this._extendRedrawBounds(t),this._redrawRequest=this._redrawRequest||x(this._redraw,this))},_extendRedrawBounds:function(t){var e;t._pxBounds&&(e=(t.options.weight||0)+1,this._redrawBounds=this._redrawBounds||new f,this._redrawBounds.extend(t._pxBounds.min.subtract([e,e])),this._redrawBounds.extend(t._pxBounds.max.add([e,e])))},_redraw:function(){this._redrawRequest=null,this._redrawBounds&&(this._redrawBounds.min._floor(),this._redrawBounds.max._ceil()),this._clear(),this._draw(),this._redrawBounds=null},_clear:function(){var t,e=this._redrawBounds;e?(t=e.getSize(),this._ctx.clearRect(e.min.x,e.min.y,t.x,t.y)):(this._ctx.save(),this._ctx.setTransform(1,0,0,1,0,0),this._ctx.clearRect(0,0,this._container.width,this._container.height),this._ctx.restore())},_draw:function(){var t,e,i=this._redrawBounds;this._ctx.save(),i&&(e=i.getSize(),this._ctx.beginPath(),this._ctx.rect(i.min.x,i.min.y,e.x,e.y),this._ctx.clip()),this._drawing=!0;for(var n=this._drawFirst;n;n=n.next)t=n.layer,(!i||t._pxBounds&&t._pxBounds.intersects(i))&&t._updatePath();this._drawing=!1,this._ctx.restore()},_updatePoly:function(t,e){if(this._drawing){var i,n,o,s,r=t._parts,a=r.length,h=this._ctx;if(a){for(h.beginPath(),i=0;i<a;i++){for(n=0,o=r[i].length;n<o;n++)s=r[i][n],h[n?"lineTo":"moveTo"](s.x,s.y);e&&h.closePath()}this._fillStroke(h,t)}}},_updateCircle:function(t){var e,i,n,o;this._drawing&&!t._empty()&&(e=t._point,i=this._ctx,n=Math.max(Math.round(t._radius),1),1!=(o=(Math.max(Math.round(t._radiusY),1)||n)/n)&&(i.save(),i.scale(1,o)),i.beginPath(),i.arc(e.x,e.y/o,n,0,2*Math.PI,!1),1!=o&&i.restore(),this._fillStroke(i,t))},_fillStroke:function(t,e){var i=e.options;i.fill&&(t.globalAlpha=i.fillOpacity,t.fillStyle=i.fillColor||i.color,t.fill(i.fillRule||"evenodd")),i.stroke&&0!==i.weight&&(t.setLineDash&&t.setLineDash(e.options&&e.options._dashArray||[]),t.globalAlpha=i.opacity,t.lineWidth=i.weight,t.strokeStyle=i.color,t.lineCap=i.lineCap,t.lineJoin=i.lineJoin,t.stroke())},_onClick:function(t){for(var e,i,n=this._map.mouseEventToLayerPoint(t),o=this._drawFirst;o;o=o.next)(e=o.layer).options.interactive&&e._containsPoint(n)&&(("click"===t.type||"preclick"===t.type)&&this._map._draggableMoved(e)||(i=e));this._fireEvent(!!i&&[i],t)},_onMouseMove:function(t){var e;!this._map||this._map.dragging.moving()||this._map._animatingZoom||(e=this._map.mouseEventToLayerPoint(t),this._handleMouseHover(t,e))},_handleMouseOut:function(t){var e=this._hoveredLayer;e&&(z(this._container,"leaflet-interactive"),this._fireEvent([e],t,"mouseout"),this._hoveredLayer=null,this._mouseHoverThrottled=!1)},_handleMouseHover:function(t,e){if(!this._mouseHoverThrottled){for(var i,n,o=this._drawFirst;o;o=o.next)(i=o.layer).options.interactive&&i._containsPoint(e)&&(n=i);n!==this._hoveredLayer&&(this._handleMouseOut(t),n&&(M(this._container,"leaflet-interactive"),this._fireEvent([n],t,"mouseover"),this._hoveredLayer=n)),this._fireEvent(!!this._hoveredLayer&&[this._hoveredLayer],t),this._mouseHoverThrottled=!0,setTimeout(a(function(){this._mouseHoverThrottled=!1},this),32)}},_fireEvent:function(t,e,i){this._map._fireDOMEvent(e,i||e.type,t)},_bringToFront:function(t){var e,i,n=t._order;n&&(e=n.next,i=n.prev,e&&((e.prev=i)?i.next=e:e&&(this._drawFirst=e),n.prev=this._drawLast,(this._drawLast.next=n).next=null,this._drawLast=n,this._requestRedraw(t)))},_bringToBack:function(t){var e,i,n=t._order;n&&(e=n.next,(i=n.prev)&&((i.next=e)?e.prev=i:i&&(this._drawLast=i),n.prev=null,n.next=this._drawFirst,this._drawFirst.prev=n,this._drawFirst=n,this._requestRedraw(t)))}});function Ui(t){return b.canvas?new Fi(t):null}var Vi=function(){try{return document.namespaces.add("lvml","urn:schemas-microsoft-com:vml"),function(t){return document.createElement("<lvml:"+t+' class="lvml">')}}catch(t){}return function(t){return document.createElement("<"+t+' xmlns="urn:schemas-microsoft.com:vml" class="lvml">')}}(),zt={_initContainer:function(){this._container=P("div","leaflet-vml-container")},_update:function(){this._map._animatingZoom||(Wi.prototype._update.call(this),this.fire("update"))},_initPath:function(t){var e=t._container=Vi("shape");M(e,"leaflet-vml-shape "+(this.options.className||"")),e.coordsize="1 1",t._path=Vi("path"),e.appendChild(t._path),this._updateStyle(t),this._layers[h(t)]=t},_addPath:function(t){var e=t._container;this._container.appendChild(e),t.options.interactive&&t.addInteractiveTarget(e)},_removePath:function(t){var e=t._container;T(e),t.removeInteractiveTarget(e),delete this._layers[h(t)]},_updateStyle:function(t){var e=t._stroke,i=t._fill,n=t.options,o=t._container;o.stroked=!!n.stroke,o.filled=!!n.fill,n.stroke?(e=e||(t._stroke=Vi("stroke")),o.appendChild(e),e.weight=n.weight+"px",e.color=n.color,e.opacity=n.opacity,n.dashArray?e.dashStyle=d(n.dashArray)?n.dashArray.join(" "):n.dashArray.replace(/( *, *)/g," "):e.dashStyle="",e.endcap=n.lineCap.replace("butt","flat"),e.joinstyle=n.lineJoin):e&&(o.removeChild(e),t._stroke=null),n.fill?(i=i||(t._fill=Vi("fill")),o.appendChild(i),i.color=n.fillColor||n.color,i.opacity=n.fillOpacity):i&&(o.removeChild(i),t._fill=null)},_updateCircle:function(t){var e=t._point.round(),i=Math.round(t._radius),n=Math.round(t._radiusY||i);this._setPath(t,t._empty()?"M0 0":"AL "+e.x+","+e.y+" "+i+","+n+" 0,23592600")},_setPath:function(t,e){t._path.v=e},_bringToFront:function(t){fe(t._container)},_bringToBack:function(t){ge(t._container)}},qi=b.vml?Vi:ct,Gi=Wi.extend({_initContainer:function(){this._container=qi("svg"),this._container.setAttribute("pointer-events","none"),this._rootGroup=qi("g"),this._container.appendChild(this._rootGroup)},_destroyContainer:function(){T(this._container),k(this._container),delete this._container,delete this._rootGroup,delete this._svgSize},_update:function(){var t,e,i;this._map._animatingZoom&&this._bounds||(Wi.prototype._update.call(this),e=(t=this._bounds).getSize(),i=this._container,this._svgSize&&this._svgSize.equals(e)||(this._svgSize=e,i.setAttribute("width",e.x),i.setAttribute("height",e.y)),Z(i,t.min),i.setAttribute("viewBox",[t.min.x,t.min.y,e.x,e.y].join(" ")),this.fire("update"))},_initPath:function(t){var e=t._path=qi("path");t.options.className&&M(e,t.options.className),t.options.interactive&&M(e,"leaflet-interactive"),this._updateStyle(t),this._layers[h(t)]=t},_addPath:function(t){this._rootGroup||this._initContainer(),this._rootGroup.appendChild(t._path),t.addInteractiveTarget(t._path)},_removePath:function(t){T(t._path),t.removeInteractiveTarget(t._path),delete this._layers[h(t)]},_updatePath:function(t){t._project(),t._update()},_updateStyle:function(t){var e=t._path,t=t.options;e&&(t.stroke?(e.setAttribute("stroke",t.color),e.setAttribute("stroke-opacity",t.opacity),e.setAttribute("stroke-width",t.weight),e.setAttribute("stroke-linecap",t.lineCap),e.setAttribute("stroke-linejoin",t.lineJoin),t.dashArray?e.setAttribute("stroke-dasharray",t.dashArray):e.removeAttribute("stroke-dasharray"),t.dashOffset?e.setAttribute("stroke-dashoffset",t.dashOffset):e.removeAttribute("stroke-dashoffset")):e.setAttribute("stroke","none"),t.fill?(e.setAttribute("fill",t.fillColor||t.color),e.setAttribute("fill-opacity",t.fillOpacity),e.setAttribute("fill-rule",t.fillRule||"evenodd")):e.setAttribute("fill","none"))},_updatePoly:function(t,e){this._setPath(t,dt(t._parts,e))},_updateCircle:function(t){var e=t._point,i=Math.max(Math.round(t._radius),1),n="a"+i+","+(Math.max(Math.round(t._radiusY),1)||i)+" 0 1,0 ",e=t._empty()?"M0 0":"M"+(e.x-i)+","+e.y+n+2*i+",0 "+n+2*-i+",0 ";this._setPath(t,e)},_setPath:function(t,e){t._path.setAttribute("d",e)},_bringToFront:function(t){fe(t._path)},_bringToBack:function(t){ge(t._path)}});function Ki(t){return b.svg||b.vml?new Gi(t):null}b.vml&&Gi.include(zt),A.include({getRenderer:function(t){t=(t=t.options.renderer||this._getPaneRenderer(t.options.pane)||this.options.renderer||this._renderer)||(this._renderer=this._createRenderer());return this.hasLayer(t)||this.addLayer(t),t},_getPaneRenderer:function(t){var e;return"overlayPane"!==t&&void 0!==t&&(void 0===(e=this._paneRenderers[t])&&(e=this._createRenderer({pane:t}),this._paneRenderers[t]=e),e)},_createRenderer:function(t){return this.options.preferCanvas&&Ui(t)||Ki(t)}});var Yi=xi.extend({initialize:function(t,e){xi.prototype.initialize.call(this,this._boundsToLatLngs(t),e)},setBounds:function(t){return this.setLatLngs(this._boundsToLatLngs(t))},_boundsToLatLngs:function(t){return[(t=g(t)).getSouthWest(),t.getNorthWest(),t.getNorthEast(),t.getSouthEast()]}});Gi.create=qi,Gi.pointsToPath=dt,wi.geometryToLayer=bi,wi.coordsToLatLng=Li,wi.coordsToLatLngs=Ti,wi.latLngToCoords=Mi,wi.latLngsToCoords=zi,wi.getFeature=Ci,wi.asFeature=Zi,A.mergeOptions({boxZoom:!0});var _t=n.extend({initialize:function(t){this._map=t,this._container=t._container,this._pane=t._panes.overlayPane,this._resetStateTimeout=0,t.on("unload",this._destroy,this)},addHooks:function(){S(this._container,"mousedown",this._onMouseDown,this)},removeHooks:function(){k(this._container,"mousedown",this._onMouseDown,this)},moved:function(){return this._moved},_destroy:function(){T(this._pane),delete this._pane},_resetState:function(){this._resetStateTimeout=0,this._moved=!1},_clearDeferredResetState:function(){0!==this._resetStateTimeout&&(clearTimeout(this._resetStateTimeout),this._resetStateTimeout=0)},_onMouseDown:function(t){if(!t.shiftKey||1!==t.which&&1!==t.button)return!1;this._clearDeferredResetState(),this._resetState(),re(),Le(),this._startPoint=this._map.mouseEventToContainerPoint(t),S(document,{contextmenu:Re,mousemove:this._onMouseMove,mouseup:this._onMouseUp,keydown:this._onKeyDown},this)},_onMouseMove:function(t){this._moved||(this._moved=!0,this._box=P("div","leaflet-zoom-box",this._container),M(this._container,"leaflet-crosshair"),this._map.fire("boxzoomstart")),this._point=this._map.mouseEventToContainerPoint(t);var t=new f(this._point,this._startPoint),e=t.getSize();Z(this._box,t.min),this._box.style.width=e.x+"px",this._box.style.height=e.y+"px"},_finish:function(){this._moved&&(T(this._box),z(this._container,"leaflet-crosshair")),ae(),Te(),k(document,{contextmenu:Re,mousemove:this._onMouseMove,mouseup:this._onMouseUp,keydown:this._onKeyDown},this)},_onMouseUp:function(t){1!==t.which&&1!==t.button||(this._finish(),this._moved&&(this._clearDeferredResetState(),this._resetStateTimeout=setTimeout(a(this._resetState,this),0),t=new s(this._map.containerPointToLatLng(this._startPoint),this._map.containerPointToLatLng(this._point)),this._map.fitBounds(t).fire("boxzoomend",{boxZoomBounds:t})))},_onKeyDown:function(t){27===t.keyCode&&(this._finish(),this._clearDeferredResetState(),this._resetState())}}),Ct=(A.addInitHook("addHandler","boxZoom",_t),A.mergeOptions({doubleClickZoom:!0}),n.extend({addHooks:function(){this._map.on("dblclick",this._onDoubleClick,this)},removeHooks:function(){this._map.off("dblclick",this._onDoubleClick,this)},_onDoubleClick:function(t){var e=this._map,i=e.getZoom(),n=e.options.zoomDelta,i=t.originalEvent.shiftKey?i-n:i+n;"center"===e.options.doubleClickZoom?e.setZoom(i):e.setZoomAround(t.containerPoint,i)}})),Zt=(A.addInitHook("addHandler","doubleClickZoom",Ct),A.mergeOptions({dragging:!0,inertia:!0,inertiaDeceleration:3400,inertiaMaxSpeed:1/0,easeLinearity:.2,worldCopyJump:!1,maxBoundsViscosity:0}),n.extend({addHooks:function(){var t;this._draggable||(t=this._map,this._draggable=new Xe(t._mapPane,t._container),this._draggable.on({dragstart:this._onDragStart,drag:this._onDrag,dragend:this._onDragEnd},this),this._draggable.on("predrag",this._onPreDragLimit,this),t.options.worldCopyJump&&(this._draggable.on("predrag",this._onPreDragWrap,this),t.on("zoomend",this._onZoomEnd,this),t.whenReady(this._onZoomEnd,this))),M(this._map._container,"leaflet-grab leaflet-touch-drag"),this._draggable.enable(),this._positions=[],this._times=[]},removeHooks:function(){z(this._map._container,"leaflet-grab"),z(this._map._container,"leaflet-touch-drag"),this._draggable.disable()},moved:function(){return this._draggable&&this._draggable._moved},moving:function(){return this._draggable&&this._draggable._moving},_onDragStart:function(){var t,e=this._map;e._stop(),this._map.options.maxBounds&&this._map.options.maxBoundsViscosity?(t=g(this._map.options.maxBounds),this._offsetLimit=_(this._map.latLngToContainerPoint(t.getNorthWest()).multiplyBy(-1),this._map.latLngToContainerPoint(t.getSouthEast()).multiplyBy(-1).add(this._map.getSize())),this._viscosity=Math.min(1,Math.max(0,this._map.options.maxBoundsViscosity))):this._offsetLimit=null,e.fire("movestart").fire("dragstart"),e.options.inertia&&(this._positions=[],this._times=[])},_onDrag:function(t){var e,i;this._map.options.inertia&&(e=this._lastTime=+new Date,i=this._lastPos=this._draggable._absPos||this._draggable._newPos,this._positions.push(i),this._times.push(e),this._prunePositions(e)),this._map.fire("move",t).fire("drag",t)},_prunePositions:function(t){for(;1<this._positions.length&&50<t-this._times[0];)this._positions.shift(),this._times.shift()},_onZoomEnd:function(){var t=this._map.getSize().divideBy(2),e=this._map.latLngToLayerPoint([0,0]);this._initialWorldOffset=e.subtract(t).x,this._worldWidth=this._map.getPixelWorldBounds().getSize().x},_viscousLimit:function(t,e){return t-(t-e)*this._viscosity},_onPreDragLimit:function(){var t,e;this._viscosity&&this._offsetLimit&&(t=this._draggable._newPos.subtract(this._draggable._startPos),e=this._offsetLimit,t.x<e.min.x&&(t.x=this._viscousLimit(t.x,e.min.x)),t.y<e.min.y&&(t.y=this._viscousLimit(t.y,e.min.y)),t.x>e.max.x&&(t.x=this._viscousLimit(t.x,e.max.x)),t.y>e.max.y&&(t.y=this._viscousLimit(t.y,e.max.y)),this._draggable._newPos=this._draggable._startPos.add(t))},_onPreDragWrap:function(){var t=this._worldWidth,e=Math.round(t/2),i=this._initialWorldOffset,n=this._draggable._newPos.x,o=(n-e+i)%t+e-i,n=(n+e+i)%t-e-i,t=Math.abs(o+i)<Math.abs(n+i)?o:n;this._draggable._absPos=this._draggable._newPos.clone(),this._draggable._newPos.x=t},_onDragEnd:function(t){var e,i,n,o,s=this._map,r=s.options,a=!r.inertia||t.noInertia||this._times.length<2;s.fire("dragend",t),!a&&(this._prunePositions(+new Date),t=this._lastPos.subtract(this._positions[0]),a=(this._lastTime-this._times[0])/1e3,e=r.easeLinearity,a=(t=t.multiplyBy(e/a)).distanceTo([0,0]),i=Math.min(r.inertiaMaxSpeed,a),t=t.multiplyBy(i/a),n=i/(r.inertiaDeceleration*e),(o=t.multiplyBy(-n/2).round()).x||o.y)?(o=s._limitOffset(o,s.options.maxBounds),x(function(){s.panBy(o,{duration:n,easeLinearity:e,noMoveStart:!0,animate:!0})})):s.fire("moveend")}})),St=(A.addInitHook("addHandler","dragging",Zt),A.mergeOptions({keyboard:!0,keyboardPanDelta:80}),n.extend({keyCodes:{left:[37],right:[39],down:[40],up:[38],zoomIn:[187,107,61,171],zoomOut:[189,109,54,173]},initialize:function(t){this._map=t,this._setPanDelta(t.options.keyboardPanDelta),this._setZoomDelta(t.options.zoomDelta)},addHooks:function(){var t=this._map._container;t.tabIndex<=0&&(t.tabIndex="0"),S(t,{focus:this._onFocus,blur:this._onBlur,mousedown:this._onMouseDown},this),this._map.on({focus:this._addHooks,blur:this._removeHooks},this)},removeHooks:function(){this._removeHooks(),k(this._map._container,{focus:this._onFocus,blur:this._onBlur,mousedown:this._onMouseDown},this),this._map.off({focus:this._addHooks,blur:this._removeHooks},this)},_onMouseDown:function(){var t,e,i;this._focused||(i=document.body,t=document.documentElement,e=i.scrollTop||t.scrollTop,i=i.scrollLeft||t.scrollLeft,this._map._container.focus(),window.scrollTo(i,e))},_onFocus:function(){this._focused=!0,this._map.fire("focus")},_onBlur:function(){this._focused=!1,this._map.fire("blur")},_setPanDelta:function(t){for(var e=this._panKeys={},i=this.keyCodes,n=0,o=i.left.length;n<o;n++)e[i.left[n]]=[-1*t,0];for(n=0,o=i.right.length;n<o;n++)e[i.right[n]]=[t,0];for(n=0,o=i.down.length;n<o;n++)e[i.down[n]]=[0,t];for(n=0,o=i.up.length;n<o;n++)e[i.up[n]]=[0,-1*t]},_setZoomDelta:function(t){for(var e=this._zoomKeys={},i=this.keyCodes,n=0,o=i.zoomIn.length;n<o;n++)e[i.zoomIn[n]]=t;for(n=0,o=i.zoomOut.length;n<o;n++)e[i.zoomOut[n]]=-t},_addHooks:function(){S(document,"keydown",this._onKeyDown,this)},_removeHooks:function(){k(document,"keydown",this._onKeyDown,this)},_onKeyDown:function(t){if(!(t.altKey||t.ctrlKey||t.metaKey)){var e,i,n=t.keyCode,o=this._map;if(n in this._panKeys)o._panAnim&&o._panAnim._inProgress||(i=this._panKeys[n],t.shiftKey&&(i=m(i).multiplyBy(3)),o.options.maxBounds&&(i=o._limitOffset(m(i),o.options.maxBounds)),o.options.worldCopyJump?(e=o.wrapLatLng(o.unproject(o.project(o.getCenter()).add(i))),o.panTo(e)):o.panBy(i));else if(n in this._zoomKeys)o.setZoom(o.getZoom()+(t.shiftKey?3:1)*this._zoomKeys[n]);else{if(27!==n||!o._popup||!o._popup.options.closeOnEscapeKey)return;o.closePopup()}Re(t)}}})),Et=(A.addInitHook("addHandler","keyboard",St),A.mergeOptions({scrollWheelZoom:!0,wheelDebounceTime:40,wheelPxPerZoomLevel:60}),n.extend({addHooks:function(){S(this._map._container,"wheel",this._onWheelScroll,this),this._delta=0},removeHooks:function(){k(this._map._container,"wheel",this._onWheelScroll,this)},_onWheelScroll:function(t){var e=He(t),i=this._map.options.wheelDebounceTime,e=(this._delta+=e,this._lastMousePos=this._map.mouseEventToContainerPoint(t),this._startTime||(this._startTime=+new Date),Math.max(i-(+new Date-this._startTime),0));clearTimeout(this._timer),this._timer=setTimeout(a(this._performZoom,this),e),Re(t)},_performZoom:function(){var t=this._map,e=t.getZoom(),i=this._map.options.zoomSnap||0,n=(t._stop(),this._delta/(4*this._map.options.wheelPxPerZoomLevel)),n=4*Math.log(2/(1+Math.exp(-Math.abs(n))))/Math.LN2,i=i?Math.ceil(n/i)*i:n,n=t._limitZoom(e+(0<this._delta?i:-i))-e;this._delta=0,this._startTime=null,n&&("center"===t.options.scrollWheelZoom?t.setZoom(e+n):t.setZoomAround(this._lastMousePos,e+n))}})),kt=(A.addInitHook("addHandler","scrollWheelZoom",Et),A.mergeOptions({tapHold:b.touchNative&&b.safari&&b.mobile,tapTolerance:15}),n.extend({addHooks:function(){S(this._map._container,"touchstart",this._onDown,this)},removeHooks:function(){k(this._map._container,"touchstart",this._onDown,this)},_onDown:function(t){var e;clearTimeout(this._holdTimeout),1===t.touches.length&&(e=t.touches[0],this._startPos=this._newPos=new p(e.clientX,e.clientY),this._holdTimeout=setTimeout(a(function(){this._cancel(),this._isTapValid()&&(S(document,"touchend",O),S(document,"touchend touchcancel",this._cancelClickPrevent),this._simulateEvent("contextmenu",e))},this),600),S(document,"touchend touchcancel contextmenu",this._cancel,this),S(document,"touchmove",this._onMove,this))},_cancelClickPrevent:function t(){k(document,"touchend",O),k(document,"touchend touchcancel",t)},_cancel:function(){clearTimeout(this._holdTimeout),k(document,"touchend touchcancel contextmenu",this._cancel,this),k(document,"touchmove",this._onMove,this)},_onMove:function(t){t=t.touches[0];this._newPos=new p(t.clientX,t.clientY)},_isTapValid:function(){return this._newPos.distanceTo(this._startPos)<=this._map.options.tapTolerance},_simulateEvent:function(t,e){t=new MouseEvent(t,{bubbles:!0,cancelable:!0,view:window,screenX:e.screenX,screenY:e.screenY,clientX:e.clientX,clientY:e.clientY});t._simulated=!0,e.target.dispatchEvent(t)}})),Ot=(A.addInitHook("addHandler","tapHold",kt),A.mergeOptions({touchZoom:b.touch,bounceAtZoomLimits:!0}),n.extend({addHooks:function(){M(this._map._container,"leaflet-touch-zoom"),S(this._map._container,"touchstart",this._onTouchStart,this)},removeHooks:function(){z(this._map._container,"leaflet-touch-zoom"),k(this._map._container,"touchstart",this._onTouchStart,this)},_onTouchStart:function(t){var e,i,n=this._map;!t.touches||2!==t.touches.length||n._animatingZoom||this._zooming||(e=n.mouseEventToContainerPoint(t.touches[0]),i=n.mouseEventToContainerPoint(t.touches[1]),this._centerPoint=n.getSize()._divideBy(2),this._startLatLng=n.containerPointToLatLng(this._centerPoint),"center"!==n.options.touchZoom&&(this._pinchStartLatLng=n.containerPointToLatLng(e.add(i)._divideBy(2))),this._startDist=e.distanceTo(i),this._startZoom=n.getZoom(),this._moved=!1,this._zooming=!0,n._stop(),S(document,"touchmove",this._onTouchMove,this),S(document,"touchend touchcancel",this._onTouchEnd,this),O(t))},_onTouchMove:function(t){if(t.touches&&2===t.touches.length&&this._zooming){var e=this._map,i=e.mouseEventToContainerPoint(t.touches[0]),n=e.mouseEventToContainerPoint(t.touches[1]),o=i.distanceTo(n)/this._startDist;if(this._zoom=e.getScaleZoom(o,this._startZoom),!e.options.bounceAtZoomLimits&&(this._zoom<e.getMinZoom()&&o<1||this._zoom>e.getMaxZoom()&&1<o)&&(this._zoom=e._limitZoom(this._zoom)),"center"===e.options.touchZoom){if(this._center=this._startLatLng,1==o)return}else{i=i._add(n)._divideBy(2)._subtract(this._centerPoint);if(1==o&&0===i.x&&0===i.y)return;this._center=e.unproject(e.project(this._pinchStartLatLng,this._zoom).subtract(i),this._zoom)}this._moved||(e._moveStart(!0,!1),this._moved=!0),r(this._animRequest);n=a(e._move,e,this._center,this._zoom,{pinch:!0,round:!1},void 0);this._animRequest=x(n,this,!0),O(t)}},_onTouchEnd:function(){this._moved&&this._zooming?(this._zooming=!1,r(this._animRequest),k(document,"touchmove",this._onTouchMove,this),k(document,"touchend touchcancel",this._onTouchEnd,this),this._map.options.zoomAnimation?this._map._animateZoom(this._center,this._map._limitZoom(this._zoom),!0,this._map.options.zoomSnap):this._map._resetView(this._center,this._map._limitZoom(this._zoom))):this._zooming=!1}})),Xi=(A.addInitHook("addHandler","touchZoom",Ot),A.BoxZoom=_t,A.DoubleClickZoom=Ct,A.Drag=Zt,A.Keyboard=St,A.ScrollWheelZoom=Et,A.TapHold=kt,A.TouchZoom=Ot,t.Bounds=f,t.Browser=b,t.CRS=ot,t.Canvas=Fi,t.Circle=vi,t.CircleMarker=gi,t.Class=et,t.Control=B,t.DivIcon=Ri,t.DivOverlay=Ai,t.DomEvent=mt,t.DomUtil=pt,t.Draggable=Xe,t.Evented=it,t.FeatureGroup=ci,t.GeoJSON=wi,t.GridLayer=Ni,t.Handler=n,t.Icon=di,t.ImageOverlay=Ei,t.LatLng=v,t.LatLngBounds=s,t.Layer=o,t.LayerGroup=ui,t.LineUtil=vt,t.Map=A,t.Marker=mi,t.Mixin=ft,t.Path=fi,t.Point=p,t.PolyUtil=gt,t.Polygon=xi,t.Polyline=yi,t.Popup=Bi,t.PosAnimation=Fe,t.Projection=wt,t.Rectangle=Yi,t.Renderer=Wi,t.SVG=Gi,t.SVGOverlay=Oi,t.TileLayer=Di,t.Tooltip=Ii,t.Transformation=at,t.Util=tt,t.VideoOverlay=ki,t.bind=a,t.bounds=_,t.canvas=Ui,t.circle=function(t,e,i){return new vi(t,e,i)},t.circleMarker=function(t,e){return new gi(t,e)},t.control=Ue,t.divIcon=function(t){return new Ri(t)},t.extend=l,t.featureGroup=function(t,e){return new ci(t,e)},t.geoJSON=Si,t.geoJson=Mt,t.gridLayer=function(t){return new Ni(t)},t.icon=function(t){return new di(t)},t.imageOverlay=function(t,e,i){return new Ei(t,e,i)},t.latLng=w,t.latLngBounds=g,t.layerGroup=function(t,e){return new ui(t,e)},t.map=function(t,e){return new A(t,e)},t.marker=function(t,e){return new mi(t,e)},t.point=m,t.polygon=function(t,e){return new xi(t,e)},t.polyline=function(t,e){return new yi(t,e)},t.popup=function(t,e){return new Bi(t,e)},t.rectangle=function(t,e){return new Yi(t,e)},t.setOptions=c,t.stamp=h,t.svg=Ki,t.svgOverlay=function(t,e,i){return new Oi(t,e,i)},t.tileLayer=ji,t.tooltip=function(t,e){return new Ii(t,e)},t.transformation=ht,t.version="1.9.4",t.videoOverlay=function(t,e,i){return new ki(t,e,i)},window.L);t.noConflict=function(){return window.L=Xi,this},window.L=t});
