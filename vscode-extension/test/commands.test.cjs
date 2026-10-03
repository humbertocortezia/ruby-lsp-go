const assert = require("node:assert/strict");
const fs = require("node:fs");
const Module = require("node:module");
const path = require("node:path");
const test = require("node:test");

test("advertised reindex command is registered once, and failures stay visible", async () => {
  const registered = new Map();
  const information = [];
  const errors = [];
  const logs = [];
  let clientOptions;
  const vscode = {
    window: {
      createOutputChannel: () => ({ appendLine: (line) => logs.push(line) }),
      showInformationMessage: (message) => information.push(message),
      showErrorMessage: (message) => errors.push(message),
    },
    workspace: {
      getConfiguration: () => ({ get: (name) => name === "path" ? process.execPath : undefined }),
      createFileSystemWatcher: () => ({ dispose() {} }),
    },
    commands: {
      registerCommand: (name, callback) => {
        assert.equal(registered.has(name), false, `duplicate command: ${name}`);
        registered.set(name, callback);
        return { dispose() {} };
      },
    },
  };
  class LanguageClient {
    constructor(_id, _name, _serverOptions, options) { clientOptions = options; }
    async start() {
      // Match ExecuteCommandFeature in vscode-languageclient: the server's
      // advertised command is registered before client.start resolves.
      vscode.commands.registerCommand("rubyLspGo.reindexWorkspace", (...args) =>
        clientOptions.middleware.executeCommand("rubyLspGo.reindexWorkspace", args, async () => ({ started: true })));
    }
    async stop() {}
  }
  const originalLoad = Module._load;
  const originalChmod = fs.chmodSync;
  Module._load = function (request, ...rest) {
    if (request === "vscode") return vscode;
    if (request === "vscode-languageclient/node") return { LanguageClient, TransportKind: { stdio: 0 } };
    return originalLoad.call(this, request, ...rest);
  };
  fs.chmodSync = () => {}; // Never change permissions on the test host's Node executable.
  let extension;
  try {
    extension = require(path.join(__dirname, "../out/extension.js"));
    await extension.activate({ subscriptions: [] });
  } finally {
    Module._load = originalLoad;
    fs.chmodSync = originalChmod;
  }
  assert.equal(registered.size, 2); // restart + client-managed reindex
  assert.ok(logs.includes("Ruby LSP Go extension activated"));
  await registered.get("rubyLspGo.reindexWorkspace")();
  assert.equal(information.length, 1);
  for (const response of [undefined, { started: false, error: "index unavailable" }]) {
    await assert.rejects(clientOptions.middleware.executeCommand("rubyLspGo.reindexWorkspace", [], async () => response));
  }
  await assert.rejects(clientOptions.middleware.executeCommand("rubyLspGo.reindexWorkspace", [], async () => { throw new Error("request failed"); }), /request failed/);
  assert.equal(information.length, 1); // No false success on failed requests.
  assert.equal(errors.length, 3);
  assert.ok(logs.some((line) => line.includes("request failed")));
  await extension.deactivate();
});
