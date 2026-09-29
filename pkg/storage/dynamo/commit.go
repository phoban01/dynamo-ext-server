package dynamo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	mrand "math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"
)

type action int

const (
	actCreate action = iota
	actUpdate
	actDelete
)

// write describes one write to one object.
type write struct {
	sk     string
	action action
	// expectRV is the version that the server read, for an update or a
	// delete.
	expectRV uint64
	// value is the new object for a create or an update, and the deleted
	// object for a delete. Objects are stored with no resource version.
	value []byte
	// prev is the previous object for an update or a delete.
	prev []byte
}

// errObjectConflict means that the object condition of a write failed.
var errObjectConflict = errors.New("dynamo: object condition failed")

// maxCounterRetries bounds the retries after a counter conflict.
const maxCounterRetries = 200

// commit applies one write in one transaction and returns its resource
// version.
func (s *store) commit(ctx context.Context, w write) (uint64, error) {
	for attempt := 0; ; attempt++ {
		n, err := s.readCounter(ctx)
		if err != nil {
			return 0, err
		}
		next := n + 1
		_, err = s.client.TransactWriteItems(ctx, s.transaction(w, n, next))
		if err == nil {
			return next, nil
		}
		switch conflict, retry := classify(err); {
		case conflict:
			return 0, errObjectConflict
		case retry && attempt < maxCounterRetries:
			//= spec/solas.md#2-3-writes
			//# If the transaction fails only on the counter condition, the server MUST
			//# read the counter again and retry the same write.

			//= spec/solas.md#2-3-writes
			//# The server SHOULD wait a short random time before each such retry.
			if err := sleepJitter(ctx, attempt); err != nil {
				return 0, err
			}
			continue
		}
		if isItemTooLarge(err) {
			//= spec/solas.md#2-3-writes
			//# The server MUST return `413 RequestEntityTooLarge` for such a write.
			return 0, apierrors.NewRequestEntityTooLargeError(err.Error())
		}
		return 0, err
	}
}

// transaction builds the three actions of a write, spec 2.3.
func (s *store) transaction(w write, n, next uint64) *dynamodb.TransactWriteItemsInput {
	//= spec/solas.md#2-3-writes
	//# Every write MUST be one `TransactWriteItems` call.

	//= spec/solas.md#2-3-writes
	//# The call MUST hold three actions: a counter update, an object action, and
	//# an event put.
	return &dynamodb.TransactWriteItemsInput{
		//= spec/solas.md#2-3-writes
		//# Each call MUST set a `ClientRequestToken`.
		ClientRequestToken: aws.String(newToken()),
		TransactItems: []types.TransactWriteItem{
			s.counterAction(n, next),
			s.objectAction(w, next),
			s.eventAction(w, next),
		},
	}
}

func (s *store) counterAction(n, next uint64) types.TransactWriteItem {
	pk, sk := s.counterKey()
	key := map[string]types.AttributeValue{attrPK: str(pk), attrSK: str(sk)}
	//= spec/solas.md#2-3-writes
	//# The counter update MUST set `n` to `n + 1` on condition that `n` still
	//# has the value that the server read.
	return types.TransactWriteItem{Update: &types.Update{
		TableName:                 aws.String(s.table),
		Key:                       key,
		UpdateExpression:          aws.String("SET #n = :next"),
		ConditionExpression:       aws.String("#n = :cur"),
		ExpressionAttributeNames:  map[string]string{"#n": attrN},
		ExpressionAttributeValues: map[string]types.AttributeValue{":next": num(next), ":cur": num(n)},
	}}
}

