import { spawnSync } from "node:child_process";

type ParsedArgs = {
  help: boolean;
  repoPath: string;
  prompt: string;
};

type StreamedRun = {
  events: AsyncIterable<unknown>;
};

type CodexThread = {
  runStreamed: (input: string) => Promise<StreamedRun>;
};

type CodexInstance = {
  startThread: (options: { workingDirectory: string }) => CodexThread;
};

type CodexOptions = {
  codexPathOverride?: string;
};

type CodexModule = {
  Codex: new (options?: CodexOptions) => CodexInstance;
};

async function loadCodexModule(): Promise<CodexModule> {
  const module = await import("@openai/codex-sdk");
  if (!module || typeof module !== "object" || !("Codex" in module)) {
    throw new Error("failed to load @openai/codex-sdk");
  }

  return module as CodexModule;
}

const USAGE = `Usage:
  codex-wrapper --repo-path <path> --prompt <text>
  codex-wrapper --help

Options:
  --repo-path <path>   Repository path to run Codex in
  --prompt <text>      Prompt sent to Codex
  --help               Show this help text`;

function parseArgs(argv: string[]): ParsedArgs {
  const parsed: ParsedArgs = {
    help: false,
    repoPath: "",
    prompt: "",
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    switch (arg) {
      case "--help":
        parsed.help = true;
        break;
      case "--repo-path": {
        const value = argv[i + 1];
        if (!value || value.startsWith("--")) {
          throw new Error("--repo-path requires a value");
        }
        parsed.repoPath = value;
        i += 1;
        break;
      }
      case "--prompt": {
        const value = argv[i + 1];
        if (!value || value.startsWith("--")) {
          throw new Error("--prompt requires a value");
        }
        parsed.prompt = value;
        i += 1;
        break;
      }
      default:
        throw new Error(`unknown argument: ${arg}`);
    }
  }

  return parsed;
}

function writeEventAsNDJSON(event: unknown): void {
  if (event === null || typeof event !== "object" || Array.isArray(event)) {
    throw new Error("received non-object event from Codex stream");
  }

  const serialized = JSON.stringify(event);
  if (!serialized) {
    throw new Error("failed to serialize Codex event");
  }

  process.stdout.write(`${serialized}\n`);
}

async function streamCodex(args: ParsedArgs): Promise<void> {
  const codexModule = await loadCodexModule();
  const { Codex } = codexModule;
  const codexPathOverride = resolveCodexPathOverride();
  if (!codexPathOverride) {
    process.stderr.write(
      "Warning: codex CLI not found on PATH and no YARALPHO_CODEX_CLI_PATH/CODEX_CLI_PATH set; relying on SDK packaged binaries.\n",
    );
  }
  const codex = new Codex(
    codexPathOverride ? { codexPathOverride } : undefined,
  );
  const thread = codex.startThread({
    workingDirectory: args.repoPath,
  });

  const { events } = await thread.runStreamed(args.prompt);
  for await (const event of events) {
    writeEventAsNDJSON(event);
  }
}

function resolveCodexPathOverride(): string | undefined {
  const fromEnv = firstNonEmpty(
    process.env.YARALPHO_CODEX_CLI_PATH,
    process.env.CODEX_CLI_PATH,
  );
  if (fromEnv) {
    return fromEnv;
  }

  const lookup = spawnSync("sh", ["-lc", "command -v codex"], {
    encoding: "utf8",
  });
  if (lookup.status === 0) {
    const resolved = (lookup.stdout ?? "").trim();
    if (resolved) {
      return resolved;
    }
  }

  return undefined;
}

function firstNonEmpty(...values: Array<string | undefined>): string | undefined {
  for (const value of values) {
    const trimmed = (value ?? "").trim();
    if (trimmed !== "") {
      return trimmed;
    }
  }
  return undefined;
}

async function run(argv: string[]): Promise<number> {
  let args: ParsedArgs;
  try {
    args = parseArgs(argv);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    process.stderr.write(`Error: ${message}\n`);
    process.stderr.write(`${USAGE}\n`);
    return 1;
  }

  if (args.help) {
    process.stdout.write(`${USAGE}\n`);
    return 0;
  }

  if (!args.repoPath || !args.prompt) {
    process.stderr.write("Error: --repo-path and --prompt are required\n");
    process.stderr.write(`${USAGE}\n`);
    return 1;
  }

  try {
    await streamCodex(args);
    return 0;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    process.stderr.write(`Error: ${message}\n`);
    return 1;
  }
}

void run(process.argv.slice(2)).then((exitCode) => {
  process.exit(exitCode);
});
