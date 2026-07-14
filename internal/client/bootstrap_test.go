package client

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/backup"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func TestDecodeBase64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		encoder *base64.Encoding
		want    string
	}{
		{"StdEncoding", base64.StdEncoding.EncodeToString([]byte("test1")), base64.StdEncoding, "test1"},
		{"RawStdEncoding", base64.RawStdEncoding.EncodeToString([]byte("test2")), base64.RawStdEncoding, "test2"},
		{"URLEncoding", base64.URLEncoding.EncodeToString([]byte("test3?")), base64.URLEncoding, "test3?"},
		{"RawURLEncoding", base64.RawURLEncoding.EncodeToString([]byte("test4?")), base64.RawURLEncoding, "test4?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeBase64(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", string(got), tt.want)
			}
		})
	}
}

func TestBootstrapRecoveryKey(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	getOlmMachine = func(_ *Client) *crypto.OlmMachine { return nil }
	err := c.bootstrapRecoveryKey(ctx, "invalid-key")
	if err == nil || !strings.Contains(err.Error(), "olm machine is not initialized") {
		t.Errorf("expected olm machine err, got %v", err)
	}

	c.Matrix = &mautrix.Client{
		DeviceID: "D1",
		UserID:   "U1",
	}

	getOlmMachine = func(_ *Client) *crypto.OlmMachine { return &crypto.OlmMachine{} }
	defer func() { getOlmMachine = defaultGetOlmMachine }()

	verifyWithRecoveryKey = func(_ context.Context, _ *crypto.OlmMachine, _ string) error {
		return errors.New("mock verify err")
	}
	defer func() { verifyWithRecoveryKey = defaultVerifyWithRecoveryKey }()

	err = c.bootstrapRecoveryKey(ctx, "invalid-key")
	if err == nil || !strings.Contains(err.Error(), "mock verify err") {
		t.Errorf("expected verify error, got %v", err)
	}

	verifyWithRecoveryKey = func(_ context.Context, _ *crypto.OlmMachine, _ string) error {
		return nil
	}
	exportCrossSigningKeys = func(_ *crypto.OlmMachine) crypto.CrossSigningSeeds {
		return crypto.CrossSigningSeeds{}
	}
	defer func() { exportCrossSigningKeys = defaultExportCrossSigningKeys }()

	doSaveCrossSigningKeys = func(_ context.Context, _ *Client, _ crypto.CrossSigningSeeds) {}
	defer func() { doSaveCrossSigningKeys = defaultSaveCrossSigningKeys }()

	ssssGetDefaultKeyData = func(_ context.Context, _ *crypto.OlmMachine) (string, *ssss.KeyMetadata, error) {
		return "", nil, errors.New("skip fetch")
	}
	defer func() { ssssGetDefaultKeyData = defaultSSSSGetDefaultKeyData }()

	getSecret = func(_ context.Context, _ *crypto.OlmMachine, _ id.Secret) (string, error) {
		return "", errors.New("mock getSecret err")
	}
	defer func() { getSecret = defaultGetSecret }()

	oldStdout := os.Stdout
	devNull, errOpen := os.OpenFile(os.DevNull, os.O_WRONLY, 0o600)
	if errOpen != nil {
		t.Fatalf("failed to open devnull: %v", errOpen)
	}
	os.Stdout = devNull
	defer func() {
		os.Stdout = oldStdout
		if errClose := devNull.Close(); errClose != nil {
			t.Logf("failed to close devnull: %v", errClose)
		}
	}()

	getOlmMachine = func(_ *Client) *crypto.OlmMachine {
		return &crypto.OlmMachine{
			SSSS:        &ssss.Machine{},
			CryptoStore: &crypto.SQLCryptoStore{},
		}
	}
	err = c.bootstrapRecoveryKey(ctx, "valid-key")
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}

	getSecret = func(_ context.Context, _ *crypto.OlmMachine, _ id.Secret) (string, error) {
		return "", nil
	}
	err = c.bootstrapRecoveryKey(ctx, "valid-key")
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}

	getSecret = func(_ context.Context, _ *crypto.OlmMachine, _ id.Secret) (string, error) {
		return "some-secret", nil
	}
	err = c.bootstrapRecoveryKey(ctx, "valid-key")
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}

	if errClose := devNull.Close(); errClose != nil {
		t.Logf("failed to close devnull early: %v", errClose)
	}
	err = c.bootstrapRecoveryKey(ctx, "valid-key")
	if err != nil {
		t.Errorf("expected success despite write error, got %v", err)
	}
}

