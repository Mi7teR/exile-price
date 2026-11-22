# poe-pricecheck

Fast native price check for Path of Exile 1 (Go, Windows overlay).

## Spike

```bash
go run ./cmd/pricecheck-cli -v testdata/items/boots_rare.txt
```

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/poe-pricecheck-spike.exe ./cmd/pricecheck
```

`poe-pricecheck-spike.exe` shows a one-line window at the cursor every 5 s for 2 s, logs whether the game kept focus to `%APPDATA%\poe-pricecheck\spike.log`, and exits after 5 minutes.

`testdata/recorded/` holds real API responses from 2026-11-23 (league Covens).
