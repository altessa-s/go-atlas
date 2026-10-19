// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/domain/behavior"

	mongo "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
)

type nestedProfile struct {
	UID     string    `bson:"uid" behavior:"identifier"`
	Name    string    `bson:"name"`
	Nick    *string   `bson:"nick"`
	Tags    []string  `bson:"tags"`
	Created time.Time `bson:"created" behavior:"immutable"`
}

type nestedOwner struct {
	Profile *nestedProfile `bson:"profile"`
}

type nestedWrap struct {
	Note  string        `bson:"note"`
	Inner nestedProfile `bson:"inner"`
}

type nestedRoot struct {
	ID   string      `bson:"_id" behavior:"identifier"`
	Wrap *nestedWrap `bson:"wrap"`
}

func updateKinds() []behavior.Option {
	return []behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)}
}

// TestUpdateTranslatorNestedProtectedUsesPaths pins that a nested object
// holding a protected field — an identifier, or a field the engine strips — is
// written as descendant dot paths rather than replaced whole: the update
// document then never touches the stored protected field. Nil pointers and
// empty collections below it go to $unset by path.
func TestUpdateTranslatorNestedProtectedUsesPaths(t *testing.T) {
	t.Parallel()

	got := update(t, &nestedOwner{Profile: &nestedProfile{UID: "p1", Name: "new", Created: time.Now()}}, updateKinds())
	require.Equal(t, bson.M{
		"$set":   bson.M{"profile.name": "new"},
		"$unset": bson.M{"profile.nick": nil, "profile.tags": nil},
	}, got, "the stored profile.uid and profile.created survive the update")

	// Several levels deep: every level holding a protected descendant is
	// addressed by path.
	got = update(t, &nestedRoot{ID: "r1", Wrap: &nestedWrap{Note: "n", Inner: nestedProfile{Name: "x", Tags: []string{"a"}, Nick: new("k")}}},
		updateKinds())
	require.Equal(t, bson.M{"$set": bson.M{
		"wrap.note":       "n",
		"wrap.inner.name": "x",
		"wrap.inner.nick": "k",
		"wrap.inner.tags": bson.A{"a"},
	}}, got)
}

// TestUpdateTranslatorNestedIdentifierOnly pins that a nested object whose
// only field is its identifier contributes nothing, instead of an empty
// replacement document that would delete the stored identifier.
func TestUpdateTranslatorNestedIdentifierOnly(t *testing.T) {
	t.Parallel()

	type ref struct {
		UID string `bson:"uid" behavior:"identifier"`
	}
	type doc struct {
		Name string `bson:"name"`
		Ref  *ref   `bson:"ref"`
	}
	got := update(t, &doc{Name: "n", Ref: &ref{UID: "r1"}}, updateKinds())
	require.Equal(t, bson.M{"$set": bson.M{"name": "n"}}, got)
}

// TestUpdateTranslatorNestedOmitOnUpdateUsesPaths pins that an omitonupdate
// field inside a nested object survives the update too: the object is written
// by path, since a replacement would delete the stored value.
func TestUpdateTranslatorNestedOmitOnUpdateUsesPaths(t *testing.T) {
	t.Parallel()

	type meta struct {
		Created string `bson:"created,omitonupdate"`
		Note    string `bson:"note"`
	}
	type doc struct {
		Meta meta `bson:"meta"`
	}
	got := update(t, &doc{Meta: meta{Created: "c", Note: "n"}}, updateKinds())
	require.Equal(t, bson.M{"$set": bson.M{"meta.note": "n"}}, got)
}

// TestUpdateTranslatorNestedUnprotectedReplaced pins that a nested object
// without protected fields keeps the full-replacement semantics.
func TestUpdateTranslatorNestedUnprotectedReplaced(t *testing.T) {
	t.Parallel()

	type addr struct {
		City string `bson:"city"`
	}
	type doc struct {
		Home *addr `bson:"home"`
	}
	got := update(t, &doc{Home: &addr{City: "NYC"}}, updateKinds())
	require.Equal(t, bson.M{"$set": bson.M{"home": bson.M{"city": "NYC"}}}, got)
}

