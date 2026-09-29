package dynamo

import (
	"context"
	"testing"

	storagetesting "k8s.io/apiserver/pkg/storage/testing"
)

// The conformance tests are the tests that the etcd3 store runs. Tests
// that need history, compaction, TTL, or a value transformer are left
// out, because this store does not have them.
func TestConformance(t *testing.T) {
	noValidation := func(context.Context, *testing.T, string) {}
	run := func(name string, f func(context.Context, *testing.T, *store)) {
		t.Run(name, func(t *testing.T) {
			f(context.Background(), t, newTestStore(t))
		})
	}

	run("Create", func(ctx context.Context, t *testing.T, s *store) {
		storagetesting.RunTestCreate(ctx, t, s, noValidation)
	})
	run("CreateWithKeyExist", func(ctx context.Context, t *testing.T, s *store) {
		storagetesting.RunTestCreateWithKeyExist(ctx, t, s)
	})
	run("Get", func(ctx context.Context, t *testing.T, s *store) {
		storagetesting.RunTestGet(ctx, t, s)
	})

	deletes := map[string]func(context.Context, *testing.T, *store){
		"UnconditionalDelete":  func(c context.Context, t *testing.T, s *store) { storagetesting.RunTestUnconditionalDelete(c, t, s) },
		"ConditionalDelete":    func(c context.Context, t *testing.T, s *store) { storagetesting.RunTestConditionalDelete(c, t, s) },
		"DeleteWithSuggestion": func(c context.Context, t *testing.T, s *store) { storagetesting.RunTestDeleteWithSuggestion(c, t, s) },
		"DeleteWithSuggestionAndConflict": func(c context.Context, t *testing.T, s *store) {
			storagetesting.RunTestDeleteWithSuggestionAndConflict(c, t, s)
		},
		"DeleteWithSuggestionOfDeletedObject": func(c context.Context, t *testing.T, s *store) {
			storagetesting.RunTestDeleteWithSuggestionOfDeletedObject(c, t, s)
		},
		"ValidateDeletionWithSuggestion": func(c context.Context, t *testing.T, s *store) {
			storagetesting.RunTestValidateDeletionWithSuggestion(c, t, s)
		},
		"ValidateDeletionWithOnlySuggestionValid": func(c context.Context, t *testing.T, s *store) {
			storagetesting.RunTestValidateDeletionWithOnlySuggestionValid(c, t, s)
		},
		"DeleteWithConflict": func(c context.Context, t *testing.T, s *store) { storagetesting.RunTestDeleteWithConflict(c, t, s) },
		"PreconditionalDeleteWithSuggestion": func(c context.Context, t *testing.T, s *store) {
			storagetesting.RunTestPreconditionalDeleteWithSuggestion(c, t, s)
		},
		"PreconditionalDeleteWithOnlySuggestionPass": func(c context.Context, t *testing.T, s *store) {
			storagetesting.RunTestPreconditionalDeleteWithOnlySuggestionPass(c, t, s)
		},
	}
	for name, f := range deletes {
		run(name, f)
	}

	run("GuaranteedUpdateWithConflict", func(ctx context.Context, t *testing.T, s *store) {
		storagetesting.RunTestGuaranteedUpdateWithConflict(ctx, t, s)
	})
	run("GuaranteedUpdateWithSuggestionAndConflict", func(ctx context.Context, t *testing.T, s *store) {
		storagetesting.RunTestGuaranteedUpdateWithSuggestionAndConflict(ctx, t, s)
	})
	run("GuaranteedUpdate", func(ctx context.Context, t *testing.T, s *store) {
		storagetesting.RunTestGuaranteedUpdate(ctx, t, noTransformer{s}, noValidation)
	})
}

// noTransformer satisfies InterfaceWithPrefixTransformer for tests that
// accept a store with no value transformer.
type noTransformer struct{ *store }

func (noTransformer) UpdatePrefixTransformer(storagetesting.PrefixTransformerModifier) func() {
	return func() {}
}
