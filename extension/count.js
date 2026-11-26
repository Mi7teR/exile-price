// titleCount reads the new-listing counter the trade page puts in its tab
// title, e.g. "(3) Trade - Path of Exile" → 3; no counter → 0.
function titleCount(title) {
  const m = /\((\d+)\)/.exec(title || "") || /\d+/.exec(title || "");
  return m ? Number(m[1] ?? m[0]) : 0;
}
if (typeof module !== "undefined") module.exports = { titleCount };
