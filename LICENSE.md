# MIT License

Copyright (c) 2024-2026 casapps

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

### Gin Web Framework
- **License**: MIT
- **Copyright**: 2014 Manuel Martínez-Almeida and Gin contributors
- **Website**: https://gin-gonic.com/
- **Repository**: https://github.com/gin-gonic/gin

### Gin Contrib - CORS
- **License**: MIT
- **Copyright**: 2016 Gin-Gonic contributors
- **Repository**: https://github.com/gin-contrib/cors

### Gin Contrib - Secure
- **License**: MIT
- **Copyright**: 2015 Gin-Gonic contributors
- **Repository**: https://github.com/gin-contrib/secure

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

### Cities Database
- **Source**: OpenWeatherMap City List
- **Repository**: https://github.com/casapps/citylist
- **License**: CC BY-SA 4.0
- **Records**: 209,579 cities worldwide
- **Attribution**: City data compiled from OpenWeatherMap and OpenStreetMap

### Countries Database
- **Source**: REST Countries API
- **Repository**: https://github.com/casapps/countries
- **License**: MPL 2.0 (Mozilla Public License 2.0)
- **Records**: 247 countries with timezones and metadata
- **Attribution**: Country data from various open sources

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

1. **Console Weather Service** - https://github.com/casapps/wthr
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

- **Repository**: https://github.com/casapps/wthr
- **Issues**: https://github.com/casapps/wthr/issues
- **Discussions**: https://github.com/casapps/wthr/discussions
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

### github.com/gin-gonic/gin v1.11.0

**Copyright:** 2014 Manuel Martínez-Almeida
**License:** MIT License
**Repository:** https://github.com/gin-gonic/gin

```
MIT License

Copyright (c) 2014 Manuel Martínez-Almeida

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

