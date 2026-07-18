package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/term"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/backup"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/crypto/ssss"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/underhax/matrix-cli/internal/store"
)

func defaultVerifyWithRecoveryKey(ctx context.Context, mach *crypto.OlmMachine, recoveryKey string) error {
	return wrapErr(mach.VerifyWithRecoveryKey(ctx, recoveryKey), "verify failed: %w")
}

var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
	stdin  io.Reader = os.Stdin

	termIsTerminal   = term.IsTerminal
	termReadPassword = term.ReadPassword
	getStdinFd       = defaultGetStdinFd
)

func defaultGetStdinFd() int { return int(os.Stdin.Fd()) }

var verifyWithRecoveryKey = defaultVerifyWithRecoveryKey

func defaultExportCrossSigningKeys(mach *crypto.OlmMachine) crypto.CrossSigningSeeds {
	return mach.ExportCrossSigningKeys()
}

var exportCrossSigningKeys = defaultExportCrossSigningKeys

func defaultImportCrossSigningKeys(mach *crypto.OlmMachine, keys crypto.CrossSigningSeeds) error {
	return wrapErr(mach.ImportCrossSigningKeys(keys), "import failed: %w")
}

var importCrossSigningKeys = defaultImportCrossSigningKeys

func defaultCryptoStoreGetSecret(ctx context.Context, mach *crypto.OlmMachine, secretID id.Secret) (string, error) {
	val, err := mach.CryptoStore.GetSecret(ctx, secretID)
	return val, wrapErr(err, "get secret failed: %w")
}

var cryptoStoreGetSecret = defaultCryptoStoreGetSecret

func defaultCryptoStorePutSecret(ctx context.Context, mach *crypto.OlmMachine, secretID id.Secret, secret string) error {
	return wrapErr(mach.CryptoStore.PutSecret(ctx, secretID, secret), "put secret failed: %w")
}

var cryptoStorePutSecret = defaultCryptoStorePutSecret

func defaultClearCrossSigningSecrets(ctx context.Context, mach *crypto.OlmMachine) error {
	sqlStore, ok := mach.CryptoStore.(*crypto.SQLCryptoStore)
	if !ok {
		return nil
	}
	_, err := sqlStore.DB.Exec(ctx, "DELETE FROM crypto_secrets WHERE account_id=$1 AND name LIKE 'm.cross_signing.%'", sqlStore.AccountID)
	return wrapErr(err, "delete cross-signing secrets failed: %w")
}

var clearCrossSigningSecrets = defaultClearCrossSigningSecrets

func defaultGenerateAndUploadCrossSigningKeys(ctx context.Context, mach *crypto.OlmMachine, cb func(*mautrix.RespUserInteractive) any, masterKey string) (string, *crypto.CrossSigningKeysCache, error) {
	key, cache, err := mach.GenerateAndUploadCrossSigningKeys(ctx, cb, masterKey)
	return key, cache, wrapErr(err, "generate failed: %w")
}

var generateAndUploadCrossSigningKeys = defaultGenerateAndUploadCrossSigningKeys

func wrapErr(err error, format string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format, err)
}

func defaultSignOwnDevice(ctx context.Context, mach *crypto.OlmMachine, identity *id.Device) error {
	return wrapErr(mach.SignOwnDevice(ctx, identity), "sign device failed: %w")
}

var signOwnDevice = defaultSignOwnDevice

func defaultSignOwnMasterKey(ctx context.Context, mach *crypto.OlmMachine) error {
	return wrapErr(mach.SignOwnMasterKey(ctx), "sign master key failed: %w")
}

var signOwnMasterKey = defaultSignOwnMasterKey

func defaultOwnIdentity(mach *crypto.OlmMachine) *id.Device {
	return mach.OwnIdentity()
}

var ownIdentity = defaultOwnIdentity

func defaultGetCrossSigningPublicKeys(ctx context.Context, mach *crypto.OlmMachine, userID id.UserID) (*crypto.CrossSigningPublicKeysCache, error) {
	pubkeys, err := mach.GetCrossSigningPublicKeys(ctx, userID)
	return pubkeys, wrapErr(err, "get cross-signing public keys failed: %w")
}

