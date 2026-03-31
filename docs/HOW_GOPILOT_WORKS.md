# How GoPilot Works

This document explains in more detail how GoPilot works internally, how requests are built, and how typical workflows run in the terminal.

## Overview

GoPilot is a terminal-based coding assistant written in Go. The program has three main areas:

- `cmd/gopilot`: CLI entry point
- `internal/app`: terminal UI, state, commands, sessions, file context, apply/undo
- `internal/gemini`: authentication, project resolution, and streaming against the Gemini Code Assist API

The overall flow is intentionally simple:

1. `gopilot` starts the Bubble Tea application.
2. The app uses the current working directory as the active workspace.
3. Optionally, a saved session is loaded.
4. The user enters prompts or uses slash commands.
5. GoPilot turns that into a `chat.Request`.
6. The Gemini backend authenticates, resolves the associated project, and opens a stream.
7. The response is streamed live into the TUI.
8. If the response contains `gopilot-file` blocks, they can be written to disk with `/apply`.
9. The conversation is continuously saved as a session.

## Startup and Entry Point

The entry point is `cmd/gopilot/main.go`.

- `gopilot` starts the app normally
- `gopilot --version` prints the version
- `gopilot --load <session-id>` loads a saved session directly

After startup, `main.go` calls `app.Run(...)`. That creates the Bubble Tea `model` and passes it into `tea.NewProgram(...)`.

## Core State in `internal/app`

The central structure is `model` in `internal/app/model.go`. It holds almost all runtime state:

- input field and viewport
- rendered messages
- shared chat history for model requests
- current session ID
- selected model
- workspace root
- path to `GOPILOT.md`
- attached files used as context
- stream status and retry state
- undo history for applied file changes
- model and session picker state
- autocomplete state for slash commands

One important detail is the separation between UI history and model history:

- `messages` is what is displayed in the interface
- `sharedHistory` is what is actually sent back to the model

That allows GoPilot to show local status messages and helper text without feeding everything back into the next model request.

## Workspace and `GOPILOT.md`

GoPilot always uses the current working directory as the workspace root.

It also searches for a `GOPILOT.md` file:

- first in the current directory
- then upward through parent directories
- until a `.git` directory is found or the filesystem root is reached

If a `GOPILOT.md` file is found, its contents are appended to the system prompt. This is the right place for repository-specific rules, conventions, and instructions the model should consistently follow.

## How a Prompt Is Processed

When the user submits a normal prompt, the flow looks roughly like this:

1. The user message is added to the visible conversation.
2. A placeholder response such as `Thinking...` is shown.
3. A `chat.Request` is prepared.
4. The selected model, prior conversation, workspace path, and attached files are included in the request.
5. `AllowFileEdits` is enabled so the model may return `gopilot-file` blocks when edits are requested.
6. The backend starts the network stream.
7. Incoming text fragments are buffered and flushed into the UI at short intervals.
8. Once the stream finishes, the final response is saved and the session is updated.

## File Context and Codebase Handling

You can attach file context with `/add <path>` or `/codebase`.

The rules are:

- only files inside the workspace are allowed
- individual files may be at most 256 KB
- obvious binary files are rejected
- directories are traversed recursively
- `.gitignore` is respected
- common build, cache, and tool directories are skipped, such as `.git`, `node_modules`, `dist`, `build`, `target`, `.idea`, and `.vscode`

The attached files are passed to the model as structured context with path, language, and full file contents.

If `/apply` changes files that are already attached, GoPilot automatically refreshes those in-memory attachments so the context stays aligned with the actual workspace state.

## Slash Commands

Slash commands are handled in `internal/app/model.go`. The most important commands are:

- `/add <file>`: attach a file or directory as context
- `/codebase`: attach the entire workspace as context
- `/drop <file>`: remove an attached file
- `/files`: list current attachments
- `/model`: choose a model
- `/load`: load a session
- `/new`: start a new session
- `/clear`: clear the conversation while keeping attached files
- `/apply`: apply proposed file edits from the last response
- `/undo`: revert the last apply operation
- `/undo session`: revert all applied edits from the current session