func TestFetchAndSaveMegolmBackupFromSSSS(_ *testing.T) {
	c := &Client{Matrix: &mautrix.Client{DeviceID: "D1", UserID: "U1"}}
	ctx := context.Background()
	mach := &crypto.OlmMachine{}

	ssssGetDefaultKeyData = func(_ context.Context, _ *crypto.OlmMachine) (string, *ssss.KeyMetadata, error) {
		return "", nil, errors.New("mock ssss err")
	}
	defer func() { ssssGetDefaultKeyData = defaultSSSSGetDefaultKeyData }()

	fetchAndSaveMegolmBackupFromSSSS(ctx, c, mach, "key")

	ssssGetDefaultKeyData = func(_ context.Context, _ *crypto.OlmMachine) (string, *ssss.KeyMetadata, error) {
		return "id", &ssss.KeyMetadata{}, nil
	}
	verifyRecoveryKey = func(_ *ssss.KeyMetadata, _ string, _ string) (*ssss.Key, error) {
		return nil, errors.New("mock verify err")
	}
	defer func() { verifyRecoveryKey = defaultVerifyRecoveryKey }()

	fetchAndSaveMegolmBackupFromSSSS(ctx, c, mach, "key")

	verifyRecoveryKey = func(_ *ssss.KeyMetadata, _ string, _ string) (*ssss.Key, error) {
		return &ssss.Key{}, nil
	}
	getDecryptedAccountData = func(_ context.Context, _ *crypto.OlmMachine, _ event.Type, _ *ssss.Key) ([]byte, error) {
		return nil, errors.New("mock decrypt err")
	}
	defer func() { getDecryptedAccountData = defaultGetDecryptedAccountData }()

	fetchAndSaveMegolmBackupFromSSSS(ctx, c, mach, "key")

	getDecryptedAccountData = func(_ context.Context, _ *crypto.OlmMachine, _ event.Type, _ *ssss.Key) ([]byte, error) {
		return []byte{}, nil
	}
	fetchAndSaveMegolmBackupFromSSSS(ctx, c, mach, "key")

	getDecryptedAccountData = func(_ context.Context, _ *crypto.OlmMachine, _ event.Type, _ *ssss.Key) ([]byte, error) {
		return []byte("dummy-key"), nil
	}
	putSecret = func(_ context.Context, _ *crypto.OlmMachine, _ id.Secret, _ string) error {
		return errors.New("mock put err")
	}
	defer func() { putSecret = defaultPutSecret }()

	fetchAndSaveMegolmBackupFromSSSS(ctx, c, mach, "key")

	putSecret = func(_ context.Context, _ *crypto.OlmMachine, _ id.Secret, _ string) error {
		return nil
	}
	fetchAndSaveMegolmBackupFromSSSS(ctx, c, mach, "key")
}

func TestBootstrapNewKeys(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	getOlmMachine = func(_ *Client) *crypto.OlmMachine { return nil }
	err := c.bootstrapNewKeys(ctx)
	if err == nil || !strings.Contains(err.Error(), "olm machine is not initialized") {
		t.Errorf("expected olm machine err, got %v", err)
	}

	c.Matrix = &mautrix.Client{
		DeviceID: "D1",
		UserID:   "U1",
	}

	getOlmMachine = func(_ *Client) *crypto.OlmMachine { return &crypto.OlmMachine{} }
	defer func() { getOlmMachine = defaultGetOlmMachine }()

	generateAndUploadCrossSigningKeys = func(_ context.Context, _ *crypto.OlmMachine, _ func(*mautrix.RespUserInteractive) any, _ string) (string, *crypto.CrossSigningKeysCache, error) {
		return "", nil, errors.New("mock generate err")
	}
	defer func() { generateAndUploadCrossSigningKeys = defaultGenerateAndUploadCrossSigningKeys }()

	err = c.bootstrapNewKeys(ctx)
	if err == nil || !strings.Contains(err.Error(), "mock generate err") {
		t.Errorf("expected generate error, got %v", err)
	}

	generateAndUploadCrossSigningKeys = func(_ context.Context, _ *crypto.OlmMachine, cb func(*mautrix.RespUserInteractive) any, _ string) (string, *crypto.CrossSigningKeysCache, error) {
		cb(&mautrix.RespUserInteractive{})
		return "new-recovery-key", nil, nil
	}
	ownIdentity = func(_ *crypto.OlmMachine) *id.Device { return nil }
	defer func() { ownIdentity = defaultOwnIdentity }()

	signOwnDevice = func(_ context.Context, _ *crypto.OlmMachine, _ *id.Device) error {
		return errors.New("mock sign device err")
	}
	defer func() { signOwnDevice = defaultSignOwnDevice }()

	signOwnMasterKey = func(_ context.Context, _ *crypto.OlmMachine) error {
		return errors.New("mock sign master err")
	}
	defer func() { signOwnMasterKey = defaultSignOwnMasterKey }()

	exportCrossSigningKeys = func(_ *crypto.OlmMachine) crypto.CrossSigningSeeds {
		return crypto.CrossSigningSeeds{}
	}
	defer func() { exportCrossSigningKeys = defaultExportCrossSigningKeys }()

	doSaveCrossSigningKeys = func(_ context.Context, _ *Client, _ crypto.CrossSigningSeeds) {}
	defer func() { doSaveCrossSigningKeys = defaultSaveCrossSigningKeys }()

	doSetupMegolmBackup = func(_ context.Context, _ *Client, _ string) error {
		return errors.New("mock setup backup err")
	}
	defer func() { doSetupMegolmBackup = defaultSetupMegolmBackup }()

	oldStdout := os.Stdout
	devNull, errOpen := os.OpenFile(os.DevNull, os.O_WRONLY, 0o600)
	if errOpen != nil {
		t.Fatalf("failed to open devnull: %v", errOpen)
	}
	os.Stdout = devNull
	defer func() {
		os.Stdout = oldStdout
		if errClose := devNull.Close(); errClose != nil {
			t.Logf("failed to close devnull: %v", errClose)
		}
	}()

	err = c.bootstrapNewKeys(ctx)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}

	if errClose := devNull.Close(); errClose != nil {
		t.Logf("failed to close devnull early: %v", errClose)
	}
	err = c.bootstrapNewKeys(ctx)
	if err != nil {
		t.Errorf("expected success despite write error, got %v", err)
	}
}