// TestUpdateTranslatorNestedPathNameChecked pins that a stored name reached by
// a descendant path is checked like a top-level one: a dot in it would address
// another field.
func TestUpdateTranslatorNestedPathNameChecked(t *testing.T) {
	t.Parallel()

	type inner struct {
		UID string `bson:"uid" behavior:"identifier"`
		Odd string `bson:"a.b"`
	}
	type doc struct {
		In inner `bson:"in"`
	}
	eng := behavior.New[bson.M](mongo.NewUpdateTranslator(), updateKinds()...)
	_, err := eng.Translate(t.Context(), &doc{In: inner{Odd: "x"}})
	require.ErrorIs(t, err, mongo.ErrUnaddressableName)
}

// TestUpdateTranslatorProtectedInCollection pins that a replaced collection
// whose elements carry a field the engine strips is rejected: the replacement
// cannot preserve the stored values of that field, and no document path can
// address it per element. An identifier inside an element is the element's own
// and is written with it, even when the engine strips identifiers.
func TestUpdateTranslatorProtectedInCollection(t *testing.T) {
	t.Parallel()

	type item struct {
		ID      string `bson:"id" behavior:"identifier"`
		Name    string `bson:"name"`
		Created string `bson:"created" behavior:"output_only"`
	}
	type holder struct {
		Item item `bson:"item"`
	}
	// One collection per document: each case fails on its own.
	type (
		itemsDoc struct {
			Items []item `bson:"items"`
		}
		byCodeDoc struct {
			ByCode map[string]item `bson:"by_code"`
		}
		holdersDoc struct {
			Holders []holder `bson:"holders"`
		}
	)
	eng := behavior.New[bson.M](mongo.NewUpdateTranslator(), updateKinds()...)
	for name, v := range map[string]any{
		"slice element":        &itemsDoc{Items: []item{{ID: "i1", Name: "a"}}},
		"map value":            &byCodeDoc{ByCode: map[string]item{"c": {ID: "i2", Name: "b"}}},
		"nested in an element": &holdersDoc{Holders: []holder{{Item: item{ID: "i3"}}}},
	} {
		_, err := eng.Translate(t.Context(), v)
		require.ErrorIs(t, err, mongo.ErrProtectedInCollection, name)
		require.False(t, errors.Is(err, mongo.ErrUnaddressableName), name)
	}

	// Only identifiers in the elements: written, whatever the engine strips.
	type tagged struct {
		ID   string `bson:"id" behavior:"identifier"`
		Name string `bson:"name"`
	}
	type list struct {
		Items []tagged `bson:"items"`
	}
	got := update(t, &list{Items: []tagged{{ID: "i1", Name: "a"}}},
		[]behavior.Option{behavior.WithKinds(behavior.OutputOnly, behavior.Identifier, behavior.Immutable)})
	require.Equal(t, bson.M{"$set": bson.M{"items": bson.A{bson.M{"id": "i1", "name": "a"}}}}, got)
}

// inlineIdentity is inlined under an identifier-tagged field; its own fields
// carry no behavior tag.
type inlineIdentity struct {
	UID string `bson:"uid"`
}

// TestUpdateTranslatorInlineIdentifier pins that an identifier tag on an
// inline struct or an inline map covers the keys it contributes to the parent:
// none of them is written, whatever kinds the engine strips.
func TestUpdateTranslatorInlineIdentifier(t *testing.T) {
	t.Parallel()

	type structDoc struct {
		Identity inlineIdentity `bson:",inline" behavior:"identifier"`
		Name     string         `bson:"name"`
	}
	type ptrDoc struct {
		Identity *inlineIdentity `bson:",inline" behavior:"identifier"`
		Name     string          `bson:"name"`
	}
	type mapDoc struct {
		Keys map[string]string `bson:",inline" behavior:"identifier"`
		Name string            `bson:"name"`
	}
	kinds := map[string][]behavior.Kind{
		"default update kinds": behavior.DefaultUpdateKinds,
		"no kinds":             nil,
		"identifier stripped":  {behavior.Identifier},
	}
	for name, k := range kinds {
		opts := []behavior.Option{behavior.WithKinds(k...)}
		want := bson.M{"$set": bson.M{"name": "n"}}
		require.Equal(t, want, update(t, &structDoc{Identity: inlineIdentity{UID: "u1"}, Name: "n"}, opts), "inline struct, %s", name)
		require.Equal(t, want, update(t, &ptrDoc{Identity: &inlineIdentity{UID: "u1"}, Name: "n"}, opts), "inline pointer, %s", name)
		require.Equal(t, want, update(t, &mapDoc{Keys: map[string]string{"tenant": "t1"}, Name: "n"}, opts), "inline map, %s", name)
	}

	// Inside a collection element the identifier belongs to the element and
	// is written with it, inline or not — unless the engine stripped it: a
	// stripped inline identifier left its children unresolved, so the
	// replacement is rejected rather than written unchecked.
	type list struct {
		Items []structDoc `bson:"items"`
	}
	for name, k := range kinds {
		opts := []behavior.Option{behavior.WithKinds(k...)}
		v := &list{Items: []structDoc{{Identity: inlineIdentity{UID: "u1"}, Name: "n"}}}
		if slices.Contains(k, behavior.Identifier) {
			_, err := behavior.New[bson.M](mongo.NewUpdateTranslator(), opts...).Translate(t.Context(), v)
			require.ErrorIs(t, err, mongo.ErrProtectedInCollection, "element, %s", name)
			continue
		}
		require.Equal(t, bson.M{"$set": bson.M{"items": bson.A{bson.M{"uid": "u1", "name": "n"}}}}, update(t, v, opts), "element, %s", name)
	}
}

