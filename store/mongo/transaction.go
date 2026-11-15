// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// InTransaction reports whether ctx carries a session with a running transaction (started and neither committed nor aborted), as the
// context a [Mongo.WithTransaction] handler receives does. A session without a transaction (causal consistency only) is not one.
//
// Use it where a transaction changes what an operation may do: a write error aborts the whole transaction, so a duplicate-key retry
// must not run inside one (only a retry of the transaction can recover), and a cache must not serve or store a transaction's reads.
func InTransaction(ctx context.Context) bool {
	sess := mongo.SessionFromContext(ctx)
	return sess != nil && sess.TransactionRunning()
}

// Transaction reports the running transaction ctx carries: tx identifies it (equal for every context of the transaction, a joined
// one included, and different for another client's transaction started inside it or for a later transaction of the same session,
// such as one started after an explicit abort) and ok is false when ctx carries none, as for [InTransaction]. It satisfies
// messaging/eventbus/uow.Detector.
func Transaction(ctx context.Context) (tx any, ok bool) {
	sess := mongo.SessionFromContext(ctx)
	if sess == nil || !sess.TransactionRunning() {
		return nil, false
	}
	return transactionID{session: sess, txnNumber: txnNumber(sess)}, true
}

// transactionID identifies one transaction: the driver numbers the transactions of a session, and the number stays fixed while
// one runs (a write inside a transaction is not retried with a new number).
type transactionID struct {
	session   *mongo.Session
	txnNumber int64
}

// txnNumber returns the driver's number of the current transaction of sess. The driver does not expose it, so it is read by
// reflection from the unexported mongo.Session.clientSession (a *session.Client embedding *session.Server and its TxnNumber); a test
// pins this layout for the driver version in go.mod. It returns -1 when the layout differs, which degrades the identity to the
// session alone.
func txnNumber(sess *mongo.Session) int64 {
	v := reflect.ValueOf(sess).Elem()
	for _, name := range [...]string{"clientSession", "Server"} {
		if v = v.FieldByName(name); v.Kind() != reflect.Pointer || v.IsNil() {
			return -1
		}
		v = v.Elem()
	}
	if v = v.FieldByName("TxnNumber"); v.Kind() != reflect.Int64 {
		return -1
	}
	return v.Int()
}

// joinableTransaction reports whether ctx carries a running transaction of client, which [Mongo.WithTransaction] joins.
func joinableTransaction(ctx context.Context, client *mongo.Client) bool {
	sess := mongo.SessionFromContext(ctx)
	return client != nil && sess != nil && sess.TransactionRunning() && sess.Client() == client
}
