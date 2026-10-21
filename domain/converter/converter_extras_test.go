// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/converter"
	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
	"github.com/altessa-s/go-atlas/domain/converter/codec/pbwrap"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

type sealedValue struct {
	Ciphertext []byte
}

type secretProvider struct {
	Headers sealedValue
}

type secretResponse struct {
	Headers *string
}

// TestConvert_DoesNotAllocateIncompatiblePointer pins the fix for a struct
// field converted into a pointer of an incompatible type: the destination must
// stay nil instead of becoming a pointer to the zero value.
func TestConvert_DoesNotAllocateIncompatiblePointer(t *testing.T) {
	t.Parallel()

	var dst secretResponse
	converter.Convert(secretProvider{Headers: sealedValue{Ciphertext: []byte("secret")}}, &dst)

	require.Nil(t, dst.Headers)
}

type pathLeaf struct {
	Password string
	Headers  string
}

type pathSource struct {
	SMTP  pathLeaf
	Other pathLeaf
	Items []pathLeaf
	ByID  map[string]pathLeaf
}

type pathDest struct {
	SMTP  pathLeaf
	Other pathLeaf
	Items []pathLeaf
	ByID  map[string]pathLeaf
}

func pathFixture() pathSource {
	return pathSource{
		SMTP:  pathLeaf{Password: "smtp-password", Headers: "smtp-headers"},
		Other: pathLeaf{Password: "other-password", Headers: "other-headers"},
		Items: []pathLeaf{{Password: "item-password", Headers: "item-headers"}},
		ByID:  map[string]pathLeaf{"k": {Password: "map-password", Headers: "map-headers"}},
	}
}

func TestWithIgnorePaths(t *testing.T) {
	t.Parallel()

	t.Run("exact nested path", func(t *testing.T) {
		t.Parallel()

		var dst pathDest
		converter.Convert(pathFixture(), &dst, converter.WithIgnorePaths("smtp.password"))

		require.Empty(t, dst.SMTP.Password)
		require.Equal(t, "smtp-headers", dst.SMTP.Headers)
		require.Equal(t, "other-password", dst.Other.Password)
		require.Equal(t, "item-password", dst.Items[0].Password)
		require.Equal(t, "map-password", dst.ByID["k"].Password)
	})

	t.Run("parent path ignores subtree", func(t *testing.T) {
		t.Parallel()

		var dst pathDest
		converter.Convert(pathFixture(), &dst, converter.WithIgnorePaths("smtp"))

		require.Empty(t, dst.SMTP.Password)
		require.Empty(t, dst.SMTP.Headers)
		require.Equal(t, "other-password", dst.Other.Password)
	})

	t.Run("slice element path", func(t *testing.T) {
		t.Parallel()

		var dst pathDest
		converter.Convert(pathFixture(), &dst, converter.WithIgnorePaths("items.password"))

		require.Empty(t, dst.Items[0].Password)
		require.Equal(t, "item-headers", dst.Items[0].Headers)
		require.Equal(t, "smtp-password", dst.SMTP.Password)
	})

	t.Run("map value path", func(t *testing.T) {
		t.Parallel()

		var dst pathDest
		converter.Convert(pathFixture(), &dst, converter.WithIgnorePaths("byid.password"))

		require.Empty(t, dst.ByID["k"].Password)
		require.Equal(t, "map-headers", dst.ByID["k"].Headers)
		require.Equal(t, "item-password", dst.Items[0].Password)
	})

	t.Run("nil containers stay nil", func(t *testing.T) {
		t.Parallel()

		var dst pathDest
		converter.Convert(pathSource{}, &dst, converter.WithIgnoreFields("password"))
		require.Nil(t, dst.Items)
		require.Nil(t, dst.ByID)
	})

	t.Run("leaf option keeps assignable nested values whole", func(t *testing.T) {
		t.Parallel()

		var dst pathDest
		converter.Convert(pathFixture(), &dst, converter.WithIgnoreFields("password"))

		// Same-type nested values take the assignable fast path, so the
		// leaf option only reaches fields that are actually walked.
		require.Equal(t, "smtp-password", dst.SMTP.Password)
		require.Equal(t, "other-password", dst.Other.Password)
		require.Equal(t, "item-password", dst.Items[0].Password)
		require.Equal(t, "map-password", dst.ByID["k"].Password)
		require.Equal(t, "smtp-headers", dst.SMTP.Headers)
	})
}

