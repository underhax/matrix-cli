package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/id"
)

type infoTestCase struct {
	jsonMarshalErr    error
	name              string
	endpointPath      string
	httpBody          string
	roomsStr          string
	expectErrContains string
	httpStatus        int
	stdoutErrNum      int
	verbose           bool
	expectErr         bool
}

func setupInfoMockServer(t *testing.T, tt *infoTestCase) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/joined_rooms"):
			writeMockResp(t, w, tt.httpStatus, tt.httpBody)
		case strings.Contains(r.URL.Path, "/devices"):
			writeMockResp(t, w, tt.httpStatus, tt.httpBody)
		case strings.Contains(r.URL.Path, "/state/m.room.name"):
			writeMockResp(t, w, 200, `{"name":"test room name"}`)
		case strings.Contains(r.URL.Path, "/state/m.room.canonical_alias"):
			writeMockResp(t, w, 200, `{"alias":"#testalias:example.com"}`)
		case strings.Contains(r.URL.Path, "/state/m.room.topic"):
			writeMockResp(t, w, 200, `{"topic":"test room topic"}`)
		case strings.Contains(r.URL.Path, "/state/m.room.encryption"):
			writeMockResp(t, w, 200, `{"algorithm":"m.megolm.v1.aes-sha2"}`)
		case strings.Contains(r.URL.Path, "/state/m.room.create"):
			writeMockResp(t, w, 200, `{"type":"m.room.create","sender":"@creator:example.com","content":{"creator":"@creator:example.com","room_version":"9"}}`)
		case strings.Contains(r.URL.Path, "/state/m.room.power_levels"):
			if tt.endpointPath == "mock_pl_error" {
				writeMockResp(t, w, 500, `{"errcode":"M_UNKNOWN","error":"power error"}`)
			} else {
				writeMockResp(t, w, 200, `{"users":{"@admin:example.com":100,"@mod:example.com":50,"@priv:example.com":25,"@user:example.com":0}}`)
			}
		case strings.Contains(r.URL.Path, "/joined_members"):
			if tt.endpointPath == "mock_jm_error" {
				writeMockResp(t, w, 500, `{"errcode":"M_UNKNOWN","error":"join error"}`)
			} else {
				writeMockResp(t, w, 200, `{"joined":{"@admin:example.com":{},"@mod:example.com":{},"@priv:example.com":{},"@user:example.com":{}}}`)
			}
		default:
			writeMockResp(t, w, 404, `{"errcode":"M_UNRECOGNIZED","error":"unrecognized endpoint"}`)
		}
	}))
}

func TestRooms(t *testing.T) {
	tests := []infoTestCase{
		{
			name:              "joined_rooms_err",
			httpStatus:        500,
			httpBody:          `{"errcode":"M_UNKNOWN","error":"internal error"}`,
			expectErr:         true,
			expectErrContains: "failed to fetch joined rooms",
		},
		{
			name:       "success_not_verbose",
			httpStatus: 200,
			httpBody:   `{"joined_rooms":["!info_r1:example.com"]}`,
			verbose:    false,
		},
		{
			name:              "marshal_err_not_verbose",
			httpStatus:        200,
			httpBody:          `{"joined_rooms":["!info_r2:example.com"]}`,
			verbose:           false,
			jsonMarshalErr:    errors.New("mock info marshal error 1"),
			expectErr:         true,
			expectErrContains: "failed to marshal rooms",
		},
		{
			name:              "stdout_err_not_verbose",
			httpStatus:        200,
			httpBody:          `{"joined_rooms":["!info_r3:example.com"]}`,
			verbose:           false,
			stdoutErrNum:      1,
			expectErr:         true,
			expectErrContains: "stdout write err",
		},
		{
			name:       "success_verbose",
			httpStatus: 200,
			httpBody:   `{"joined_rooms":["!info_r4:example.com"]}`,
			verbose:    true,
		},
		{
			name:              "marshal_err_verbose",
			httpStatus:        200,
			httpBody:          `{"joined_rooms":["!info_r5:example.com"]}`,
			verbose:           true,
			jsonMarshalErr:    errors.New("mock info marshal error 2"),
			expectErr:         true,
			expectErrContains: "failed to marshal detailed rooms",
		},
		{
			name:              "stdout_err_verbose",
			httpStatus:        200,
			httpBody:          `{"joined_rooms":["!info_r6:example.com"]}`,
			verbose:           true,
			stdoutErrNum:      1,
			expectErr:         true,
			expectErrContains: "stdout write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupInfoMockServer(t, &tt)
			defer server.Close()

			matrixClient, err := mautrix.NewClient(server.URL, "@user:example.com", "token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			c := &Client{Matrix: matrixClient}

			if tt.jsonMarshalErr != nil {
				origJSON := jsonMarshalIndent
				defer func() { jsonMarshalIndent = origJSON }()
				jsonMarshalIndent = func(_ any, _, _ string) ([]byte, error) {
					return nil, tt.jsonMarshalErr
				}
			}

			origStdout := stdout
			defer func() { stdout = origStdout }()
			if tt.stdoutErrNum > 0 {
				stdout = &errorWriter{failOnWriteNum: tt.stdoutErrNum}
			} else {
				stdout = io.Discard
			}

			err = c.Rooms(context.Background(), tt.verbose)
			verifyInfoResult(t, &tt, err)
		})
	}
}

