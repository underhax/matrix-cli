# matrix-cli

`matrix-cli` is a headless and lightweight Matrix client written in Go, designed to operate from the terminal.

**Primary Use Case:** It acts as an E2EE (End-to-End Encryption) helper tool for external scripts and bots (like router management bots). While unencrypted Matrix rooms can be managed with simple HTTP API requests (e.g., using `curl`), interacting with encrypted rooms requires complex cryptographic state management. `matrix-cli` handles all the heavy lifting of E2EE, authentication, and sync, outputting structured JSON that can be easily parsed and consumed by your external tools.

## Features

Currently, `matrix-cli` supports the following operations:
- **Authentication**: Login to your Matrix account using Password or SSO/OAuth (supports `.well-known` discovery for server URLs).
- **Key Bootstrap**: Initialize and manage cross-signing keys for the session (required for E2EE). Can also be used to verify a new device if you have a recovery key.
- **Listening**: Sync and listen for incoming messages and events in real-time. Events are printed to stdout as JSON for easy parsing by external tools.
- **Messaging**: Send text messages to specific encrypted or unencrypted rooms.
- **Verification**: Start an interactive device verification (SAS) flow to support E2EE.
- **Rooms Management**: List all joined rooms and fetch detailed information about specific rooms (including member counts, power levels, and encryption status) in JSON format.
- **Devices**: List active devices for your account.