type copySource struct {
	Tags []string
}

type copyDest struct {
	Tags []string
}

type primitiveSliceSource struct {
	Data []byte
}

type primitiveSliceDest struct {
	Data []byte
}

func TestIgnoreFieldsDoesNotForcePrimitiveSliceCopy(t *testing.T) {
	t.Parallel()

	var dst primitiveSliceDest
	converter.Convert(primitiveSliceSource{}, &dst, converter.WithIgnoreFields("missing"))
	require.Nil(t, dst.Data)
}

func TestWithCopySlices(t *testing.T) {
	t.Parallel()

	t.Run("copies backing array", func(t *testing.T) {
		t.Parallel()

		src := copySource{Tags: []string{"a", "b"}}
		var dst copyDest
		converter.Convert(src, &dst, converter.WithCopySlices())

		require.Equal(t, src.Tags, dst.Tags)
		dst.Tags[0] = "changed"
		require.Equal(t, "a", src.Tags[0])
	})

	t.Run("nil stays nil", func(t *testing.T) {
		t.Parallel()

		var dst copyDest
		converter.Convert(copySource{}, &dst, converter.WithCopySlices())
		require.Nil(t, dst.Tags)
	})

	t.Run("empty non-nil stays empty non-nil", func(t *testing.T) {
		t.Parallel()

		src := copySource{Tags: []string{}}
		var dst copyDest
		converter.Convert(src, &dst, converter.WithCopySlices())
		require.NotNil(t, dst.Tags)
		require.Empty(t, dst.Tags)
	})
}

type errorSource struct {
	Name string
}

type errorDest struct {
	Name string
}

func TestConvertE(t *testing.T) {
	t.Parallel()

	t.Run("successful conversion", func(t *testing.T) {
		t.Parallel()

		var dst errorDest
		out, err := converter.ConvertE(errorSource{Name: "ok"}, &dst)
		require.NoError(t, err)
		require.Same(t, &dst, out)
		require.Equal(t, "ok", dst.Name)
	})

	t.Run("invalid type combination returns an error", func(t *testing.T) {
		t.Parallel()

		var dst string
		_, err := converter.ConvertE(errorSource{Name: "x"}, &dst)
		require.Error(t, err)
	})

	t.Run("nil slice element stays nil", func(t *testing.T) {
		t.Parallel()

		src := []*errorSource{nil}
		var dst []*errorDest
		_, err := converter.ConvertE(src, &dst)
		require.NoError(t, err)
		require.Nil(t, dst[0])
	})
}

type rootSliceSource struct {
	Items []pathLeaf
}

type rootSliceDest struct {
	Items []pathLeaf
}

func TestWithIgnorePaths_RootContainers(t *testing.T) {
	t.Parallel()

	t.Run("root slice", func(t *testing.T) {
		t.Parallel()
		var dst []pathLeaf
		converter.Convert([]pathLeaf{{Password: "x", Headers: "h"}}, &dst, converter.WithIgnorePaths("password"))
		require.Empty(t, dst[0].Password)
		require.Equal(t, "h", dst[0].Headers)
	})

	t.Run("ConvertSeq", func(t *testing.T) {
		t.Parallel()
		conv := converter.New[pathLeaf, pathLeaf](converter.WithIgnorePaths("password"))
		var got []pathLeaf
		for out := range conv.ConvertSeq([]pathLeaf{{Password: "x", Headers: "h"}}) {
			got = append(got, out)
		}
		require.Len(t, got, 1)
		require.Empty(t, got[0].Password)
		require.Equal(t, "h", got[0].Headers)
	})

	t.Run("ConvertMapSeq", func(t *testing.T) {
		t.Parallel()
		var got []pathLeaf
		for _, out := range converter.ConvertMapSeq[string, pathLeaf, string, pathLeaf](
			map[string]pathLeaf{"k": {Password: "x", Headers: "h"}},
			converter.WithIgnorePaths("password"),
		) {
			got = append(got, out)
		}
		require.Len(t, got, 1)
		require.Empty(t, got[0].Password)
		require.Equal(t, "h", got[0].Headers)
	})
}

