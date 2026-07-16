package client

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func TestRefreshCrossSigningKeys(t *testing.T) {
	c := &Client{}

	tests := []struct {
		fetchKeysErr error
		dbExecErr    error
		cryptoStore  crypto.Store
		name         string
		machNil      bool
	}{
		{
			name:    "mach is nil",
			machNil: true,
		},
		{
			name:        "store is not SQLCryptoStore",
			cryptoStore: &crypto.MemoryStore{},
		},
		{
			name:         "store is not SQLCryptoStore, fetchKeys fails",
			cryptoStore:  &crypto.MemoryStore{},
			fetchKeysErr: errors.New("fetch failed"),
		},
		{
			name:        "store is SQLCryptoStore, success",
			cryptoStore: &crypto.SQLCryptoStore{},
		},
		{
			name:        "store is SQLCryptoStore, db fails",
			cryptoStore: &crypto.SQLCryptoStore{},
			dbExecErr:   errors.New("db error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getOlmMachine = func(_ *Client) *crypto.OlmMachine {
				if tt.machNil {
					return nil
				}
				mach := &crypto.OlmMachine{
					CryptoStore: tt.cryptoStore,
				}

				if sqlStore, ok := tt.cryptoStore.(*crypto.SQLCryptoStore); ok {
					setupTestDB(t, sqlStore, tt.dbExecErr)
				}

				return mach
			}
			defer func() { getOlmMachine = defaultGetOlmMachine }()

			fetchKeys = func(_ context.Context, _ *crypto.OlmMachine, _ []id.UserID, _ bool) (map[id.UserID]map[id.DeviceID]*id.Device, error) {
				return nil, tt.fetchKeysErr
			}
			defer func() { fetchKeys = defaultFetchKeys }()

			c.refreshCrossSigningKeys(context.Background(), "test_user")
		})
	}
}

func setupTestDB(t *testing.T, sqlStore *crypto.SQLCryptoStore, dbExecErr error) {
	t.Helper()

	sqldb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sql: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := sqldb.Close(); closeErr != nil {
			t.Logf("failed to close sqldb: %v", closeErr)
		}
	})

	db, err := dbutil.NewWithDB(sqldb, "sqlite3")
	if err != nil {
		t.Fatalf("failed to create dbutil: %v", err)
	}

	if dbExecErr == nil {
		if _, err := db.Exec(context.Background(), "CREATE TABLE crypto_cross_signing_keys (user_id TEXT)"); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
		if _, err := db.Exec(context.Background(), "CREATE TABLE crypto_devices (user_id TEXT)"); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
		if _, err := db.Exec(context.Background(), "CREATE TABLE crypto_cross_signing_signatures (user_id TEXT, sign_user_id TEXT)"); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
	}

	sqlStore.DB = db
}

