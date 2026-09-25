# MIT License

Copyright (c) 2024-2026 webappsgo

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

---

# Third-Party Licenses and Attributions

## Go Dependencies

This software includes the following third-party Go libraries:

| Library | Version | License | Copyright |
|---------|---------|---------|-----------|
| github.com/99designs/gqlgen | v0.17.60 | MIT | 2016 Adam Scarr |
| github.com/charmbracelet/bubbletea | v1.3.10 | MIT | 2020-2023 Charmbracelet, Inc. |
| github.com/charmbracelet/lipgloss | v1.1.0 | MIT | 2021-2023 Charmbracelet, Inc. |
| github.com/coreos/go-oidc/v3 | v3.11.0 | Apache-2.0 | 2014 CoreOS, Inc. |
| github.com/cretz/bine | v0.2.0 | MIT | 2018 Chad Retz |
| github.com/fsnotify/fsnotify | v1.9.0 | BSD-3-Clause | 2012 The Go Authors, 2012-2019 fsnotify Authors |
| github.com/go-acme/lego/v4 | v4.34.0 | MIT | 2015-2017 Sebastian Erhart |
| github.com/go-chi/chi/v5 | v5.3.2 | MIT | 2015-present Peter Kieltyka, Google Inc. |
| github.com/go-chi/httprate | v0.15.0 | MIT | 2020-present Peter Kieltyka |
| github.com/go-ldap/ldap/v3 | v3.4.13 | MIT | 2011-2015 Michael Mitton and contributors |
| github.com/go-playground/validator/v10 | v10.23.0 | MIT | 2015 Dean Karn |
| github.com/go-sql-driver/mysql | v1.9.3 | MPL-2.0 | 2012 The Go-MySQL-Driver Authors |
| github.com/go-webauthn/webauthn | v0.11.2 | BSD-3-Clause | 2019 Duo Labs |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause | 2009,2014 Google Inc. |
| github.com/gorilla/websocket | v1.5.3 | BSD-3-Clause | 2023 The Gorilla Authors |
| github.com/jackc/pgx/v5 | v5.9.2 | MIT | 2013-2021 Jack Christensen |
| github.com/microsoft/go-mssqldb | v1.9.3 | BSD-3-Clause | 2012 The Go Authors, 2021 Microsoft Corporation |
| github.com/oklog/ulid/v2 | v2.1.0 | Apache-2.0 | 2016 The Oklog Authors |
| github.com/oschwald/maxminddb-golang | v1.13.1 | ISC | 2015 Gregory J. Oschwald |
| github.com/patrickmn/go-cache | v2.1.0 | MIT | 2012-2019 Patrick Mylund Nielsen and the go-cache contributors |
| github.com/pquerna/otp | v1.5.0 | Apache-2.0 | 2014 Paul Querna |
| github.com/prometheus/client_golang | v1.22.0 | Apache-2.0 | 2012-2015 The Prometheus Authors |
| github.com/prometheus/client_model | v0.6.2 | Apache-2.0 | 2012-2015 The Prometheus Authors |
| github.com/redis/go-redis/v9 | v9.14.0 | BSD-2-Clause | 2013 The github.com/redis/go-redis Authors |
| github.com/rs/cors | v1.11.1 | MIT | 2014 Olivier Poitrey |
| github.com/swaggo/http-swagger/v2 | v2.0.2 | MIT | 2017 Swaggo |
| github.com/vektah/gqlparser/v2 | v2.5.22 | MIT | 2018 Adam Scarr |
| golang.org/x/crypto | v0.55.0 | BSD-3-Clause | 2009 The Go Authors |
| golang.org/x/net | v0.57.0 | BSD-3-Clause | 2009 The Go Authors |
| golang.org/x/oauth2 | v0.36.0 | BSD-3-Clause | 2009 The Go Authors |
| golang.org/x/sys | v0.47.0 | BSD-3-Clause | 2009 The Go Authors |
| golang.org/x/term | v0.45.0 | BSD-3-Clause | 2009 The Go Authors |
| golang.org/x/text | v0.41.0 | BSD-3-Clause | 2009 The Go Authors |
| gopkg.in/yaml.v3 | v3.0.1 | MIT and Apache-2.0 | 2006-2011 Kirill Simonov, 2011-2019 Canonical Ltd. |
| modernc.org/sqlite | v1.39.0 | BSD-3-Clause | 2017 The Sqlite Authors |

Full license texts: https://spdx.org/licenses/

### BSD-3-Clause non-endorsement clause

