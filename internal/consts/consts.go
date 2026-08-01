// Package consts centralizes shared string literals and keys to enforce uniformity across modules.
package consts

// Constants below define keys for JSON payloads and CLI operating modes.
const (
	KeyStatus      = "status"
	KeyUserID      = "user_id"
	KeyDeviceID    = "device_id"
	KeyDeviceName  = "device_name"
	KeyAccessToken = "access_token"

	ModeAuth      = "auth"
	ModeBootstrap = "bootstrap"
	ModeListen    = "listen"
	ModeSend      = "send"
	ModeVerify    = "verify"
	ModeRooms     = "rooms"
	ModeRoomInfo  = "room-info"
	ModeDevices   = "devices"
	ModeLogout    = "logout"
)