func TestBootstrap(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	getOlmMachine = func(_ *Client) *crypto.OlmMachine { return &crypto.OlmMachine{} }
	verifyWithRecoveryKey = func(_ context.Context, _ *crypto.OlmMachine, _ string) error { return nil }
	exportCrossSigningKeys = func(_ *crypto.OlmMachine) crypto.CrossSigningSeeds { return crypto.CrossSigningSeeds{} }
	doSaveCrossSigningKeys = func(_ context.Context, _ *Client, _ crypto.CrossSigningSeeds) {}
	generateAndUploadCrossSigningKeys = func(_ context.Context, _ *crypto.OlmMachine, cb func(*mautrix.RespUserInteractive) any, _ string) (string, *crypto.CrossSigningKeysCache, error) {
		cb(&mautrix.RespUserInteractive{})
		return "ok", nil, nil
	}
	ownIdentity = func(_ *crypto.OlmMachine) *id.Device { return nil }
	signOwnDevice = func(_ context.Context, _ *crypto.OlmMachine, _ *id.Device) error { return nil }
	signOwnMasterKey = func(_ context.Context, _ *crypto.OlmMachine) error { return nil }
	doSetupMegolmBackup = func(_ context.Context, _ *Client, _ string) error { return nil }

	defer func() {
		getOlmMachine = defaultGetOlmMachine
		verifyWithRecoveryKey = defaultVerifyWithRecoveryKey
		exportCrossSigningKeys = defaultExportCrossSigningKeys
		doSaveCrossSigningKeys = defaultSaveCrossSigningKeys
		generateAndUploadCrossSigningKeys = defaultGenerateAndUploadCrossSigningKeys
		ownIdentity = defaultOwnIdentity
		signOwnDevice = defaultSignOwnDevice
		signOwnMasterKey = defaultSignOwnMasterKey
		doSetupMegolmBackup = defaultSetupMegolmBackup
	}()

	err := c.Bootstrap(ctx, false, "direct-key")
	if err != nil {
		t.Errorf("Bootstrap with direct key failed: %v", err)
	}

	err = c.Bootstrap(ctx, true, "")
	if err != nil {
		t.Errorf("Bootstrap with new keys failed: %v", err)
	}

	readPassword = func(_ string) (string, error) {
		return "", errors.New("mock read err")
	}
	err = c.Bootstrap(ctx, false, "")
	if err == nil || !strings.Contains(err.Error(), "failed to prompt") {
		t.Errorf("expected prompt err, got %v", err)
	}

	readPassword = func(_ string) (string, error) {
		return "  ", nil
	}
	err = c.Bootstrap(ctx, false, "")
	if err == nil || !strings.Contains(err.Error(), "recovery key cannot be empty") {
		t.Errorf("expected empty key error, got %v", err)
	}

	readPassword = func(_ string) (string, error) {
		return "prompted-key", nil
	}
	err = c.Bootstrap(ctx, false, "")
	if err != nil {
		t.Errorf("expected success with prompted key, got %v", err)
	}
	defer func() { readPassword = ReadPassword }()

	err = c.Bootstrap(ctx, true, "both-provided")
	if err != nil {
		t.Errorf("if both provided, should prefer recovery key, got %v", err)
	}
}