func verifyInfoResult(t *testing.T, tt *infoTestCase, err error) {
	switch {
	case tt.expectErr && err == nil:
		t.Errorf("expected error containing %q, got nil", tt.expectErrContains)
	case !tt.expectErr && err != nil:
		t.Errorf("expected no error, got: %v", err)
	case tt.expectErr && err != nil && !strings.Contains(err.Error(), tt.expectErrContains):
		t.Errorf("expected error containing %q, got: %v", tt.expectErrContains, err)
	}
}

func TestRoomInfo(t *testing.T) {
	tests := []infoTestCase{
		{
			name:              "no_rooms_specified",
			roomsStr:          "   ",
			expectErr:         true,
			expectErrContains: "no rooms specified",
		},
		{
			name:       "roominfo_success",
			roomsStr:   "!info_roominfo1:example.com",
			httpStatus: 200,
		},
		{
			name:         "power_levels_err",
			roomsStr:     "!info_roominfo2:example.com",
			httpStatus:   200,
			endpointPath: "mock_pl_error",
		},
		{
			name:         "info_joined_members_err",
			roomsStr:     "!info_roominfo3:example.com",
			httpStatus:   200,
			endpointPath: "mock_jm_error",
		},
		{
			name:              "roominfo_marshal_err",
			roomsStr:          "!info_roominfo4:example.com",
			httpStatus:        200,
			jsonMarshalErr:    errors.New("mock info marshal error 3"),
			expectErr:         true,
			expectErrContains: "failed to marshal room details",
		},
		{
			name:              "roominfo_stdout_err",
			roomsStr:          "!info_roominfo5:example.com",
			httpStatus:        200,
			stdoutErrNum:      1,
			expectErr:         true,
			expectErrContains: "write error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupInfoMockServer(t, &tt)
			defer server.Close()

			matrixClient, err := mautrix.NewClient(server.URL, "@user:example.com", "token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			c := &Client{Matrix: matrixClient}

			if tt.jsonMarshalErr != nil {
				origJSON := jsonMarshalIndent
				defer func() { jsonMarshalIndent = origJSON }()
				jsonMarshalIndent = func(_ any, _, _ string) ([]byte, error) {
					return nil, tt.jsonMarshalErr
				}
			}

			origStdout := stdout
			origStderr := stderr
			defer func() {
				stdout = origStdout
				stderr = origStderr
			}()

			if tt.stdoutErrNum > 0 {
				stdout = &errorWriter{failOnWriteNum: tt.stdoutErrNum}
			} else {
				stdout = io.Discard
			}
			stderr = io.Discard

			err = c.RoomInfo(context.Background(), tt.roomsStr)
			verifyInfoResult(t, &tt, err)
		})
	}
}

