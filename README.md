# ppr – PGSD pkg repair

**ppr** is a terminal utility that repairs and verifies FreeBSD and GhostBSD package databases.  
It provides clear, progressive feedback and real-time status updates.

- **Name:** PGSD pkg repair (ppr)  
- **Author:** Pacific Grove Software Distribution Foundation  
- **License:** BSD 2-Clause  
- **Copyright:** © 2025 Pacific Grove Software Distribution Foundation  

---

## Requirements

- GhostBSD or PGSD  
- Go 1.21 or newer  
- Root privileges  
- `pkg(8)` utility available in `$PATH`

---

## Installation

### Building from Source

```sh
git clone https://github.com/pgsdf/ppr.git
cd ppr
make build
````

### Building for Multiple Architectures

The included `Makefile` supports both `amd64` and `arm64` builds:

```sh
make build-amd64
make build-arm64
make release
```

All build artifacts and checksums will be placed in the `bin/` directory.

---

## Usage

Run `ppr` as root to perform all repair stages:

```sh
sudo ./ppr
```

### Command-Line Options

| Option                 | Description                                    | Default |
| ---------------------- | ---------------------------------------------- | ------- |
| `--dry-run`            | Show intended actions without applying changes | false   |
| `--compact`            | Compact, minimal output mode                   | false   |
| `--report-json <file>` | Write detailed JSON event log to file          | none    |
| `--timeout <duration>` | Set overall timeout (e.g. 30m, 1h)             | 20m     |

### Example

```sh
sudo ./ppr --compact --report-json /var/log/ppr-$(date +%Y%m%d).json
```

---

## Execution Stages

1. **Check Repository Network**

   Verifies all repositories are reachable and their metadata endpoints respond.
   
   Verifies DNS resolution for repository hosts
   
2. **Detect Environment**

   Confirms execution as root and checks system compatibility.

3. **Clear Repository Cache**

   Removes outdated or corrupted `repo-*.sqlite*` files.

4. **Force Package Update**

   Refreshes repository data with `pkg update -f`.

5. **Verify Package Database**

   Performs integrity checks with `pkg check -da`.

6. **Recompute Package Metadata**

   Rebuilds dependency and manifest data with `pkg check -r -a`.

7. **Last Resort Recovery**

   Moves `local.sqlite` aside if needed.
   If none exists, ppr reports:
   *“No local.sqlite found — package database is already in a clean state.”*

---

## JSON Report Example

```json
[
  {
    "time": "2025-02-01T05:22:00Z",
    "stage": "repo_network_check",
    "status": "ok",
    "message": "Repository network reachable",
    "detail": "[✓] https://pkg.ghostbsd.org/stable/FreeBSD:14:amd64/latest (ok)"
  },
  {
    "time": "2025-02-01T05:23:00Z",
    "stage": "move_local_sqlite",
    "status": "ok",
    "message": "No local.sqlite found",
    "detail": "Package database is already in a clean state"
  }
]
```

Statuses: `ok`, `warn`, `skip`, `error`

---

## Troubleshooting

### Must run as root

```sh
sudo ./ppr
```

### Repository unreachable

Check your network configuration or `/etc/pkg/GhostBSD.conf`.

### Package database remains inconsistent

Manual recovery steps:

```sh
sudo rm -f /var/db/pkg/repo-*.sqlite*
sudo rm -f /var/db/pkg/local.sqlite
sudo pkg bootstrap -f
sudo pkg update -f
sudo pkg upgrade -f
```

---

## Exit Codes

| Code | Meaning                             |
| ---- | ----------------------------------- |
| 0    | Success                             |
| 1    | General failure                     |
| 2    | Invalid arguments                   |
| 126  | Permission denied (not run as root) |

---

## Project Layout

```
ppr/
├── main.go        # Program source
├── Makefile       # Cross-platform build definitions
├── go.mod         # Go module dependencies
└── README.md      # Project documentation
```

---

## License

BSD 2-Clause License
© 2025 Pacific Grove Software Distribution Foundation