// compositeKey is a composite _id holding a protected field, an ordinary one
// and a nil pointer.
type compositeKey struct {
	Tenant string  `bson:"tenant" behavior:"immutable"`
	Name   string  `bson:"name"`
	Nick   *string `bson:"nick"`
}

// TestUpdateTranslatorCompositeIDUntouched pins that no operator path rooted at
// the root _id is written, even when the _id is a composite subdocument
// holding a protected field (which would otherwise be written by descendant
// paths): MongoDB rejects any change to the _id subtree.
func TestUpdateTranslatorCompositeIDUntouched(t *testing.T) {
	t.Parallel()

	type direct struct {
		ID   compositeKey `bson:"_id"`
		Note string       `bson:"note"`
	}
	type viaInline struct {
		Keys struct {
			ID *compositeKey `bson:"_id"`
		} `bson:",inline"`
		Note string `bson:"note"`
	}
	want := bson.M{"$set": bson.M{"note": "n"}}
	require.Equal(t, want, update(t, &direct{ID: compositeKey{Tenant: "t", Name: "x"}, Note: "n"}, updateKinds()))
	require.Equal(t, want, update(t, &viaInline{Keys: struct {
		ID *compositeKey `bson:"_id"`
	}{ID: &compositeKey{Tenant: "t", Name: "x"}}, Note: "n"}, updateKinds()))
}

// idItem is a collection value with its own _id.
type idItem struct {
	ID   string `bson:"_id"`
	Name string `bson:"name"`
}

// taggedIDItem is a collection value whose _id is tagged identifier.
type taggedIDItem struct {
	ID   string `bson:"_id" behavior:"identifier"`
	Name string `bson:"name"`
}

// TestUpdateTranslatorCollectionValueIDKept pins that only the root
// document's _id is skipped: an _id inside a collection value — the values of
// a root inline map, or of an ordinary map or slice — belongs to that value
// and stays in its replacement, tagged identifier or not.
func TestUpdateTranslatorCollectionValueIDKept(t *testing.T) {
	t.Parallel()

	type inlineValues struct {
		Extras map[string]idItem `bson:",inline"`
	}
	type inlinePtrs struct {
		Extras map[string]*idItem `bson:",inline"`
	}
	type inlineTagged struct {
		Extras map[string]taggedIDItem `bson:",inline"`
	}
	type plain struct {
		ID    string            `bson:"_id"`
		ByKey map[string]idItem `bson:"by_key"`
		Items []idItem          `bson:"items"`
	}
	item := bson.M{"_id": "p1", "name": "new"}
	for name, kinds := range map[string][]behavior.Option{"default update kinds": updateKinds(), "no kinds": nil} {
		require.Equal(t, bson.M{"$set": bson.M{"profile": item}},
			update(t, &inlineValues{Extras: map[string]idItem{"profile": {ID: "p1", Name: "new"}}}, kinds), "inline values, %s", name)
		require.Equal(t, bson.M{"$set": bson.M{"profile": item}},
			update(t, &inlinePtrs{Extras: map[string]*idItem{"profile": {ID: "p1", Name: "new"}}}, kinds), "inline pointers, %s", name)
		require.Equal(t, bson.M{"$set": bson.M{"profile": item}},
			update(t, &inlineTagged{Extras: map[string]taggedIDItem{"profile": {ID: "p1", Name: "new"}}}, kinds), "inline tagged, %s", name)
		require.Equal(t, bson.M{"$set": bson.M{"by_key": bson.M{"k": item}, "items": bson.A{item}}},
			update(t, &plain{ID: "r1", ByKey: map[string]idItem{"k": {ID: "p1", Name: "new"}}, Items: []idItem{{ID: "p1", Name: "new"}}}, kinds),
			"plain collections, %s", name)
	}

	// Keys inside a collection value are stored literally, not read as
	// operator paths: a dotted name there is not an addressability error.
	type dotted struct {
		Odd string `bson:"a.b"`
	}
	type inlineDotted struct {
		Extras map[string]dotted `bson:",inline"`
	}
	require.Equal(t, bson.M{"$set": bson.M{"k": bson.M{"a.b": "x"}}},
		update(t, &inlineDotted{Extras: map[string]dotted{"k": {Odd: "x"}}}, updateKinds()))
}

