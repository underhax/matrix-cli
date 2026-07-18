package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/underhax/matrix-cli/internal/logger"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type mockStateStore struct {
	mautrix.StateStore
	setEncryptionEventCalled bool
	setMembershipCalled      bool
	returnError              bool
}

func (m *mockStateStore) SetEncryptionEvent(_ context.Context, _ id.RoomID, _ *event.EncryptionEventContent) error {
	m.setEncryptionEventCalled = true
	if m.returnError {
		return errors.New("mock encryption error")
	}
	return nil
}

func (m *mockStateStore) SetMembership(_ context.Context, _ id.RoomID, _ id.UserID, _ event.Membership) error {
	m.setMembershipCalled = true
	if m.returnError {
		return errors.New("mock member error")
	}
	return nil
}

type mockSyncer struct {
	mautrix.Syncer
}

func TestRegisterStateHooks_NotExtensible(_ *testing.T) {
	c := &Client{
		Matrix: &mautrix.Client{
			Syncer: &mockSyncer{},
		},
	}
	c.registerStateHooks()
}

type mockExtensibleSyncer struct {
	mautrix.Syncer
	onEventTypeCalled bool
}

func (m *mockExtensibleSyncer) OnEventType(_ event.Type, _ mautrix.EventHandler) {
	m.onEventTypeCalled = true
}

func (m *mockExtensibleSyncer) OnSync(_ mautrix.SyncHandler)   {}
func (m *mockExtensibleSyncer) OnEvent(_ mautrix.EventHandler) {}

func TestRegisterStateHooks_Extensible(t *testing.T) {
	syncer := &mockExtensibleSyncer{}
	c := &Client{
		Matrix: &mautrix.Client{
			Syncer: syncer,
		},
	}
	c.registerStateHooks()
	if !syncer.onEventTypeCalled {
		t.Errorf("expected OnEventType to be called")
	}
}

func TestOnStateEncryption(t *testing.T) {
	store := &mockStateStore{}
	c := &Client{
		Matrix: &mautrix.Client{
			StateStore: store,
		},
	}

	evt := &event.Event{
		RoomID: "test_room_1",
		Content: event.Content{
			Parsed: &event.EncryptionEventContent{},
		},
	}
	c.onStateEncryption(context.Background(), evt)

	if !store.setEncryptionEventCalled {
		t.Errorf("expected SetEncryptionEvent to be called")
	}

	evtInvalid := &event.Event{
		RoomID: "test_room_invalid",
		Content: event.Content{
			Parsed: "invalid_content",
		},
	}
	c.onStateEncryption(context.Background(), evtInvalid)

	store.returnError = true
	evtErr := &event.Event{
		RoomID: "test_room_err",
		Content: event.Content{
			Parsed: &event.EncryptionEventContent{},
		},
	}
	c.onStateEncryption(context.Background(), evtErr)
}

func TestOnStateMember(t *testing.T) {
	store := &mockStateStore{}
	c := &Client{
		Matrix: &mautrix.Client{
			StateStore: store,
		},
	}

	evt := &event.Event{
		RoomID:   "test_room_2",
		StateKey: &[]string{"@user:example.net"}[0],
		Content: event.Content{
			Parsed: &event.MemberEventContent{
				Membership: event.MembershipJoin,
			},
		},
	}
	c.onStateMember(context.Background(), evt)

	if !store.setMembershipCalled {
		t.Errorf("expected SetMembership to be called")
	}

	store.returnError = true
	evtErr := &event.Event{
		RoomID:   "test_room_3",
		StateKey: &[]string{"@user:example.org"}[0],
		Content: event.Content{
			Parsed: &event.MemberEventContent{
				Membership: event.MembershipJoin,
			},
		},
	}
	c.onStateMember(context.Background(), evtErr)
}

func TestOnSecretRequest(_ *testing.T) {
	c := &Client{}

	evtInvalid := &event.Event{
		Content: event.Content{
			Parsed: &event.EncryptionEventContent{},
		},
	}
	c.onSecretRequest(context.Background(), evtInvalid)

	origGetCryptoMachine := getCryptoMachine
	defer func() { getCryptoMachine = origGetCryptoMachine }()
	getCryptoMachine = mockGetCryptoMachineNil

	evtValid := &event.Event{
		Sender: "@user:sub.example.com",
		Content: event.Content{
			Parsed: &event.SecretRequestEventContent{
				Name: "test_secret",
			},
		},
	}
	c.onSecretRequest(context.Background(), evtValid)

	getCryptoMachine = mockGetCryptoMachineEmpty

	evtCancel := &event.Event{
		Sender: "@user:sub.example.com",
		Content: event.Content{
			Parsed: &event.SecretRequestEventContent{
				Name:   "test_secret_cancel",
				Action: event.SecretRequestCancellation,
			},
		},
	}
	c.onSecretRequest(context.Background(), evtCancel)

	origSecretRequestDelay := secretRequestDelay
	secretRequestDelay = 10 * time.Millisecond
	defer func() { secretRequestDelay = origSecretRequestDelay }()

	origDoDebugHandleSecretRequest := doDebugHandleSecretRequest
	defer func() { doDebugHandleSecretRequest = origDoDebugHandleSecretRequest }()

	testCalledChan = make(chan struct{})
	doDebugHandleSecretRequest = mockDoDebugHandleSecretRequestClose

	c.onSecretRequest(context.Background(), evtValid)
	<-testCalledChan
}

