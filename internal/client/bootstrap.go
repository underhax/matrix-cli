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
	_, _ = fmt.Fprintln(os.Stderr, "Fetching cross-signing keys from SSSS using recovery key...")
	if err := verifyWithRecoveryKey(ctx, mach, recoveryKey); err != nil {
		return fmt.Errorf("failed to verify with recovery key: %w", err)
	}

	keys := exportCrossSigningKeys(mach)
	doSaveCrossSigningKeys(ctx, c, keys)

	_, _ = fmt.Fprintln(os.Stderr, "Successfully fetched and saved cross-signing keys.")

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

func (c *Client) bootstrapNewKeys(ctx context.Context) error {
	mach := getOlmMachine(c)
	_, _ = fmt.Fprintln(os.Stderr, "Generating new cross-signing keys and SSSS...")
	newRecoveryKey, _, err := generateAndUploadCrossSigningKeys(ctx, mach, func(uiResp *mautrix.RespUserInteractive) any {
		return handleUIA(c, uiResp)
	}, "")
	if err != nil {
		return fmt.Errorf("failed to generate and upload keys: %w", err)
	}

	if errDev := signOwnDevice(ctx, mach, ownIdentity(mach)); errDev != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to sign own device: %v\n", errDev)
	}
	if errMast := signOwnMasterKey(ctx, mach); errMast != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to sign own master key: %v\n", errMast)
	}

	keys := exportCrossSigningKeys(mach)
	doSaveCrossSigningKeys(ctx, c, keys)

	if errSetup := doSetupMegolmBackup(ctx, c, newRecoveryKey); errSetup != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to setup megolm backup: %v\n", errSetup)
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
		return fmt.Errorf("save backup key to store: %w", err)
	}

	err = setEncryptedAccountData(ctx, mach, event.AccountDataMegolmBackupKey, megolmKey.Bytes(), ssssKey)
	if err != nil {
		return fmt.Errorf("upload backup key to SSSS: %w", err)
	}

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
