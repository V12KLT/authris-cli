# authris CLI

Talk to your Authris server from a terminal, run the coding agent on your project, or bridge local-only AI agents to it over MCP (stdio). Standard library only, no dependencies. Account actions run on the server under your token; project file tools run locally in the directory you launch from, fenced in by permissions you control.

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

`authris ai` (or just `authris`) is a coding agent for your project. It reads
and edits files where you run it, runs install and test commands, and manages
your Authris account through the same assistant as the dashboard. Reads run
freely and writes, edits, and commands run without asking unless you
restrict them. It also runs your build and tests itself to verify changes.
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

The CLI checks for a new release once a day and tells you when one lands:

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