func TestVerify(t *testing.T) {
	c := &Client{
		Matrix: &mautrix.Client{},
	}

	tests := []struct {
		syncErr1             error
		syncErr2             error
		startVerificationErr error
		name                 string
		targetUser           string
		expectedErr          bool
	}{
		{
			name:        "empty target user, sync success",
			targetUser:  "",
			expectedErr: false,
		},
		{
			name:        "empty target user, sync fails",
			targetUser:  "",
			syncErr1:    errors.New("sync failed"),
			expectedErr: true,
		},
		{
			name:                 "non-empty target user, start verification fails",
			targetUser:           "@user2:example.com",
			startVerificationErr: errors.New("start failed"),
			expectedErr:          true,
		},
		{
			name:        "non-empty target user, sync fails",
			targetUser:  "@user3:example.com",
			syncErr2:    errors.New("sync failed"),
			expectedErr: true,
		},
		{
			name:        "non-empty target user, success",
			targetUser:  "@user4:example.com",
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matrixSyncWithContext = func(_ context.Context, _ *mautrix.Client) error {
				if tt.targetUser == "" {
					return tt.syncErr1
				}
				return tt.syncErr2
			}
			defer func() { matrixSyncWithContext = defaultMatrixSyncWithContext }()

			startVerification = func(_ context.Context, _ *verificationhelper.VerificationHelper, _ id.UserID) (id.VerificationTransactionID, error) {
				return "txn123", tt.startVerificationErr
			}
			defer func() { startVerification = defaultStartVerification }()

			getOlmMachine = func(_ *Client) *crypto.OlmMachine {
				return nil
			}
			defer func() { getOlmMachine = defaultGetOlmMachine }()

			err := c.Verify(context.Background(), tt.targetUser)
			if (err != nil) != tt.expectedErr {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestVerificationRequested(t *testing.T) {
	c := &Client{}
	h := &VerificationHandler{client: c}

	tests := []struct {
		acceptErr error
		name      string
	}{
		{
			name:      "accept success",
			acceptErr: nil,
		},
		{
			name:      "accept fails",
			acceptErr: errors.New("accept failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			var wg sync.WaitGroup
			wg.Add(1)

			acceptVerification = func(_ context.Context, _ *verificationhelper.VerificationHelper, _ id.VerificationTransactionID) error {
				defer wg.Done()
				return tt.acceptErr
			}
			defer func() { acceptVerification = defaultAcceptVerification }()

			h.VerificationRequested(context.Background(), "txn123", "@user1:example.com", "device1")

			wg.Wait()
		})
	}
}

func TestVerificationReady(t *testing.T) {
	c := &Client{}
	h := &VerificationHandler{client: c}

	tests := []struct {
		startErr    error
		name        string
		supportsSAS bool
	}{
		{
			name:        "sas not supported",
			supportsSAS: false,
		},
		{
			name:        "sas supported, success",
			supportsSAS: true,
			startErr:    nil,
		},
		{
			name:        "sas supported, fails",
			supportsSAS: true,
			startErr:    errors.New("start failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			var wg sync.WaitGroup

			if tt.supportsSAS {
				wg.Add(1)
			}

			startSAS = func(_ context.Context, _ *verificationhelper.VerificationHelper, _ id.VerificationTransactionID) error {
				defer wg.Done()
				return tt.startErr
			}
			defer func() { startSAS = defaultStartSAS }()

			h.VerificationReady(context.Background(), "txn123", "device1", tt.supportsSAS, false, nil)

			if tt.supportsSAS {
				wg.Wait()
			}
		})
	}
}

func TestVerificationCancelled(_ *testing.T) {
	c := &Client{}
	h := &VerificationHandler{client: c}

	h.VerificationCancelled(context.Background(), "txn123", "m.user", "user cancelled")
}

func TestVerificationDone(_ *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	getOlmMachine = func(_ *Client) *crypto.OlmMachine {
		defer wg.Done()
		return nil
	}
	defer func() { getOlmMachine = defaultGetOlmMachine }()

	c := &Client{
		Matrix: &mautrix.Client{DeviceID: "test"},
	}
	h := &VerificationHandler{client: c}

	h.VerificationDone(context.Background(), "txn123", "")
	wg.Wait()
}

type errReader struct{ wg *sync.WaitGroup }

func (e errReader) Read(_ []byte) (int, error) {
	defer e.wg.Done()
	return 0, errors.New("read error")
}

func TestShowSAS(t *testing.T) {
	c := &Client{
		VH: &verificationhelper.VerificationHelper{},
	}
	h := &VerificationHandler{client: c}

	emojis := []rune{'🐶', '🐱'}
	emojiDescs := []string{"Dog", "Cat"}

	t.Run("read_error", func(_ *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		originalStdin := stdin
		stdin = errReader{wg: &wg}
		defer func() { stdin = originalStdin }()

		h.ShowSAS(context.Background(), "txn1", emojis, emojiDescs, nil)
		wg.Wait()
	})

	t.Run("confirm_yes", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		originalStdin := stdin
		stdin = strings.NewReader("y\n")
		defer func() { stdin = originalStdin }()

		confirmSAS = func(_ context.Context, _ *verificationhelper.VerificationHelper, _ id.VerificationTransactionID) error {
			defer wg.Done()
			return nil
		}
		defer func() { confirmSAS = defaultConfirmSAS }()

		cancelVerification = func(_ context.Context, _ *verificationhelper.VerificationHelper, _ id.VerificationTransactionID, _ event.VerificationCancelCode, _ string) error {
			t.Fatal("unexpected call to cancelVerification")
			return nil
		}
		defer func() { cancelVerification = defaultCancelVerification }()

		h.ShowSAS(context.Background(), "txn2", emojis, emojiDescs, nil)
		wg.Wait()
	})

	t.Run("cancel_no", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		originalStdin := stdin
		stdin = strings.NewReader("n\n")
		defer func() { stdin = originalStdin }()

		confirmSAS = func(_ context.Context, _ *verificationhelper.VerificationHelper, _ id.VerificationTransactionID) error {
			t.Fatal("unexpected call to confirmSAS")
			return nil
		}
		defer func() { confirmSAS = defaultConfirmSAS }()

		cancelVerification = func(_ context.Context, _ *verificationhelper.VerificationHelper, _ id.VerificationTransactionID, _ event.VerificationCancelCode, _ string) error {
			defer wg.Done()
			return nil
		}
		defer func() { cancelVerification = defaultCancelVerification }()

		emojisShort := []rune{'🐶'}
		emojiDescsShort := []string{}

		h.ShowSAS(context.Background(), "txn3", emojisShort, emojiDescsShort, nil)
		wg.Wait()
	})

	t.Run("confirm_error", func(_ *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		stdin = strings.NewReader("y\n")
		confirmSAS = func(_ context.Context, _ *verificationhelper.VerificationHelper, _ id.VerificationTransactionID) error {
			defer wg.Done()
			return errors.New("confirm failed")
		}
		h.ShowSAS(context.Background(), "txn4", emojis, emojiDescs, nil)
		wg.Wait()
	})

	t.Run("cancel_error", func(_ *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		stdin = strings.NewReader("n\n")
		cancelVerification = func(_ context.Context, _ *verificationhelper.VerificationHelper, _ id.VerificationTransactionID, _ event.VerificationCancelCode, _ string) error {
			defer wg.Done()
			return errors.New("cancel failed")
		}
		h.ShowSAS(context.Background(), "txn5", emojis, emojiDescs, nil)
		wg.Wait()
	})
}