var getCrossSigningPublicKeys = defaultGetCrossSigningPublicKeys

func defaultGetOwnCrossSigningPublicKeys(ctx context.Context, mach *crypto.OlmMachine) (*crypto.CrossSigningPublicKeysCache, error) {
	pubkeys, err := mach.GetOwnCrossSigningPublicKeys(ctx)
	return pubkeys, wrapErr(err, "get own cross-signing public keys failed: %w")
}

var getOwnCrossSigningPublicKeys = defaultGetOwnCrossSigningPublicKeys

func defaultSetupMegolmBackup(ctx context.Context, c *Client, recoveryKey string) error {
	return c.setupMegolmBackup(ctx, recoveryKey)
}

var doSetupMegolmBackup = defaultSetupMegolmBackup

func defaultSaveCrossSigningKeys(ctx context.Context, c *Client, keys crypto.CrossSigningSeeds) {
	c.saveCrossSigningKeys(ctx, keys)
}

var doSaveCrossSigningKeys = defaultSaveCrossSigningKeys

func defaultGetOlmMachine(c *Client) *crypto.OlmMachine {
	if c.Crypto == nil {
		return nil
	}
	return c.Crypto.Machine()
}

var getOlmMachine = defaultGetOlmMachine

func defaultSSSSGetDefaultKeyData(ctx context.Context, mach *crypto.OlmMachine) (string, *ssss.KeyMetadata, error) {
	keyID, data, err := mach.SSSS.GetDefaultKeyData(ctx)
	return keyID, data, wrapErr(err, "get default key data failed: %w")
}

var ssssGetDefaultKeyData = defaultSSSSGetDefaultKeyData

func defaultVerifyRecoveryKey(keyData *ssss.KeyMetadata, keyID, recoveryKey string) (*ssss.Key, error) {
	key, err := keyData.VerifyRecoveryKey(keyID, recoveryKey)
	return key, wrapErr(err, "verify recovery key failed: %w")
}

var verifyRecoveryKey = defaultVerifyRecoveryKey

func defaultNewMegolmBackupKey() (*backup.MegolmBackupKey, error) {
	key, err := backup.NewMegolmBackupKey()
	return key, wrapErr(err, "new megolm backup key failed: %w")
}

var newMegolmBackupKey = defaultNewMegolmBackupKey

func defaultPutSecret(ctx context.Context, mach *crypto.OlmMachine, secretID id.Secret, secret string) error {
	return wrapErr(mach.CryptoStore.PutSecret(ctx, secretID, secret), "put secret failed: %w")
}

var putSecret = defaultPutSecret

func defaultSetEncryptedAccountData(ctx context.Context, mach *crypto.OlmMachine, eventType event.Type, data []byte, key *ssss.Key) error {
	return wrapErr(mach.SSSS.SetEncryptedAccountData(ctx, eventType, data, key), "set encrypted account data failed: %w")
}

var setEncryptedAccountData = defaultSetEncryptedAccountData

func defaultGetDecryptedAccountData(ctx context.Context, mach *crypto.OlmMachine, eventType event.Type, key *ssss.Key) ([]byte, error) {
	if mach == nil || mach.SSSS == nil {
		return nil, errors.New("machine or ssss is nil")
	}
	data, err := mach.SSSS.GetDecryptedAccountData(ctx, eventType, key)
	return data, wrapErr(err, "get decrypted account data failed: %w")
}

var getDecryptedAccountData = defaultGetDecryptedAccountData

func defaultCreateKeyBackupVersion(ctx context.Context, mach *crypto.OlmMachine, req *mautrix.ReqRoomKeysVersionCreate[backup.MegolmAuthData]) (*mautrix.RespRoomKeysVersionCreate, error) {
	resp, err := mach.Client.CreateKeyBackupVersion(ctx, req)
	return resp, wrapErr(err, "create key backup version failed: %w")
}

var createKeyBackupVersion = defaultCreateKeyBackupVersion

var mautrixNewClient = mautrix.NewClient

var dbutilNewWithDB = dbutil.NewWithDB

var getOrGeneratePickleKey = store.GetOrGeneratePickleKey

var newCryptoHelper = cryptohelper.NewCryptoHelper