// layoutMeta is inlined through a pointer; its identifier lives in the
// enclosing subdocument.
type layoutMeta struct {
	UID string `bson:"uid" behavior:"identifier"`
}

// AuditStamp is embedded anonymously with inline,omitonupdate; its fields
// carry no tag of their own.
type AuditStamp struct {
	Created string `bson:"created"`
}

// TestUpdateTranslatorNestedProtectedFollowsLayout pins that the decision to
// write a nested struct by path follows the BSON document layout, not the
// resolved Go fields: keys an unset inline pointer would hold, omitonupdate
// propagated from an embedded struct, and fields BSON ignores.
func TestUpdateTranslatorNestedProtectedFollowsLayout(t *testing.T) {
	t.Parallel()

	// (a) A nil inline pointer: its keys (profile.uid) live in the
	// subdocument; a replacement would delete them.
	type profile struct {
		Name string      `bson:"name"`
		Meta *layoutMeta `bson:",inline"`
	}
	type owner struct {
		Profile *profile `bson:"profile"`
	}
	require.Equal(t, bson.M{"$set": bson.M{"profile.name": "Alice"}},
		update(t, &owner{Profile: &profile{Name: "Alice"}}, updateKinds()))

	// (b) omitonupdate on an anonymous embedded inline struct reaches its
	// promoted fields through the layout.
	type stamped struct {
		AuditStamp `bson:",inline,omitonupdate"`
		Note       string `bson:"note"`
	}
	type holder struct {
		Doc stamped `bson:"doc"`
	}
	require.Equal(t, bson.M{"$set": bson.M{"doc.note": "n"}},
		update(t, &holder{Doc: stamped{AuditStamp: AuditStamp{Created: "c"}, Note: "n"}}, updateKinds()))

	// (c) A field BSON ignores protects nothing: the subdocument is replaced.
	type sub struct {
		Hidden string `bson:"-" behavior:"identifier"`
		Name   string `bson:"name"`
	}
	type plain struct {
		Sub sub `bson:"sub"`
	}
	require.Equal(t, bson.M{"$set": bson.M{"sub": bson.M{"name": "x"}}},
		update(t, &plain{Sub: sub{Hidden: "h", Name: "x"}}, updateKinds()))
}

// elementKey is a compound identifier with a server-managed child.
type elementKey struct {
	Tenant string `bson:"tenant" behavior:"output_only"`
	Code   string `bson:"code"`
}

