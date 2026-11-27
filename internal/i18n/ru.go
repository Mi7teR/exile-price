package i18n

var ruNouns = map[string][3]string{
	"listing": {"лот", "лота", "лотов"},
	"fixer":   {"фиксер", "фиксера", "фиксеров"},
}

var ru = map[string]string{
	// price line
	"base ~%s · poe.ninja":              "база ~%s · poe.ninja",
	" · base ~%s":                       " · база ~%s",
	"no listings · %d on the site":      "нет лотов · на сайте %d",
	" · rough estimate":                 " · оценка грубая",
	"Open the last search (Ctrl+Alt+O)": "Открыть последний поиск (Ctrl+Alt+O)",
	"no trade search yet":               "поиска на trade ещё не было",
	"any %d of %d mods":                 "любые %d из %d модов",
	" · searched: %s":                   " · искал: %s",

	// pricer errors
	"%s is not priced yet":        "%s пока не оценивается",
	"%s: not on poe.ninja":        "%s: нет на poe.ninja",
	"no mod matched a trade stat": "ни один мод не сопоставлен с трейд-статом",
	"the trade league list has no temporary softcore league; set league in config.toml": "в списке трейд-лиг нет временной софткор-лиги; задайте league в config.toml",

	// overlay
	"exile-price %s · loading prices…":                                          "exile-price %s · загрузка цен…",
	"no connection to the trade site or poe.ninja, retrying in 30 s":            "нет связи с трейдом или poe.ninja, повтор через 30 с",
	"exile-price %s · league %s · Ctrl+Alt+C on an item · settings in the tray": "exile-price %s · лига %s · Ctrl+Alt+C на предмете · настройки в трее",
	"config.toml has an error, using defaults · log: %s":                        "config.toml с ошибкой, взяты значения по умолчанию · лог: %s",
	"new listing":                             "новый лот",
	"cannot listen to the clipboard: %s":      "не удалось слушать буфер: %s",
	"pricing…":                                "оцениваю…",
	"trade request limit · try again in %d s": "лимит запросов трейда · повтори через %d с",
	"error: %s":                               "ошибка: %s",
	"Settings…":                               "Настройки…",
	"Open log":                                "Открыть лог",
	"Quit":                                    "Выход",
	"settings open once prices are loaded":    "настройки откроются после загрузки цен",
	"exile-price — settings":                  "exile-price — настройки",
	"cannot save config.toml: %s":             "не удалось сохранить config.toml: %s",
	"settings saved · loading prices…":        "настройки сохранены · загрузка цен…",
	"prices did not load, the old ones stay; see the log": "цены не загрузились, остались прежние; подробности в логе",
	"done · league %s": "готово · лига %s",

	// settings form
	"League": "Лига",
	"Auto: the current temporary softcore league": "Авто: текущая временная софткор-лига",
	" (now %s)": " (сейчас %s)",
	"Optional: the trade site works without a login today. Where to get it: pathofexile.com → F12 → Application → Cookies → POESESSID. Stored in config.toml, never written to the log.": "Необязательно: трейд сейчас работает и без входа. Где взять: pathofexile.com → F12 → Application → Cookies → POESESSID. Хранится в config.toml и не пишется в лог.",
	"empty — no login": "пусто — без входа",
	"Show":             "Показать",
	"Cancel":           "Отмена",
	"Save":             "Сохранить",
	"POESESSID is 32 characters 0–9 and a–f": "POESESSID — это 32 символа 0–9 и a–f",

	// build comparison
	"instead of %s: ":               "вместо %s: ",
	"into empty %s: ":               "в пустой %s: ",
	"no change":                     "без изменений",
	" · not all counted":            " · не всё учтено",
	"Load build from clipboard":     "Загрузить билд из буфера",
	"build loaded: %s":              "билд загружен: %s",
	"no PoB code on the clipboard":  "в буфере не PoB-код",
	"build does not compute: %s":    "билд не считается: %s",
	"build file not found":          "файл билда не найден",
	"cannot read the clipboard: %s": "не удалось прочитать буфер: %s",

	// PoB build settings
	"PoB build":                           "Билд PoB",
	"PoB builds folder":                   "Папка билдов PoB",
	"do not compare":                      "не сравнивать",
	"code from the clipboard (build.txt)": "код из буфера (build.txt)",
	"Compare items with this build. Files are re-read after you save in PoB.": "Сравнивать предметы с этим билдом. Файлы перечитываются после сохранения в PoB.",
}