func (s *store) objectAction(w write, next uint64) types.TransactWriteItem {
	key := map[string]types.AttributeValue{attrPK: str(s.objectPK()), attrSK: str(w.sk)}
	item := map[string]types.AttributeValue{
		attrPK: str(s.objectPK()),
		attrSK: str(w.sk),
		//= spec/solas.md#2-2-items
		//# An object item MUST hold the resource version of its last write in the
		//# number attribute `rv`.

		//= spec/solas.md#3-3-objects
		//# The `rv` attribute of the object item MUST equal the same version.
		attrRV: num(next),
		//= spec/solas.md#2-2-items
		//# An object item MUST hold the encoded object in the binary attribute
		//# `value`.
		attrValue: &types.AttributeValueMemberB{Value: w.value},
	}
	rvMatches := map[string]types.AttributeValue{":rv": num(w.expectRV)}
	names := map[string]string{"#rv": attrRV}
	switch w.action {
	case actCreate:
		//= spec/solas.md#2-3-writes
		//# For a create, the object action MUST be a put on condition that the
		//# object item does not exist.
		return types.TransactWriteItem{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                item,
			ConditionExpression: aws.String("attribute_not_exists(pk)"),
		}}
	case actUpdate:
		//= spec/solas.md#2-3-writes
		//# For an update, the object action MUST be a put on condition that `rv`
		//# equals the version that the server read.
		return types.TransactWriteItem{Put: &types.Put{
			TableName:                 aws.String(s.table),
			Item:                      item,
			ConditionExpression:       aws.String("#rv = :rv"),
			ExpressionAttributeNames:  names,
			ExpressionAttributeValues: rvMatches,
		}}
	default:
		//= spec/solas.md#2-3-writes
		//# For a delete, the object action MUST be a delete on condition that `rv`
		//# equals the version that the server read.
		return types.TransactWriteItem{Delete: &types.Delete{
			TableName:                 aws.String(s.table),
			Key:                       key,
			ConditionExpression:       aws.String("#rv = :rv"),
			ExpressionAttributeNames:  names,
			ExpressionAttributeValues: rvMatches,
		}}
	}
}

func (s *store) eventAction(w write, next uint64) types.TransactWriteItem {
	eventType := map[action]watch.EventType{
		actCreate: watch.Added,
		actUpdate: watch.Modified,
		actDelete: watch.Deleted,
	}[w.action]
	//= spec/solas.md#4-1-event-log
	//# Each write MUST add one event item to the event log of its resource.

	//= spec/solas.md#4-1-event-log
	//# The event item MUST have the same resource version as the write.
	item := map[string]types.AttributeValue{
		attrPK: str(s.eventPK()),
		attrSK: str(eventSK(next)),
		//= spec/solas.md#2-2-items
		//# An event item MUST hold the event type in the attribute `type`.
		attrType: str(string(eventType)),
		attrKey:  str(w.sk),
		//= spec/solas.md#2-2-items
		//# An `ADDED`, `MODIFIED`, or `DELETED` event item MUST hold the encoded
		//# object in `value`.
		attrValue: &types.AttributeValueMemberB{Value: w.value},
		//= spec/solas.md#2-2-items
		//# An event item MUST hold its expiry time, in Unix seconds, in `expires`.

		//= spec/solas.md#4-1-event-log
		//# An event item MUST expire a set retention time after its write.
		attrExpires: num(uint64(s.now().Add(s.retention).Unix())),
	}
	if w.action != actCreate {
		//= spec/solas.md#2-2-items
		//# An event item for a `MODIFIED` or `DELETED` event MUST hold the encoded
		//# previous object in `prev`.
		item[attrPrev] = &types.AttributeValueMemberB{Value: w.prev}
	}
	//= spec/solas.md#2-3-writes
	//# The event put MUST be on condition that the event item does not exist.
	return types.TransactWriteItem{Put: &types.Put{
		TableName:           aws.String(s.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	}}
}

// readCounter returns the last issued resource version. It creates the
// counter when it does not exist.
func (s *store) readCounter(ctx context.Context) (uint64, error) {
	for {
		pk, sk := s.counterKey()
		out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(s.table),
			Key:       map[string]types.AttributeValue{attrPK: str(pk), attrSK: str(sk)},
			//= spec/solas.md#2-4-reads
			//# Every read of a counter item MUST be a strongly consistent read.
			ConsistentRead: aws.Bool(true),
		})
		if err != nil {
			return 0, fmt.Errorf("read counter: %w", err)
		}
		if out.Item != nil {
			return numAttr(out.Item, attrN)
		}
		if err := s.initCounter(ctx); err != nil {
			return 0, err
		}
	}
}

