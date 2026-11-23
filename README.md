<p align="center">
  <img src="assets/logo.svg" alt="pgtwig logo" width="110" height="110" />
</p>

<h1 align="center">pgtwig</h1>

<p align="center">
  Give every Git branch its own local PostgreSQL database — automatically.
</p>

<p align="center">
  <a href="https://github.com/ayyshim/pgtwig/releases"><img src="https://img.shields.io/github/v/release/ayyshim/pgtwig?style=flat" alt="Latest Release" /></a>
  <a href="https://github.com/ayyshim/pgtwig/actions/workflows/release.yml"><img src="https://github.com/ayyshim/pgtwig/actions/workflows/release.yml/badge.svg" alt="Release Workflow" /></a>
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go 1.22+" />
  <img src="https://img.shields.io/badge/PostgreSQL-12+-4169E1?style=flat&logo=postgresql&logoColor=white" alt="PostgreSQL" />
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-64748B?style=flat" alt="macOS | Linux | Windows" />
  <img src="https://img.shields.io/badge/dependencies-0-22C55E?style=flat" alt="Zero Dependencies" />
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat" alt="MIT License" /></a>
</p>

---

Your app keeps using the exact same database name and `DATABASE_URL`. Whenever you switch Git branches, `pgtwig` automatically swaps the database behind the scenes.

- Works on **macOS, Linux, and Windows**.
- One small binary with **no external dependencies**.
- Works with **local PostgreSQL** or **Docker**.
- Never blocks `git checkout` if Postgres or Docker is turned off.

---

## How It Works

Your active Git branch always uses `<db_name>`. Other branches are saved in Postgres as `"<db_name>-<branch_hash>"`.

When you run `git checkout <other-branch>`:

1. `pgtwig` renames `<db_name>` to save the branch you are leaving.
2. If the branch you are switching to already has a saved database, `pgtwig` renames it back to `<db_name>` (instant).
3. If it is a new branch, `pgtwig` copies a database for it:
   - If the new branch was created from the branch you just left, it copies from that branch.
   - Otherwise, it copies from your default branch (like `main`).

---

## Install

### Download a prebuilt binary

