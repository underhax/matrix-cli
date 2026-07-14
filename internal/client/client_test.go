package client

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/underhax/matrix-cli/internal/logger"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/id"

	"github.com/underhax/matrix-cli/internal/config"
)

func TestNew_Success(t *testing.T) {
	ctx := context.Background()
	session := &config.Session{
		HomeserverURL: "https://example.com",
		UserID:        "@user:success.example.com",
		AccessToken:   "token",
		DeviceID:      "DEV",
	}
	db := &sql.DB{}
	picklePath := "test.pickle"

	origMautrixNewClient := mautrixNewClient
	origDbutilNewWithDB := dbutilNewWithDB
	origGetOrGeneratePickleKey := getOrGeneratePickleKey
	origNewCryptoHelper := newCryptoHelper
	origCryptoHelperInit := cryptoHelperInit
	origGetCryptoMachine := getCryptoMachine
	origDoMigrateSecrets := doMigrateSecrets
	origDoRegisterStateHooks := doRegisterStateHooks
	origDoLoadSecrets := doLoadSecrets
	origNewVerificationHelper := newVerificationHelper
	origVerificationHelperInit := verificationHelperInit

	defer func() {
		mautrixNewClient = origMautrixNewClient
		dbutilNewWithDB = origDbutilNewWithDB
		getOrGeneratePickleKey = origGetOrGeneratePickleKey
		newCryptoHelper = origNewCryptoHelper
		cryptoHelperInit = origCryptoHelperInit
		getCryptoMachine = origGetCryptoMachine
		doMigrateSecrets = origDoMigrateSecrets
		doRegisterStateHooks = origDoRegisterStateHooks
		doLoadSecrets = origDoLoadSecrets
		newVerificationHelper = origNewVerificationHelper
		verificationHelperInit = origVerificationHelperInit
	}()

	mautrixNewClient = func(_ string, _ id.UserID, _ string) (*mautrix.Client, error) {
		return &mautrix.Client{}, nil
	}
	dbutilNewWithDB = func(_ *sql.DB, _ string) (*dbutil.Database, error) {
		return &dbutil.Database{}, nil
	}
	getOrGeneratePickleKey = func(_ string) ([]byte, error) {
		return []byte("pickle"), nil
	}
	newCryptoHelper = func(_ *mautrix.Client, _ []byte, _ any) (*cryptohelper.CryptoHelper, error) {
		return &cryptohelper.CryptoHelper{}, nil
	}
	cryptoHelperInit = func(_ context.Context, _ *cryptohelper.CryptoHelper) error { return nil }
	getCryptoMachine = func(_ *cryptohelper.CryptoHelper) *crypto.OlmMachine { return nil }
	doMigrateSecrets = func(_ context.Context, _ *Client) {}
	doRegisterStateHooks = func(_ *Client) {}
	doLoadSecrets = func(_ context.Context, _ *Client) {}
	newVerificationHelper = func(_ *mautrix.Client, _ *crypto.OlmMachine, _ verificationhelper.VerificationStore, _ any, _ bool, _ bool, _ bool) *verificationhelper.VerificationHelper {
		return &verificationhelper.VerificationHelper{}
	}
	verificationHelperInit = func(_ context.Context, _ *verificationhelper.VerificationHelper) error { return nil }

	nopLog := logger.Nop()
	clientObj, err := New(ctx, session, db, picklePath, &nopLog)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if clientObj == nil || clientObj.Matrix == nil {
		t.Fatalf("expected client object to be populated")
	}
}

