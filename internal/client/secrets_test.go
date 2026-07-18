package client

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/id"
)

func TestSecrets_DecodeBase64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []byte
		wantErr bool
	}{
		{
			name:    "std_encoding_with_padding",
			input:   "+/8=",
			want:    []byte{251, 255},
			wantErr: false,
		},
		{
			name:    "raw_std_encoding",
			input:   "+/8",
			want:    []byte{251, 255},
			wantErr: false,
		},
		{
			name:    "url_encoding_with_padding",
			input:   "-_8=",
			want:    []byte{251, 255},
			wantErr: false,
		},
		{
			name:    "raw_url_encoding",
			input:   "-_8",
			want:    []byte{251, 255},
			wantErr: false,
		},
		{
			name:    "invalid_base64",
			input:   "&^%",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeBase64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeBase64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !bytes.Equal(got, tt.want) {
				t.Errorf("decodeBase64() got = %v, want %v", got, tt.want)
			}
		})
	}
}

type requestSecretsTest struct {
	mockSecretKeys      map[id.Secret]string
	mockSecretErr       map[id.Secret]error
	mockLocalSecrets    map[id.Secret]string
	mockSignErr         error
	name                string
	crossSigningKeysNil bool
	cryptoStoreNil      bool
}

func mockGetOrRequestSecretImpl(done chan struct{}, tt requestSecretsTest, name id.Secret, cb func(string) (bool, error)) error {
	if tt.mockSecretErr != nil && tt.mockSecretErr[name] != nil {
		if name == id.SecretMegolmBackupV1 && tt.mockSecretKeys[id.SecretXSMaster] == "" {
			close(done)
		}
		return tt.mockSecretErr[name]
	}
	if val := tt.mockSecretKeys[name]; val != "" {
		if _, err := cb(val); err != nil {
			return err
		}
	}

	if name == id.SecretMegolmBackupV1 && tt.mockSecretKeys[id.SecretXSMaster] == "" {
		close(done)
	}
	return nil
}

func runRequestSecretsTest(t *testing.T, tt requestSecretsTest) {
	ctx := context.Background()
	done := make(chan struct{})
	c := &Client{
		Matrix: &mautrix.Client{DeviceID: "test_device"},
	}

	getOlmMachine = func(_ *Client) *crypto.OlmMachine {
		if tt.name == "mach_nil" {
			close(done)
			return nil
		}
		mach := &crypto.OlmMachine{
			Client: c.Matrix,
		}
		if !tt.cryptoStoreNil {
			mach.CryptoStore = &crypto.SQLCryptoStore{}
		}
		if !tt.crossSigningKeysNil {
			mach.CrossSigningKeys = &crypto.CrossSigningKeysCache{}
		}
		return mach
	}
	defer func() { getOlmMachine = defaultGetOlmMachine }()

	cryptoStoreGetSecret = func(_ context.Context, _ *crypto.OlmMachine, name id.Secret) (string, error) {
		if tt.mockLocalSecrets != nil && tt.mockLocalSecrets[name] != "" {
			return tt.mockLocalSecrets[name], nil
		}
		return "", errors.New("not found")
	}
	defer func() { cryptoStoreGetSecret = defaultCryptoStoreGetSecret }()

	getOrRequestSecret = func(_ context.Context, _ *crypto.OlmMachine, name id.Secret, cb func(string) (bool, error), _ time.Duration) error {
		return mockGetOrRequestSecretImpl(done, tt, name, cb)
	}
	defer func() { getOrRequestSecret = defaultGetOrRequestSecret }()

	doLoadSecrets = func(_ context.Context, _ *Client) {
		if tt.crossSigningKeysNil {
			close(done)
		}
	}
	defer func() { doLoadSecrets = defaultLoadSecrets }()

	signOwnDevice = func(_ context.Context, _ *crypto.OlmMachine, _ *id.Device) error {
		close(done)
		return tt.mockSignErr
	}
	defer func() { signOwnDevice = defaultSignOwnDevice }()

	ownIdentity = func(_ *crypto.OlmMachine) *id.Device {
		return &id.Device{}
	}
	defer func() { ownIdentity = defaultOwnIdentity }()

	complete := make(chan struct{})
	c.requestSecrets(ctx, func(_ bool) {
		close(complete)
	})

	select {
	case <-complete:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for requestSecrets goroutine")
	}
}

