# shop

Free online shop management app that works on localhost with a `.exe`
or `.sh` file — no install, no internet connection required.

## Prerequisites

- Go 1.22 or later

## Run

Run it from the project folder (the one containing `go.mod`):

```sh
go run ./cmd/server
```

Or build a standalone double-clickable app — see "Desktop app" below.

## Environment variables

All optional:

| Variable | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8080` | Port to listen on (binds `127.0.0.1` only) |
| `DB_PATH` | next to the executable (see below) | Path to the SQLite database file |
| `CURRENCY` | `so'm` | Currency label shown in the UI |
| `DECIMALS` | `0` | Decimal places shown in the UI (0-3) |
| `SHOP_NAME` | `ShopKeeper` | Shop name shown in printed reports and receipts |
| `BACKUP_DIR` | `backups` folder next to the database file | Where automatic daily backups are written |
| `BACKUP_KEEP_DAYS` | `30` | How many days of daily backups to keep before they're rotated out |
| `NO_BROWSER` | unset | Set to `1` to stop the app from opening your browser on startup |

## Currency

The app defaults to Uzbek so'm (`CURRENCY=so'm`, `DECIMALS=0`): all
amounts are whole numbers, entered and displayed without decimals
(e.g. `150000`, shown as "150 000 so'm"). Prices, billiard amounts and
sale totals accept whole numbers from 0 to 100,000,000,000.

To use a currency with cents/decimals instead (e.g. US dollars), set
`CURRENCY=USD` and `DECIMALS=2`; amounts are then entered and
displayed with up to `DECIMALS` decimal places (e.g. `12.50`).
`DECIMALS` must be an integer from 0 to 3.

## Database location

By default, `shop.db` lives next to the running executable (so a
double-clicked `ShopKeeper.exe` keeps its data in its own folder). The
one exception is `go run`, which builds to a throwaway temp directory:
in that case (and under `go run` only) it falls back to the previous
behavior of `./shop.db` in whatever directory you ran it from. Set
`DB_PATH` to override either way.

## Desktop app

Build double-clickable binaries for Windows and Linux:

```sh
./build.sh   # or build.bat on Windows
```

This produces `dist/ShopKeeper.exe` and `dist/shopkeeper-linux`.

**Installing:** put the exe in its own folder (e.g. `C:\ShopKeeper`) —
`shop.db` and `backups/` will appear right beside it the first time you
run it. Don't put it in `Downloads` or similar: that folder is just as
good a home for its data as any other, but it's nicer to keep it
somewhere dedicated.

**Desktop shortcut:** right-click `ShopKeeper.exe` → Send to → Desktop
(create shortcut) on Windows, or right-click → Make Link on most Linux
file managers. Double-clicking the shortcut starts the server and
opens your browser to it automatically.

**Stopping it:** closing the console/terminal window that opens stops
the app (it shuts down gracefully, taking one last backup first). If
you start it a second time while it's already running, it just opens
your browser to the existing instance instead of starting a second
copy.

**Moving existing data:** stop the app, copy your old `shop.db` into
the new exe's folder, then start it again.

**Windows SmartScreen:** since this exe isn't signed, Windows may warn
"Windows protected your PC" the first time you run it. Click "More
info", then "Run anyway".

## Backups

### Automatic daily backups

The server keeps its own daily backups without any action needed:

- On startup, if today's backup doesn't exist yet, it takes one.
- Every 6 hours while running, it takes one (overwriting today's file).
- On graceful shutdown (Ctrl-C or SIGTERM), it takes one last backup
  before closing the database.

Backups live in `BACKUP_DIR` (default: a `backups` folder next to the
database file), named `shop-YYYY-MM-DD.db`. The absolute path is
printed at startup. Backups older than `BACKUP_KEEP_DAYS` (default 30)
are deleted after each backup runs. Files that don't match the
`shop-YYYY-MM-DD.db` pattern — including any `shop-premigration-*`
files — are never touched by rotation.

Tip: point `BACKUP_DIR` at a USB drive or a cloud-synced folder (e.g.
Dropbox/Google Drive/OneDrive) to automatically get an off-machine copy.

**To restore from a backup:**

1. Stop the app.
2. Copy the backup file over `shop.db` (the path from `DB_PATH`, or
   next to the executable by default).
3. Delete `shop.db-wal` and `shop.db-shm` if they exist, next to
   `shop.db`.
4. Start the app again.

### On-demand backup

Click "Download backup" in the app's navbar, or fetch it directly:

```sh
curl -o shop-backup.db http://127.0.0.1:8080/api/backup
```

This streams a consistent snapshot of the live database, safe to copy
even while the server is running.

Every stock change — sales, voids, manual adjustments, and each product's initial balance — is recorded and viewable on the **Stock log** page (`/stock-log`), filterable by product.

The **Billiard** page (`/billiard`) records money received for table time, with no stock or product involved.
Billiard revenue is included in the dashboard's totals and the daily sales chart alongside regular shop sales, shown stacked with shop revenue.

## Printing

On the Sales page, "Print daily report" prints that day's totals, by-product breakdown and full sale list, and each sale's "Receipt" button prints a narrow till-style receipt.

The dashboard's "Daily sales" chart shows the last 7 or 30 days of revenue or units sold; click or tap a bar to open that day on the Sales page.
