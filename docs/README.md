
# Atlas DNS Controller Documentation

Current documentation explains how to install, use, administer, and operate the
product. Development chronology is retained separately for traceability and is
not part of the operator documentation set.

## Get started

- [Docker Compose installation](getting-started/docker.md)
- [Portainer Stack installation](getting-started/portainer.md)
- [Debian 13 and systemd installation](getting-started/native-systemd.md)
- [Manual release archive installation](getting-started/manual-release.md)
- [Guided first-run onboarding](getting-started/onboarding.md)
- [User guide](user-guide/overview.md)
- [Administration guide](administration/administration.md)

## Operate

- [Operations runbook](operations/runbook.md)
- [Backup and restore](operations/backup-and-restore.md)
- [Backup format](operations/backup-format.md)
- [Compatibility matrix](operations/compatibility-matrix.md)
- [Upgrade and migration policy](operations/upgrade-policy.md)
- [Release 1.0 readiness record](operations/release-1.0-readiness.md)
- [Support and deprecation policy](product/support-and-deprecation-policy.md)

## Understand the product

- [Feature catalogue](reference/features.md)
- [Architecture overview](architecture/architecture.md)
- [Security guide](security/security.md)
- [Configuration model](architecture/configuration-model.md)
- [Controller API](api/controller-api.md) and [node API boundary](api/node-api.md)
- [Database schema](database/schema.md)
- [Architecture decisions](decisions/README.md)

## Develop and contribute

- [Local development](development/local-development.md)
- [Testing](development/testing.md)
- [Coding standards](development/coding-standards.md)
- [Release process](development/release-process.md)
- [Documentation inventory](reference/documentation-inventory.md)
- [Release 1.0 rename inventory](reference/rename-inventory.md)

## History and direction

- [Changelog](../CHANGELOG.md) — release chronology
- [Roadmap](roadmap/roadmap.md) — future direction only
- [Physical pre-1.0 archive](archive/pre-1.0/README.md) — implementation audits,
  release validation, and superseded planning material

Accepted ADRs remain in `docs/decisions/`; they are durable decision records,
not development diary entries.