// TestUpdateTranslatorElementIdentifierExemptionNarrow pins that the
// collection-element identifier exemption covers only a field the engine
// stripped for being an identifier and nothing else, with a value that has no
// children to inspect. A field also stripped for another kind, or a stripped
// compound identifier (nested struct, inline struct, inline map) whose
// children the engine did not resolve, fails with ErrProtectedInCollection.
func TestUpdateTranslatorElementIdentifierExemptionNarrow(t *testing.T) {
	t.Parallel()

	type outID struct {
		ID   string `bson:"id" behavior:"output_only,identifier"`
		Name string `bson:"name"`
	}
	type immID struct {
		ID   string `bson:"id" behavior:"immutable,identifier"`
		Name string `bson:"name"`
	}
	type nestedKey struct {
		Key  elementKey `bson:"key" behavior:"identifier"`
		Name string     `bson:"name"`
	}
	type inlineKey struct {
		Key  elementKey `bson:",inline" behavior:"identifier"`
		Name string     `bson:"name"`
	}
	type inlineMapKey struct {
		Keys map[string]string `bson:",inline" behavior:"identifier"`
		Name string            `bson:"name"`
	}
	// One collection per document: a nil collection whose elements hold a
	// protected field is itself rejected (see TestUpdateTranslatorProtectedRemoval).
	type (
		outIDDoc struct {
			Items []outID `bson:"items"`
		}
		immIDDoc struct {
			Items []immID `bson:"items"`
		}
		nestedKeyDoc struct {
			Items []nestedKey `bson:"items"`
		}
		inlineKeyDoc struct {
			Items []inlineKey `bson:"items"`
		}
		inlineMapDoc struct {
			Items []inlineMapKey `bson:"items"`
		}
	)
	key := elementKey{Tenant: "t", Code: "c"}
	strippingID := []behavior.Option{behavior.WithKinds(behavior.OutputOnly, behavior.Identifier, behavior.Immutable)}
	tests := []struct {
		name string
		v    any
		// failsByDefault: the case fails under DefaultUpdateKinds too, where
		// the identifier itself is not stripped.
		failsByDefault bool
	}{
		{name: "output_only,identifier", v: &outIDDoc{Items: []outID{{ID: "i", Name: "n"}}}, failsByDefault: true},
		{name: "immutable,identifier", v: &immIDDoc{Items: []immID{{ID: "i", Name: "n"}}}, failsByDefault: true},
		// Not stripped by default: the engine resolves the key and its
		// output_only tenant fails like any stripped element field.
		{name: "nested identifier", v: &nestedKeyDoc{Items: []nestedKey{{Key: key, Name: "n"}}}, failsByDefault: true},
		{name: "inline identifier", v: &inlineKeyDoc{Items: []inlineKey{{Key: key, Name: "n"}}}, failsByDefault: true},
		{name: "inline map identifier", v: &inlineMapDoc{Items: []inlineMapKey{{Keys: map[string]string{"k": "v"}, Name: "n"}}}},
	}
	for _, tt := range tests {
		_, err := behavior.New[bson.M](mongo.NewUpdateTranslator(), strippingID...).Translate(t.Context(), tt.v)
		require.ErrorIs(t, err, mongo.ErrProtectedInCollection, "%s, identifier stripped", tt.name)

		_, err = behavior.New[bson.M](mongo.NewUpdateTranslator(), updateKinds()...).Translate(t.Context(), tt.v)
		if tt.failsByDefault {
			require.ErrorIs(t, err, mongo.ErrProtectedInCollection, "%s, default kinds", tt.name)
		} else {
			require.NoError(t, err, "%s, default kinds", tt.name)
		}
	}

	// Under DefaultUpdateKinds an inline map identifier is not stripped: it
	// belongs to the element and is written with it.
	got := update(t, &inlineMapDoc{Items: []inlineMapKey{{Keys: map[string]string{"k": "v"}, Name: "n"}}}, updateKinds())
	require.Equal(t, bson.A{bson.M{"k": "v", "name": "n"}}, got["$set"].(bson.M)["items"])
}

// removalItem is a collection element with a server-managed field.
type removalItem struct {
	ID      string `bson:"id" behavior:"identifier"`
	Name    string `bson:"name"`
	Created string `bson:"created" behavior:"output_only"`
}

// removalNode is a recursive type without protected fields.
type removalNode struct {
	Name string       `bson:"name"`
	Next *removalNode `bson:"next"`
}