func TestSetupMegolmBackup(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	getOlmMachine = func(_ *Client) *crypto.OlmMachine { return &crypto.OlmMachine{} }
	defer func() { getOlmMachine = defaultGetOlmMachine }()

	ssssGetDefaultKeyData = func(_ context.Context, _ *crypto.OlmMachine) (string, *ssss.KeyMetadata, error) {
		return "", nil, errors.New("mock ssss err")
	}
	err := c.setupMegolmBackup(ctx, "key")
	if err == nil || !strings.Contains(err.Error(), "mock ssss err") {
		t.Errorf("expected ssss err, got %v", err)
	}

	ssssGetDefaultKeyData = func(_ context.Context, _ *crypto.OlmMachine) (string, *ssss.KeyMetadata, error) {
		return "key-id", &ssss.KeyMetadata{}, nil
	}
	defer func() { ssssGetDefaultKeyData = defaultSSSSGetDefaultKeyData }()

	verifyRecoveryKey = func(_ *ssss.KeyMetadata, _ string, _ string) (*ssss.Key, error) {
		return nil, errors.New("mock verify err")
	}
	err = c.setupMegolmBackup(ctx, "key")
	if err == nil || !strings.Contains(err.Error(), "mock verify err") {
		t.Errorf("expected verify err, got %v", err)
	}

	verifyRecoveryKey = func(_ *ssss.KeyMetadata, _ string, _ string) (*ssss.Key, error) {
		return &ssss.Key{}, nil
	}
	defer func() { verifyRecoveryKey = defaultVerifyRecoveryKey }()

	newMegolmBackupKey = func() (*backup.MegolmBackupKey, error) {
		return nil, errors.New("mock generate err")
	}
	err = c.setupMegolmBackup(ctx, "key")
	if err == nil || !strings.Contains(err.Error(), "mock generate err") {
		t.Errorf("expected generate err, got %v", err)
	}

	newMegolmBackupKey = func() (*backup.MegolmBackupKey, error) {
		key, genErr := backup.NewMegolmBackupKey()
		if genErr != nil {
			return nil, genErr
		}
		return key, nil
	}
	defer func() { newMegolmBackupKey = defaultNewMegolmBackupKey }()

	putSecret = func(_ context.Context, _ *crypto.OlmMachine, _ id.Secret, _ string) error {
		return errors.New("mock put err")
	}
	err = c.setupMegolmBackup(ctx, "key")
	if err == nil || !strings.Contains(err.Error(), "mock put err") {
		t.Errorf("expected put err, got %v", err)
	}

	putSecret = func(_ context.Context, _ *crypto.OlmMachine, _ id.Secret, _ string) error {
		return nil
	}
	defer func() { putSecret = defaultPutSecret }()

	setEncryptedAccountData = func(_ context.Context, _ *crypto.OlmMachine, _ event.Type, _ []byte, _ *ssss.Key) error {
		return errors.New("mock encrypt err")
	}
	err = c.setupMegolmBackup(ctx, "key")
	if err == nil || !strings.Contains(err.Error(), "mock encrypt err") {
		t.Errorf("expected encrypt err, got %v", err)
	}

	setEncryptedAccountData = func(_ context.Context, _ *crypto.OlmMachine, _ event.Type, _ []byte, _ *ssss.Key) error {
		return nil
	}
	defer func() { setEncryptedAccountData = defaultSetEncryptedAccountData }()

	createKeyBackupVersion = func(_ context.Context, _ *crypto.OlmMachine, _ *mautrix.ReqRoomKeysVersionCreate[backup.MegolmAuthData]) (*mautrix.RespRoomKeysVersionCreate, error) {
		return nil, errors.New("mock create err")
	}
	err = c.setupMegolmBackup(ctx, "key")
	if err == nil || !strings.Contains(err.Error(), "mock create err") {
		t.Errorf("expected create err, got %v", err)
	}

	createKeyBackupVersion = func(_ context.Context, _ *crypto.OlmMachine, _ *mautrix.ReqRoomKeysVersionCreate[backup.MegolmAuthData]) (*mautrix.RespRoomKeysVersionCreate, error) {
		return &mautrix.RespRoomKeysVersionCreate{}, nil
	}
	defer func() { createKeyBackupVersion = defaultCreateKeyBackupVersion }()

	err = c.setupMegolmBackup(ctx, "key")
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
}
