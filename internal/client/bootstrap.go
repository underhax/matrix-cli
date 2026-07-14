package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/backup"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Bootstrap handles either importing cross-signing keys via recovery key,
// or generating new ones.
func (c *Client) Bootstrap(ctx context.Context, newKeys bool, recoveryKey string) error {
	if recoveryKey != "" {
		return c.bootstrapRecoveryKey(ctx, recoveryKey)
	}

	if newKeys {
		return c.bootstrapNewKeys(ctx)
	}

	var promptErr error
	recoveryKey, promptErr = readPassword("Enter recovery key: ")
	if promptErr != nil {
		return fmt.Errorf("failed to prompt for recovery key: %w", promptErr)
	}
	recoveryKey = strings.TrimSpace(recoveryKey)
	if recoveryKey == "" {
		return errors.New("recovery key cannot be empty. use --new-keys to generate new ones")
	}

	return c.bootstrapRecoveryKey(ctx, recoveryKey)
}

func (c *Client) bootstrapRecoveryKey(ctx context.Context, recoveryKey string) error {
	mach := getOlmMachine(c)
	if mach == nil {
		return errors.New("olm machine is not initialized")
	}
	ownDeviceID := ""
	ownUserID := ""
	if c.Matrix != nil {
		ownDeviceID = string(c.Matrix.DeviceID)
		ownUserID = string(c.Matrix.UserID)
	}
	c.Log.Debug().Str("own_device_id", ownDeviceID).Str("user_id", ownUserID).Msg("Fetching cross-signing keys from SSSS...")

	if err := verifyWithRecoveryKey(ctx, mach, recoveryKey); err != nil {
		c.Log.Debug().Err(err).Msg("Failed to verify with recovery key")
		return fmt.Errorf("failed to verify with recovery key: %w", err)
	}

	keys := exportCrossSigningKeys(mach)
	c.Log.Debug().
		Bool("has_master", keys.MasterKey != nil).
		Bool("has_self_signing", keys.SelfSigningKey != nil).
		Bool("has_user_signing", keys.UserSigningKey != nil).
		Msg("Exported cross-signing keys")

	doSaveCrossSigningKeys(ctx, c, keys)
	c.Log.Debug().Msg("Saved cross-signing keys locally")

	if mach.SSSS != nil && mach.CryptoStore != nil {
		fetchAndSaveMegolmBackupFromSSSS(ctx, c, mach, recoveryKey)
	}

	if mach.CryptoStore != nil {
		backupSecret, err := mach.CryptoStore.GetSecret(ctx, id.SecretMegolmBackupV1)
		switch {
		case err != nil:
			c.Log.Debug().Err(err).Str("secret", string(id.SecretMegolmBackupV1)).Msg("Failed to check megolm backup key in store")
		case backupSecret == "":
			c.Log.Debug().Str("secret", string(id.SecretMegolmBackupV1)).Msg("Megolm backup key is MISSING in local store after recovery key bootstrap")
		default:
			c.Log.Debug().Str("secret", string(id.SecretMegolmBackupV1)).Msg("Megolm backup key is PRESENT in local store after recovery key bootstrap")
		}
	}

	out := map[string]string{
		jsonKeyStatus: statusSuccess,
		"method":      "recovery_key",
	}
	if payload, errMarshal := json.Marshal(out); errMarshal == nil {
		if _, errWrite := fmt.Fprintln(os.Stdout, string(payload)); errWrite != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write output: %v\n", errWrite)
		}
	}
	return nil
}

func fetchAndSaveMegolmBackupFromSSSS(ctx context.Context, c *Client, mach *crypto.OlmMachine, recoveryKey string) {
	ownDeviceID := ""
	ownUserID := ""
	if c.Matrix != nil {
		ownDeviceID = string(c.Matrix.DeviceID)
		ownUserID = string(c.Matrix.UserID)
	}
	c.Log.Debug().Str("own_device_id", ownDeviceID).Str("user_id", ownUserID).Msg("Fetching megolm backup key from SSSS...")

	keyID, keyData, errSSSS := ssssGetDefaultKeyData(ctx, mach)
	if errSSSS != nil {
		c.Log.Debug().Err(errSSSS).Msg("Failed to get SSSS default key data")
		return
	}

	ssssKey, errV := verifyRecoveryKey(keyData, keyID, recoveryKey)
	if errV != nil {
		c.Log.Debug().Err(errV).Msg("Failed to verify recovery key for SSSS")
		return
	}

	decryptedBackup, errDec := mach.SSSS.GetDecryptedAccountData(ctx, event.AccountDataMegolmBackupKey, ssssKey)
	if errDec != nil {
		c.Log.Debug().Err(errDec).Msg("Failed to fetch/decrypt megolm backup key from SSSS")
		return
	}

	if len(decryptedBackup) == 0 {
		c.Log.Debug().Msg("Decrypted megolm backup key is empty")
		return
	}

	encoded := base64.RawStdEncoding.EncodeToString(decryptedBackup)
	if errPut := putSecret(ctx, mach, id.SecretMegolmBackupV1, encoded); errPut != nil {
		c.Log.Debug().Err(errPut).Msg("Failed to save fetched megolm backup key")
	} else {
		c.Log.Debug().Msg("Saved megolm backup key locally")
	}
}

