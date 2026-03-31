# GoPilot

A experimental terminal-based AI coding assistant built with Goland powered by Google Gemini.

```
   ______      ____  _ __      __
  / ____/___  / __ \(_) /___  / /_
 / / __/ __ \/ /_/ / / / __ \/ __/
/ /_/ / /_/ / ____/ / / /_/ / /_
\____/\____/_/   /_/_/\____/\__/
```

## Features

- **Stream responses** — Real-time streaming from Gemini models with live markdown rendering
- **File context** — Attach workspace files for code-aware conversations (respects `.gitignore`)
- **File editing** — AI proposes edits in `gopilot-file` blocks, apply with `/apply`, revert with `/undo`
- **Project instructions** — Commit a `GOPILOT.md` file to define repository-specific AI rules and context
- **Session persistence** — Automatic session save/load with structured and searchable history
- **Multiple models** — Switch between Gemini models on the fly
- **Smart autocomplete** — Tab-completion for commands, file paths, and model names
- **Rich code highlighting** — Chroma-powered syntax highlighting for code blocks, inline code, and markdown
- **Copy to clipboard** — Copy full responses or specific code blocks
- **Exponential backoff** — Automatic retry on rate limits (429) with jitter

## Screenshot
<p align="center">
  <img src="./docs/screenshot.png" alt="Terminal-UI" />
</p>


## Prerequisites

GoPilot uses Google's Gemini API through OAuth credentials. You need:

