// Package client provides the Matrix client operations and E2EE state management.
package client

import (
	"context"
	"database/sql"
	"fmt"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/id"

	"matrix-cli/internal/config"
	"matrix-cli/internal/store"
)

// DebugMode enables verbose debug logging for client operations.
var DebugMode bool

// Client encapsulates the Matrix client, cryptographic state machine, and persistence layer
// to orchestrate E2EE operations in a headless environment.
type Client struct {
	Matrix *mautrix.Client
	Crypto *cryptohelper.CryptoHelper
	DB     *sql.DB
	VH     *verificationhelper.VerificationHelper
}

const (
	jsonKeyStatus   = "status"
	statusCancelled = "cancelled"
	statusSuccess   = "success"
)

// New initializes the mautrix client and delegates all cryptographic lifecycle management
// (store creation, table migrations, OlmMachine setup, syncer hooks) to cryptohelper.CryptoHelper.
func New(ctx context.Context, session *config.Session, db *sql.DB, picklePath string) (*Client, error) {
	cli, err := mautrix.NewClient(session.HomeserverURL, id.UserID(session.UserID), session.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create matrix client: %w", err)
	}
	cli.DeviceID = id.DeviceID(session.DeviceID)

	dbWrap, err := dbutil.NewWithDB(db, "sqlite3")
	if err != nil {
		return nil, fmt.Errorf("failed to wrap database: %w", err)
	}

	pickleKey, err := store.GetOrGeneratePickleKey(picklePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get pickle key: %w", err)
	}

	ch, err := cryptohelper.NewCryptoHelper(cli, pickleKey, dbWrap)
	if err != nil {
		return nil, fmt.Errorf("failed to create crypto helper: %w", err)
	}

	if err := ch.Init(ctx); err != nil {
		return nil, fmt.Errorf("failed to init crypto helper: %w", err)
	}
	cli.Crypto = ch

	mach := ch.Machine()

	clientObj := &Client{
		Matrix: cli,
		Crypto: ch,
		DB:     db,
	}

	clientObj.migrateSecrets(ctx)

	clientObj.registerStateHooks()

	clientObj.loadSecrets(ctx)

	handler := &VerificationHandler{client: clientObj}

	supportsQRShow := false
	supportsQRScan := false
	supportsSAS := true
	vh := verificationhelper.NewVerificationHelper(cli, mach, verificationhelper.NewInMemoryVerificationStore(), handler, supportsQRShow, supportsQRScan, supportsSAS)
	if err := vh.Init(ctx); err != nil {
		return nil, fmt.Errorf("failed to init verification helper: %w", err)
	}
	clientObj.VH = vh

	return clientObj, nil
}
