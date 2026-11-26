// node extension/count.test.js
const assert = require("assert");
const { titleCount } = require("./count.js");
for (const [title, want] of [
  ["Trade - Path of Exile", 0],
  ["(3) Trade - Path of Exile", 3],
  ["(12) Trade - Path of Exile", 12],
  ["", 0],
  [undefined, 0],
]) assert.strictEqual(titleCount(title), want, String(title));
console.log("ok");
