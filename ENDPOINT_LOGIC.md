# Endpoint Logic Manual

This document summarizes how each API endpoint resolves inputs, which internal identifiers it uses, and which database tables it queries.

## Conventions

- `PZN` routes usually resolve input PZNs to `Key_FAM` or `Key_STO`.
- Compound routes usually resolve names through `SNA_DB.Name`.
- Compound interaction/QT/Priscus compound routes use `common.StoToCompoundsMap(...)`:
  - `SNA_DB.Name` is matched case-insensitively.
  - If `VSS_DB.Typ = 100`, the effective active-substance key is `Key_STO_1`.
  - Otherwise the effective key is `SNA_DB.Key_STO`.
- `common.FamToPZN(...)` maps `PAE_DB.PZN -> Key_FAM`.

## System

### `GET /sys/ping`

- No database access.
- Returns a static `pong`.

### `GET /sys/info`

- No database access.
- Returns config metadata and API limits from in-memory config.

## User / Auth

These routes use GORM, not hand-written SQL. The tables are:

- `users`
- `user_pwd_resets`
- `user_email_changes`

### `POST /user/login`

- Resolves `login` by `users.email`.
- Verifies password hash from `users.pwd_hash`.
- Updates `users.last_login`.
- No joins unless model relations are explicitly loaded.

### `POST /user/refresh-token`

- Validates JWT refresh token first.
- Resolves user by `users.id`.
- Updates `users.last_login`.

### `POST /user/password/reset`

- Resolves user by `users.email` with `PwdReset` relation.
- Writes or updates `user_pwd_resets`.

### `POST /user/password/init`
### `POST /user/password/reset/confirm`

- Resolves user by `users.email` with `PwdReset`.
- Validates reset token hash from `user_pwd_resets`.
- Updates `users.pwd_hash`.
- Deletes `user_pwd_resets` row.

### `PATCH /user/password`

- Resolves authenticated user by `users.id`.
- Verifies current password hash.
- Updates `users.pwd_hash`.

### `PATCH /user/email`

- Resolves authenticated user by `users.id` with `EmailChange`.
- Checks email availability against both `users.email` and `user_email_changes.new_email`.
- Writes or updates `user_email_changes`.

### `POST /user/email/confirm`

- Resolves authenticated user by `users.id` with `EmailChange`.
- Validates token hash from `user_email_changes`.
- Copies `user_email_changes.new_email` into `users.email`.
- Deletes `user_email_changes` row.

### `GET /user/profile`

- Resolves authenticated user by `users.id`.

### `PATCH /user/profile`

- Resolves authenticated user by `users.id`.
- Updates `users.first_name`, `users.last_name`, `users.org`.

### `DELETE /user`

- Resolves authenticated user by `users.id`.
- If admin, checks active admin count in `users`.
- Soft deletes the user row.

## Admin

These also use the same GORM user tables.

### `POST /admin/users/service`

- Validates payload.
- Checks email availability in `users` and `user_email_changes`.
- Inserts active user directly into `users` with password hash.

### `POST /admin/users`

- Checks email availability in `users` and `user_email_changes`.
- Inserts user into `users`.
- Creates initial `user_pwd_resets` token row.

### `GET /admin/users`

- Reads `users`, optionally filtered by `role` and/or `status`.

### `GET /admin/users/:email`

- Resolves a single user by `users.email`.

### `DELETE /admin/users/:email`

- Resolves the target user by `users.email`.
- Rejects deleting the currently authenticated admin account.
- Soft deletes the user row.

### `PATCH /admin/users/:email`

- Resolves the target user by `users.email`.
- Applies requested `role` and/or `status` updates.
- Rejects empty updates.
- Protects against deactivating/changing the last active admin in unsafe ways.

## Formulations

### `GET /formulations`

- Direct lookup only.
- Tables:
  - `DAR_DB`
- Query:
  - `SELECT Key_DAR, Name FROM DAR_DB ORDER BY Key_DAR`

## Compounds

### `GET /compounds/names`

- Input: comma-separated compound names.
- Logic:
  - Exact name match against `SNA_DB b.Name`.
  - Expand to all synonyms sharing the same `Key_STO` via `SNA_DB a`.
- Tables:
  - `SNA_DB a`
  - `SNA_DB b`
- Join path:
  - `SNA_DB a JOIN SNA_DB b ON a.Key_STO = b.Key_STO`

### `GET /compounds/guidelines`

- First runs the same compound expansion as `/compounds/names`.
- Extracts all matched compound names.
- Looks up guideline rows by lowercased drug name.
- Tables:
  - `SNA_DB`
  - `guideline_table`

## Product

### `GET /product/list`

- Input: `page`.
- Logic:
  - Counts distinct marketable PZNs.
  - Reads page of PZNs.
  - Builds product rows and active-compound list.
- Tables:
  - `PAE_DB`
  - `FAM_DB`
  - `FAI_DB`
  - `VSS_DB`
  - `SNA_DB`
- Key joins:
  - `FAM_DB -> PAE_DB` by `Key_FAM`
  - `FAM_DB -> FAI_DB` by `Key_FAM`
  - `FAI_DB -> VSS_DB` by `Key_STO`
  - `FAI_DB -> SNA_DB` by `Key_STO`

### `GET /product/activecompounds/pzns`

- Input PZNs are validated first.
- Logic:
  - Resolves `PZN -> Key_FAM` in `PAE_DB`
  - Reads active substances in `FAI_DB`
  - Enriches names/synonyms from `SNA_DB`
  - Uses `VSS_DB` to mark derivatives
- Tables:
  - `PAE_DB`
  - `FAI_DB`
  - `VSS_DB`
  - `SNA_DB`

### `GET /product/info/pzns`

