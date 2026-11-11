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

  if (typeof Notifications !== 'undefined') Notifications.startPolling(30000);
})();
