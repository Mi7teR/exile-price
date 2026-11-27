# exile-price

Fast native price check for Path of Exile 1: copy an item in the game, a price line appears at the cursor. Go, no Electron.

## Use (Windows)

1. Download `exile-price.exe` from [Releases](https://github.com/Mi7teR/exile-price/releases) and run it. Windows SmartScreen may warn about an unsigned file: "More info" → "Run anyway".
2. A line shows the league; an icon appears in the tray.
3. In the game, hover an item and press **Ctrl+Alt+C** (Ctrl+C works too). The price appears at the cursor:
   - currency, divination cards, gems: instantly, from poe.ninja;
   - uniques: from poe.ninja; one worth 20c or more, or with an enchant, is also searched on the trade site against copies rolled at least as well, with the same corruption, links and enchant, and the line shows both prices;
   - normal and magic bases: from poe.ninja (item level 82+); a base with an enchant is searched on the trade site with the same enchant, influences and at least its item level;
   - rares (armour, jewellery, jewels, abyss jewels, weapons), rare cluster jewels (by size, small passive and notables) and regular maps: in about a second, from the trade site. Resistances and life are searched as totals, weapons by physical and elemental DPS, influences, eldritch implicits and enchants (lab enchants, anointments) are kept, a 5- or 6-link or six sockets is required (with the off-colours when more than half are off-colour), bench crafts are ignored; junk mods and, in a Ctrl+Alt+C copy, tier 5 and worse mods are not required. Each seller counts once. The same trade search is reused for 15 minutes, so checking an item again costs no trade request. Price fixers (a few baits far under the rest, not a whole cheap market) and lots older than a week (when enough fresh ones exist) are left out. "rough estimate" means few listings or not all mods matched. For rares the line ends with what was searched, e.g. "searched: 68 life, 79% res, 30% increased Movement Speed +1".
4. The line hides after 5 s, on Esc, or when the mouse moves away. **Ctrl+Alt+O** opens the last trade search in the browser.
5. The tray icon: left click or "Settings…" opens the settings (league, POESESSID); the menu also has "Open the last search", "Open log" and "Quit". Running `exile-price.exe` again quits too.
6. The interface follows the Windows display language: Russian for Russian, Belarusian and Kazakh, English otherwise.

**Live search notices (optional):** a trade live search only beeps in the browser. The extension in `extension/` makes exile-price show a card in the top right corner over the game, with a sound: the search's name (the search field, else the item or category from the search link) and its enabled stat filters. Cards stack, newest on top, for 20 s; another listing from the same tab moves its card up and counts it. A click on a card brings that tab to the front in the browser that sent it. It only reads the tab: the new-listing counter in the live search tab's title or that the tab plays its sound, and the text of the item search field; it sends nothing to the trade site and changes nothing on the page. Install: download the source zip from Releases, unpack, open `chrome://extensions`, turn on "Developer mode", "Load unpacked" → the `extension` folder. exile-price listens on `127.0.0.1:47317` for this extension only.

**Compare with your build (optional):** under the price, exile-price shows how your Path of Building build changes if you equip the item.

1. Save your build in Path of Building (Ctrl+S).
2. Tray icon → "Settings…" → the "PoB build" section: pick the build (the PoB builds folder is found by itself; for a portable PoB, point it at its `Builds` folder) → "Save".
3. In the game, press **Ctrl+Alt+C** on a ring, armour piece, belt, amulet or weapon. A second line appears under the price, e.g. `instead of Ring 2: DPS +3.1% · Life +68 · Fire +12% · EHP +4%`: green is better, red is worse.
4. Changed the build in PoB and saved? The next Ctrl+Alt+C picks it up.
5. A build that is not in your PoB (pobb.in, a friend): copy its PoB code, then tray icon → "Load build from clipboard".

Notes:
- The item is compared against the build's active item set and active weapon set. A ring or a one-handed weapon is tried in both slots and the better result is shown (more DPS, then more EHP). Jewels, flasks and tinctures are not compared.
- DPS is Full DPS when the build has skills in Full DPS, otherwise the main skill's DPS. Resistances are capped values.
- "not all counted" means the engine did not understand some of the item's mods.
- Timeless jewels are not applied without PoB's timeless data; the build is still computed.
- No network request is made for the build.
- A pasted code is kept in `build.txt`. The settings are the `build_source` ("pob" or "clipboard"), `pob_build` and `pob_dir` keys in `config.toml`.

**League:** "Авто" picks the first temporary softcore league in GGG's trade list; pick another one (Hardcore, Ruthless, Standard) in the settings. **POESESSID** is optional: the trade site answers without a login today. It is stored in `config.toml` and never written to the log.

Works over the game in windowed and windowed fullscreen modes; over exclusive fullscreen it is not guaranteed.

Files live in `%APPDATA%\exile-price\`: `config.toml` (written with defaults on first run), `build.txt` (the pasted build code), `pricecheck.log` (with the older `pricecheck.log.1`, at most 10 MB together), `cache\`. Before the rename the app was poe-pricecheck: on the first run exile-price moves `%APPDATA%\poe-pricecheck\` to the new folder, settings included.

## Develop (any OS)

```bash
go test ./...
```

```bash
node extension/count.test.js
```

On macOS, `go run ./cmd/notify-dev` shows the same cards in a plain window, to try the extension without Windows.

```bash
go run ./cmd/pricecheck-cli -v testdata/items/boots_rare.txt
```

```bash
go run ./cmd/pricecheck-cli -build testdata/builds/assassin.code testdata/items/real_ring_rare_crafted.txt
```

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/exile-price.exe ./cmd/pricecheck
```

`testdata/recorded/` holds real API responses from 2026-11-23 (league Covens); `testdata/items/real_*.txt` are real item texts taken from trade listings.

The icon is `assets/icon.svg`, rendered to `assets/icon.png`. After changing it, rebuild the Windows resource:

```bash
cd cmd/pricecheck && go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64
```