func TestNew_Failure(t *testing.T) {
	ctx := context.Background()
	session := &config.Session{
		HomeserverURL: "https://example.com",
		UserID:        "@user:failure.example.com",
		AccessToken:   "token",
		DeviceID:      "DEV",
	}
	db := &sql.DB{}
	picklePath := "test.pickle"

	origMautrixNewClient := mautrixNewClient
	origDbutilNewWithDB := dbutilNewWithDB
	origGetOrGeneratePickleKey := getOrGeneratePickleKey
	origNewCryptoHelper := newCryptoHelper
	origCryptoHelperInit := cryptoHelperInit
	origGetCryptoMachine := getCryptoMachine
	origDoMigrateSecrets := doMigrateSecrets
	origDoRegisterStateHooks := doRegisterStateHooks
	origDoLoadSecrets := doLoadSecrets
	origNewVerificationHelper := newVerificationHelper
	origVerificationHelperInit := verificationHelperInit

	defer func() {
		mautrixNewClient = origMautrixNewClient
		dbutilNewWithDB = origDbutilNewWithDB
		getOrGeneratePickleKey = origGetOrGeneratePickleKey
		newCryptoHelper = origNewCryptoHelper
		cryptoHelperInit = origCryptoHelperInit
		getCryptoMachine = origGetCryptoMachine
		doMigrateSecrets = origDoMigrateSecrets
		doRegisterStateHooks = origDoRegisterStateHooks
		doLoadSecrets = origDoLoadSecrets
		newVerificationHelper = origNewVerificationHelper
		verificationHelperInit = origVerificationHelperInit
	}()

	mautrixNewClient = func(_ string, _ id.UserID, _ string) (*mautrix.Client, error) {
		return &mautrix.Client{}, nil
	}
	dbutilNewWithDB = func(_ *sql.DB, _ string) (*dbutil.Database, error) {
		return &dbutil.Database{}, nil
	}
	getOrGeneratePickleKey = func(_ string) ([]byte, error) {
		return []byte("pickle"), nil
	}
	newCryptoHelper = func(_ *mautrix.Client, _ []byte, _ any) (*cryptohelper.CryptoHelper, error) {
		return &cryptohelper.CryptoHelper{}, nil
	}
	cryptoHelperInit = func(_ context.Context, _ *cryptohelper.CryptoHelper) error { return nil }
	getCryptoMachine = func(_ *cryptohelper.CryptoHelper) *crypto.OlmMachine { return nil }
	doMigrateSecrets = func(_ context.Context, _ *Client) {}
	doRegisterStateHooks = func(_ *Client) {}
	doLoadSecrets = func(_ context.Context, _ *Client) {}
	newVerificationHelper = func(_ *mautrix.Client, _ *crypto.OlmMachine, _ verificationhelper.VerificationStore, _ any, _ bool, _ bool, _ bool) *verificationhelper.VerificationHelper {
		return &verificationhelper.VerificationHelper{}
	}
	verificationHelperInit = func(_ context.Context, _ *verificationhelper.VerificationHelper) error { return nil }

	mautrixNewClient = func(_ string, _ id.UserID, _ string) (*mautrix.Client, error) {
		return nil, errors.New("mock mautrix error")
	}
	nopLog := logger.Nop()
	_, err := New(ctx, session, db, picklePath, &nopLog)
	if err == nil || !strings.Contains(err.Error(), "mock mautrix error") {
		t.Errorf("expected mautrix error, got %v", err)
	}
	mautrixNewClient = origMautrixNewClient
	mautrixNewClient = func(_ string, _ id.UserID, _ string) (*mautrix.Client, error) {
		return &mautrix.Client{}, nil
	}

	dbutilNewWithDB = func(_ *sql.DB, _ string) (*dbutil.Database, error) {
		return nil, errors.New("mock dbutil error")
	}
	_, err = New(ctx, session, db, picklePath, &nopLog)
	if err == nil || !strings.Contains(err.Error(), "mock dbutil error") {
		t.Errorf("expected dbutil error, got %v", err)
	}
	dbutilNewWithDB = func(_ *sql.DB, _ string) (*dbutil.Database, error) { return &dbutil.Database{}, nil }

	getOrGeneratePickleKey = func(_ string) ([]byte, error) {
		return nil, errors.New("mock pickle error")
	}
	_, err = New(ctx, session, db, picklePath, &nopLog)
	if err == nil || !strings.Contains(err.Error(), "mock pickle error") {
		t.Errorf("expected pickle error, got %v", err)
	}
	getOrGeneratePickleKey = func(_ string) ([]byte, error) { return []byte("pickle"), nil }

	newCryptoHelper = func(_ *mautrix.Client, _ []byte, _ any) (*cryptohelper.CryptoHelper, error) {
		return nil, errors.New("mock new ch error")
	}
	_, err = New(ctx, session, db, picklePath, &nopLog)
	if err == nil || !strings.Contains(err.Error(), "mock new ch error") {
		t.Errorf("expected new ch error, got %v", err)
	}
	newCryptoHelper = func(_ *mautrix.Client, _ []byte, _ any) (*cryptohelper.CryptoHelper, error) {
		return &cryptohelper.CryptoHelper{}, nil
	}

	cryptoHelperInit = func(_ context.Context, _ *cryptohelper.CryptoHelper) error {
		return errors.New("mock init ch error")
	}
	_, err = New(ctx, session, db, picklePath, &nopLog)
	if err == nil || !strings.Contains(err.Error(), "mock init ch error") {
		t.Errorf("expected init ch error, got %v", err)
	}
	cryptoHelperInit = func(_ context.Context, _ *cryptohelper.CryptoHelper) error { return nil }

	verificationHelperInit = func(_ context.Context, _ *verificationhelper.VerificationHelper) error {
		return errors.New("mock init vh error")
	}
	_, err = New(ctx, session, db, picklePath, &nopLog)
	if err == nil || !strings.Contains(err.Error(), "mock init vh error") {
		t.Errorf("expected init vh error, got %v", err)
	}
}
