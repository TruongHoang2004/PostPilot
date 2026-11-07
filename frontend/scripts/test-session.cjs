const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const ts = require("typescript");
const path = require("node:path");

const storage = new Map();
const events = [];
const pending = [];
const uploads = [];
class XHR {
  constructor() { this.upload = {}; this.status = 0; this.responseText = '{"error":"expired"}'; uploads.push(this); }
  open() {} setRequestHeader() {} send() {}
}
const sandbox = {
  exports: {}, Headers, Response, URLSearchParams, FormData, process: { env: {} },
  localStorage: { getItem: key => storage.get(key) || null, setItem: (key,value) => storage.set(key,value), removeItem: key => storage.delete(key) },
  window: { dispatchEvent: event => events.push(event.type) }, Event: class { constructor(type) { this.type = type; } },
  XMLHttpRequest: XHR,
  fetch: () => new Promise(resolve => pending.push(resolve)),
};
const source = fs.readFileSync(path.join(__dirname, "../src/lib/api.ts"), "utf8");
vm.runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText, sandbox);
const api = sandbox.exports.api;

(async () => {
  api.setToken("old");
  const old = api.getPosts();
  api.setToken("new");
  pending.shift()(new Response('{"error":"expired"}', {status:401}));
  await assert.rejects(old);
  assert.equal(api.getToken(), "new", "old401 erased the new login");
  assert.equal(events.length, 0);

  const current = api.getPosts();
  pending.shift()(new Response('{"error":"expired"}', {status:401}));
  await assert.rejects(current);
  assert.equal(api.getToken(), null, "current401 must end the expired session");
  assert.equal(events.length, 1);

  api.setToken("upload-old");
  const upload = api.uploadMedia(new Blob(["test"]));
  api.setToken("upload-new");
  const xhr = uploads.shift(); xhr.status=401; xhr.onload();
  await assert.rejects(upload);
  assert.equal(api.getToken(), "upload-new", "stale upload401 erased a new login");

  process.env.TZ = "Asia/Ho_Chi_Minh";
  const schedule = {exports:{}, require};
  const scheduleSource = fs.readFileSync(path.join(__dirname,"../src/lib/schedule.ts"),"utf8");
  vm.runInNewContext(ts.transpileModule(scheduleSource,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText,schedule);
  const original="2026-10-28T03:30:00.000Z";
  const local=schedule.exports.toLocalInputValue(new Date(original));
  assert.equal(local,"2026-10-28T10:30");
  assert.equal(new Date(local).toISOString(),original,"unchanged schedule shifted timezone");
  console.log("PASS: stale/current fetch401, stale upload401 and Vietnam schedule round trip");
})().catch(error => {console.error(error); process.exitCode=1;});