1. **Google AI Pro membership** (or access to the Gemini API)
2. **OAuth credentials** from the [Gemini CLI](https://github.com/google-gemini/gemini-cli)

### Setting up credentials

Install and authenticate with the Gemini CLI first:

```bash
npx https://github.com/google-gemini/gemini-cli
```

This creates the OAuth credentials at `~/.gemini/oauth_creds.json` that GoPilot uses.

## Installation

### With Homebrew

```bash
brew install JakobAIOdev/tap/gopilot
```

Then run:

```bash
gopilot
```

### From source

```bash
git clone https://github.com/JakobAIOdev/GoPilot.git
cd GoPilot
make build
```

### With Go

```bash
go install github.com/JakobAIOdev/GoPilot/cmd/gopilot@latest
```

If your Go bin directory is not on your `PATH` yet:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Usage

Run from your project directory:

```bash
gopilot
```

GoPilot uses the current working directory as the workspace root. Attach files, ask questions, and let the AI suggest code changes.
If the repository contains a `GOPILOT.md` file, GoPilot automatically loads it and appends its contents to the model's system instructions for that workspace.

## How It Works

GoPilot is split into two main runtime layers:

- `internal/app` handles the terminal UI, slash commands, sessions, file attachments, apply/undo, and markdown rendering
- `internal/gemini` handles OAuth, Gemini Code Assist project resolution, request construction, retries, and streaming responses

At runtime the flow looks like this:

1. `gopilot` starts the Bubble Tea app
2. the current working directory becomes the workspace root
3. optional repository instructions are loaded from `GOPILOT.md`
4. attached files are packed into workspace context
5. prompts are sent to Gemini with conversation history plus file context
6. responses stream back live into the TUI
7. when the model returns `gopilot-file` blocks, `/apply` can write them to disk
8. every conversation is saved as a resumable session

For a more detailed explanation of architecture, request flow, sessions, context loading, and file editing, see [docs/HOW_GOPILOT_WORKS.md](./docs/HOW_GOPILOT_WORKS.md).

### CLI Flags

| Flag | Description |
|------|-------------|
| `--version`, `-v` | Print the current GoPilot version |
| `--load <session-id>` | Open GoPilot and preload a saved session |

Example:

```bash
gopilot --load 20260331-182708-d81dca6c
```

### Quick Start

1. Start GoPilot in your project: `gopilot`
2. Attach relevant files: `/add main.go`
3. Or attach the whole codebase: `/codebase`
4. Ask your question: `What does this code do?`
5. If the AI suggests edits: `/apply` to accept, `/undo` to revert

### Typical Workflow

GoPilot is designed around an explicit review-and-apply loop:

1. Start in the repository you want to work on
2. Attach only the files you want the model to inspect, or use `/codebase`
3. Ask for analysis, refactors, fixes, or new code
4. Read the streamed answer directly in the terminal
5. If the answer contains `gopilot-file` blocks, apply them with `/apply`
6. If needed, revert the last apply with `/undo`
7. Resume later with `/load` or `gopilot --load <session-id>`

This means GoPilot does not silently edit your files. The model proposes full file contents first, and the actual write only happens when you explicitly run `/apply`.

## Commands

| Command | Description |
|---------|-------------|
| `/add <file>` | Attach a file or directory as context |
| `/apply` | Apply proposed file edits from the last response |
| `/clear` | Reset conversation (keeps attached files) |
| `/codebase` | Attach entire working directory |
| `/copy` | Copy last response to clipboard |
| `/copy code` | Copy all code blocks from last response |
| `/copy code N` | Copy the Nth code block |
| `/delete <id>` | Delete a saved session |
| `/delete all` | Delete all saved sessions |
| `/drop <file>` | Remove an attached file |
| `/files` | List attached files |
| `/help` | Show available commands and shortcuts |
| `/load` | Open session browser |
| `/load <id>` | Load a specific session |
| `/model` | Open model selector |
| `/model <name>` | Switch to a specific model |
| `/new` | Start a new session |
| `/plain` | Show last response as plain text |
| `/sessions` | List saved sessions |
| `/undo` | Revert last applied edits |
| `/undo session` | Revert all edits from this session |

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Enter` | Send prompt |
| `Tab` | Autocomplete commands, paths, models |
| `Ctrl+N` / `Ctrl+P` | Cycle models forward / backward |
| `Up` / `Down` | Navigate completions or scroll |
| `Esc` | Quit (or exit current menu) |

## Supported Models

| Model | Description |
|-------|-------------|
| `gemini-3-flash-preview` | Latest flash model (default) |
| `gemini-3.1-pro-preview` | Latest pro model |
| `gemini-2.5-flash` | Stable flash model |
| `gemini-2.5-flash-lite` | Lightweight flash model |
| `gemini-2.5-pro` | Stable pro model |

## Configuration

GoPilot uses environment variables for optional configuration:

| Variable | Default | Description |
|----------|---------|-------------|
| `GEMINI_API_BASE_URL` | `https://cloudcode-pa.googleapis.com/v1internal` | API endpoint |
| `GOOGLE_CLOUD_PROJECT` | empty | Optional project hint for Gemini Code Assist |
| `GOOGLE_CLOUD_PROJECT_ID` | empty | Alternative project hint for Gemini Code Assist |

Sessions are stored in the OS user config directory, for example:

- macOS: `~/Library/Application Support/gopilot/sessions/`
- Linux: `~/.config/gopilot/sessions/`

## Project Structure

```
GoPilot/
├── cmd/gopilot/          # Entry point
├── internal/
│   ├── app/              # TUI application (Bubble Tea)
│   │   ├── model.go      # Main state & slash commands
│   │   ├── update.go     # Event loop & stream handling
│   │   ├── render.go     # Markdown rendering & layout
│   │   ├── styles.go     # Lipgloss styles
│   │   ├── helpers.go    # File editing, autocomplete, context
│   │   ├── sessions.go   # Session persistence
│   │   └── models.go     # Available model list
│   ├── chat/             # Shared types (Message, Request, Backend)
│   └── gemini/           # Gemini API client & auth
│       ├── backend.go    # Stream, context, retry logic
│       └── auth.go       # OAuth token management
├── Makefile
└── go.mod
```

## Sessions, Context, and File Edits

### Sessions

GoPilot automatically stores sessions as JSON files and can reopen them later with `/load` or `--load`.

Each saved session includes:

- visible chat messages
- model-facing shared history
- attached files
- current model
- workspace root
- undo history for applied edits

If a session is loaded from a different workspace than the current directory, GoPilot keeps the conversation but clears attached files to avoid mixing file context across repositories.

### File Context

Attached files are sent as structured workspace context:

- path relative to the workspace root
- detected language
- full file contents

When attaching directories, GoPilot walks them recursively, respects `.gitignore`, skips common generated/tool directories, rejects binary files, and ignores files larger than 256 KB.

### `gopilot-file` edit blocks

When the user asks for file creation or modification, GoPilot instructs the model to return edits like this:

    ```gopilot-file path=relative/path/from/workspace
    full file contents here
    ```

Important details:

- the path must be relative to the workspace
- the content must be the complete file, not a patch
- nothing is written automatically
- `/apply` writes the files
- `/undo` restores the previous state

That makes the editing flow explicit and reversible.

## Built With

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — Terminal UI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) — TUI components
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — Style definitions
- [Chroma](https://github.com/alecthomas/chroma) — Syntax highlighting

## Roadmap

- **MCP Servers Integration** — Direct support for Model Context Protocol (MCP) servers to give the AI access to local tools and resources.
- **Enhanced Linter Integration** — Automatic detection and feeding of workspace compilation errors back to the model.
- **Startup Context Flags** — Launch with preloaded files, codebase context, or a preferred model directly from the CLI.
- **Session Resume UX** — Better exit summaries, recent-session shortcuts, and one-key resume flows.
- **Project Diagnostics Panel** — Surface build, test, and lint failures in the UI without leaving the chat loop.
- **Diff Review Before Apply** — Preview proposed file edits as diffs before writing them to disk.
- **Searchable Session Picker** — Faster session recovery with richer filters, previews, and sorting.
- **Repo-Aware Instructions** — Support instruction layering such as personal defaults plus per-repository `GOPILOT.md`.

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes (`git commit -am 'Add my feature'`)
4. Push to the branch (`git push origin feature/my-feature`)
5. Open a Pull Request

## License

This project is open source. See the repository for license details.

## Acknowledgments

Inspired by:
- [gmn](https://github.com/tomohiro-owada/gmn) — Terminal Gemini client
- [gemini-cli](https://github.com/google-gemini/gemini-cli) — Official Gemini CLI