func (c *Client) bootstrapNewKeys(ctx context.Context) error {
	mach := getOlmMachine(c)
	if mach == nil {
		return errors.New("olm machine is not initialized")
	}
	ownDeviceID := ""
	ownUserID := ""
	if c.Matrix != nil {
		ownDeviceID = string(c.Matrix.DeviceID)
		ownUserID = string(c.Matrix.UserID)
	}
	c.Log.Debug().Str("own_device_id", ownDeviceID).Str("user_id", ownUserID).Msg("Generating new cross-signing keys and SSSS...")

	newRecoveryKey, _, err := generateAndUploadCrossSigningKeys(ctx, mach, func(uiResp *mautrix.RespUserInteractive) any {
		return handleUIA(c, uiResp)
	}, "")
	if err != nil {
		c.Log.Debug().Err(err).Msg("Failed to generate and upload keys")
		return fmt.Errorf("failed to generate and upload keys: %w", err)
	}

	if errDev := signOwnDevice(ctx, mach, ownIdentity(mach)); errDev != nil {
		c.Log.Debug().Err(errDev).Msg("Failed to sign own device")
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to sign own device: %v\n", errDev)
	}
	if errMast := signOwnMasterKey(ctx, mach); errMast != nil {
		c.Log.Debug().Err(errMast).Msg("Failed to sign own master key")
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to sign own master key: %v\n", errMast)
	}

	keys := exportCrossSigningKeys(mach)
	c.Log.Debug().
		Bool("has_master", keys.MasterKey != nil).
		Bool("has_self_signing", keys.SelfSigningKey != nil).
		Bool("has_user_signing", keys.UserSigningKey != nil).
		Msg("Exported NEW cross-signing keys")

	doSaveCrossSigningKeys(ctx, c, keys)
	c.Log.Debug().Msg("Saved NEW cross-signing keys locally")

	if errSetup := doSetupMegolmBackup(ctx, c, newRecoveryKey); errSetup != nil {
		c.Log.Debug().Err(errSetup).Msg("Failed to setup megolm backup")
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to setup megolm backup: %v\n", errSetup)
	} else {
		c.Log.Debug().Msg("Successfully setup new megolm backup")
	}

	_, _ = fmt.Fprintf(os.Stderr, "Successfully generated new keys.\n\nIMPORTANT: Save this new Recovery Key: %s\n\n", newRecoveryKey)

	out := map[string]any{
		jsonKeyStatus:  statusSuccess,
		"method":       "new_keys",
		"recovery_key": newRecoveryKey,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if errEnc := enc.Encode(out); errEnc != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to encode output: %v\n", errEnc)
	}

	return nil
}

func (c *Client) setupMegolmBackup(ctx context.Context, recoveryKey string) error {
	mach := getOlmMachine(c)

	keyID, keyData, err := ssssGetDefaultKeyData(ctx, mach)
	if err != nil {
		return fmt.Errorf("get SSSS key data: %w", err)
	}
	ssssKey, err := verifyRecoveryKey(keyData, keyID, recoveryKey)
	if err != nil {
		return fmt.Errorf("verify recovery key: %w", err)
	}

	megolmKey, err := newMegolmBackupKey()
	if err != nil {
		return fmt.Errorf("generate backup key: %w", err)
	}

	err = putSecret(ctx, mach, id.SecretMegolmBackupV1, base64.RawStdEncoding.EncodeToString(megolmKey.Bytes()))
	if err != nil {
		c.Log.Debug().Err(err).Msg("Failed to save backup key to local store")
		return fmt.Errorf("save backup key to store: %w", err)
	}
	c.Log.Debug().Str("secret", string(id.SecretMegolmBackupV1)).Msg("Successfully saved backup key to local store")

	err = setEncryptedAccountData(ctx, mach, event.AccountDataMegolmBackupKey, megolmKey.Bytes(), ssssKey)
	if err != nil {
		c.Log.Debug().Err(err).Msg("Failed to upload backup key to SSSS")
		return fmt.Errorf("upload backup key to SSSS: %w", err)
	}
	c.Log.Debug().Msg("Successfully uploaded backup key to SSSS")

	req := &mautrix.ReqRoomKeysVersionCreate[backup.MegolmAuthData]{
		Algorithm: id.KeyBackupAlgorithmMegolmBackupV1,
		AuthData: backup.MegolmAuthData{
			PublicKey: id.Ed25519(base64.RawStdEncoding.EncodeToString(megolmKey.PublicKey().Bytes())),
		},
	}
	_, err = createKeyBackupVersion(ctx, mach, req)
	if err != nil {
		return fmt.Errorf("create key backup version: %w", err)
	}

	return nil
}