The following libraries are BSD-3-Clause licensed: `github.com/fsnotify/fsnotify`,
`github.com/go-webauthn/webauthn`, `github.com/google/uuid`,
`github.com/gorilla/websocket`, `github.com/microsoft/go-mssqldb`,
`golang.org/x/crypto`, `golang.org/x/net`, `golang.org/x/oauth2`,
`golang.org/x/sys`, `golang.org/x/term`, `golang.org/x/text`, and
`modernc.org/sqlite`. For each of them:

Neither the name of the copyright holder nor the names of its contributors
may be used to endorse or promote products derived from this software
without specific prior written permission.

Full license: https://spdx.org/licenses/BSD-3-Clause.html

### MPL-2.0 notice

`github.com/go-sql-driver/mysql` is licensed under the Mozilla Public License
2.0. The source form of that library is available from its repository, and the
full license text is available at https://mozilla.org/MPL/2.0/.

## Data Sources

### Open-Meteo Weather API
- **License**: CC BY 4.0 (Creative Commons Attribution 4.0 International)
- **Data Provider**: Open-Meteo.com
- **Website**: https://open-meteo.com/
- **Terms**: https://open-meteo.com/en/terms
- **Attribution**: Weather data provided by Open-Meteo.com
- **Usage**: Free for non-commercial and commercial use with attribution

### OpenStreetMap Nominatim (Reverse Geocoding)
- **License**: ODbL (Open Database License)
- **Copyright**: OpenStreetMap contributors
- **Website**: https://nominatim.openstreetmap.org/
- **Terms**: https://operations.osmfoundation.org/policies/nominatim/
- **Attribution**: © OpenStreetMap contributors
- **Usage**: Free with attribution and fair use policy

### Cities and Countries Database (GeoNames)
- **Source**: GeoNames.org (`cities15000.txt`, `countryInfo.txt`, `admin1CodesASCII.txt`)
- **Website**: https://www.geonames.org/
- **License**: CC BY 4.0 (Creative Commons Attribution 4.0 International)
- **Terms**: https://www.geonames.org/export/
- **Records**: 34,133 cities (population >= 15,000) and 252 countries with timezone and admin1/state metadata
- **Attribution**: Geographical data by GeoNames, licensed under CC BY 4.0
- **Usage**: Vendored as static JSON (`src/server/service/data/countries.json`,
  `src/server/service/data/cities.json`) and embedded into the binary via
  `go:embed` (`src/server/service/location_enhancer.go`) — a one-time
  transform of the raw GeoNames source files, not a live API dependency

