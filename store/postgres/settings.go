// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// TransactionSettings overrides selected settings for a transaction. Zero fields inherit the pool settings.
type TransactionSettings struct {
	// ForceCustomPlan requests a custom execution plan.
	ForceCustomPlan bool
	// SynchronousCommit accepts on, off, local, remote_write or remote_apply.
	SynchronousCommit string
	// LockTimeout bounds lock waits.
	LockTimeout time.Duration
	// IdleTransactionTimeout bounds idle time in the transaction.
	IdleTransactionTimeout time.Duration
	// StatementTimeout limits statement execution.
	StatementTimeout time.Duration
}

// BeginAsyncCommit starts an independent transaction on a pool opened by Open.
// The connector sends BEGIN and the transaction-local setting in the same query.
func BeginAsyncCommit(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	if t := attached(ctx); t != nil && t.life.replay {
		return nil, ErrUnsafeReplay
	}
	ctx = context.WithValue(ctx, settingsKey{}, transactionSettingsSQL(ctx)+"; SET LOCAL synchronous_commit = off")
	return selectedPool(ctx, db).BeginTx(ctx, nil)
}

// WithTransactionSettings attaches settings to the next BEGIN. When ctx already holds a
// transaction on db, it applies the settings in one Exec. Joined transactions keep them until they end.
func WithTransactionSettings(ctx context.Context, db *sql.DB, settings TransactionSettings) (context.Context, error) {
	var local []string
	if settings.ForceCustomPlan {
		local = append(local, "SET LOCAL plan_cache_mode = force_custom_plan")
	}
	switch settings.SynchronousCommit {
	case "":
	case "on", "off", "local", "remote_write", "remote_apply":
		local = append(local, "SET LOCAL synchronous_commit = "+settings.SynchronousCommit)
	default:
		return nil, errors.New("invalid synchronous_commit mode")
	}
	for _, setting := range []struct {
		name  string
		value time.Duration
	}{
		{statementTimeoutSetting, settings.StatementTimeout}, {lockTimeoutSetting, settings.LockTimeout}, {idleTimeoutSetting, settings.IdleTransactionTimeout},
	} {
		if setting.value == 0 {
			continue
		}
		ms := setting.value.Milliseconds()
		if ms < 1 || ms > math.MaxInt32 {
			return nil, fmt.Errorf("invalid transaction %s", setting.name)
		}
		local = append(local, "SET LOCAL "+setting.name+" = '"+strconv.FormatInt(ms, 10)+"ms'")
	}
	if len(local) == 0 {
		return ctx, nil
	}
	query := strings.Join(local, "; ")
	if tx, open := Tx(ctx); tx != nil {
		if !open {
			return nil, sql.ErrTxDone
		}
		if err := CheckDatabase(ctx, db); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return nil, err
		}
		return ctx, nil
	}
	return context.WithValue(ctx, settingsKey{}, transactionSettingsSQL(ctx)+"; "+query), nil
}

type settingsKey struct{}

func transactionSettingsSQL(ctx context.Context) string {
	if settings, ok := ctx.Value(settingsKey{}).(string); ok {
		return settings
	}
	return ""
}