// initCounter creates the counter at 1 with an INIT event at version 1.
// Kubernetes rejects 0 as the resource version of a list, so the counter
// never shows 0. The INIT event keeps the event log free of gaps.
func (s *store) initCounter(ctx context.Context) error {
	pk, sk := s.counterKey()
	//= spec/solas.md#3-1-issue
	//# When the server first uses a resource, it MUST create the counter item
	//# with `n` equal to 1 and an event item of type `INIT` at version 1, in one
	//# transaction.
	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		ClientRequestToken: aws.String(newToken()),
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{
				TableName:           aws.String(s.table),
				Item:                map[string]types.AttributeValue{attrPK: str(pk), attrSK: str(sk), attrN: num(1)},
				ConditionExpression: aws.String("attribute_not_exists(pk)"),
			}},
			{Put: &types.Put{
				TableName: aws.String(s.table),
				Item: map[string]types.AttributeValue{
					attrPK:      str(s.eventPK()),
					attrSK:      str(eventSK(1)),
					attrType:    str(eventInit),
					attrExpires: num(uint64(s.now().Add(s.retention).Unix())),
				},
				ConditionExpression: aws.String("attribute_not_exists(pk)"),
			}},
		},
	})
	var tce *types.TransactionCanceledException
	if err != nil && !errors.As(err, &tce) {
		return fmt.Errorf("create counter: %w", err)
	}
	// Success, or another server created the counter first.
	return nil
}

// classify reads the cancellation reasons of a failed transaction. The
// reasons are in the order of the actions: counter, object, event.
func classify(err error) (conflict, retry bool) {
	var tce *types.TransactionCanceledException
	if !errors.As(err, &tce) {
		var tcx *types.TransactionConflictException
		return false, errors.As(err, &tcx)
	}
	code := func(i int) string {
		if i < len(tce.CancellationReasons) {
			return aws.ToString(tce.CancellationReasons[i].Code)
		}
		return ""
	}
	if code(1) == "ConditionalCheckFailed" {
		return true, false
	}
	for i := range tce.CancellationReasons {
		switch code(i) {
		case "ConditionalCheckFailed", "TransactionConflict":
			return false, true
		}
	}
	return false, false
}

func isItemTooLarge(err error) bool {
	return strings.Contains(err.Error(), "Item size has exceeded") ||
		strings.Contains(err.Error(), "Item size to update has exceeded")
}

// sleepJitter waits a random time that grows with the attempt, up to 50 ms.
func sleepJitter(ctx context.Context, attempt int) error {
	ceiling := time.Millisecond << min(attempt, 6)
	d := time.Duration(mrand.Int64N(int64(ceiling)) + 1)
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on supported platforms.
		n, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		return n.String()
	}
	return hex.EncodeToString(b)
}

func str(v string) *types.AttributeValueMemberS { return &types.AttributeValueMemberS{Value: v} }

func num(v uint64) *types.AttributeValueMemberN {
	return &types.AttributeValueMemberN{Value: strconv.FormatUint(v, 10)}
}

func numAttr(item map[string]types.AttributeValue, name string) (uint64, error) {
	n, ok := item[name].(*types.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("attribute %s is not a number", name)
	}
	return strconv.ParseUint(n.Value, 10, 64)
}

func strAttr(item map[string]types.AttributeValue, name string) string {
	if s, ok := item[name].(*types.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}

func binAttr(item map[string]types.AttributeValue, name string) []byte {
	if b, ok := item[name].(*types.AttributeValueMemberB); ok {
		return b.Value
	}
	return nil
}