### IP Geolocation Databases (sapics/ip-location-db)
- **License**: CC BY 4.0 (Creative Commons Attribution 4.0 International)
- **ASN database**: RouteViews, NRO, DB-IP (merged)
- **Country database**: NRO (RIR whois + geofeed + ASN data, merged)
- **City database**: DB-IP
- **Attribution**: [IP Geolocation by DB-IP](https://db-ip.com/)
- **Attribution**: Country and ASN data licensed CC BY 4.0 by the Number Resource Organization (NRO).
- **Usage**: Downloaded on first run to `{data_dir}/security/geoip` and refreshed weekly by the built-in scheduler — never embedded in the binary

## Design and Theme

### Dracula Theme
- **License**: MIT
- **Copyright**: 2016 Dracula Theme contributors
- **Website**: https://draculatheme.com/
- **Repository**: https://github.com/dracula/dracula-theme
- **Colors**: Color palette and design inspiration from Dracula Theme

## Inspiration

### wttr.in
- **License**: Apache-2.0
- **Author**: Igor Chubin
- **Website**: https://wttr.in
- **Repository**: https://github.com/chubin/wttr.in
- **Inspiration**: Format parameter design and ASCII art concepts

## Fonts and Typography

### System Monospace Fonts
The service uses system monospace fonts for terminal display:
- **Monaco** (macOS) - Apple Inc.
- **Menlo** (macOS) - Apple Inc.
- **Ubuntu Mono** (Linux) - Canonical Ltd., Ubuntu Font Licence
- **Consolas** (Windows) - Microsoft Corporation
- **Courier New** (fallback) - Various

## Container and Deployment

### Docker
- **License**: Apache-2.0
- **Copyright**: 2013-2024 Docker, Inc.
- **Website**: https://www.docker.com/

### Alpine Linux (Docker Base Image)
- **License**: Various open source licenses
- **Website**: https://alpinelinux.org/
- **Usage**: Minimal container base image

## Build Tools

### GitHub Actions
- **License**: MIT
- **Copyright**: 2019-2024 GitHub, Inc.
- **Website**: https://github.com/features/actions
- **Usage**: CI/CD automation for multi-platform builds

## Attribution Requirements

When using this software, please provide attribution to:

1. **Console Weather Service** - https://github.com/webappsgo/wthr
2. **Open-Meteo** - Weather data provided by Open-Meteo.com
3. **OpenStreetMap** - Geocoding data © OpenStreetMap contributors

### Example Attribution

```
Weather data provided by Open-Meteo.com
Geocoding data © OpenStreetMap contributors
Powered by Console Weather Service
```

## Data Usage and Privacy

### Weather Data
- Weather data is fetched from Open-Meteo.com in real-time
- No personal data is collected or stored
- Cached for 10 minutes to reduce API calls

### Location Data
- IP-based location detection uses HTTP headers
- No IP addresses are logged or stored
- GPS coordinates are processed server-side only
- No tracking or analytics

### Cookies and Storage
- No cookies are set by this service
- No user data is stored
- No third-party tracking scripts

## Disclaimer

THIS SOFTWARE AND RELATED DATA ARE PROVIDED "AS IS" WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, AND NONINFRINGEMENT.

Weather data accuracy is not guaranteed and should not be used for critical decision-making or emergency planning. Always consult official weather services for important weather information.

Location data accuracy may vary depending on the data source and should not be used for navigation or precise positioning.

## Contributing

By contributing to this project, you agree that:
- Your contributions will be licensed under the same MIT License
- You have the right to submit the contributions
- Your contributions are your original work or properly attributed

## Open Source Commitment

This project is committed to:
- 🔓 Open source development
- 🆓 Free access to weather information
- 🌍 Global accessibility
- 🤝 Community contributions
- 📖 Transparent operations

## Acknowledgments

Special thanks to:
- The Go programming language community
- Open-Meteo for providing free weather data
- OpenStreetMap contributors for geocoding data
- wttr.in for inspiration and format design
- Dracula Theme for the beautiful color scheme
- All contributors who helped improve this project

## Contact and Support

- **Repository**: https://github.com/webappsgo/wthr
- **Issues**: https://github.com/webappsgo/wthr/issues
- **Discussions**: https://github.com/webappsgo/wthr/discussions
- **Live Demo**: http://wthr.top

## Updates

- **Last Updated**: 2024
- **License Version**: 1.0
- **Go Version**: 2.0.0

---

For the full list of dependencies and their licenses, see `go.mod` and run:
```bash
go list -m -json all
```

---

## Embedded Third-Party Licenses

This software includes the following open-source libraries. Full license texts are provided below as required by their respective licenses.

---

### modernc.org/sqlite v1.39.0

**Copyright:** 2017 The Sqlite Authors
**License:** BSD-3-Clause
**Repository:** https://modernc.org/sqlite

```
BSD 3-Clause License

Copyright (c) 2017 The Sqlite Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

---

### gopkg.in/yaml.v3 v3.0.1

**Copyright:** 2006-2011 Kirill Simonov
**License:** MIT License & Apache-2.0
**Repository:** https://github.com/go-yaml/yaml

```
This project is covered by two different licenses: MIT and Apache.

MIT License:

Copyright (c) 2011-2019 Canonical Ltd

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

### golang.org/x/crypto

**Copyright:** 2009 The Go Authors
**License:** BSD-3-Clause
**Repository:** https://go.googlesource.com/crypto

```
Copyright (c) 2009 The Go Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

---

### react + react-dom v18.2.0

**Copyright:** Meta Platforms, Inc. and affiliates
**License:** MIT License
**Repository:** https://github.com/facebook/react
**Usage:** Vendored (not npm-installed) into `src/graphql/static/vendor/` and
embedded via `go:embed` to render the local GraphQL Playground UI
(`src/graphql/playground.go`) without loading a script from a CDN.

```
MIT License

Copyright (c) Meta Platforms, Inc. and affiliates.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

### graphiql v3.7.0

**Copyright:** GraphQL Contributors
**License:** MIT License
**Repository:** https://github.com/graphql/graphiql
**Usage:** Vendored (not npm-installed) into `src/graphql/static/vendor/` and
embedded via `go:embed` to render the local GraphQL Playground UI
(`src/graphql/playground.go`) without loading a script from a CDN.

```
MIT License

Copyright (c) GraphQL Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

**Note:** This is a representative sample of embedded licenses. For a complete list of all dependencies and their licenses, run:

```bash
go list -m all
```

All dependencies in go.mod are licensed under permissive open-source licenses (MIT, BSD, Apache-2.0, ISC) compatible with this project's MIT license. No GPL/AGPL/LGPL dependencies are used.
