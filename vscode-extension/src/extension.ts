import * as path from "path";
import * as fs from "fs";
import {
  ExtensionContext,
  workspace,
  window,
  OutputChannel,
  commands,
  Uri,
  TextDocument,
  TextEditor,
  Range,
  Position,
  WorkspaceFolder,
} from "vscode";

import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from "vscode-languageclient/node";

let client: LanguageClient;
let outputChannel: OutputChannel;

export async function activate(context: ExtensionContext) {
  outputChannel = window.createOutputChannel("Ruby LSP Go");

  const rubyLspGoPath = getRubyLspGoPath();
  if (!rubyLspGoPath) {
    window.showErrorMessage(
      "Ruby LSP Go executable not found. Please install ruby-lsp-go and ensure it is in your PATH, or configure rubyLspGo.path in your settings."
    );
    return;
  }

  // Ensure the binary is executable. When a .vsix is installed, the execute
  // permission bit is often lost during extraction, which would make the
  // language server fail to spawn (EACCES) and silently disable all features.
  ensureExecutable(rubyLspGoPath);

  // Create the language client
  const serverOptions: ServerOptions = {
    run: {
      command: rubyLspGoPath,
      transport: TransportKind.stdio,
    },
    debug: {
      command: rubyLspGoPath,
      transport: TransportKind.stdio,
    },
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [
      { scheme: "file", language: "ruby" },
      { scheme: "file", language: "erb" },
      { scheme: "file", language: "rbs" },
    ],
    synchronize: {
      // Git checkouts and other external edits do not emit didSave. The
      // watched-file notification lets the server incrementally reindex them.
      fileEvents: workspace.createFileSystemWatcher("**/*.{rb,erb,rbs}"),
    },
    outputChannel: outputChannel,
    initializationOptions: {
      enabledFeatures: getEnabledFeatures(),
      formatter: workspace.getConfiguration("rubyLspGo").get("formatter"),
      linters: workspace.getConfiguration("rubyLspGo").get("linters"),
    },
  };

  client = new LanguageClient(
    "rubyLspGo",
    "Ruby LSP Go",
    serverOptions,
    clientOptions
  );

  outputChannel.appendLine(`Starting Ruby LSP Go server: ${rubyLspGoPath}`);

  try {
    await client.start();
    outputChannel.appendLine("Ruby LSP Go server started successfully");
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    outputChannel.appendLine(`Failed to start Ruby LSP Go server: ${message}`);
    window.showErrorMessage(
      `Ruby LSP Go failed to start: ${message}. See the "Ruby LSP Go" output channel for details.`
    );
    return;
  }

  // Register commands
  context.subscriptions.push(
    commands.registerCommand("rubyLspGo.restart", async () => {
      await client.stop();
      await client.start();
      window.showInformationMessage("Ruby LSP Go restarted");
    }),
    commands.registerCommand("rubyLspGo.reindexWorkspace", async () => {
      await client.sendRequest("rubyLspGo/reindexWorkspace");
      window.showInformationMessage("Ruby LSP Go workspace reindex started");
    })
  );

  outputChannel.appendLine("Ruby LSP Go extension activated");
}

function ensureExecutable(binaryPath: string): void {
  if (process.platform === "win32") {
    return;
  }
  try {
    fs.chmodSync(binaryPath, 0o755);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    outputChannel.appendLine(`Warning: could not chmod +x ${binaryPath}: ${message}`);
  }
}

export async function deactivate(): Promise<void> {
  if (client) {
    await client.stop();
  }
}

function getRubyLspGoPath(): string | undefined {
  // 1. User-configured path (highest priority)
  const configPath = workspace
    .getConfiguration("rubyLspGo")
    .get<string>("path");

  if (configPath) {
    const resolved = path.isAbsolute(configPath) ? configPath : path.join(workspace.rootPath || "", configPath);
    if (fs.existsSync(resolved)) {
      outputChannel.appendLine(`Using user-configured ruby-lsp-go: ${resolved}`);
      return resolved;
    }
    outputChannel.appendLine(`Configured path not found: ${resolved}`);
  }

  // 2. Bundled binary for this platform inside the extension's bin/ folder.
  const extensionDir = path.resolve(__dirname, "..");
  const executableName = process.platform === "win32" ? "ruby-lsp-go.exe" : "ruby-lsp-go";
  const bundledPaths = [
    path.join(extensionDir, "bin", `${process.platform}-${process.arch}`, executableName),
    // Compatibility with packages produced before platform-specific binaries.
    path.join(extensionDir, "bin", executableName),
  ];
  for (const bundledPath of bundledPaths) {
    if (isRunnableFile(bundledPath)) {
      outputChannel.appendLine(`Using bundled ruby-lsp-go: ${bundledPath}`);
      return bundledPath;
    }
  }

  // 3. Fall back to system PATH
  const systemPath = findInPath("ruby-lsp-go");
  if (systemPath) {
    outputChannel.appendLine(`Using system ruby-lsp-go: ${systemPath}`);
    return systemPath;
  }

  outputChannel.appendLine("Could not find ruby-lsp-go in PATH or bundled with the extension");
  return undefined;
}

function isRunnableFile(filePath: string): boolean {
  try {
    const stats = fs.statSync(filePath);
    if (!stats.isFile()) {
      return false;
    }
    if (process.platform !== "win32") {
      fs.accessSync(filePath, fs.constants.X_OK);
    }
    return true;
  } catch {
    return false;
  }
}

// findInPath searches the PATH environment variable for an executable,
// avoiding the external "which" dependency so the packaged extension has
// no runtime node_modules requirements.
function findInPath(executable: string): string | undefined {
  const pathEnv = process.env.PATH || "";
  const separator = process.platform === "win32" ? ";" : ":";
  const exts =
    process.platform === "win32"
      ? (process.env.PATHEXT || ".EXE;.CMD;.BAT;.COM").split(";")
      : [""];

  for (const dir of pathEnv.split(separator)) {
    if (!dir) {
      continue;
    }
    for (const ext of exts) {
      const candidate = path.join(dir, executable + ext);
      try {
        if (fs.existsSync(candidate) && fs.statSync(candidate).isFile()) {
          return candidate;
        }
      } catch {
        // ignore unreadable entries
      }
    }
  }
  return undefined;
}

function getEnabledFeatures(): Record<string, boolean> {
  const config = workspace.getConfiguration("rubyLspGo");
  const enabledFeatures = config.get<Record<string, boolean>>("enabledFeatures", {});

  // Default features - setting them to true if not explicitly configured
  const defaults = {
    codeActions: true,
    diagnostics: true,
    documentHighlights: true,
    documentSymbols: true,
    foldingRanges: true,
    formatting: true,
    hover: true,
    inlayHint: false,
    onTypeFormatting: true,
    selectionRanges: true,
    semanticHighlighting: true,
    completion: true,
    definition: true,
    references: true,
    signaturesHelp: true,
    workspaceSymbol: true,
  };

  return { ...defaults, ...enabledFeatures };
}
