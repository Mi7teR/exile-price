// When a trade live search tab gets a new listing (its title counter goes
// up, or it plays its sound), tell poe-pricecheck on this computer which
// search it was. Nothing is sent to the trade site: the tab's URL, title and
// the search field's text are all we read.
importScripts("count.js");
const PORT = 47317; // internal/notify.Port
const LIVE = /^https:\/\/www\.pathofexile\.com\/trade\/search\/[^/]+\/[^/]+\/live/;
const FOCUS = `http://127.0.0.1:${PORT}/focus?tab=`; // internal/notify.FocusURL
const last = new Map(); // tab id → time of its last notice
const counts = new Map(); // tab id → counter last seen in its title

// A click on the notice opens FOCUS in the browser: close that tab and bring
// the live search tab to the front. Only the browser's own tabs are touched.
chrome.tabs.onUpdated.addListener(async (tabId, info, tab) => {
  const url = info.url || tab.url || "";
  if (!url.startsWith(FOCUS)) return;
  const target = Number(url.slice(FOCUS.length));
  try {
    const t = await chrome.tabs.update(target, { active: true });
    await chrome.windows.update(t.windowId, { focused: true });
    await chrome.tabs.remove(tabId);
  } catch (e) {} // the live search tab is gone: leave the page, it says so
});

chrome.tabs.onUpdated.addListener((tabId, info, tab) => {
  if (!LIVE.test(tab.url || "")) return;
  let fresh = info.audible === true;
  if (info.title !== undefined) {
    const n = titleCount(info.title);
    fresh = fresh || n > (counts.get(tabId) || 0); // read listings reset it
    counts.set(tabId, n);
  }
  if (fresh) notice(tabId, tab.url);
});

async function notice(tabId, url) {
  const now = Date.now();
  if (now - (last.get(tabId) || 0) < 2000) return; // title and beep of one listing
  last.set(tabId, now);
  let label = "";
  try {
    label = await chrome.tabs.sendMessage(tabId, "label");
  } catch (e) {} // page still loading: the tool falls back to the URL
  try {
    await fetch(`http://127.0.0.1:${PORT}/notify`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ label: label || "", url, tab: tabId }),
    });
  } catch (e) {} // poe-pricecheck is not running
}

chrome.tabs.onRemoved.addListener((tabId) => {
  last.delete(tabId);
  counts.delete(tabId);
});