type arraySource struct {
	Pair [2]pathLeaf
}

type arrayDest struct {
	Pair [2]pathLeaf
}

func TestWithIgnorePaths_Array(t *testing.T) {
	t.Parallel()
	src := arraySource{Pair: [2]pathLeaf{{Password: "a", Headers: "ha"}, {Password: "b", Headers: "hb"}}}
	var dst arrayDest
	converter.Convert(src, &dst, converter.WithIgnorePaths("pair.password"))
	require.Empty(t, dst.Pair[0].Password)
	require.Empty(t, dst.Pair[1].Password)
	require.Equal(t, "ha", dst.Pair[0].Headers)
}

type nestedMapSource struct {
	Groups map[string][]pathLeaf
}

type nestedMapDest struct {
	Groups map[string][]pathLeaf
}

func TestWithIgnorePaths_NestedMapOfSlices(t *testing.T) {
	t.Parallel()
	src := nestedMapSource{Groups: map[string][]pathLeaf{"g": {{Password: "x", Headers: "h"}}}}
	var dst nestedMapDest
	converter.Convert(src, &dst, converter.WithIgnorePaths("groups.password"))
	require.Empty(t, dst.Groups["g"][0].Password)
	require.Equal(t, "h", dst.Groups["g"][0].Headers)
}

type pointerSliceSource struct {
	Items []*pathLeaf
	ByID  map[string]*pathLeaf
}

type pointerSliceDest struct {
	Items []*pathLeaf
	ByID  map[string]*pathLeaf
}

func TestWithIgnorePaths_NilPointers(t *testing.T) {
	t.Parallel()
	src := pointerSliceSource{Items: []*pathLeaf{nil}, ByID: map[string]*pathLeaf{"k": nil}}
	var dst pointerSliceDest
	require.NotPanics(t, func() {
		converter.Convert(src, &dst, converter.WithIgnorePaths("password"))
	})
	require.Nil(t, dst.Items[0])
	require.Nil(t, dst.ByID["k"])
}

type profile struct {
	Tags []string
}

type profileSource struct {
	Profile profile
}

type profileDest struct {
	Profile profile
}

func TestWithCopySlices_NestedStruct(t *testing.T) {
	t.Parallel()
	src := profileSource{Profile: profile{Tags: []string{"a"}}}
	var dst profileDest
	converter.Convert(src, &dst, converter.WithCopySlices())
	dst.Profile.Tags[0] = "changed"
	require.Equal(t, "a", src.Profile.Tags[0])
}

type embeddedBase struct {
	Password string
}

type embeddedSource struct {
	Password string
}

type embeddedDest struct {
	embeddedBase
	Password string
}

func TestWithIgnorePaths_EmbeddedDestination(t *testing.T) {
	t.Parallel()
	src := embeddedSource{Password: "secret"}
	var dst embeddedDest
	converter.Convert(src, &dst, converter.WithHandleEmbeddedStructs(true), converter.WithIgnorePaths("password"))
	require.Empty(t, dst.Password)
	require.Empty(t, dst.embeddedBase.Password)
}

type embeddedSourceSide struct {
	embeddedBase
	Name string
}

type embeddedDestSide struct {
	Password string
	Name     string
}