This project relies on the excellent [mautrix-go](https://github.com/mautrix/go) library for all Matrix API interactions and cryptographic operations.

## Installation

You can download the pre-compiled binary for your architecture (Linux, macOS, Windows) directly from the [Releases](https://github.com/underhax/matrix-cli/releases) page. The binaries are packed as `.tar.gz` (for Linux/macOS) or `.zip` (for Windows).

Below is an example of how to manually download, extract, and install the Linux AMD64 binary. You can modify the variables for your specific OS/Architecture:

```bash
# 1. Set the variables for your target system
REPO="underhax/matrix-cli"
VERSION=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | head -n 1 | awk -F'"' '{print $4}')
ASSET_NAME="matrix-cli-linux-amd64.tar.gz"
INSTALL_DIR="/usr/local/bin"

# 2. Download the archive
curl -sSL -O "https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"

# 3. Extract only the binary from the archive
tar -xzf "${ASSET_NAME}" matrix-cli

# 4. Install the binary (requires sudo if moving to /usr/local/bin)
sudo mv matrix-cli "${INSTALL_DIR}/"

# 5. Clean up
rm "${ASSET_NAME}"
```

It is recommended to place the binary in your system's PATH (such as `/usr/local/bin`), but you can place it in any directory you prefer. Once installed, refer to the [Usage](#usage) section below to get started.

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
  matrix-cli --mode listen 2>/dev/null
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
Usage: matrix-cli --mode verify [--user <@user:example.com>] [--data-dir <PATH>]
Start an interactive device verification (SAS) flow.

Examples:
  # Wait for incoming verification requests:
  matrix-cli --mode verify

  # Initiate verification with another user (or your own devices):
  matrix-cli --mode verify --user '@bob:example.com'
```

### Logging Out (`logout`)
```text
Usage: matrix-cli --mode logout [--data-dir <PATH>]
Logout from the homeserver and delete the local session and database.

Examples:
  matrix-cli --mode logout
```

### Global Options
All commands support the `--data-dir` flag to specify where the session and database files are stored. By default, this points to your OS's configuration directory (e.g., `~/.config/matrix-cli`).

## Docker Deployment

<details>
<summary><b>📦 Docker Deployment & SSH Gateway (Click to expand)</b></summary>

A minimal, hardened Docker container is available via GHCR (`ghcr.io/underhax/matrix-cli`). It runs `matrix-cli` behind an SSH reverse tunnel, acting as a secure Matrix protocol gateway for remote clients (e.g. routers or IoT devices).

### Architecture
```
[Remote client]
    |
    | SSH reverse tunnel  (-R 2222:localhost:2222)
    v
[Host: 127.0.0.1:2222]
    |
    | Docker port mapping
    v
[matrix-cli container :2222]
    |
    | sshd — PubkeyAuthentication only, UsePAM=no
    v
[matrix-cli interactive session / scripts]
```
The container does not bind to any public interface. All external access is funnelled exclusively through the authenticated SSH tunnel established by the remote client.

### Security Posture

| Control | Status |
|---|---|
| Runs as UID 10001 (non-root) | ✅ |
| `no-new-privileges` | ✅ |
| All Linux capabilities dropped | ✅ |
| Read-only root filesystem | ✅ |
| `/run` and `/tmp` as `noexec`/`nosuid` tmpfs | ✅ |
| PAM disabled (`UsePAM=no`) | ✅ |
| SSH password authentication disabled | ✅ |
| SSH root login disabled | ✅ |
| SSH `MaxAuthTries 3`, `LoginGraceTime 20` | ✅ |
| SSH X11 forwarding, PermitTunnel, GatewayPorts disabled | ✅ |
| Port bound to `127.0.0.1` only | ✅ |
| Multi-arch binary (`amd64` / `arm64`) | ✅ |

> **Note on `UsePAM=no`:** sshd invokes the PAM helper `unix_chkpwd` (a setuid binary) even when password authentication is disabled. This conflicts with `no-new-privileges:true`. Since PAM provides no value in a pubkey-only, single-user container, it is explicitly disabled, which resolves the conflict without sacrificing any meaningful security control.

### Setup

To make installation simple, set your preferred base directory as a variable and run the initial setup commands on the Docker host:

```bash
# 1. Define your base directory (change this path to your preference)
export BASE_DIR="/home/user/matrix-cli"

# 2. Create directories and download the compose file
mkdir -p "${BASE_DIR}/bot_data" "${BASE_DIR}/host_keys"
curl -sSL -o "${BASE_DIR}/docker-compose.yaml" https://raw.githubusercontent.com/underhax/matrix-cli/main/docker/docker-compose.yaml

# 3. Set strict ownership for the container (UID/GID 10001)
# All read-write volume mount points on the host must be owned by UID 10001,
# or sshd's StrictModes check will refuse to start.
sudo chown -R 10001:10001 "${BASE_DIR}/bot_data" "${BASE_DIR}/host_keys"
```

#### 4. Install the remote client's public key
Copy the contents of the public SSH key from your remote client (e.g., your router or IoT device). Then, back on the Docker host, open the `authorized_keys` file:
```bash
nano "${BASE_DIR}/authorized_keys"
```
Paste the public key into the file, save, and exit. Then secure the file permissions:
```bash
sudo chown 10001:10001 "${BASE_DIR}/authorized_keys"
sudo chmod 0600 "${BASE_DIR}/authorized_keys"
```

#### 5. Start the container
Now you can start the gateway service in the background:
```bash
docker compose -f "${BASE_DIR}/docker-compose.yaml" up -d
```

### Initial Matrix session login and verification
On first run, `matrix-cli` requires interactive login to create its session database. Start a shell inside the container:
```bash
# Before running this command:
# 1. Create a dedicated Matrix account for the bot on your homeserver.
#    Do NOT use your personal account — the bot account will be used
#    for automated messaging and device verification.

# Use the same BASE_DIR variable from the setup step
docker compose -f "${BASE_DIR}/docker-compose.yaml" exec -it matrix-cli /bin/sh
```
Once inside the container shell, refer to the **[Usage](#usage)** section above to:
1. Safely authenticate your new bot account.
2. Setup E2EE cross-signing by either:
   - Generating new keys (`bootstrap`) — *Use this only if this is a brand new account with no prior devices or keys.*
   - Importing an existing recovery key (`bootstrap`) — *The recommended method if the bot already has an established account.*
   - Performing interactive device verification with emojis (`verify`) — *Alternative method to verify your own devices (if you have another session open) or to verify other users' accounts.*

After authentication and verification are complete, exit the shell (`exit`).

### Restart the service
Once the session is initialised and verification is complete, restart the container in its normal operating mode — without an interactive shell:

```bash
docker compose -f "${BASE_DIR}/docker-compose.yaml" down
docker compose -f "${BASE_DIR}/docker-compose.yaml" up -d
```

The container will now run as a persistent background service, ready to accept incoming tunnel connections from the remote client.

### Volume Reference
| Host path | Container path | Mode | Ownership requirement |
|---|---|---|---|
| `./authorized_keys` | `/home/bot/.ssh/authorized_keys` | `ro` | `chown 10001:10001`, `chmod 0600` |
| `./bot_data/` | `/home/bot/data` | `rw` | `chown -R 10001:10001` |
| `./host_keys/` | `/home/bot/keys` | `rw` | `chown -R 10001:10001` |

### SSH Host Key Persistence
On first startup, the entrypoint generates an SSH host key and persists it to the host at `${BASE_DIR}/host_keys/ssh_host_ed25519_key`. This prevents host key churn across container restarts, which would otherwise produce `REMOTE HOST IDENTIFICATION HAS CHANGED` warnings on the connecting client.

To rotate the host key intentionally:
```bash
docker compose -f "${BASE_DIR}/docker-compose.yaml" down
sudo rm "${BASE_DIR}/host_keys/ssh_host_ed25519_key"
docker compose -f "${BASE_DIR}/docker-compose.yaml" up -d
```

</details>
