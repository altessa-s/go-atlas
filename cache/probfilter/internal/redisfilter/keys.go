// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
)

// MetaPrefix starts every key probfilter keeps next to a filter (rebuild
// staging keys, commit markers, generation tokens, delete deduplication).
// Filter keys must not start with it, so metadata can never collide with —
// and overwrite — a filter.
const MetaPrefix = "__probfilter__:"

// ErrReservedKey is returned by every operation of a [Core] whose filter key
// lies in the reserved [MetaPrefix] namespace.
var ErrReservedKey = errors.New("redis filter key uses the reserved " + MetaPrefix + " namespace")

// randomBytes is the number of random bytes in staging and request ids.
const randomBytes = 8

// metaKey returns the metadata key of the given kind for the live filter key:
//
//	__probfilter__:{<tag>}:<kind>:<hex(live)>[:<id>]
//
// The tag puts it in live's Redis Cluster slot (so scripts may touch both);
// the hex-encoded live key makes it unique per filter and unambiguous.
func metaKey(live, kind, id string) string {
	key := MetaPrefix + "{" + slotTagFor(live) + "}:" + kind + ":" + hex.EncodeToString([]byte(live))
	if id != "" {
		key += ":" + id
	}
	return key
}

// slotTagFor returns a hash tag whose Redis Cluster slot equals live's:
// live's own tag; else live itself when it can be wrapped in braces (no "}");
// else a searched numeric tag with the same CRC16 slot.
func slotTagFor(live string) string {
	if tag, ok := hashTag(live); ok {
		return tag
	}
	if live != "" && !strings.Contains(live, "}") {
		return live
	}
	return slotTag(keySlot(live))
}

// randomID returns a random hex id.
func randomID() (string, error) {
	var b [randomBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// clusterSlots is the number of Redis Cluster hash slots.
const clusterSlots = 16384

// hashTag returns the hash tag of key per the Redis Cluster rules: the text
// between the first "{" and the first "}" after it, if non-empty.
func hashTag(key string) (string, bool) {
	start := strings.IndexByte(key, '{')
	if start < 0 {
		return "", false
	}
	end := strings.IndexByte(key[start+1:], '}')
	if end <= 0 {
		return "", false
	}
	return key[start+1 : start+1+end], true
}

// keySlot returns the Redis Cluster hash slot of key.
func keySlot(key string) uint16 {
	if tag, ok := hashTag(key); ok {
		key = tag
	}
	return crc16(key) % clusterSlots
}

// slotTag returns the smallest decimal tag whose slot is slot. Every slot is
// reached within a few tens of thousands of candidates.
func slotTag(slot uint16) string {
	for i := 0; ; i++ {
		tag := strconv.Itoa(i)
		if crc16(tag)%clusterSlots == slot {
			return tag
		}
	}
}

// CRC-16/XMODEM parameters.
const (
	crc16Poly    = 0x1021
	crc16TopBit  = 0x8000
	bitsPerByte  = 8
	crc16ByteShf = 8
)

// crc16 is CRC-16/XMODEM, the checksum Redis Cluster uses for key slots.
func crc16(s string) uint16 {
	var crc uint16
	for i := range len(s) {
		crc ^= uint16(s[i]) << crc16ByteShf
		for range bitsPerByte {
			if crc&crc16TopBit != 0 {
				crc = crc<<1 ^ crc16Poly
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
