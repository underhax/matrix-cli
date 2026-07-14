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
	_, _ = fmt.Fprintf(os.Stderr, "Requesting cross-signing and megolm backup keys from trusted devices (own_device_id=%s)...\n", c.Matrix.DeviceID)

	go func() {
		mach := getOlmMachine(c)
		if mach == nil {
			return
		}
		checkLocal := func(name id.Secret) bool {
			if mach.CryptoStore != nil {
				s, err := cryptoStoreGetSecret(ctx, mach, name)
				return err == nil && s != ""
			}
			return false
		}
		masterLocal := checkLocal(id.SecretXSMaster)
		selfLocal := checkLocal(id.SecretXSSelfSigning)
		userLocal := checkLocal(id.SecretXSUserSigning)
		backupLocal := checkLocal(id.SecretMegolmBackupV1)

		var master, self, user, backupKey string
		var errM, errS, errU, errB error

		errM = getOrRequestSecret(ctx, mach, id.SecretXSMaster, func(s string) (bool, error) {
			master = s
			return true, nil
		}, 15*time.Second)
		errS = getOrRequestSecret(ctx, mach, id.SecretXSSelfSigning, func(s string) (bool, error) {
			self = s
			return true, nil
		}, 15*time.Second)
		errU = getOrRequestSecret(ctx, mach, id.SecretXSUserSigning, func(s string) (bool, error) {
			user = s
			return true, nil
		}, 15*time.Second)
		errB = getOrRequestSecret(ctx, mach, id.SecretMegolmBackupV1, func(s string) (bool, error) {
			backupKey = s
			return true, nil
		}, 15*time.Second)

		logStatus := func(name id.Secret, wasLocal bool, val string, err error) {
			if err != nil || val == "" {
				c.Log.Debug().Err(err).Str("secret", string(name)).Msg("Failed to obtain secret")
				return
			}
			if wasLocal {
				c.Log.Debug().Str("secret", string(name)).Msg("Loaded secret directly from local database")
			} else {
				c.Log.Debug().Str("secret", string(name)).Msg("Received secret from network")
			}
		}

		logStatus(id.SecretXSMaster, masterLocal, master, errM)
		logStatus(id.SecretXSSelfSigning, selfLocal, self, errS)
		logStatus(id.SecretXSUserSigning, userLocal, user, errU)
		logStatus(id.SecretMegolmBackupV1, backupLocal, backupKey, errB)

		var missing []string
		for _, v := range []struct {
			name id.Secret
			val  string
		}{
			{id.SecretXSMaster, master},
			{id.SecretXSSelfSigning, self},
			{id.SecretXSUserSigning, user},
			{id.SecretMegolmBackupV1, backupKey},
		} {
			if v.val == "" {
				missing = append(missing, string(v.name))
			}
		}

		if len(missing) > 0 {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to receive the following keys (own_device_id=%s): %s\n", mach.Client.DeviceID, strings.Join(missing, ", "))
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "Successfully received all requested keys (own_device_id=%s).\n", mach.Client.DeviceID)
		}

		if master == "" || self == "" || user == "" {
			return
		}

		c.Log.Debug().Msg("Processing cross-signing keys and loading into memory...")

		doLoadSecrets(ctx, c)

		if mach.CrossSigningKeys != nil {
			identity := ownIdentity(mach)
			if err := signOwnDevice(ctx, mach, identity); err != nil {
				c.Log.Debug().Err(err).Msg("Failed to sign own device after receiving keys")
			} else {
				c.Log.Debug().Str("own_device_id", string(identity.DeviceID)).Msg("Successfully signed own device with cross-signing keys")
			}
		}
	}()
}

func (c *Client) loadSecrets(ctx context.Context) {
	mach := getOlmMachine(c)
	if mach == nil {
		return
	}
	master, err1 := cryptoStoreGetSecret(ctx, mach, id.SecretXSMaster)
	self, err2 := cryptoStoreGetSecret(ctx, mach, id.SecretXSSelfSigning)
	user, err3 := cryptoStoreGetSecret(ctx, mach, id.SecretXSUserSigning)

	if !areSecretsValid(err1, err2, err3, master, self, user) {
		return
	}

	decMaster, errM := decodeBase64(master)
	decSelf, errS := decodeBase64(self)
	decUser, errU := decodeBase64(user)

	if !areDecodedSecretsValid(errM, errS, errU, decMaster, decSelf, decUser) {
		return
	}

	err := importCrossSigningKeys(mach, crypto.CrossSigningSeeds{
		MasterKey:      decMaster,
		SelfSigningKey: decSelf,
		UserSigningKey: decUser,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to import cross-signing keys: %v\n", err)
	} else {
		c.Log.Debug().Msg("Successfully loaded cross-signing keys from local store")
	}
}

func areSecretsValid(err1, err2, err3 error, m, s, u string) bool {
	return err1 == nil && err2 == nil && err3 == nil && m != "" && s != "" && u != ""
}

func areDecodedSecretsValid(err1, err2, err3 error, m, s, u []byte) bool {
	return err1 == nil && err2 == nil && err3 == nil && len(m) > 0 && len(s) > 0 && len(u) > 0
}

func (c *Client) saveCrossSigningKeys(ctx context.Context, keys crypto.CrossSigningSeeds) {
	mach := getOlmMachine(c)
	if mach == nil {
		return
	}

	if err1 := cryptoStorePutSecret(ctx, mach, id.SecretXSMaster, base64.RawStdEncoding.EncodeToString(keys.MasterKey)); err1 != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to save master key: %v\n", err1)
	}
	if err2 := cryptoStorePutSecret(ctx, mach, id.SecretXSSelfSigning, base64.RawStdEncoding.EncodeToString(keys.SelfSigningKey)); err2 != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to save self-signing key: %v\n", err2)
	}
	if err3 := cryptoStorePutSecret(ctx, mach, id.SecretXSUserSigning, base64.RawStdEncoding.EncodeToString(keys.UserSigningKey)); err3 != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to save user-signing key: %v\n", err3)
	}
}

func (c *Client) migrateSecrets(ctx context.Context) {
	mach := getOlmMachine(c)
	if mach == nil {
		return
	}
	sqlStore, ok := mach.CryptoStore.(*crypto.SQLCryptoStore)
	if !ok {
		return
	}
	rows, err := sqlStore.DB.Query(ctx, "SELECT name, secret FROM crypto_secrets")
	if err != nil {
		return
	}
	defer func() {
		if err := rowsClose(rows); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to close rows: %v\n", err)
		}
	}()

	updates := make(map[string]string)
	for rows.Next() {
		var name, secret string
		if err := rows.Scan(&name, &secret); err == nil {
			if secret != "" && secret[len(secret)-1] == '=' {
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
