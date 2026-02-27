import * as path from "path";
import * as fs from "fs";
import * as cp from "child_process";
import * as which from "which";
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
      fileEvents: workspace.createFileSystemWatcher("**/*.rb"),
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

  await client.start();

  // Register commands
  context.subscriptions.push(
    commands.registerCommand("rubyLspGo.restart", async () => {
      await client.stop();
      await client.start();
      window.showInformationMessage("Ruby LSP Go restarted");
    })
  );

  outputChannel.appendLine("Ruby LSP Go extension activated");
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

  // 2. Bundled binary inside the extension's bin/ folder
  const extensionDir = path.resolve(__dirname, "..");
  const bundledPath = path.join(extensionDir, "bin", "ruby-lsp-go");
  if (fs.existsSync(bundledPath)) {
    outputChannel.appendLine(`Using bundled ruby-lsp-go: ${bundledPath}`);
    return bundledPath;
  }

  // 3. Fall back to system PATH
  try {
    const systemPath = which.sync("ruby-lsp-go");
    outputChannel.appendLine(`Using system ruby-lsp-go: ${systemPath}`);
    return systemPath;
  } catch (error) {
    outputChannel.appendLine("Could not find ruby-lsp-go in PATH or bundled with the extension");
    return undefined;
  }
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

