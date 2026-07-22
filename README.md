# catalog-data.go

[![CI](https://github.com/sweetrpg/catalog-data.go/actions/workflows/ci.yaml/badge.svg)](https://github.com/sweetrpg/catalog-data.go/actions/workflows/ci.yaml)
[![License](https://img.shields.io/github/license/sweetrpg/catalog-data.go.svg)](https://img.shields.io/github/license/sweetrpg/catalog-data.go.svg)
[![Issues](https://img.shields.io/github/issues/sweetrpg/catalog-data.go.svg)](https://img.shields.io/github/issues/sweetrpg/catalog-data.go.svg)
[![PRs](https://img.shields.io/github/issues-pr/sweetrpg/catalog-data.go.svg)](https://img.shields.io/github/issues-pr/sweetrpg/catalog-data.go.svg)
[![Dependabot](https://badgen.net/github/dependabot/sweetrpg/catalog-data.go)](https://badgen.net/github/dependabot/sweetrpg/catalog-data.go)

Database access layer for the Catalog microservice: `Get`/`Query` functions per entity
(Contribution, License, Person, Publisher, Review, Studio, System, Volume) that translate
between `catalog-objects.go` persistence models and their API value objects. Volume is
currently the only entity with a write path (`AddVolume`); `Update`/`DeleteVolume` are
unimplemented stubs, and no other entity has `Add`/`Update`/`Delete` at all.

## Install

```bash
go get github.com/sweetrpg/catalog-data.go
```

## Documentation

Package documentation: [pkg.go.dev/github.com/sweetrpg/catalog-data.go](https://pkg.go.dev/github.com/sweetrpg/catalog-data.go).
Test coverage reports are published to [sweetrpg.github.io/catalog-data.go](https://sweetrpg.github.io/catalog-data.go)
on every merge to `develop`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow (including running the
MongoDB-backed test suite locally) and [RELEASE.md](RELEASE.md) for how versions get cut.
