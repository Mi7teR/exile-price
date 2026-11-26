// Reads, never changes: the text of the trade page's item search field,
// e.g. "Tabula Rasa Simple Robe", when the service worker asks for it.
chrome.runtime.onMessage.addListener((msg, _sender, reply) => {
  if (msg !== "label") return;
  const el = document.querySelector(".search-bar .search-select input.multiselect__input");
  const v = el ? el.value.trim() : "";
  reply(v === "Any" ? "" : v);
});
