package client

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/backup"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func defaultVerifyWithRecoveryKey(ctx context.Context, mach *crypto.OlmMachine, recoveryKey string) error {
	if err := mach.VerifyWithRecoveryKey(ctx, recoveryKey); err != nil {
		return fmt.Errorf("verify failed: %w", err)
	}
	return nil
}

var verifyWithRecoveryKey = defaultVerifyWithRecoveryKey

func defaultExportCrossSigningKeys(mach *crypto.OlmMachine) crypto.CrossSigningSeeds {
	return mach.ExportCrossSigningKeys()
}

var exportCrossSigningKeys = defaultExportCrossSigningKeys

func defaultGenerateAndUploadCrossSigningKeys(ctx context.Context, mach *crypto.OlmMachine, cb func(*mautrix.RespUserInteractive) any, masterKey string) (string, *crypto.CrossSigningKeysCache, error) {
	key, cache, err := mach.GenerateAndUploadCrossSigningKeys(ctx, cb, masterKey)
	if err != nil {
		return "", nil, fmt.Errorf("generate failed: %w", err)
	}
	return key, cache, nil
}

var generateAndUploadCrossSigningKeys = defaultGenerateAndUploadCrossSigningKeys

func defaultSignOwnDevice(ctx context.Context, mach *crypto.OlmMachine, identity *id.Device) error {
	if err := mach.SignOwnDevice(ctx, identity); err != nil {
		return fmt.Errorf("sign device failed: %w", err)
	}
	return nil
}

var signOwnDevice = defaultSignOwnDevice

func defaultSignOwnMasterKey(ctx context.Context, mach *crypto.OlmMachine) error {
	if err := mach.SignOwnMasterKey(ctx); err != nil {
		return fmt.Errorf("sign master key failed: %w", err)
	}
	return nil
}

var signOwnMasterKey = defaultSignOwnMasterKey

func defaultOwnIdentity(mach *crypto.OlmMachine) *id.Device {
	return mach.OwnIdentity()
}

var ownIdentity = defaultOwnIdentity

func defaultSetupMegolmBackup(ctx context.Context, c *Client, recoveryKey string) error {
	return c.setupMegolmBackup(ctx, recoveryKey)
}

var doSetupMegolmBackup = defaultSetupMegolmBackup

func defaultSaveCrossSigningKeys(ctx context.Context, c *Client, keys crypto.CrossSigningSeeds) {
	c.saveCrossSigningKeys(ctx, keys)
}

var doSaveCrossSigningKeys = defaultSaveCrossSigningKeys

func defaultGetOlmMachine(c *Client) *crypto.OlmMachine {
	return c.Crypto.Machine()
}

var getOlmMachine = defaultGetOlmMachine

func defaultSSSSGetDefaultKeyData(ctx context.Context, mach *crypto.OlmMachine) (string, *ssss.KeyMetadata, error) {
	keyID, data, err := mach.SSSS.GetDefaultKeyData(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("get default key data failed: %w", err)
	}
	return keyID, data, nil
}

var ssssGetDefaultKeyData = defaultSSSSGetDefaultKeyData

func defaultVerifyRecoveryKey(keyData *ssss.KeyMetadata, keyID, recoveryKey string) (*ssss.Key, error) {
	key, err := keyData.VerifyRecoveryKey(keyID, recoveryKey)
	if err != nil {
		return nil, fmt.Errorf("verify recovery key failed: %w", err)
	}
	return key, nil
}

var verifyRecoveryKey = defaultVerifyRecoveryKey

func defaultNewMegolmBackupKey() (*backup.MegolmBackupKey, error) {
	key, err := backup.NewMegolmBackupKey()
	if err != nil {
		return nil, fmt.Errorf("new megolm backup key failed: %w", err)
	}
	return key, nil
}

var newMegolmBackupKey = defaultNewMegolmBackupKey

func defaultPutSecret(ctx context.Context, mach *crypto.OlmMachine, secretID id.Secret, secret string) error {
	if err := mach.CryptoStore.PutSecret(ctx, secretID, secret); err != nil {
		return fmt.Errorf("put secret failed: %w", err)
	}
	return nil
}

var putSecret = defaultPutSecret

func defaultSetEncryptedAccountData(ctx context.Context, mach *crypto.OlmMachine, eventType event.Type, data []byte, key *ssss.Key) error {
	if err := mach.SSSS.SetEncryptedAccountData(ctx, eventType, data, key); err != nil {
		return fmt.Errorf("set encrypted account data failed: %w", err)
	}
	return nil
}

var setEncryptedAccountData = defaultSetEncryptedAccountData

func defaultCreateKeyBackupVersion(ctx context.Context, mach *crypto.OlmMachine, req *mautrix.ReqRoomKeysVersionCreate[backup.MegolmAuthData]) (*mautrix.RespRoomKeysVersionCreate, error) {
	resp, err := mach.Client.CreateKeyBackupVersion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create key backup version failed: %w", err)
	}
	return resp, nil
}

var createKeyBackupVersion = defaultCreateKeyBackupVersion
