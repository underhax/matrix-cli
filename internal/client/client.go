// Package client provides the Matrix client operations and E2EE state management.
package client

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/id"

	"github.com/underhax/matrix-cli/internal/config"
	"github.com/underhax/matrix-cli/internal/logger"
)

// Client encapsulates the Matrix client, cryptographic state machine, and persistence layer
// to orchestrate E2EE operations in a headless environment.
type Client struct {
	Matrix                 *mautrix.Client
	Crypto                 *cryptohelper.CryptoHelper
	DB                     *sql.DB
	VH                     *verificationhelper.VerificationHelper
	secretTimer            *time.Timer
	Log                    logger.Logger
	ActiveVerificationUser id.UserID
	secretsMu              sync.Mutex
}

const (
	jsonKeyStatus   = "status"
	statusCancelled = "cancelled"
	statusSuccess   = "success"
)

// New creates a new Client instance.
func New(ctx context.Context, session *config.Session, db *sql.DB, picklePath string, log *logger.Logger) (*Client, error) {
	cli, err := mautrixNewClient(session.HomeserverURL, id.UserID(session.UserID), session.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create matrix client: %w", err)
	}
	cli.DeviceID = id.DeviceID(session.DeviceID)

	dbWrap, err := dbutilNewWithDB(db, "sqlite3")
	if err != nil {
		return nil, fmt.Errorf("failed to wrap database: %w", err)
	}

	pickleKey, err := getOrGeneratePickleKey(picklePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get pickle key: %w", err)
	}

	ch, err := newCryptoHelper(cli, pickleKey, dbWrap)
	if err != nil {
		return nil, fmt.Errorf("failed to create crypto helper: %w", err)
	}

	if err := cryptoHelperInit(ctx, ch); err != nil {
		return nil, fmt.Errorf("failed to init crypto helper: %w", err)
	}
	cli.Crypto = ch

	mach := getCryptoMachine(ch)

	clientObj := &Client{
		Matrix: cli,
		Crypto: ch,
		DB:     db,
		Log:    *log,
	}

	doMigrateSecrets(ctx, clientObj)

	doRegisterStateHooks(clientObj)

	doLoadSecrets(ctx, clientObj)

	handler := &VerificationHandler{client: clientObj}

	supportsQRShow := false
	supportsQRScan := false
	supportsSAS := true
	vh := newVerificationHelper(cli, mach, verificationhelper.NewInMemoryVerificationStore(), handler, supportsQRShow, supportsQRScan, supportsSAS)
	if err := verificationHelperInit(ctx, vh); err != nil {
		return nil, fmt.Errorf("failed to init verification helper: %w", err)
	}
	clientObj.VH = vh

	return clientObj, nil
}