Grab the latest archive for your OS and CPU from the [GitHub Releases](https://github.com/ayyshim/pgtwig/releases) page (`darwin`, `linux`, or `windows` for `amd64` / `arm64`), unpack it, and place `pgtwig` (or `pgtwig.exe`) somewhere in your `PATH` (for example `~/.local/bin`).

### Or build from source

You need Go 1.22+ and `psql` (either installed locally or inside your Postgres Docker container).

#### macOS / Linux

```sh
go build -o ~/.local/bin/pgtwig ./cmd/pgtwig
```

Make sure `~/.local/bin` is in your `PATH`.

#### Windows

```powershell
go build -o "$HOME\.local\bin\pgtwig.exe" .\cmd\pgtwig
```

Add `%USERPROFILE%\.local\bin` to your `PATH` so Git Bash can find `pgtwig`.

---

## Quick Start

Run `pgtwig init` inside your Git project:

```sh
# Local Postgres
pgtwig init --db myapp_dev

# Docker Postgres
pgtwig init --db myapp_dev --container my-postgres
```

By default, `pgtwig` uses your current branch as the default branch (you can override it with `--default-branch main`).

Now just use Git as usual:

```sh
git checkout -b feature/billing   # saves main's DB, makes a fresh copy for feature/billing
# run migrations or test changes...
git checkout main                 # saves feature/billing, brings back main's DB right away
```

---

## Commands

| Command | What it does |
| --- | --- |
| `pgtwig init --db <name>` | Sets up `pgtwig` in `.git/config` and adds the `post-checkout` hook. Optional flags: `--default-branch`, `--container`, `--user`. |
| `pgtwig list` | Shows your active database and every local Git branch with its database status. |
| `pgtwig reset [--force]` | Deletes the current branch's database and makes a fresh copy from the default branch. Does nothing if you are already on the default branch. |
| `pgtwig clean [--force]` | Finds and deletes saved databases for Git branches that no longer exist. |

Run `pgtwig <command> --help` to see all options for any command.

---

## Passwords & Docker

- **Local Postgres**: Uses your normal `~/.pgpass`, `PGHOST`, `PGPORT`, and `PGUSER` settings.
- **Docker**: Connects inside the container using `docker exec -i` without `-h`, so standard Postgres Docker images usually need no password.
- **If a password is required**: Set `PGTWIG_PGPASSWORD` in your shell. `pgtwig` passes it to Postgres without saving it in `.git/config`.

```sh
export PGTWIG_PGPASSWORD="your-password"
```

---

## How `pgtwig` Compares to Other Tools

| Tool | Where it runs | How it works | Speed & disk use | Data diff / merge | Good for teams / CI? | Setup effort |
| --- | --- | --- | --- | --- | --- | --- |
| **`pgtwig`** | Local machine (Postgres or Docker) | Renames and copies Postgres databases on `git checkout` | Instant when switching back; full copy on a brand-new branch | No | No (local only) | Very easy |
| **Neon** | Cloud | Instant cloud database branching | Instant, uses almost no extra disk | Schema diff | Yes (great for PR previews & CI) | Medium (cloud account) |
| **Supabase Branching** | Cloud | Spins up a preview Postgres per Git branch | Takes a few minutes per branch | No | Yes (GitHub PR previews) | Medium (Supabase + GitHub) |
| **DBLab / ZFS / Btrfs** | Local Linux or self-hosted server | Filesystem-level snapshots | Instant even for huge databases (100 GB+) | No | Optional | Hard (special filesystem setup) |
| **Dolt / DoltgreSQL** | Local or Cloud | A separate database built like Git | Fast branches, shared storage | Yes (row-level diff, merge, push/pull) | Yes | Medium–Hard (replaces standard Postgres) |
| **Migration + Seed scripts** (Prisma, Flyway, Goose, etc.) | Anywhere | Wipe the DB and re-run migrations & seed data | Depends on how fast your seed script runs | Schema only | Yes (scripts live in Git) | Easy (you likely already have this) |

---

## When to Use `pgtwig` vs. Other Tools

### Use `pgtwig` if:
- You work locally with normal PostgreSQL or Docker and don't want to change your setup.
- Your local database is small or medium (a few MBs to a few GBs).
- You switch Git branches often and hate fixing broken migrations or losing your local test data.
- You want something free, offline, and automatic every time you run `git checkout`.

### Use **Neon** or **Supabase Branching** if:
- You want a fresh cloud database automatically created for every Pull Request or CI run.
- Your team needs to share branch databases online for reviews or QA.

### Use **DBLab / ZFS / Btrfs** if:
- Your database is very large (tens of GBs to terabytes). Copying it with normal Postgres would be too slow and fill up your disk.

### Use **Dolt / DoltgreSQL** if:
- You want real Git features for your **data itself**—like seeing row-by-row diffs, merging data from two branches, or pushing data changes to teammates.
- Keep in mind it replaces standard Postgres, so check if all your Postgres features/extensions work with it.

### Just use **Migrations + Seed scripts** if:
- Rebuilding and seeding your local database from scratch already takes only a second or two, and you don't care about keeping branch-specific test data around.

---

## Good to Know (Limits)

- **New branches make a full copy**: Switching back to a branch you already visited is instant (just a rename), but creating a database for a brand-new branch copies the data on disk.
- **Open connections get closed on switch**: `pgtwig` disconnects active sessions before renaming or copying a database. Your app or DB client will simply reconnect.
- **Local only**: Saved branch databases stay on your computer and are not pushed to Git.
- **Database name length**: Your base database name can be at most 54 characters so the saved name fits inside Postgres's 63-byte limit.

---

## Development & Releases

```sh
go vet ./...
go test -v ./...
go build -o ~/.local/bin/pgtwig ./cmd/pgtwig
```

To publish a release with prebuilt macOS, Linux, and Windows binaries, push a `vX.Y.Z` tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions ([release.yml](.github/workflows/release.yml)) will run the tests, build binaries for macOS, Linux, and Windows (`amd64` and `arm64`), generate `checksums.txt`, and create a GitHub Release automatically.
