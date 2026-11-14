# TODO

Issues in AI.md itself (contradictions, ambiguities). Also logged in ~/Projects/github/webappsgo/TODO.API.md.

- AI.md lines 21132, 21162, 51791, 52853 and 53260 (Tor has no enable/disable toggle) contradict lines 32732 (`tor.enabled` Toggle in admin settings), 31901 (admins can toggle Tor), 13272 (`tor.enabled` restart list) and 36256 (`tor_enabled` gauge). The project keeps the admin toggle and reports a switched-off Tor as `disabled`.
- AI.md line 31901 (`config/network/tor`) contradicts line 31385 (no `config` segment under `/server/{admin_path}/`).
- AI.md line 16925 (schema migration, PART 10 "Migrations") contradicts lines 14844, 14850 and 67221 (no migrations) and refers to a PART 10 section that does not exist.
