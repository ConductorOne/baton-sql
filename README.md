![Baton Logo](./baton-logo.png)

# `baton-sql` [![Go Reference](https://pkg.go.dev/badge/github.com/conductorone/baton-sql.svg)](https://pkg.go.dev/github.com/conductorone/baton-sql) ![main ci](https://github.com/conductorone/baton-sql/actions/workflows/main.yaml/badge.svg)

`baton-sql` is a connector for built using the [Baton SDK](https://github.com/conductorone/baton-sdk).

## Overview

`baton-sql` is a flexible connector that enables you to sync identities, resources, and permissions from SQL databases. It provides a powerful configuration system that allows you to map database queries to resources and entitlements, with full support for account provisioning and automated password management.

## Key Features

- **Multi-Database Support**: Works with MySQL, PostgreSQL, Oracle, SQL Server, SQLite, and WordPress
- **Account Provisioning**: Create user accounts with automatic random password generation
- **Secure Password Management**: Database-appropriate password hashing (SHA2, bcrypt, MD5)
- **Flexible Configuration**: Map any SQL query results to resources and entitlements
- **Role Management**: Sync and manage role assignments and permissions
- **Custom Schemas**: Support for any database schema through configurable SQL queries

## Supported Database Engines

- MySQL
- Microsoft SQL Server
- Oracle
- PostgreSQL

## Configuration

The connector is configured using a YAML file that defines:

- **Database Connection**: Connection details via DSN (Data Source Name)
- **Resource Types**: Map database tables/queries to resources (users, roles, etc.)
- **Account Provisioning**: Define schemas and credential options for user creation
- **Entitlements**: Permissions and roles that can be granted to resources
- **Provisioning Actions**: SQL queries for granting/revoking entitlements

See examples in the [examples](https://github.com/ConductorOne/baton-sql/tree/main/examples) directory.

## `baton-sql` Command Line Usage
```
Usage:
  baton-sql [flags]
  baton-sql [command]

Available Commands:
  capabilities       Get connector capabilities
  completion         Generate the autocompletion script for the specified shell
  help               Help about any command

Flags:
      --client-id string       The client ID used to authenticate with ConductorOne ($BATON_CLIENT_ID)
      --client-secret string   The client secret used to authenticate with ConductorOne ($BATON_CLIENT_SECRET)
      --config-path string     required: The file path to the baton-sql config to use ($BATON_CONFIG_PATH)
  -f, --file string            The path to the c1z file to sync with ($BATON_FILE) (default "sync.c1z")
  -h, --help                   help for baton-sql
      --log-format string      The output format for logs: json, console ($BATON_LOG_FORMAT) (default "json")
      --log-level string       The log level: debug, info, warn, error ($BATON_LOG_LEVEL) (default "info")
  -p, --provisioning           This must be set in order for provisioning actions to be enabled ($BATON_PROVISIONING)
      --skip-full-sync         This must be set to skip a full sync ($BATON_SKIP_FULL_SYNC)
      --ticketing              This must be set to enable ticketing support ($BATON_TICKETING)
  -v, --version                version for baton-sql

Use "baton-sql [command] --help" for more information about a command.
```

# Contributing, Support and Issues

We started Baton because we were tired of taking screenshots and manually
building spreadsheets. We welcome contributions, and ideas, no matter how
small&mdash;our goal is to make identity and permissions sprawl less painful for
everyone. If you have questions, problems, or ideas: Please open a GitHub Issue!

Check out [Baton](https://github.com/conductorone/baton) to learn more the project in general.

See [CONTRIBUTING.md](https://github.com/ConductorOne/baton/blob/main/CONTRIBUTING.md) for more details.