func TestDebugHandleSecretRequest(_ *testing.T) {
	ctx := context.Background()
	mach := &crypto.OlmMachine{
		Client: &mautrix.Client{
			UserID:   "@user:hooktest.example.com",
			DeviceID: "mydevice",
		},
	}
	req := &event.SecretRequestEventContent{
		Action:             event.SecretRequestRequest,
		RequestingDeviceID: "device1",
		Name:               "test_secret",
		RequestID:          "req1",
	}

	origGetOrFetchDevice := getOrFetchDevice
	defer func() { getOrFetchDevice = origGetOrFetchDevice }()
	origResolveTrustContext := resolveTrustContext
	defer func() { resolveTrustContext = origResolveTrustContext }()
	origGetSecret := getSecret
	defer func() { getSecret = origGetSecret }()
	origSendEncrypted := sendEncryptedToDevice
	defer func() { sendEncryptedToDevice = origSendEncrypted }()

	c := &Client{Log: logger.Nop()}

	mach.Client.UserID = "@user:test1.example.com"
	req.Action = event.SecretRequestCancellation
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test1.example.com", req)
	req.Action = event.SecretRequestRequest

	mach.Client.UserID = "@user:test2.example.com"
	defaultDebugHandleSecretRequest(ctx, c, mach, "@other:test2.example.com", req)

	req.RequestingDeviceID = ""
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test2.example.com", req)

	mach.Client.UserID = "@user:test3.example.com"
	req.RequestingDeviceID = "mydevice"
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test3.example.com", req)
	req.RequestingDeviceID = "device2"

	mach.Client.UserID = "@user:test4.example.com"
	getOrFetchDevice = mockGetOrFetchDeviceErr
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test4.example.com", req)

	mach.Client.UserID = "@user:test5.example.com"
	getOrFetchDevice = mockGetOrFetchDeviceSuccess
	resolveTrustContext = mockResolveTrustContextErr
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test5.example.com", req)

	mach.Client.UserID = "@user:test6.example.com"
	resolveTrustContext = mockResolveTrustContextUntrusted
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test6.example.com", req)

	mach.Client.UserID = "@user:test7.example.com"
	resolveTrustContext = mockResolveTrustContextVerified
	getSecret = mockGetSecretErr
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test7.example.com", req)

	mach.Client.UserID = "@user:test8.example.com"
	getSecret = mockGetSecretEmpty
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test8.example.com", req)

	mach.Client.UserID = "@user:test9.example.com"
	getSecret = mockGetSecretValue
	sendEncryptedToDevice = mockSendEncryptedToDeviceErr
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test9.example.com", req)

	mach.Client.UserID = "@user:test10.example.com"
	sendEncryptedToDevice = mockSendEncryptedToDeviceSuccess

	origTimeAfterFunc := timeAfterFunc
	defer func() { timeAfterFunc = origTimeAfterFunc }()
	timeAfterFunc = mockTimeAfterFunc

	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test10.example.com", req)

	mach.Client.UserID = "@user:test11.example.com"
	defaultDebugHandleSecretRequest(ctx, c, mach, "@user:test11.example.com", req)
}

func mockTimeAfterFunc(_ time.Duration, f func()) *time.Timer {
	f()
	return time.NewTimer(0)
}

var testCalledChan chan struct{}

func mockGetCryptoMachineNil(_ *cryptohelper.CryptoHelper) *crypto.OlmMachine {
	return nil
}

func mockGetCryptoMachineEmpty(_ *cryptohelper.CryptoHelper) *crypto.OlmMachine {
	return &crypto.OlmMachine{}
}

func mockDoDebugHandleSecretRequestClose(_ context.Context, _ *Client, _ *crypto.OlmMachine, _ id.UserID, _ *event.SecretRequestEventContent) {
	close(testCalledChan)
}

func mockGetOrFetchDeviceErr(_ context.Context, _ *crypto.OlmMachine, _ id.UserID, _ id.DeviceID) (*id.Device, error) {
	return nil, errors.New("device error")
}

func mockGetOrFetchDeviceSuccess(_ context.Context, _ *crypto.OlmMachine, _ id.UserID, _ id.DeviceID) (*id.Device, error) {
	return &id.Device{DeviceID: "device3"}, nil
}

func mockResolveTrustContextErr(_ context.Context, _ *crypto.OlmMachine, _ *id.Device) (id.TrustState, error) {
	return 0, errors.New("trust error")
}

func mockResolveTrustContextUntrusted(_ context.Context, _ *crypto.OlmMachine, _ *id.Device) (id.TrustState, error) {
	return id.TrustStateCrossSignedUntrusted, nil
}

func mockResolveTrustContextVerified(_ context.Context, _ *crypto.OlmMachine, _ *id.Device) (id.TrustState, error) {
	return id.TrustStateCrossSignedVerified, nil
}

func mockGetSecretErr(_ context.Context, _ *crypto.OlmMachine, _ id.Secret) (string, error) {
	return "", errors.New("secret error")
}

func mockGetSecretEmpty(_ context.Context, _ *crypto.OlmMachine, _ id.Secret) (string, error) {
	return "", nil
}

func mockGetSecretValue(_ context.Context, _ *crypto.OlmMachine, _ id.Secret) (string, error) {
	return "secret_value", nil
}

func mockSendEncryptedToDeviceErr(_ context.Context, _ *crypto.OlmMachine, _ *id.Device, _ event.Type, _ event.Content) error {
	return errors.New("send error")
}

func mockSendEncryptedToDeviceSuccess(_ context.Context, _ *crypto.OlmMachine, _ *id.Device, _ event.Type, _ event.Content) error {
	return nil
}
