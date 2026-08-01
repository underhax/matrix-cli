package main

import (
	"fmt"
	"os"

	"github.com/underhax/matrix-cli/internal/consts"
)

func printUsage(modeVal string) {
	switch modeVal {
	case consts.ModeAuth:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode auth --server <DOMAIN_OR_URL> [--user <ID>] [--pass <PASSWORD>] [--sso-callback-port <PORT>] [--device <NAME>] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Login to Matrix and save session. Supports both SSO/OAuth and password login.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Auto-discover API URL and use SSO or prompt interactively (recommended):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode auth --server 'matrix.org'\n\n")
		fmt.Fprintf(os.Stderr, "  # Specify exact HTTPS URL and force password login:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode auth --server 'https://synapse.example.com' --user '@bot:example.com' --pass 's3cret'\n\n")
		fmt.Fprintf(os.Stderr, "  # Use SSO with a specific callback port:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode auth --server 'matrix.example.com' --sso-callback-port 8080\n")
	case consts.ModeBootstrap:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode bootstrap [--new-keys] [--recovery-key <KEY_STRING>] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Initialize cross-signing keys for the current session.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Interactively prompt for recovery key (secure and recommended):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode bootstrap\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate new keys (may prompt for password depending on UIA):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode bootstrap --new-keys\n\n")
		fmt.Fprintf(os.Stderr, "  # Load keys explicitly (pass the actual 48-character string, not a file path):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode bootstrap --recovery-key 'XXXX-XXXX-XXXX-XXXX'\n")
	case consts.ModeListen:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode listen [--rooms '<ID1> <ID2>'] [--json] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Listen for incoming messages and events. If --rooms is provided, only events from those rooms are processed.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode listen\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode listen --rooms '!room1:example.com !opaque-v12_roomid'\n\n")
		fmt.Fprintf(os.Stderr, "  # Listen for incoming events and stream output as JSON:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode listen --json\n")
	case consts.ModeSend:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode send --rooms '<ID>' --message '<TEXT>' [--html | --markdown] [--json] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Send a message to one or more rooms. Supports optional HTML or Markdown formatting.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Send a standard text message:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode send --rooms '!room1:example.com' --message 'Hello world!'\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode send --rooms '!room1:example.com !opaque-v12_roomid' --message 'Broadcast!'\n\n")
		fmt.Fprintf(os.Stderr, "  # Send a message parsed as Markdown:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode send --rooms '!room1:example.com' --markdown --message '**Hello** from *Matrix CLI*!'\n\n")
		fmt.Fprintf(os.Stderr, "  # Send a message with raw HTML tags:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode send --rooms '!room1:example.com' --html --message '<b>Hello</b> from <i>Matrix CLI</i>!'\n\n")
		fmt.Fprintf(os.Stderr, "  # Send a message and output results in JSON format:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode send --rooms '!room1:example.com !opaque-v12_roomid' --message 'Hello' --json\n")
	case consts.ModeVerify:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode verify [--user <@user:example.com>] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Start an interactive device verification (SAS) flow.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Wait for incoming verification requests:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode verify\n\n")
		fmt.Fprintf(os.Stderr, "  # Initiate verification with another user (or your own devices):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode verify --user '@bob:example.com'\n")
	case consts.ModeRooms:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode rooms [--verbose] [--json] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "List joined rooms.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode rooms\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode rooms --verbose --data-dir ./Data\n\n")
		fmt.Fprintf(os.Stderr, "  # Get rooms as a JSON array:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode rooms --json\n")
	case consts.ModeRoomInfo:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode room-info --rooms '<ID>' [--json] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Get detailed info for specific room(s).\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode room-info --rooms '!room1:example.com !opaque-v12_roomid'\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode room-info --rooms '!opaque-v12_roomid'\n\n")
		fmt.Fprintf(os.Stderr, "  # Output detailed info in JSON format:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode room-info --rooms '!opaque-v12_roomid' --json\n")
	case consts.ModeDevices:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode devices [--json] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "List active devices for the account.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode devices\n\n")
		fmt.Fprintf(os.Stderr, "  # Output active devices in JSON format:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode devices --json\n")
	case consts.ModeLogout:
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
	fmt.Fprintf(os.Stderr, "  matrix-cli --mode <mode> [options]\n")
	fmt.Fprintf(os.Stderr, "  matrix-cli update\n\n")
	fmt.Fprintf(os.Stderr, "Modes:\n")
	printModeList("")
	fmt.Fprintf(os.Stderr, "Tip: Run 'matrix-cli --mode <mode> -h' for mode-specific help.\n\n")
	fmt.Fprintf(os.Stderr, "Global Options:\n")
	fmt.Fprintf(os.Stderr, "  --json            JSON output (modes: listen, send, rooms, room-info, devices)\n")
	fmt.Fprintf(os.Stderr, "  -data-dir string  Directory to store session and database files (default %q)\n", getDefaultDataDir())
}

func printUsageFooter(exclude string) {
	fmt.Fprintf(os.Stderr, "\nOther commands:\n")
	fmt.Fprintf(os.Stderr, "  update     Update the client to the latest version\n")
	fmt.Fprintf(os.Stderr, "  version    Print the client version\n")
	fmt.Fprintf(os.Stderr, "\nOther modes:\n")
	printModeList(exclude)
	fmt.Fprintf(os.Stderr, "Global Options:\n")
	fmt.Fprintf(os.Stderr, "  --json            JSON output (modes: listen, send, rooms, room-info, devices)\n")
	fmt.Fprintf(os.Stderr, "  -data-dir string  Directory to store session and database files (default %q)\n", getDefaultDataDir())
}

func printModeList(exclude string) {
	modes := []struct {
		name string
		desc string
	}{
		{consts.ModeAuth, "Login to Matrix and save session"},
		{consts.ModeBootstrap, "Initialize cross-signing keys (generate new or import from SSSS)"},
		{consts.ModeListen, "Listen for incoming messages and events"},
		{consts.ModeSend, "Send a message to a room"},
		{consts.ModeVerify, "Start an interactive device verification (SAS) flow"},
		{consts.ModeRooms, "List joined rooms"},
		{consts.ModeRoomInfo, "Get detailed info for a specific room"},
		{consts.ModeDevices, "List active devices for the account"},
		{consts.ModeLogout, "Logout and clear local session"},
	}
	for _, m := range modes {
		if m.name != exclude {
			fmt.Fprintf(os.Stderr, "  %-10s %s\n", m.name, m.desc)
		}
	}
	fmt.Fprintln(os.Stderr)
}