func TestDevices(t *testing.T) {
	tests := []infoTestCase{
		{
			name:              "get_devices_err",
			httpStatus:        500,
			httpBody:          `{"errcode":"M_UNKNOWN","error":"internal error"}`,
			expectErr:         true,
			expectErrContains: "failed to fetch devices",
		},
		{
			name:       "devices_success",
			httpStatus: 200,
			httpBody:   `{"devices":[{"device_id":"DEV1"}]}`,
		},
		{
			name:              "devices_marshal_err",
			httpStatus:        200,
			httpBody:          `{"devices":[{"device_id":"DEV2"}]}`,
			jsonMarshalErr:    errors.New("mock info marshal error 4"),
			expectErr:         true,
			expectErrContains: "failed to marshal devices",
		},
		{
			name:              "devices_stdout_err",
			httpStatus:        200,
			httpBody:          `{"devices":[{"device_id":"DEV3"}]}`,
			stdoutErrNum:      1,
			expectErr:         true,
			expectErrContains: "out write error",
		},
		{
			name:       "devices_verified",
			httpStatus: 200,
			httpBody:   `{"devices":[{"device_id":"DEV_V"}]}`,
		},
		{
			name:       "devices_tofu",
			httpStatus: 200,
			httpBody:   `{"devices":[{"device_id":"DEV_T"}]}`,
		},
		{
			name:       "devices_unverified",
			httpStatus: 200,
			httpBody:   `{"devices":[{"device_id":"DEV_U"}]}`,
		},
		{
			name:       "devices_blacklisted",
			httpStatus: 200,
			httpBody:   `{"devices":[{"device_id":"DEV_B"}]}`,
		},
		{
			name:       "devices_untrusted",
			httpStatus: 200,
			httpBody:   `{"devices":[{"device_id":"DEV_UT"}]}`,
		},
		{
			name:       "devices_unknown_trust",
			httpStatus: 200,
			httpBody:   `{"devices":[{"device_id":"DEV_UNK"}]}`,
		},
		{
			name:       "devices_fetch_err",
			httpStatus: 200,
			httpBody:   `{"devices":[{"device_id":"DEV_ERR"}]}`,
		},
		{
			name:       "devices_trust_err",
			httpStatus: 200,
			httpBody:   `{"devices":[{"device_id":"DEV_TERR"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupInfoMockServer(t, &tt)
			defer server.Close()

			matrixClient, err := mautrix.NewClient(server.URL, "@user:example.com", "token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			c := &Client{Matrix: matrixClient}

			if tt.jsonMarshalErr != nil {
				origJSON := jsonMarshalIndent
				defer func() { jsonMarshalIndent = origJSON }()
				jsonMarshalIndent = func(_ any, _, _ string) ([]byte, error) {
					return nil, tt.jsonMarshalErr
				}
			}

			origGetOlmMachine := getOlmMachine
			getOlmMachine = func(_ *Client) *crypto.OlmMachine { return &crypto.OlmMachine{} }
			defer func() { getOlmMachine = origGetOlmMachine }()

			origGetOrFetchDevice := getOrFetchDevice
			getOrFetchDevice = func(_ context.Context, _ *crypto.OlmMachine, _ id.UserID, devID id.DeviceID) (*id.Device, error) {
				if devID == "DEV_ERR" {
					return nil, errors.New("mock fetch err")
				}
				return &id.Device{DeviceID: devID}, nil
			}
			defer func() { getOrFetchDevice = origGetOrFetchDevice }()

			origResolveTrustContext := resolveTrustContext
			resolveTrustContext = func(_ context.Context, _ *crypto.OlmMachine, dev *id.Device) (id.TrustState, error) {
				if dev.DeviceID == "DEV_TERR" {
					return 0, errors.New("mock trust err")
				}
				switch dev.DeviceID {
				case "DEV_V":
					return id.TrustStateCrossSignedVerified, nil
				case "DEV_T":
					return id.TrustStateCrossSignedTOFU, nil
				case "DEV_U":
					return id.TrustStateUnset, nil
				case "DEV_B":
					return id.TrustStateBlacklisted, nil
				case "DEV_UT":
					return id.TrustStateCrossSignedUntrusted, nil
				default:
					return id.TrustStateUnknownDevice, nil
				}
			}
			defer func() { resolveTrustContext = origResolveTrustContext }()

			origStdout := stdout
			defer func() { stdout = origStdout }()
			if tt.stdoutErrNum > 0 {
				stdout = &errorWriter{failOnWriteNum: tt.stdoutErrNum}
			} else {
				stdout = io.Discard
			}

			err = c.Devices(context.Background())
			verifyInfoResult(t, &tt, err)
		})
	}
}
