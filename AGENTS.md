# AGENTS.md

This file provides guidance to Claude Code, Codex, GitHub Copilot, and other AI coding agents
working in this repository.

## About This Project

`catalog-data.go` is the database access layer for the Catalog microservice: `Get`/`Query`
(and, for Volume only, `Add`) functions per entity (Contribution, License, Person, Publisher,
Review, Studio, System, Volume) that translate between `catalog-objects.go` persistence models
and their API value objects.

## Known gaps

- `UpdateVolume`/`DeleteVolume` are `// TODO` stubs.
- Every entity except Volume has no write path at all (`Add`/`Update`/`Delete` don't exist).

Don't assume these exist when implementing consumers - check before wiring up write endpoints.

## Dependencies

Depends on `api-core.go` (query param parsing/tracing), `catalog-objects.go` (models/VOs),
`common.go` (logging), `model-core.go` (Property/Tag conversion), and `mongodb.go` (database
access). Depended on by `catalog-api`.

## Committing Code

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>
```

## Branches and Workflow

* `develop` - integration branch, default branch, target for all PRs.
* `master` - latest released state, nothing committed directly.
* `feature/*`, `fix/*` branched from `develop`; `hotfix/*` branched from `master`.

See `CONTRIBUTING.md` for the full workflow, including running the database-backed test suite
locally.

## Running Checks Locally

```bash
docker run --rm -d -p 27017:27017 --name mongodb-test mongo:7.0
export TEST_DB_URI="mongodb://localhost:27017/unit-tests"
export TEST_COLLECTION=unit-tests
go build -v ./...
go vet ./...
go test -v -coverprofile coverage.out ./...
docker stop mongodb-test
```

## Releases

Merges to `develop` auto-tag a patch release via CI (`.github/workflows/go-ci.yml`). Use the
"Bump version" workflow (`.github/workflows/bump-version.yml`, manually dispatched) for a minor
or major bump instead.