// TestUpdateTranslatorProtectedRemoval pins that a value removing stored data
// with no resolved fields to check — a nil parent, a nil or empty collection, a
// nil element, a nil value nested in a written one — is judged from its
// declared type: when that holds a protected field the update fails instead of
// deleting the stored, server-managed value. Removing a value whose type holds
// nothing protected still unsets it.
func TestUpdateTranslatorProtectedRemoval(t *testing.T) {
	t.Parallel()

	type stamped struct {
		Created string `bson:"created,omitonupdate"`
		Note    string `bson:"note"`
	}
	type (
		wrapPtr struct {
			Note  string         `bson:"note"`
			Inner *nestedProfile `bson:"inner"`
		}
		nestedNil struct {
			Wrap *wrapPtr `bson:"wrap"`
		}
		stampedNil struct {
			Stamp *stamped `bson:"stamp"`
		}
		sliceDoc struct {
			Items []removalItem `bson:"items"`
		}
		mapDoc struct {
			ByKey map[string]removalItem `bson:"by_key"`
		}
		ptrSliceDoc struct {
			Items *[]removalItem `bson:"items"`
		}
		ptrElemDoc struct {
			Items []*removalItem `bson:"items"`
		}
		ptrValueDoc struct {
			ByKey map[string]*removalItem `bson:"by_key"`
		}
		nestedListDoc struct {
			Items [][]removalItem `bson:"items"`
		}
		holder struct {
			Name string       `bson:"name"`
			Item *removalItem `bson:"item"`
		}
		holderDoc struct {
			Holders []holder `bson:"holders"`
		}
		inlineHolder struct {
			Name string       `bson:"name"`
			Item *removalItem `bson:",inline"`
		}
		inlineHolderDoc struct {
			Holders []inlineHolder `bson:"holders"`
		}
	)
	tests := []struct {
		name string
		v    any
		want error
		// always: protected whatever the engine strips — an identifier or an
		// omitonupdate field outside a collection, which the update never
		// writes.
		always bool
	}{
		{name: "nil parent", v: &nestedOwner{}, want: mongo.ErrProtectedUnset, always: true},
		{name: "nested nil", v: &nestedNil{Wrap: &wrapPtr{Note: "n"}}, want: mongo.ErrProtectedUnset, always: true},
		{name: "nil parent of omitonupdate", v: &stampedNil{}, want: mongo.ErrProtectedUnset, always: true},
		{name: "nil slice", v: &sliceDoc{}, want: mongo.ErrProtectedInCollection},
		{name: "empty slice", v: &sliceDoc{Items: []removalItem{}}, want: mongo.ErrProtectedInCollection},
		{name: "empty map", v: &mapDoc{ByKey: map[string]removalItem{}}, want: mongo.ErrProtectedInCollection},
		{name: "pointer to empty slice", v: &ptrSliceDoc{Items: &[]removalItem{}}, want: mongo.ErrProtectedInCollection},
		{name: "nil slice element", v: &ptrElemDoc{Items: []*removalItem{nil}}, want: mongo.ErrProtectedInCollection},
		{name: "nil map value", v: &ptrValueDoc{ByKey: map[string]*removalItem{"k": nil}}, want: mongo.ErrProtectedInCollection},
		{name: "empty nested collection", v: &nestedListDoc{Items: [][]removalItem{{}}}, want: mongo.ErrProtectedInCollection},
		{name: "nil pointer in an element", v: &holderDoc{Holders: []holder{{Name: "h"}}}, want: mongo.ErrProtectedInCollection},
		{name: "nil inline pointer in an element", v: &inlineHolderDoc{Holders: []inlineHolder{{Name: "h"}}}, want: mongo.ErrProtectedInCollection},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := behavior.New[bson.M](mongo.NewUpdateTranslator(), updateKinds()...).Translate(t.Context(), tt.v)
			require.ErrorIs(t, err, tt.want)
			// Judged by the engine's policy: with nothing stripped only the
			// fields the update never writes are protected.
			_, err = behavior.New[bson.M](mongo.NewUpdateTranslator(), behavior.WithKinds()).Translate(t.Context(), tt.v)
			if tt.always {
				require.ErrorIs(t, err, tt.want, "no kinds")
			} else {
				require.NoError(t, err, "no kinds")
			}
		})
	}

	// Nothing protected below: removed as before.
	type (
		addr struct {
			City string `bson:"city"`
		}
		idOnly struct {
			ID   string `bson:"id" behavior:"identifier"`
			Name string `bson:"name"`
		}
		plainDoc struct {
			Home  *addr        `bson:"home"`
			Tags  []string     `bson:"tags"`
			Refs  []idOnly     `bson:"refs"`
			Ptrs  []*idOnly    `bson:"ptrs"`
			Chain *removalNode `bson:"chain"`
		}
	)
	got := update(t, &plainDoc{Ptrs: []*idOnly{nil}}, updateKinds())
	require.Equal(t, bson.M{
		"$set":   bson.M{"ptrs": bson.A{nil}},
		"$unset": bson.M{"home": nil, "tags": nil, "refs": nil, "chain": nil},
	}, got, "an element identifier belongs to the element and goes with it")
}