- Input PZNs are validated first.
- Logic:
  - Resolves product family and product group.
- Tables:
  - `PAE_DB`
  - `FAM_DB`

## QT

### `GET /qt/pzns`

- Input PZNs are validated first.
- Logic:
  - Resolves `PZN -> Key_FAM -> FAI_DB.Key_STO`
  - Reads QT category from `SZG_DB.Key_SGR`
  - Relevant QT codes:
    - `10079780` known risk
    - `10079781` possible risk
    - `10079782` conditional risk
- Tables:
  - `FAI_DB`
  - `SZG_DB`
  - `PAE_DB`
- Join path:
  - `FAI_DB LEFT JOIN SZG_DB ON FAI_DB.Key_STO = SZG_DB.Key_STO`
  - `FAI_DB LEFT JOIN PAE_DB ON PAE_DB.Key_FAM = FAI_DB.Key_FAM`

### `GET /qt/compounds`

- Input: `compounds=...`
- Logic:
  - Resolves compound names with `StoToCompoundsMap`.
  - Uses effective active-substance `Key_STO`.
  - Reads QT category directly from `SZG_DB`.
- Tables:
  - `SNA_DB`
  - `VSS_DB`
  - `SZG_DB`
- Internal flow:
  - input compounds -> effective `Key_STO` -> QT category lookup in `SZG_DB`

## Priscus

### `GET /priscus/pzns`

- Input PZNs are validated first.
- Logic:
  - Resolves `PZN -> Key_FAM -> FAI_DB.Key_STO`
  - Marks a product as Priscus if `SZG_DB.Key_SGR = 10084520`
- Tables:
  - `FAI_DB`
  - `SZG_DB`
  - `PAE_DB`

### `GET /priscus/compounds`

- Input: `compounds=...`
- Logic:
  - Resolves compounds with `StoToCompoundsMap`.
  - Uses effective active-substance `Key_STO`.
  - Marks the compound as Priscus if `SZG_DB.Key_SGR = 10084520`.
- Tables:
  - `SNA_DB`
  - `VSS_DB`
  - `SZG_DB`

## ADRs

### `GET /adrs/pzns`

- Input PZNs are validated first.
- Logic:
  - Resolves `PZN -> Key_FAM` with `FamToPZN`.
  - Reads ADR rows from `NEB_C`.
  - Joins ADR text from `MIN_C`.
  - Language handling:
    - `english` uses `Sprache = 2`
    - `german` / `german-simple` use `Sprache = 1`
    - `german-simple` also requires `Vorzugsbezeichnung_L = 1`
- Tables:
  - `PAE_DB`
  - `NEB_C`
  - `MIN_C`

### `GET /adrs/compounds`

- Input: `compound=...` search term.
- Logic:
  - Matches `SNA_DB.Name` with a case-insensitive partial search.
  - Restricts to active ingredients with `FAI_DB.Stofftyp = 1`.
  - Resolves matching families and formulations through `FAI_DB`, `FAM_DB`, `DAR_DB`, and `FAP_DB`.
  - Reads ADR rows from `NEB_C`.
  - Joins ADR text from `MIN_C`.
  - Supports `application` filter values:
    - `extern`
    - `invasive`
    - `peroral`
    - `all`
  - Default `application` is `peroral`.
  - Returns the first matching formulation per application bucket within each input.
  - For `application=all`, returns at most one item each for `extern`, `invasive`, and `peroral`.
  - Response items now expose only:
    - `application`
    - `compound_name`
    - `adrs`
  - Language handling matches `/adrs/pzns`:
    - `english` uses `Sprache = 2`
    - `german` / `german-simple` use `Sprache = 1`
    - `german-simple` also requires `Vorzugsbezeichnung_L = 1`
- Tables:
  - `SNA_DB`
  - `FAI_DB`
  - `FAM_DB`
  - `DAR_DB`
  - `FAP_DB`
  - `NEB_C`
  - `MIN_C`

## Interactions

### `GET /interactions/description`

- No database access.
- Returns static translator metadata for plausibility, relevance, frequency, credibility, direction.

### `GET /interactions/compounds`
### `POST /interactions/compounds`

- Input compounds are validated first.
- Logic:
  - Resolves compounds with `StoToCompoundsMap`.
  - Uses effective active-substance `Key_STO`.
  - Queries interaction graph in `SZI_C` and metadata in `INT_C`.
  - Maps left/right STO keys back to input compound names.
  - Optional:
    - `doses=true` fetches dose/formulation details from `FZI_C`, `FAI_DB`, `FAM_DB`
    - `text=true` fetches text blocks from `ITX_C` via `INT_C.Textverweis`
- Core tables:
  - `SNA_DB`
  - `VSS_DB`
  - `SZI_C`
  - `INT_C`
- Optional tables:
  - `ITX_C`
  - `FZI_C`
  - `FAI_DB`
  - `FAM_DB`

### `GET /interactions/pzns`
### `POST /interactions/pzns`

- Input PZNs are validated first.
- Logic:
  - Resolves `PZN -> Key_FAM` with `FamToPZN`.
  - Queries family-based interaction graph in `FZI_C`.
  - Joins `SZI_C` to preserve left/right STO localization.
  - Joins `INT_C` for metadata.
  - Maps `Key_FAM` buckets back to input PZNs.
  - Optional:
    - `text=true` fetches text blocks from `ITX_C` via `INT_C.Textverweis`
- Tables:
  - `PAE_DB`
  - `FZI_C`
  - `SZI_C`
  - `INT_C`
  - `ITX_C` when text is requested

## Swagger / Root

### `GET /`

- No database access.
- Redirects to Swagger UI.

### `GET /swagger/*`

- No database access.
- Serves generated OpenAPI docs.