func defaultCryptoHelperInit(ctx context.Context, ch *cryptohelper.CryptoHelper) error {
	return wrapErr(ch.Init(ctx), "crypto helper init failed: %w")
}

var cryptoHelperInit = defaultCryptoHelperInit

func defaultGetCryptoMachine(ch *cryptohelper.CryptoHelper) *crypto.OlmMachine {
	return ch.Machine()
}

var getCryptoMachine = defaultGetCryptoMachine

func defaultMigrateSecrets(ctx context.Context, c *Client) {
	c.migrateSecrets(ctx)
}

var doMigrateSecrets = defaultMigrateSecrets

func defaultRegisterStateHooks(c *Client) {
	c.registerStateHooks()
}

var doRegisterStateHooks = defaultRegisterStateHooks

func defaultLoadSecrets(ctx context.Context, c *Client) {
	c.loadSecrets(ctx)
}

var doLoadSecrets = defaultLoadSecrets

var newVerificationHelper = verificationhelper.NewVerificationHelper

func defaultVerificationHelperInit(ctx context.Context, vh *verificationhelper.VerificationHelper) error {
	return wrapErr(vh.Init(ctx), "verification helper init failed: %w")
}

var verificationHelperInit = defaultVerificationHelperInit

var getOrFetchDevice = defaultGetOrFetchDevice

func defaultGetOrFetchDevice(ctx context.Context, mach *crypto.OlmMachine, userID id.UserID, deviceID id.DeviceID) (*id.Device, error) {
	if mach == nil {
		return nil, errors.New("machine is nil")
	}
	dev, err := mach.GetOrFetchDevice(ctx, userID, deviceID)
	return dev, wrapErr(err, "get device failed: %w")
}

var resolveTrustContext = defaultResolveTrustContext

func defaultResolveTrustContext(ctx context.Context, mach *crypto.OlmMachine, device *id.Device) (id.TrustState, error) {
	if mach == nil {
		return 0, errors.New("machine is nil")
	}
	trust, err := mach.ResolveTrustContext(ctx, device)
	return trust, wrapErr(err, "resolve trust failed: %w")
}

var getSecret = defaultGetSecret

func defaultGetSecret(ctx context.Context, mach *crypto.OlmMachine, name id.Secret) (string, error) {
	if mach == nil || mach.CryptoStore == nil {
		return "", errors.New("machine or cryptostore is nil")
	}
	secret, err := mach.CryptoStore.GetSecret(ctx, name)
	return secret, wrapErr(err, "get secret failed: %w")
}

var getOrRequestSecret = defaultGetOrRequestSecret

func defaultGetOrRequestSecret(ctx context.Context, mach *crypto.OlmMachine, name id.Secret, cb func(string) (bool, error), timeout time.Duration) error {
	if mach == nil {
		return errors.New("machine is nil")
	}
	return wrapErr(mach.GetOrRequestSecret(ctx, name, cb, timeout), "get or request secret failed: %w")
}

var sendEncryptedToDevice = defaultSendEncryptedToDevice

func defaultSendEncryptedToDevice(ctx context.Context, mach *crypto.OlmMachine, device *id.Device, evtType event.Type, content event.Content) error {
	if mach == nil {
		return errors.New("machine is nil")
	}
	return wrapErr(mach.SendEncryptedToDevice(ctx, device, evtType, content), "send encrypted failed: %w")
}

func defaultFetchKeys(ctx context.Context, mach *crypto.OlmMachine, users []id.UserID, force bool) (map[id.UserID]map[id.DeviceID]*id.Device, error) {
	devs, err := mach.FetchKeys(ctx, users, force)
	return devs, wrapErr(err, "fetch keys failed: %w")
}

var fetchKeys = defaultFetchKeys

func defaultMatrixSyncWithContext(ctx context.Context, client *mautrix.Client) error {
	return wrapErr(client.SyncWithContext(ctx), "sync failed: %w")
}

var matrixSyncWithContext = defaultMatrixSyncWithContext

func defaultStartVerification(ctx context.Context, vh *verificationhelper.VerificationHelper, userID id.UserID) (id.VerificationTransactionID, error) {
	txnID, err := vh.StartVerification(ctx, userID)
	return txnID, wrapErr(err, "start verification failed: %w")
}