func TestRequestSecrets(t *testing.T) {
	tests := []requestSecretsTest{
		{
			name:        "all_secrets_success",
			mockSignErr: nil,
			mockSecretKeys: map[id.Secret]string{
				id.SecretXSMaster:       "m1",
				id.SecretXSSelfSigning:  "s1",
				id.SecretXSUserSigning:  "u1",
				id.SecretMegolmBackupV1: "b1",
			},
			mockSecretErr:       nil,
			crossSigningKeysNil: false,
		},
		{
			name:        "some_secrets_timeout",
			mockSignErr: nil,
			mockSecretKeys: map[id.Secret]string{
				id.SecretXSMaster:       "m2",
				id.SecretXSSelfSigning:  "s2",
				id.SecretXSUserSigning:  "u2",
				id.SecretMegolmBackupV1: "",
			},
			mockSecretErr: map[id.Secret]error{
				id.SecretMegolmBackupV1: errors.New("timeout"),
			},
			crossSigningKeysNil: false,
		},
		{
			name:        "missing_mandatory_keys",
			mockSignErr: nil,
			mockSecretKeys: map[id.Secret]string{
				id.SecretXSMaster:       "",
				id.SecretXSSelfSigning:  "",
				id.SecretXSUserSigning:  "",
				id.SecretMegolmBackupV1: "",
			},
			mockSecretErr:       nil,
			crossSigningKeysNil: false,
		},
		{
			name:        "sign_own_device_error",
			mockSignErr: errors.New("sign error"),
			mockSecretKeys: map[id.Secret]string{
				id.SecretXSMaster:       "m3",
				id.SecretXSSelfSigning:  "s3",
				id.SecretXSUserSigning:  "u3",
				id.SecretMegolmBackupV1: "",
			},
			mockSecretErr:       nil,
			crossSigningKeysNil: false,
		},
		{
			name:        "no_cross_signing_keys_to_sign",
			mockSignErr: nil,
			mockSecretKeys: map[id.Secret]string{
				id.SecretXSMaster:       "m4",
				id.SecretXSSelfSigning:  "s4",
				id.SecretXSUserSigning:  "u4",
				id.SecretMegolmBackupV1: "",
			},
			mockSecretErr:       nil,
			crossSigningKeysNil: true,
		},
		{
			name:                "mach_nil",
			mockSignErr:         nil,
			mockSecretKeys:      nil,
			mockSecretErr:       nil,
			crossSigningKeysNil: false,
		},
		{
			name:        "cryptostore_nil",
			mockSignErr: nil,
			mockSecretKeys: map[id.Secret]string{
				id.SecretXSMaster:       "m1",
				id.SecretXSSelfSigning:  "s1",
				id.SecretXSUserSigning:  "u1",
				id.SecretMegolmBackupV1: "b1",
			},
			mockSecretErr:       nil,
			crossSigningKeysNil: false,
			cryptoStoreNil:      true,
		},
		{
			name:        "some_secrets_local",
			mockSignErr: nil,
			mockSecretKeys: map[id.Secret]string{
				id.SecretXSMaster:       "m1",
				id.SecretXSSelfSigning:  "s1",
				id.SecretXSUserSigning:  "u1",
				id.SecretMegolmBackupV1: "b1",
			},
			mockLocalSecrets: map[id.Secret]string{
				id.SecretXSMaster:      "m1",
				id.SecretXSSelfSigning: "s1",
			},
			mockSecretErr:       nil,
			crossSigningKeysNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runRequestSecretsTest(t, tt)
		})
	}
}

func TestLoadSecrets(t *testing.T) {
	tests := []struct {
		secretErr error
		importErr error
		secrets   map[id.Secret]string
		name      string
		machNil   bool
	}{
		{
			name:    "mach_nil_load",
			machNil: true,
		},
		{
			name:      "get_secret_error",
			secrets:   map[id.Secret]string{},
			secretErr: errors.New("get error"),
		},
		{
			name: "missing_secret",
			secrets: map[id.Secret]string{
				id.SecretXSMaster:      "m",
				id.SecretXSSelfSigning: "s",
				id.SecretXSUserSigning: "",
			},
		},
		{
			name: "decode_error",
			secrets: map[id.Secret]string{
				id.SecretXSMaster:      "invalid-base64-!@#$",
				id.SecretXSSelfSigning: "validbase64",
				id.SecretXSUserSigning: "validbase64",
			},
		},
		{
			name: "import_error",
			secrets: map[id.Secret]string{
				id.SecretXSMaster:      "dmFsaWQx",
				id.SecretXSSelfSigning: "dmFsaWQy",
				id.SecretXSUserSigning: "dmFsaWQz",
			},
			importErr: errors.New("import error"),
		},
		{
			name: "success_load",
			secrets: map[id.Secret]string{
				id.SecretXSMaster:      "dmFsaWQx",
				id.SecretXSSelfSigning: "dmFsaWQy",
				id.SecretXSUserSigning: "dmFsaWQz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			c := &Client{}

			getOlmMachine = func(_ *Client) *crypto.OlmMachine {
				if tt.machNil {
					return nil
				}
				return &crypto.OlmMachine{}
			}
			defer func() { getOlmMachine = defaultGetOlmMachine }()

			cryptoStoreGetSecret = func(_ context.Context, _ *crypto.OlmMachine, secretID id.Secret) (string, error) {
				if tt.secretErr != nil {
					return "", tt.secretErr
				}
				return tt.secrets[secretID], nil
			}
			defer func() { cryptoStoreGetSecret = defaultCryptoStoreGetSecret }()

			importCrossSigningKeys = func(_ *crypto.OlmMachine, _ crypto.CrossSigningSeeds) error {
				return tt.importErr
			}
			defer func() { importCrossSigningKeys = defaultImportCrossSigningKeys }()

			c.loadSecrets(context.Background())
		})
	}
}

