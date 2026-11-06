# shop

Free online shop management app that works on localhost with a `.exe`
or `.sh` file — no install, no internet connection required. On Windows
it opens in its own window (no browser, no console); on Linux it opens
your browser.

## Prerequisites

- Go 1.27.1 or later

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

Build the binaries for Windows and Linux:

```sh
./build.sh   # or build.bat on Windows
```

This produces `dist/ShopKeeper.exe` and `dist/shopkeeper-linux`.

**Windows: a native window.** `ShopKeeper.exe` is a normal Windows
program (title bar, taskbar icon, no browser, no console, no web server
and no extra runtime to install). Everything it needs at run time is on
the PC: it works with no internet.

> **Work in progress:** the native window has the Products and Sell
> screens so far. For the other pages (Dashboard, Sales, Stock log,
> Billiard) run it with `SHOPKEEPER_UI=web`, which shows the web pages in
> your browser as before.

- **Closing the window stops the app** (it takes one last backup
  first). Start it again from the shortcut.
- **Starting it twice** brings the first window to the front instead
  of opening a second copy.
- **Log:** with no console, the log is written to `shop.log` next to the
  database (the previous one is kept as `shop.log.old` once it passes
  2 MB). A start-up error (for example, the port is taken) is shown in a
  message box.
- **`SHOPKEEPER_UI=web`** serves the web pages on `localhost` and opens
  your browser instead of the native window (the same as on Linux).
- **`NO_BROWSER=1`** runs the server with no window at all (scripted
  or headless use).

**Linux:** `shopkeeper-linux` runs in the terminal and opens your
browser; closing the terminal (or Ctrl-C) stops it.

**Installer (Windows).** To build `dist/ShopKeeper-Setup.exe` (needs
[Inno Setup 6](https://jrsoftware.org/isdl.php)):

```powershell
powershell -File installer\build-installer.ps1
```

It installs to `C:\ShopKeeper` (its data lives next to the program, so
not Program Files), adds a Start menu entry and an optional desktop
shortcut, and offers to start the app. Uninstalling removes only the
program: `shop.db`, `backups\` and `shop.log` stay.

**Without the installer:** put the exe in its own folder (e.g.
`C:\ShopKeeper`) — `shop.db` and `backups/` appear right beside it the
first time you run it. Right-click `ShopKeeper.exe` → Send to → Desktop
(create shortcut) for a shortcut.

**Moving existing data:** stop the app, copy your old `shop.db` into
the new exe's folder, then start it again.

**Windows SmartScreen:** since this exe isn't signed, Windows may warn
"Windows protected your PC" the first time you run it. Click "More
info", then "Run anyway".

## Updating

Click **Update** in the navbar (the installed version is shown next to the
name). The app asks GitHub for the latest release and, if it is newer, shows
what changed and offers **Update now**. It then downloads `ShopKeeper.exe`
(or `shopkeeper-linux`), checks it against the release's `SHA256SUMS.txt`,
takes a last backup, replaces itself and restarts. `shop.db`, `backups/` and
`shop.log` are never touched. Nothing is checked until you click, so the app
still works without internet.

- It needs write access to the program's folder (the installer's
  `C:\ShopKeeper` is fine).
- If anything fails (no internet, damaged download) the current version is
  kept and the message is shown in the dialog.
- A build from `go run` is version `dev` and never updates itself. To set a
  version: `VERSION=1.2.0 ./build.sh` (`set VERSION=1.2.0` before `build.bat`).
  Pushing a `v*` tag does this in the release workflow and publishes the files
  the updater needs.

## Icon

The icon is `assets/icon.svg`. `assets/build-icons.sh` regenerates the
`.ico` files and the browser favicon from it (needs `rsvg-convert` and Pillow).
It is used for the window, the taskbar, the installer, the shortcuts and the
browser tab.

## Backups

### Automatic daily backups

The server keeps its own daily backups without any action needed, and
writes two different kinds of file each day:

- `shop-YYYY-MM-DD.db` — the rotating daily backup. On startup, if
  today's doesn't exist yet, it takes one. Every 6 hours while
  running, it takes one (overwriting today's file). On graceful
  shutdown (Ctrl-C or SIGTERM), it takes one last backup before
  closing the database. Because it's repeatedly overwritten, it always
  reflects roughly "the last few hours," not any one specific moment.
- `shop-YYYY-MM-DD-start.db` — a start-of-day snapshot, written once
  at startup if it doesn't already exist, and never overwritten again
  that day (even across restarts). This is your way back to "this
  morning": if something goes wrong partway through the day, it's the
  one backup guaranteed not to already contain today's changes.

Backups live in `BACKUP_DIR` (default: a `backups` folder next to the
database file). The absolute path is printed at startup. Backups older
than `BACKUP_KEEP_DAYS` (default 30) are deleted after each backup
runs, whether `-start` or not. Files that don't match either pattern —
including any `shop-premigration-*` files — are never touched by
rotation.

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