var startVerification = defaultStartVerification

func defaultAcceptVerification(ctx context.Context, vh *verificationhelper.VerificationHelper, txnID id.VerificationTransactionID) error {
	return wrapErr(vh.AcceptVerification(ctx, txnID), "accept verification failed: %w")
}

var acceptVerification = defaultAcceptVerification

func defaultStartSAS(ctx context.Context, vh *verificationhelper.VerificationHelper, txnID id.VerificationTransactionID) error {
	return wrapErr(vh.StartSAS(ctx, txnID), "start sas failed: %w")
}

var startSAS = defaultStartSAS

func defaultConfirmSAS(ctx context.Context, vh *verificationhelper.VerificationHelper, txnID id.VerificationTransactionID) error {
	return wrapErr(vh.ConfirmSAS(ctx, txnID), "confirm sas failed: %w")
}

var confirmSAS = defaultConfirmSAS

func defaultCancelVerification(ctx context.Context, vh *verificationhelper.VerificationHelper, txnID id.VerificationTransactionID, code event.VerificationCancelCode, reason string) error {
	return wrapErr(vh.CancelVerification(ctx, txnID, code, reason), "cancel verification failed: %w")
}

var cancelVerification = defaultCancelVerification

func defaultJSONMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

var jsonMarshal = defaultJSONMarshal

func defaultJSONMarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

var jsonMarshalIndent = defaultJSONMarshalIndent

func defaultDiscoverClientAPI(ctx context.Context, server string) (*mautrix.ClientWellKnown, error) {
	resp, err := mautrix.DiscoverClientAPI(ctx, server)
	return resp, wrapErr(err, "discover client api failed: %w")
}

var discoverClientAPI = defaultDiscoverClientAPI

func defaultListenContext(ctx context.Context, network, address string) (net.Listener, error) {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, network, address)
	return listener, wrapErr(err, "listen failed: %w")
}

var listenContext = defaultListenContext

func defaultRowsClose(rows dbutil.Rows) error {
	if rows == nil {
		return nil
	}
	return wrapErr(rows.Close(), "failed to close rows: %w")
}

var rowsClose = defaultRowsClose

func fprintfStderr(format string, a ...any) {
	if _, err := fmt.Fprintf(stderr, format, a...); err != nil {
		return
	}
}

func fprintlnStderr(a ...any) {
	if _, err := fmt.Fprintln(stderr, a...); err != nil {
		return
	}
}

func defaultClearCryptoCache(ctx context.Context, mach *crypto.OlmMachine, userID id.UserID) error {
	if mach == nil {
		return errors.New("machine is nil")
	}
	if sqlStore, ok := mach.CryptoStore.(*crypto.SQLCryptoStore); ok {
		if _, err := sqlStore.DB.Exec(ctx, "DELETE FROM crypto_cross_signing_keys WHERE user_id=$1", userID); err != nil {
			return wrapErr(err, "delete keys: %w")
		}
		if _, err := sqlStore.DB.Exec(ctx, "DELETE FROM crypto_device WHERE user_id=$1", userID); err != nil {
			return wrapErr(err, "delete devices: %w")
		}
		if _, err := sqlStore.DB.Exec(ctx, "DELETE FROM crypto_cross_signing_signatures WHERE signed_user_id=$1 OR signer_user_id=$1", userID); err != nil {
			return wrapErr(err, "delete signatures: %w")
		}
	}
	return nil
}

var clearCryptoCache = defaultClearCryptoCache

func defaultClearCrossSigningSignatures(ctx context.Context, mach *crypto.OlmMachine, userID string) error {
	if mach == nil {
		return errors.New("machine is nil")
	}
	if sqlStore, ok := mach.CryptoStore.(*crypto.SQLCryptoStore); ok {
		_, err := sqlStore.DB.Exec(ctx, "DELETE FROM crypto_cross_signing_signatures WHERE signed_user_id=$1 OR signer_user_id=$1", userID)
		return wrapErr(err, "clear signatures failed: %w")
	}
	return nil
}

var clearCrossSigningSignatures = defaultClearCrossSigningSignatures