func TestSaveCrossSigningKeys(t *testing.T) {
	tests := []struct {
		putErr  error
		name    string
		machNil bool
	}{
		{
			name:    "mach_nil_save",
			machNil: true,
		},
		{
			name:   "put_error",
			putErr: errors.New("put error"),
		},
		{
			name:   "success_save",
			putErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			c := &Client{}

			getOlmMachine = func(_ *Client) *crypto.OlmMachine {
				if tt.machNil {
					return nil
				}
				return &crypto.OlmMachine{}
			}
			defer func() { getOlmMachine = defaultGetOlmMachine }()

			cryptoStorePutSecret = func(_ context.Context, _ *crypto.OlmMachine, _ id.Secret, _ string) error {
				return tt.putErr
			}
			defer func() { cryptoStorePutSecret = defaultCryptoStorePutSecret }()

			c.saveCrossSigningKeys(context.Background(), crypto.CrossSigningSeeds{
				MasterKey:      []byte("m"),
				SelfSigningKey: []byte("s"),
				UserSigningKey: []byte("u"),
			})
		})
	}
}

func TestMigrateSecrets(t *testing.T) {
	c := &Client{}

	getOlmMachine = func(_ *Client) *crypto.OlmMachine {
		return nil
	}
	c.migrateSecrets(context.Background())

	mach := &crypto.OlmMachine{
		CryptoStore: nil,
	}
	getOlmMachine = func(_ *Client) *crypto.OlmMachine {
		return mach
	}
	c.migrateSecrets(context.Background())

	sqldb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create sql db: %v", err)
	}
	db, err := dbutil.NewWithDB(sqldb, "sqlite3")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	_, err = db.Exec(context.Background(), "CREATE TABLE crypto_secrets (name TEXT, secret TEXT, CHECK(secret != 'ZmFpbF92YWx1ZQ'))")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	rawSec := []byte("secret_valu")
	encodedSec := base64.StdEncoding.EncodeToString(rawSec)

	_, err = db.Exec(context.Background(), "INSERT INTO crypto_secrets (name, secret) VALUES ($1, $2)", string(id.SecretXSMaster), encodedSec)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	_, err = db.Exec(context.Background(), "INSERT INTO crypto_secrets (name, secret) VALUES ($1, $2)", string(id.SecretXSSelfSigning), "not_base64_!@#$=")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	failRaw := []byte("fail_value")
	failEncoded := base64.StdEncoding.EncodeToString(failRaw)
	_, err = db.Exec(context.Background(), "INSERT INTO crypto_secrets (name, secret) VALUES ($1, $2)", string(id.SecretXSUserSigning), failEncoded)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	mach.CryptoStore = &crypto.SQLCryptoStore{DB: db}
	c.migrateSecrets(context.Background())

	var migrated string
	err = db.QueryRow(context.Background(), "SELECT secret FROM crypto_secrets WHERE name=$1", string(id.SecretXSMaster)).Scan(&migrated)
	if err != nil {
		t.Fatalf("failed to query migrated: %v", err)
	}

	if migrated != base64.RawStdEncoding.EncodeToString(rawSec) {
		t.Errorf("expected %s, got %s", base64.RawStdEncoding.EncodeToString(rawSec), migrated)
	}

	_, err = db.Exec(context.Background(), "DROP TABLE crypto_secrets")
	if err != nil {
		t.Fatalf("failed to drop table: %v", err)
	}
	c.migrateSecrets(context.Background())

	_, err = db.Exec(context.Background(), "CREATE TABLE crypto_secrets (name TEXT, secret TEXT)")
	if err != nil {
		t.Fatalf("failed to create table again: %v", err)
	}
	rowsClose = func(_ dbutil.Rows) error {
		return errors.New("rows close error")
	}
	c.migrateSecrets(context.Background())
	rowsClose = defaultRowsClose

	getOlmMachine = defaultGetOlmMachine
}
