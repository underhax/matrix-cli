package main

import (
	"fmt"
	"os"
)

func printUsage(modeVal string) {
	switch modeVal {
	case modeAuth:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode auth --server <DOMAIN_OR_URL> [--user <ID>] [--pass <PASSWORD>] [--sso-callback-port <PORT>] [--device <NAME>] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Login to Matrix and save session. Supports both SSO/OAuth and password login.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Auto-discover API URL and use SSO or prompt interactively (recommended):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode auth --server 'matrix.org'\n\n")
		fmt.Fprintf(os.Stderr, "  # Specify exact HTTPS URL and force password login:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode auth --server 'https://synapse.example.com' --user '@bot:example.com' --pass 's3cret'\n\n")
		fmt.Fprintf(os.Stderr, "  # Use SSO with a specific callback port:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode auth --server 'matrix.example.com' --sso-callback-port 8080\n")
	case modeBootstrap:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode bootstrap [--new-keys] [--recovery-key <KEY_STRING>] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Initialize cross-signing keys for the current session.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Interactively prompt for recovery key (secure and recommended):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode bootstrap\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate new keys (may prompt for password depending on UIA):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode bootstrap --new-keys\n\n")
		fmt.Fprintf(os.Stderr, "  # Load keys explicitly (pass the actual 48-character string, not a file path):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode bootstrap --recovery-key 'XXXX-XXXX-XXXX-XXXX'\n")
	case modeListen:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode listen [--rooms \"<ID1> <ID2>\"] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Listen for incoming messages and events. If --rooms is provided, only events from those rooms are processed.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode listen\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode listen --rooms \"!room1:example.com !room2v12\"\n")
	case modeSend:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode send --rooms \"<ID>\" --message \"<TEXT>\" [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Send a message to one or more rooms.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode send --rooms \"!room1:example.com\" --message \"Hello world!\"\n")
	case modeVerify:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode verify [--user <@user:example.com>] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Start an interactive device verification (SAS) flow.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Wait for incoming verification requests:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode verify\n\n")
		fmt.Fprintf(os.Stderr, "  # Initiate verification with another user (or your own devices):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode verify --user '@bob:example.com'\n")
	case modeRooms:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode rooms [--verbose] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "List joined rooms.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode rooms\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode rooms --verbose --data-dir ./Data\n")
	case modeRoomInfo:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode room-info --rooms \"<ID>\" [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Get detailed info for specific room(s).\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode room-info --rooms \"!room1:example.com !room2v12\"\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode room-info --rooms \"!room3v12\"\n")
	case modeDevices:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode devices [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "List active devices for the account.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode devices\n")
	case modeLogout:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode logout [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Logout from the homeserver and delete the local session and database.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode logout\n")
	default:
		printGlobalUsage()
		return
	}
	printUsageFooter(modeVal)
}

func printGlobalUsage() {
	fmt.Fprintf(os.Stderr, "matrix-cli - A headless Matrix client (%s)\n\n", AppVersion)
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  matrix-cli --mode <mode> [options]\n\n")
	fmt.Fprintf(os.Stderr, "Modes:\n")
	printModeList("")
	fmt.Fprintf(os.Stderr, "Tip: Run 'matrix-cli --mode <mode> -h' for mode-specific help.\n\n")
	fmt.Fprintf(os.Stderr, "Global Options:\n")
	fmt.Fprintf(os.Stderr, "  -data-dir string\n")
	fmt.Fprintf(os.Stderr, "        Directory to store session and database files (default %q)\n", getDefaultDataDir())
}

func printUsageFooter(exclude string) {
	fmt.Fprintf(os.Stderr, "\nOther modes:\n")
	printModeList(exclude)
	fmt.Fprintf(os.Stderr, "Global Options:\n")
	fmt.Fprintf(os.Stderr, "  -data-dir string\n")
	fmt.Fprintf(os.Stderr, "        Directory to store session and database files (default %q)\n", getDefaultDataDir())
}

func printModeList(exclude string) {
	modes := []struct {
		name string
		desc string
	}{
		{modeAuth, "Login to Matrix and save session"},
		{modeBootstrap, "Initialize cross-signing keys (generate new or import from SSSS)"},
		{modeListen, "Listen for incoming messages and events"},
		{modeSend, "Send a message to a room"},
		{modeVerify, "Start an interactive device verification (SAS) flow"},
		{modeRooms, "List joined rooms"},
		{modeRoomInfo, "Get detailed info for a specific room"},
		{modeDevices, "List active devices for the account"},
		{modeLogout, "Logout and clear local session"},
	}
	for _, m := range modes {
		if m.name != exclude {
			fmt.Fprintf(os.Stderr, "  %-10s %s\n", m.name, m.desc)
		}
	}
	fmt.Fprintln(os.Stderr)
}