func TestWithIgnorePaths_EmbeddedSourceSubtree(t *testing.T) {
	t.Parallel()
	src := embeddedSourceSide{embeddedBase: embeddedBase{Password: "secret"}, Name: "n"}
	var dst embeddedDestSide
	converter.Convert(src, &dst, converter.WithHandleEmbeddedStructs(true), converter.WithIgnorePaths("embeddedbase"))
	require.Empty(t, dst.Password)
	require.Equal(t, "n", dst.Name)
}

type promotedBase struct {
	Password string
}

type promotedSource struct {
	promotedBase
	Name string
}

type promotedDest struct {
	Password string
	Name     string
}

func TestWithIgnorePaths_PromotedField(t *testing.T) {
	t.Parallel()
	src := promotedSource{promotedBase: promotedBase{Password: "secret"}, Name: "n"}
	var dst promotedDest
	converter.Convert(src, &dst, converter.WithHandleEmbeddedStructs(true), converter.WithIgnorePaths("password"))
	require.Empty(t, dst.Password)
	require.Equal(t, "n", dst.Name)
}

type keySource struct {
	ID string
}

type keyDest struct {
	ID string
}

type keyMapSource struct {
	Items map[keySource]string
}

type keyMapDest struct {
	Items map[keyDest]string
}

func TestWithIgnorePaths_DoesNotTouchMapKeys(t *testing.T) {
	t.Parallel()
	src := keyMapSource{Items: map[keySource]string{{ID: "a"}: "1", {ID: "b"}: "2"}}
	var dst keyMapDest
	converter.Convert(src, &dst, converter.WithIgnorePaths("id"))
	require.Len(t, dst.Items, 2)
	for k := range dst.Items {
		require.NotEmpty(t, k.ID)
	}
}

type pointerPathSource struct {
	SMTP *pathLeaf
}

type pointerPathDest struct {
	SMTP *pathLeaf
}

func TestWithIgnorePaths_PointerField(t *testing.T) {
	t.Parallel()
	src := pointerPathSource{SMTP: &pathLeaf{Password: "secret", Headers: "h"}}
	var dst pointerPathDest
	converter.Convert(src, &dst, converter.WithIgnorePaths("smtp.password"))
	require.Empty(t, dst.SMTP.Password)
	require.Equal(t, "h", dst.SMTP.Headers)
}

func TestWithIgnorePaths_CaseAndDots(t *testing.T) {
	t.Parallel()
	var dst pathDest
	converter.Convert(pathFixture(), &dst, converter.WithIgnorePaths(".SMTP.Password."))
	require.Empty(t, dst.SMTP.Password)
	require.Equal(t, "smtp-headers", dst.SMTP.Headers)
}

type slicePointerSource struct {
	Tags *[]string
}

type slicePointerDest struct {
	Tags *[]string
}

func TestWithCopySlices_PointerToSlice(t *testing.T) {
	t.Parallel()
	tags := []string{"a"}
	src := slicePointerSource{Tags: &tags}
	var dst slicePointerDest
	converter.Convert(src, &dst, converter.WithCopySlices())
	(*dst.Tags)[0] = "changed"
	require.Equal(t, "a", tags[0])
}

type unexportedProfile struct {
	Tags []string
	hash string
}

func newUnexportedProfile() unexportedProfile {
	return unexportedProfile{Tags: []string{"a"}, hash: "h"}
}

func (p unexportedProfile) Hash() string { return p.hash }

type unexportedOuter struct {
	Profile unexportedProfile
}

func TestWithCopySlices_PreservesUnexportedFields(t *testing.T) {
	t.Parallel()
	src := unexportedOuter{Profile: newUnexportedProfile()}
	var dst unexportedOuter
	converter.Convert(src, &dst, converter.WithCopySlices())
	require.Equal(t, "h", dst.Profile.Hash())
	dst.Profile.Tags[0] = "changed"
	require.Equal(t, "a", src.Profile.Tags[0])
}

type overflowSource struct {
	N int64
}

type overflowDest struct {
	N int8
}

