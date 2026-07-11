# matrix-cli

`matrix-cli` is a headless and lightweight Matrix client written in Go, designed to operate from the terminal.

**Primary Use Case:** It acts as an E2EE (End-to-End Encryption) helper tool for external scripts and bots (like router management bots). While unencrypted Matrix rooms can be managed with simple HTTP API requests (e.g., using `curl`), interacting with encrypted rooms requires complex cryptographic state management. `matrix-cli` handles all the heavy lifting of E2EE, authentication, and sync, outputting structured JSON that can be easily parsed and consumed by your external tools.

## Features

Currently, `matrix-cli` supports the following operations:
- **Authentication**: Login to your Matrix account (supports `.well-known` discovery for server URLs).
- **Listening**: Sync and listen for incoming messages and events in real-time. Events are printed to stdout as JSON for easy parsing by external tools.
- **Messaging**: Send text messages to specific encrypted or unencrypted rooms.
- **Verification**: Start an interactive device verification (SAS) flow to support End-to-End Encryption (E2EE).
- **Rooms Management**: List all joined rooms and fetch detailed information about specific rooms (including member counts, power levels, and encryption status) in JSON format.
- **Devices**: List active devices for your account.

This project relies on the excellent [mautrix-go](https://github.com/mautrix/go) library for all Matrix API interactions and cryptographic operations.

## Building from Source

1. Clone the repository:
   ```bash
   git clone https://github.com/underhax/matrix-cli.git
   cd matrix-cli
   ```

2. Compile the binary. **Important:** You must include the `goolm` tag to enable the pure-Go implementation of the Olm and Megolm cryptographic ratchets, which are required for E2EE support:
   ```bash
   go build -tags goolm -o matrix-cli ./cmd/matrix-cli/
   ```

## Usage

`matrix-cli` operates in different modes. You can run `matrix-cli --mode <mode> -h` at any time to see detailed instructions.

### Authentication (`auth`)
```text
Usage: matrix-cli --mode auth --server <DOMAIN_OR_URL> [--user <ID>] [--pass <PASSWORD>] [--sso-callback-port <PORT>] [--device <NAME>] [--data-dir <PATH>]
Login to Matrix and save session. Supports both SSO/OAuth and password login.

Examples:
  # Auto-discover API URL and use SSO or prompt interactively (recommended):
  matrix-cli --mode auth --server 'matrix.org'

  # Specify exact HTTPS URL and force password login:
  matrix-cli --mode auth --server 'https://synapse.example.com' --user '@bot:example.com' --pass 's3cret'

  # Use SSO with a specific callback port:
  matrix-cli --mode auth --server 'matrix.example.com' --sso-callback-port 8080
```

### Key Bootstrap (`bootstrap`)
```text
Usage: matrix-cli --mode bootstrap [--new-keys] [--recovery-key <KEY_STRING>] [--data-dir <PATH>]
Initialize cross-signing keys for the current session.

Examples:
  # Interactively prompt for recovery key (secure and recommended):
  matrix-cli --mode bootstrap

  # Generate new keys (may prompt for password depending on UIA):
  matrix-cli --mode bootstrap --new-keys

  # Load keys explicitly (pass the actual 48-character string, not a file path):
  matrix-cli --mode bootstrap --recovery-key 'XXXX-XXXX-XXXX-XXXX'
```

### Sending Messages (`send`)
```text
Usage: matrix-cli --mode send --rooms "<ID>" --message "<TEXT>" [--data-dir <PATH>]
Send a message to one or more rooms.

Examples:
  matrix-cli --mode send --rooms '!abc123:matrix.org' --message 'Hello!'
  matrix-cli --mode send --rooms '!abc123:matrix.org !def456:matrix.org' --message 'Broadcast!'
```

### Listening for Events (`listen`)
```text
Usage: matrix-cli --mode listen [--rooms "<ID1> <ID2>"] [--data-dir <PATH>]
Listen for incoming messages and events. If --rooms is provided, only events from those rooms are processed.

Examples:
  matrix-cli --mode listen
  matrix-cli --mode listen --rooms '!abc123:matrix.org !def456:matrix.org'
```

### Fetching Room Information (`rooms` & `room-info`)
```text
Usage: matrix-cli --mode rooms [--verbose] [--data-dir <PATH>]
List joined rooms. Use --verbose to fetch name, topic, and alias for each room.

Examples:
  matrix-cli --mode rooms
  matrix-cli --mode rooms --verbose
```
```text
Usage: matrix-cli --mode room-info --rooms "<ID>" [--data-dir <PATH>]
Get detailed info for specific room(s).

Examples:
  matrix-cli --mode room-info --rooms '!abc123:matrix.org'
  matrix-cli --mode room-info --rooms '!abc123:matrix.org !abc123defg456'
```

### Device Verification (`verify`)
```text
Usage: matrix-cli --mode verify [--data-dir <PATH>]
Start an interactive device verification (SAS) flow.

Examples:
  matrix-cli --mode verify
```

### Logging Out (`logout`)
```text
Usage: matrix-cli --mode logout [--data-dir <PATH>]
Logout from the homeserver and delete the local session and database.

Examples:
  matrix-cli --mode logout
```

### Global Options
All commands support the `-data-dir` flag to specify where the session and database files are stored. By default, this points to your OS's configuration directory (e.g., `~/.config/matrix-cli`).