One easy-to-miss detail: `/add <path> <prompt>` supports an immediate follow-up prompt. That lets you attach context and send the next question in a single step.

## File Edits with `gopilot-file`

GoPilot uses a custom response format for file changes:

    ```gopilot-file path=relative/path/from/workspace
    full file contents
    ```

Important details:

- the model must return the complete file contents
- the path must be relative to the workspace
- changes are not written automatically
- files are only written when you run `/apply`

When applying edits, GoPilot parses all blocks, normalizes paths, and writes the files into the workspace. For each write, it records undo information:

- whether the file existed before
- the previous file contents
- when the apply operation happened

That is what allows `/undo` to restore the previous state precisely.

## Session System

GoPilot stores sessions automatically as JSON files in the user config directory.

Typical locations:

- macOS: `~/Library/Application Support/gopilot/sessions/`
- Linux: `~/.config/gopilot/sessions/`

A session includes, among other things:

- ID
- title
- created and updated timestamps
- workspace root
- active model
- visible messages
- model-facing shared history
- attached files
- undo history

Session titles are derived from the first meaningful user message. Slash commands are intentionally ignored for title generation.

There is also an important safety behavior when loading sessions:

- if a session was created in a different workspace than the current working directory, GoPilot keeps the conversation
- but clears the attached files

That avoids mixing file context from one repository into another workspace by accident.

## Streaming and Status Updates

Responses are processed as streams rather than waiting for a full response first.

During that process the UI may show status steps such as:

- `Authenticating...`
- `Loading project...`
- `Preparing context...`
- `Sending request...`
- `Thinking...`

Text fragments from the stream are first collected in a buffer and then flushed into the interface at short intervals. That reduces flicker and keeps streaming readable.

## Authentication and Gemini Backend

The authentication logic lives in `internal/gemini/auth.go`.

GoPilot uses the Gemini CLI OAuth credentials from:

- `~/.gemini/oauth_creds.json`

The flow is:

1. Load credentials
2. Check whether the access token is still valid
3. Refresh it with the refresh token if needed
4. Save the refreshed credentials back to disk

After that, GoPilot resolves the Code Assist project through the backend endpoints. If required, the user is onboarded into an allowed tier first. Only then is the actual content stream opened.

The target project can also be influenced through these environment variables:

- `GOOGLE_CLOUD_PROJECT`
- `GOOGLE_CLOUD_PROJECT_ID`

The API base URL can be overridden through `GEMINI_API_BASE_URL`.

## Retry Behavior for API Errors

GoPilot automatically retries common temporary failures, including:

- `RESOURCE_EXHAUSTED`
- HTTP `429`
- `503` or `UNAVAILABLE`

The retry delay depends on the error type. The number of automatic retries is limited. While retrying, the UI shows the error message and the next retry timing.

## Model Selection

The currently supported models are defined in `internal/app/models.go`. The active model can be changed in three ways:

- `/model`
- `/model <name>`
- `Ctrl+N` and `Ctrl+P`

The selected model is stored as part of the session.

## Rendering in the TUI

The terminal UI is built on:

- Bubble Tea for the event loop and state model
- Bubbles for input and viewport components
- Lip Gloss for styling
- Chroma for syntax highlighting

The app renders markdown, code blocks, and status output directly in the terminal. There is also a plain view mode that is especially useful for raw code or JSON output.

## Typical Practical Workflow

A normal working cycle looks like this:

1. Start `gopilot` inside a project
2. Attach individual files with `/add` or the whole project with `/codebase`
3. Ask a concrete question or request a change
4. Read the streamed response live
5. Run `/apply` if the assistant returned file blocks you want to accept
6. Use `/undo` if you need to roll the last apply back
7. Resume later with `/load`

## Current Design Limits

A few things are intentionally still simple:

- there is no diff preview before `/apply`
- file editing is based on full file contents rather than patches
- external tools, linters, and tests are not yet integrated directly into the chat loop
- session search exists, but is still relatively lightweight

That simplicity is also why the current codebase stays understandable: the core behavior is small, direct, and easy to extend.