// ProtectedStamp is embedded inline,omitonupdate into collection elements; its
// field is server-managed.
type ProtectedStamp struct {
	Created string `bson:"created" behavior:"output_only"`
}

// TestUpdateTranslatorOmittedProtectedInCollection pins that an omitonupdate
// field inside a replaced collection element is checked like any other: the
// replacement drops its stored value, so a protected one — tagged directly,
// propagated from an inline struct, held below it, or in an inline map — fails
// with ErrProtectedInCollection instead of being silently left out. An
// omitonupdate field with nothing protected is still left out.
func TestUpdateTranslatorOmittedProtectedInCollection(t *testing.T) {
	t.Parallel()

	type meta struct {
		Created string `bson:"created" behavior:"output_only"`
	}
	type (
		direct struct {
			Created string `bson:"created,omitonupdate" behavior:"output_only"`
			Name    string `bson:"name"`
		}
		propagated struct {
			ProtectedStamp `bson:",inline,omitonupdate"`
			Name           string `bson:"name"`
		}
		below struct {
			Meta meta   `bson:"meta,omitonupdate"`
			Name string `bson:"name"`
		}
		inlineMap struct {
			Extra map[string]meta `bson:",inline,omitonupdate"`
			Name  string          `bson:"name"`
		}
		emptyInlineMap struct {
			Extra map[string]meta `bson:",inline"`
			Name  string          `bson:"name"`
		}
		directDoc struct {
			Items []direct `bson:"items"`
		}
		propagatedDoc struct {
			Items []propagated `bson:"items"`
		}
		belowDoc struct {
			Items []below `bson:"items"`
		}
		inlineMapDoc struct {
			Items []inlineMap `bson:"items"`
		}
		emptyInlineMapDoc struct {
			Items []emptyInlineMap `bson:"items"`
		}
	)
	tests := []struct {
		name string
		v    any
	}{
		{name: "tagged", v: &directDoc{Items: []direct{{Created: "c", Name: "n"}}}},
		{name: "propagated from an inline struct", v: &propagatedDoc{Items: []propagated{{ProtectedStamp: ProtectedStamp{Created: "c"}, Name: "n"}}}},
		{name: "below an omitted field", v: &belowDoc{Items: []below{{Meta: meta{Created: "c"}, Name: "n"}}}},
		{name: "omitted inline map", v: &inlineMapDoc{Items: []inlineMap{{Extra: map[string]meta{"k": {Created: "c"}}, Name: "n"}}}},
		{name: "empty inline map", v: &emptyInlineMapDoc{Items: []emptyInlineMap{{Name: "n"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := behavior.New[bson.M](mongo.NewUpdateTranslator(), updateKinds()...).Translate(t.Context(), tt.v)
			require.ErrorIs(t, err, mongo.ErrProtectedInCollection)
		})
	}

	type plain struct {
		Note string `bson:"note,omitonupdate"`
		Name string `bson:"name"`
	}
	type plainDoc struct {
		Items []plain `bson:"items"`
	}
	require.Equal(t, bson.M{"$set": bson.M{"items": bson.A{bson.M{"name": "n"}}}},
		update(t, &plainDoc{Items: []plain{{Note: "x", Name: "n"}}}, updateKinds()))
}

// policyItem carries its behaviors under a custom tag key.
type policyItem struct {
	Name   string `bson:"name"`
	Secret string `bson:"secret" beh:"input_only"`
}

// TestUpdateTranslatorRemovalFollowsEnginePolicy pins that values judged from
// their declared type follow the engine's effective policy — its tag key and
// its strip kinds — as resolved values do: a field the engine strips cannot be
// deleted by a nil parent, an empty collection, a nil element or an omitted
// subtree, while one it does not strip is replaced and cleared alike.
func TestUpdateTranslatorRemovalFollowsEnginePolicy(t *testing.T) {
	t.Parallel()

	type (
		owner struct {
			Item *policyItem `bson:"item"`
		}
		list struct {
			Items []policyItem `bson:"items"`
		}
		ptrList struct {
			Items []*policyItem `bson:"items"`
		}
		omitted struct {
			Item policyItem `bson:"item,omitonupdate"`
			Name string     `bson:"name"`
		}
		omittedList struct {
			Items []omitted `bson:"items"`
		}
	)
	strict := []behavior.Option{behavior.WithTagName("beh"), behavior.WithKinds(behavior.InputOnly)}
	tests := []struct {
		name string
		v    any
		want error
	}{
		{name: "nil parent", v: &owner{}, want: mongo.ErrProtectedUnset},
		{name: "nil collection", v: &list{}, want: mongo.ErrProtectedInCollection},
		{name: "empty collection", v: &list{Items: []policyItem{}}, want: mongo.ErrProtectedInCollection},
		{name: "nil element", v: &ptrList{Items: []*policyItem{nil}}, want: mongo.ErrProtectedInCollection},
		{name: "omitted subtree", v: &omittedList{Items: []omitted{{Item: policyItem{Name: "a"}, Name: "n"}}}, want: mongo.ErrProtectedInCollection},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := behavior.New[bson.M](mongo.NewUpdateTranslator(), strict...).Translate(t.Context(), tt.v)
			require.ErrorIs(t, err, tt.want, "custom tag key and kinds")
			// The default tag key does not see the beh tags.
			_, err = behavior.New[bson.M](mongo.NewUpdateTranslator(), behavior.WithKinds(behavior.InputOnly)).Translate(t.Context(), tt.v)
			require.NoError(t, err, "default tag key")
			// Under the default kinds input_only is written.
			_, err = behavior.New[bson.M](mongo.NewUpdateTranslator(), behavior.WithTagName("beh"),
				behavior.WithKinds(behavior.DefaultUpdateKinds...)).Translate(t.Context(), tt.v)
			require.NoError(t, err, "input_only not stripped")
		})
	}

	// One translator value serves engines with different policies.
	tr := mongo.NewUpdateTranslator()
	_, err := behavior.New[bson.M](tr, strict...).Translate(t.Context(), &list{})
	require.ErrorIs(t, err, mongo.ErrProtectedInCollection)
	got, err := behavior.New[bson.M](tr).Translate(t.Context(), &list{})
	require.NoError(t, err)
	require.Equal(t, bson.M{"$set": bson.M{}, "$unset": bson.M{"items": nil}}, got)
}

