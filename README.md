[![CI](https://github.com/nuntiiscore/envmerge/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/nuntiiscore/envmerge/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nuntiiscore/envmerge)](https://goreportcard.com/report/github.com/nuntiiscore/envmerge)
[![Coverage](https://codecov.io/gh/nuntiiscore/envmerge/branch/main/graph/badge.svg)](https://codecov.io/gh/nuntiiscore/envmerge)

# envmerge

`envmerge` is a small Go CLI tool that synchronizes `.env` files by appending missing
(or, in `--force` mode, changed) variables from `.env.example`.

The tool is designed to be:

* deterministic
* git-diff friendly
* strict (no shell emulation)

---

## ✨ Features

* ✅ Appends **missing variables** from `.env.example` to `.env`
* 🔁 `--force` mode appends **updates** for keys whose values differ
* 🧾 Append-only: never rewrites your `.env`
* 📐 Deterministic output (sorted keys)
* 🧵 Supports multiline values inside double quotes
* 🚫 No variable expansion, no shell emulation

---

## ✅ Run (versioned, recommended)

Use an explicit version tag for reproducible runs:

```bash
go run github.com/nuntiiscore/envmerge/cmd@v0.1.2 -- --help
```

Basic sync (append only missing keys):

```bash
go run github.com/nuntiiscore/envmerge/cmd@v0.1.2 --
```

Force mode (append updates for differing keys and missing keys):

```bash
go run github.com/nuntiiscore/envmerge/cmd@v0.1.2 -- --force
```

Custom paths:

```bash
go run github.com/nuntiiscore/envmerge/cmd@v0.1.2 -- --src ./configs/.env.example --dst ./configs/.env
```

> Note: arguments must be passed **after `--`** when using `go run <module>@<version>`.

---

## 📦 Install

Install a specific version:

```bash
go install github.com/nuntiiscore/envmerge/cmd@v0.1.2
```

Run:

```bash
envmerge
```

---

## ⚙️ Flags

* `--src` (default: `.env.example`) — source template file
* `--dst` (default: `.env`) — destination env file
* `--force` — append updates for existing keys when values differ

---

## 🧠 Supported `.env` format

Single-line values:

```env
PORT=8080
DATABASE_URL=postgres://user:pass@localhost/db
```

Quoted values:

```env
GREETING="Hello world"
```

Multiline values (supported only inside double quotes):

```env
PRIVATE_KEY="-----BEGIN KEY-----
line1
line2
-----END KEY-----"
```

---

## 🚫 Not supported (by design)

`envmerge` intentionally does not support:

* `export KEY=value`
* `${VAR}` expansion
* shell escaping semantics
* heredoc (`<<EOF`)

The goal is predictability and environment-agnostic behavior.

---

## 📜 License

Licensed under the Apache License, Version 2.0.

See the [LICENSE](LICENSE) file for details.
