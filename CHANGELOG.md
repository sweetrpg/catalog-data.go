# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added

- CONTRIBUTING.md, CODE_OF_CONDUCT.md, AGENTS.md/CLAUDE.md repo scaffolding.
- MongoDB service in the PR workflow so `go test` can actually run against a live database on
  PRs (it previously only ran on push to `develop`).

### Fixed

- **CI has been red on develop for at least the last 5 runs.** Root cause: `AddVolume` never
  set `models.Volume.ID` before insert, so MongoDB stored a literal empty-string `_id` instead
  of a generated one. `database.Insert`'s type-asserted return then silently produced the
  all-zero ObjectID, which the test suite had (incorrectly) hardcoded as the expected ID.
  `AddVolume` now generates and assigns a proper ObjectID hex string before inserting, and
  returns that value directly instead of trusting `database.Insert`'s return (which can't
  recognize a non-`primitive.ObjectID` `_id` type).
- **N+1 query bug across every entity's `QueryX` function** (Contribution, License, Person,
  Publisher, Review, Studio, System, Volume): each queried a page of documents, discarded them,
  then re-fetched every item individually by ID to build its VO - doubling database round
  trips on every list/search request. Each `GetX` now delegates to a shared `xModelToVO`
  converter that `QueryX` calls directly on the already-fetched models.
- Rewrote `volume_test.go` to seed its own fixture per test instead of depending on
  execution-order side effects and a hardcoded ID that only "worked" because of the bug above.
- Bumped `golang.org/x/crypto`, `golang.org/x/net`, `go.opentelemetry.io/otel*`, and
  `google.golang.org/grpc`, resolving 18 open Dependabot alerts (10 critical, 3 high).
- Dropped the PR workflow's `golint` step (unconditionally broken - pulls a transitive dep
  requiring Go >=1.25).

### Known gaps (not addressed in this pass)

- `UpdateVolume` and `DeleteVolume` are unimplemented stubs (`// TODO`).
- Contribution, License, Person, Publisher, Review, Studio, and System have no
  Add/Update/Delete functions at all - read-only today. Implementing full CRUD for these is a
  feature-development task, out of scope for this scaffolding/hardening pass.