func TestConvertE_Overflow(t *testing.T) {
	t.Parallel()
	var dst overflowDest
	_, err := converter.ConvertE(overflowSource{N: 300}, &dst, converter.WithOverflowCheck())
	require.Error(t, err)
	var overflow converter.OverflowError
	require.ErrorAs(t, err, &overflow)
}

func TestConvert_DoesNotAllocateIncompatiblePointerWithCodecs(t *testing.T) {
	t.Parallel()
	noop := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		next(fieldName, src, dst)
	}
	var dst secretResponse
	converter.Convert(secretProvider{Headers: sealedValue{Ciphertext: []byte("secret")}}, &dst,
		converter.WithCodecs(noop))
	require.Nil(t, dst.Headers)
}

type ptrContainerSource struct {
	Items *[]pathLeaf
	Pair  *[2]pathLeaf
	ByID  *map[string]pathLeaf
}

type ptrContainerDest struct {
	Items *[]pathLeaf
	Pair  *[2]pathLeaf
	ByID  *map[string]pathLeaf
}

func TestWithIgnorePaths_PointerContainers(t *testing.T) {
	t.Parallel()
	items := []pathLeaf{{Password: "i", Headers: "ih"}}
	pair := [2]pathLeaf{{Password: "a", Headers: "ah"}, {Password: "b", Headers: "bh"}}
	byID := map[string]pathLeaf{"k": {Password: "m", Headers: "mh"}}
	src := ptrContainerSource{Items: &items, Pair: &pair, ByID: &byID}
	var dst ptrContainerDest
	converter.Convert(src, &dst,
		converter.WithIgnorePaths("items.password", "pair.password", "byid.password"))
	require.Empty(t, (*dst.Items)[0].Password)
	require.Empty(t, (*dst.Pair)[0].Password)
	require.Empty(t, (*dst.Pair)[1].Password)
	require.Empty(t, (*dst.ByID)["k"].Password)
	require.Equal(t, "ih", (*dst.Items)[0].Headers)
}

type sealedSliceSource struct {
	Items []sealedValue
}

type stringPointerSliceDest struct {
	Items []*string
}

func TestConvert_SliceOfStructToSliceOfStringPointers(t *testing.T) {
	t.Parallel()
	src := sealedSliceSource{Items: []sealedValue{{Ciphertext: []byte("secret")}}}
	var dst stringPointerSliceDest
	converter.Convert(src, &dst)
	require.Len(t, dst.Items, 1)
	require.Nil(t, dst.Items[0])
}

type distinctKeySource struct {
	ID string
}

type distinctKeyDest struct {
	ID    string
	Extra int
}

type distinctKeyMapSource struct {
	Items map[distinctKeySource]string
}

type distinctKeyMapDest struct {
	Items map[distinctKeyDest]string
}

func TestWithIgnorePaths_DoesNotTouchNonConvertibleMapKeys(t *testing.T) {
	t.Parallel()
	src := distinctKeyMapSource{Items: map[distinctKeySource]string{{ID: "a"}: "1", {ID: "b"}: "2"}}
	var dst distinctKeyMapDest
	converter.Convert(src, &dst, converter.WithIgnorePaths("id"))
	require.Len(t, dst.Items, 2)
	for k := range dst.Items {
		require.NotEmpty(t, k.ID)
	}
}

type embedPtrBase struct {
	Password string
}

type embedPtrSource struct {
	*embedPtrBase
	Name string
}

type embedPtrDest struct {
	*embedPtrBase
	Name string
}

func TestWithIgnorePaths_IgnoredEmbeddedPointerStaysNil(t *testing.T) {
	t.Parallel()
	src := embedPtrSource{embedPtrBase: &embedPtrBase{Password: "secret"}, Name: "n"}
	var dst embedPtrDest
	converter.Convert(src, &dst, converter.WithHandleEmbeddedStructs(true), converter.WithIgnorePaths("embedptrbase"))
	require.Nil(t, dst.embedPtrBase)
	require.Equal(t, "n", dst.Name)
}

type updateLeaf struct {
	Password string
	TLS      *string
}