// TestUpdateTranslatorReplaceAndClearAgree pins that one policy governs both
// replacing a collection with populated elements and clearing it: allowed
// together when the engine strips nothing the elements hold, rejected together
// when it does.
func TestUpdateTranslatorReplaceAndClearAgree(t *testing.T) {
	t.Parallel()

	type list struct {
		Items []removalItem `bson:"items"`
	}
	populated := &list{Items: []removalItem{{ID: "i1", Name: "a", Created: "c"}}}
	for name, opts := range map[string][]behavior.Option{
		"nothing stripped": {behavior.WithKinds()},
		"identifier only":  {behavior.WithKinds(behavior.Identifier)},
	} {
		got := update(t, populated, opts)
		require.Equal(t, bson.M{"$set": bson.M{"items": bson.A{bson.M{"id": "i1", "name": "a", "created": "c"}}}}, got, "replace, %s", name)
		require.Equal(t, bson.M{"$set": bson.M{}, "$unset": bson.M{"items": nil}}, update(t, &list{}, opts), "clear nil, %s", name)
		require.Equal(t, bson.M{"$set": bson.M{}, "$unset": bson.M{"items": nil}}, update(t, &list{Items: []removalItem{}}, opts), "clear empty, %s", name)
	}

	eng := behavior.New[bson.M](mongo.NewUpdateTranslator(), updateKinds()...)
	for name, v := range map[string]*list{"replace": populated, "clear nil": {}, "clear empty": {Items: []removalItem{}}} {
		_, err := eng.Translate(t.Context(), v)
		require.ErrorIs(t, err, mongo.ErrProtectedInCollection, "%s, default kinds", name)
	}
}
