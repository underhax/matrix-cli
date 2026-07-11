package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/id"
)

func decodeBase64(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func (c *Client) requestSecrets(ctx context.Context) {
	_, _ = fmt.Fprintf(os.Stderr, "Requesting cross-signing keys from trusted devices...\n")

	go func() {
		var master, self, user, backupKey string
		var errM, errS, errU, errB error

		errM = c.Crypto.Machine().GetOrRequestSecret(ctx, id.SecretXSMaster, func(s string) (bool, error) {
			master = s
			return true, nil
		}, 15*time.Second)
		errS = c.Crypto.Machine().GetOrRequestSecret(ctx, id.SecretXSSelfSigning, func(s string) (bool, error) {
			self = s
			return true, nil
		}, 15*time.Second)
		errU = c.Crypto.Machine().GetOrRequestSecret(ctx, id.SecretXSUserSigning, func(s string) (bool, error) {
			user = s
			return true, nil
		}, 15*time.Second)
		errB = c.Crypto.Machine().GetOrRequestSecret(ctx, id.SecretMegolmBackupV1, func(s string) (bool, error) {
			backupKey = s
			return true, nil
		}, 15*time.Second)

		if errM != nil || errS != nil || errU != nil || errB != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: some secrets timed out or failed to download.\n")
		}

		if master == "" || self == "" || user == "" {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to receive all cross-signing keys.\n")
			return
		}

		if backupKey != "" {
			_, _ = fmt.Fprintf(os.Stderr, "Received megolm backup key.\n")
		}

		_, _ = fmt.Fprintf(os.Stderr, "Received cross-signing keys. Saving to local store...\n")
		c.loadSecrets(ctx)

		if c.Crypto.Machine().CrossSigningKeys != nil {
			if err := c.Crypto.Machine().SignOwnDevice(ctx, c.Crypto.Machine().OwnIdentity()); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to sign own device after receiving keys: %v\n", err)
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "Successfully signed own device. You are now fully verified.\n")
			}
		}
	}()
}

func (c *Client) loadSecrets(ctx context.Context) {
	mach := c.Crypto.Machine()
	master, err1 := mach.CryptoStore.GetSecret(ctx, id.SecretXSMaster)
	self, err2 := mach.CryptoStore.GetSecret(ctx, id.SecretXSSelfSigning)
	user, err3 := mach.CryptoStore.GetSecret(ctx, id.SecretXSUserSigning)

	if err1 != nil || err2 != nil || err3 != nil || master == "" || self == "" || user == "" {
		return
	}

	decMaster, errM := decodeBase64(master)
	decSelf, errS := decodeBase64(self)
	decUser, errU := decodeBase64(user)

	if errM != nil || errS != nil || errU != nil || len(decMaster) == 0 || len(decSelf) == 0 || len(decUser) == 0 {
		return
	}

	err := mach.ImportCrossSigningKeys(crypto.CrossSigningSeeds{
		MasterKey:      decMaster,
		SelfSigningKey: decSelf,
		UserSigningKey: decUser,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to import cross-signing keys: %v\n", err)
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "Successfully loaded cross-signing keys from local store.\n")
	}
}

func (c *Client) saveCrossSigningKeys(ctx context.Context, keys crypto.CrossSigningSeeds) {
	mach := c.Crypto.Machine()

	if err1 := mach.CryptoStore.PutSecret(ctx, id.SecretXSMaster, base64.RawStdEncoding.EncodeToString(keys.MasterKey)); err1 != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to save master key: %v\n", err1)
	}
	if err2 := mach.CryptoStore.PutSecret(ctx, id.SecretXSSelfSigning, base64.RawStdEncoding.EncodeToString(keys.SelfSigningKey)); err2 != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to save self-signing key: %v\n", err2)
	}
	if err3 := mach.CryptoStore.PutSecret(ctx, id.SecretXSUserSigning, base64.RawStdEncoding.EncodeToString(keys.UserSigningKey)); err3 != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to save user-signing key: %v\n", err3)
	}
}

func (c *Client) migrateSecrets(ctx context.Context) {
	sqlStore, ok := c.Crypto.Machine().CryptoStore.(*crypto.SQLCryptoStore)
	if !ok {
		return
	}
	rows, err := sqlStore.DB.Query(ctx, "SELECT name, secret FROM crypto_secrets")
	if err != nil {
		return
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to close rows: %v\n", err)
		}
	}()

	updates := make(map[string]string)
	for rows.Next() {
		var name, secret string
		if err := rows.Scan(&name, &secret); err == nil {
			if strings.HasSuffix(secret, "=") {
				if dec, err := base64.StdEncoding.DecodeString(secret); err == nil {
					updates[name] = base64.RawStdEncoding.EncodeToString(dec)
				}
			}
		}
	}
	for name, secret := range updates {
		if _, err := sqlStore.DB.Exec(ctx, "UPDATE crypto_secrets SET secret=$1 WHERE name=$2", secret, name); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to update secret %s: %v\n", name, err)
		}
	}
}
