# authris CLI

Install and integrate the Authris key system from a terminal, or bridge local-only AI agents to it over MCP (stdio). Standard library only, no dependencies. Account actions run on the server under your token; project file tools run locally in the directory you launch from, fenced in by permissions you control.

## Install

Windows (PowerShell):

```
irm https://raw.githubusercontent.com/V12KLT/authris-cli/main/install.ps1 | iex
```

Linux or macOS:

```
curl -fsSL https://raw.githubusercontent.com/V12KLT/authris-cli/main/install.sh | sh
```

## Build

```
go build -o authris .
```

## Use

```
authris login --server https://your-server
authris
authris status
authris keys
authris keys myproject
authris call project_stats '{"project":"myproject"}'
authris ai "how many keys are left?"
authris ai
authris logout
```

Tokens come from Dashboard > AI Access > Personal tokens. They are stored with `0600` permissions under your OS config dir (`~/.config/authris/credentials.json` on Linux).

## Agent

`authris ai` (or just `authris`) is a focused key-system installer. It reads
and edits files where you run it, installs the Authris SDK, and runs focused
build and test commands to verify licensing integration. It refuses unrelated
coding and UI work. Reads run freely and writes, edits, and commands run
without asking unless you restrict them.
The first run asks you to accept the agent terms.

```
authris ai "add authris licensing to this project"
authris ai --effort high "wire up license checks"
authris ai
```

In interactive mode, `/effort low|medium|high` sets reasoning effort, `/perms`
shows tool permissions, and `/quit` exits.

## Permissions

```
authris perms                          show every tool and setting
authris perms set exec deny            never run commands
authris perms set fs_write ask         ask before writing files
authris perms effort high              default reasoning effort
authris perms autoupdate off           disable update checks
```

Settings live in `~/.config/authris/config.json` next to your login.

## Updates

The CLI checks for a new release once a day and installs it automatically when
automatic updates are enabled:

```
authris update
authris version
```

## As a local MCP server

After login, point any stdio-capable agent at the bridge:

```
authris mcp
```

Every JSON-RPC line on stdin is forwarded to the server `/mcp` endpoint with your saved token and the response is written to stdout.
