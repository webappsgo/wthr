# TODO

Issues in AI.md itself (contradictions, ambiguities). Also logged in ~/Projects/github/webappsgo/TODO.SERVER.md.

- AI.md line 31715 (admin login page is `/server/{admin_path}` when not logged in) contradicts lines 31226 and 31740 (unauthenticated admin requests redirect to the shared `/server/auth/login`). The project follows the redirect.
- AI.md line 21681 (`/server/{admin_path}/*` is HTML for every client) contradicts line 21694 (frontend `Accept: text/plain` returns formatted text) for an explicit `Accept: text/plain` on an admin page. The project keeps admin pages HTML.
- AI.md line 31350 (`validAdminRootPaths` is a closed set and no other direct child is allowed) leaves no home for project-specific admin pages (here weather settings, admin search, sidebar state). The project files them under `settings/`.
- AI.md line 24030 (`reservedNames`, digit-suffix rule only for privileged names) and line 59540 (`UsernameBlocklist`, suffix and substring rules) define two overlapping reserved lists with different matching; it is unclear which wins for a regular username. The project applies both.
- AI.md line 8176 (`srv_`-prefixed tables, no config table in single-instance mode) and line 62548 (`usr_admins` listed as a user table) disagree with line 8189 (`admins` / `usr_admins`) and with the separate `admins` table of PART 17; table naming outside Cluster Mode is not stated.
- AI.md line 22261 (never manually edit the GraphQL schema, generate it from code) and line 22276 (`src/graphql/schema.go`) cannot be met by a schema-first generator such as gqlgen (`schema.graphqls`).