type updateSource struct {
	SMTP updateLeaf
}

type updateDest struct {
	SMTP updateLeaf
}

func TestWithIgnorePaths_PreservesPrePopulatedDestination(t *testing.T) {
	t.Parallel()
	tls := "on"
	dst := updateDest{SMTP: updateLeaf{Password: "old", TLS: &tls}}
	converter.Convert(updateSource{SMTP: updateLeaf{Password: "new"}}, &dst,
		converter.WithIgnorePaths("smtp.password"))
	require.Equal(t, "old", dst.SMTP.Password)
	require.NotNil(t, dst.SMTP.TLS)
	require.Equal(t, "on", *dst.SMTP.TLS)
}

type recursiveNode struct {
	Parent *recursiveNode
	Tags   []string
}

type recursiveHolder struct {
	Items []recursiveNode
}

func TestWithCopySlices_RecursiveType(t *testing.T) {
	t.Parallel()
	src := recursiveHolder{Items: []recursiveNode{{Tags: []string{"a"}}}}
	var dst recursiveHolder
	require.NotPanics(t, func() {
		converter.Convert(src, &dst, converter.WithCopySlices())
	})
	dst.Items[0].Tags[0] = "changed"
	require.Equal(t, "a", src.Items[0].Tags[0])
}

func TestConvertMapSeq_DoesNotIgnoreKeys(t *testing.T) {
	t.Parallel()
	src := map[pathLeaf]string{{Password: "k"}: "v"}
	var got []pathLeaf
	for key := range converter.ConvertMapSeq[pathLeaf, string, pathLeaf, string](
		src, converter.WithIgnorePaths("password"),
	) {
		got = append(got, key)
	}
	require.Len(t, got, 1)
	require.Equal(t, "k", got[0].Password)
}

type recursiveMap map[string]recursiveMap

type recursiveMapHolder struct {
	Meta recursiveMap
}

func TestWithIgnorePaths_SelfReferentialContainer(t *testing.T) {
	t.Parallel()
	src := recursiveMapHolder{Meta: recursiveMap{"a": recursiveMap{}}}
	var dst recursiveMapHolder
	require.NotPanics(t, func() {
		converter.Convert(src, &dst, converter.WithIgnorePaths("other"))
	})
	require.Len(t, dst.Meta, 1)
}

type mapSliceItem struct {
	Name string
}

type mapToSliceSource struct {
	Items map[string]mapSliceItem
}

type mapToSliceDest struct {
	Items map[string][]mapSliceItem
}

func TestConvert_MapStructToSubject(t *testing.T) {
	t.Parallel()
	src := mapToSliceSource{Items: map[string]mapSliceItem{"k": {Name: "n"}}}
	var dst mapToSliceDest
	converter.Convert(src, &dst)
	require.Len(t, dst.Items["k"], 1)
	require.Equal(t, "n", dst.Items["k"][0].Name)
}

func TestConvert_CodecDoesNotAllocateIncompatiblePointerElement(t *testing.T) {
	t.Parallel()
	noop := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		next(fieldName, src, dst)
	}
	src := sealedSliceSource{Items: []sealedValue{{Ciphertext: []byte("secret")}}}
	var dst stringPointerSliceDest
	converter.Convert(src, &dst, converter.WithCodecs(noop))
	require.Len(t, dst.Items, 1)
	require.Nil(t, dst.Items[0])
}

type wrapperSliceSource struct {
	Items []*wrapperspb.StringValue
}

func TestConvert_CodecPreservesExplicitZeroPointer(t *testing.T) {
	t.Parallel()
	src := wrapperSliceSource{Items: []*wrapperspb.StringValue{wrapperspb.String("")}}
	var dst stringPointerSliceDest
	converter.Convert(src, &dst, converter.WithCodecs(pbwrap.New()))
	require.Len(t, dst.Items, 1)
	require.NotNil(t, dst.Items[0])
	require.Empty(t, *dst.Items[0])
}
