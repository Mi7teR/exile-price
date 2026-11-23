# poe-pricecheck

Fast native price check for Path of Exile 1: copy an item in the game, a price line appears at the cursor. Go, no Electron.

## Use (Windows)

1. Download `poe-pricecheck.exe` from [Releases](https://github.com/Mi7teR/poe-pricecheck/releases) and run it. Windows SmartScreen may warn about an unsigned file: "More info" → "Run anyway".
2. A line shows the league; an icon appears in the tray.
3. In the game, hover an item and press **Ctrl+Alt+C** (Ctrl+C works too). The price appears at the cursor:
   - currency, divination cards, gems, uniques: instantly, from poe.ninja;
   - rares and regular maps: in about a second, from the trade site, with price fixers removed.
4. The line hides after 5 s, on Esc, or when the mouse moves away.
5. The tray icon: left click or "Настройки…" opens the settings (league, POESESSID); the menu also has "Открыть лог" and "Выход". Running `poe-pricecheck.exe` again quits too.

**League:** "Авто" picks the first temporary softcore league in GGG's trade list; pick another one (Hardcore, Ruthless, Standard) in the settings. **POESESSID** is optional: the trade site answers without a login today. It is stored in `config.toml` and never written to the log.

Works over the game in windowed and windowed fullscreen modes; over exclusive fullscreen it is not guaranteed.

Files live in `%APPDATA%\poe-pricecheck\`: `config.toml` (written with defaults on first run), `pricecheck.log`, `cache\`.

## Develop (any OS)

```bash
go test ./...
```

```bash
go run ./cmd/pricecheck-cli -v testdata/items/boots_rare.txt
```

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/poe-pricecheck.exe ./cmd/pricecheck
```

`testdata/recorded/` holds real API responses from 2026-11-23 (league Covens); `testdata/items/real_*.txt` are real item texts taken from trade listings.
