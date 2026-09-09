package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"io"
	"math/big"
	"strings"
	"testing"
)

func newDiscardState() *State {
	return &State{
		keys:        make(map[string]struct{}),
		keysWriter:  bufio.NewWriter(io.Discard),
		seeds:       make(map[string]struct{}),
		seedsWriter: bufio.NewWriter(io.Discard),
	}
}

func encodeBase58ForTest(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var n big.Int
	n.SetBytes(data)
	var mod big.Int
	var encoded []byte
	for n.Sign() > 0 {
		n.DivMod(&n, bigBase58, &mod)
		encoded = append(encoded, base58Alphabet[mod.Int64()])
	}

	for _, b := range data {
		if b != 0 {
			break
		}
		encoded = append(encoded, '1')
	}

	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	return string(encoded)
}

func makeTestWIF(key []byte, compressed bool) string {
	payload := append([]byte{0x80}, key...)
	if compressed {
		payload = append(payload, 0x01)
	}
	first := sha256.Sum256(payload)
	second := sha256.Sum256(first[:])
	full := append(append([]byte(nil), payload...), second[:4]...)
	return encodeBase58ForTest(full)
}

func corruptBase58Checksum(s string) string {
	if strings.HasSuffix(s, "1") {
		return s[:len(s)-1] + "2"
	}
	return s[:len(s)-1] + "1"
}

func TestBIP39ChecksumValidation(t *testing.T) {
	valid := strings.Fields("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about")
	invalid := strings.Fields("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon")
	mixed := strings.Fields("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon sobre")

	if !isValidBIP39Mnemonic(valid) {
		t.Fatal("expected valid mnemonic to pass checksum validation")
	}
	if isValidBIP39Mnemonic(invalid) {
		t.Fatal("expected checksum-broken mnemonic to be rejected")
	}
	if isValidBIP39Mnemonic(mixed) {
		t.Fatal("expected mixed-language mnemonic to be rejected")
	}
}

func TestScanTextSkipsChecksumGarbage(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 32)
	validWIF := makeTestWIF(key, true)
	invalidWIF := corruptBase58Checksum(validWIF)

	validMini := "S6c56bnXQiBjk9mqSYE7ykVQ7NzrRy"
	invalidMini := validMini[:len(validMini)-1] + "z"

	validSeed := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	invalidSeed := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon"

	s := newDiscardState()
	s.scanText([]byte(strings.Join([]string{invalidWIF, invalidMini, invalidSeed}, "\n")))
	if s.keyCount() != 0 {
		t.Fatalf("expected no keys from checksum garbage, got %d", s.keyCount())
	}
	if s.seedCount() != 0 {
		t.Fatalf("expected no seeds from checksum garbage, got %d", s.seedCount())
	}

	s.scanText([]byte(strings.Join([]string{validWIF, validMini, validSeed}, "\n")))
	if s.keyCount() != 2 {
		t.Fatalf("expected 2 valid keys, got %d", s.keyCount())
	}
	if s.seedCount() != 1 {
		t.Fatalf("expected 1 valid seed, got %d", s.seedCount())
	}
}

func TestScanTextAccepts0xHexKeys(t *testing.T) {
	plainHex := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	prefixedHex := "0x" + plainHex

	s := newDiscardState()
	s.scanText([]byte(prefixedHex + "\n" + plainHex))

	if s.keyCount() != 1 {
		t.Fatalf("expected prefixed and plain hex to dedupe to 1 key, got %d", s.keyCount())
	}
	if _, ok := s.keys[plainHex]; !ok {
		t.Fatal("expected normalized bare hex key to be stored")
	}
}

func TestLineHasMatchUsesChecksumValidation(t *testing.T) {
	key := bytes.Repeat([]byte{0x22}, 32)
	validWIF := makeTestWIF(key, false)
	invalidWIF := corruptBase58Checksum(validWIF)
	validSeed := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	invalidSeed := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon"

	s := newDiscardState()
	if s.lineHasMatch([]byte(invalidWIF)) {
		t.Fatal("expected invalid WIF to be treated as garbage in invalid mode")
	}
	if s.lineHasMatch([]byte(invalidSeed)) {
		t.Fatal("expected invalid mnemonic to be treated as garbage in invalid mode")
	}
	if !s.lineHasMatch([]byte(validWIF)) {
		t.Fatal("expected valid WIF to be recognized")
	}
	if !s.lineHasMatch([]byte(validSeed)) {
		t.Fatal("expected valid mnemonic to be recognized")
	}
}

func TestLineHasMatchRecognizesWhitespaceFormattedKeys(t *testing.T) {
	hexKey := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	spacedHex := "00112233 44556677 8899aabb ccddeeff 00112233 44556677 8899aabb ccddeeff"
	prefixedHex := "0x" + hexKey
	spacedPrefixedHex := "0x00112233 44556677 8899aabb ccddeeff 00112233 44556677 8899aabb ccddeeff"
	key := bytes.Repeat([]byte{0x33}, 32)
	validWIF := makeTestWIF(key, true)
	spacedWIF := validWIF[:10] + " " + validWIF[10:25] + " " + validWIF[25:]
	s := newDiscardState()

	if !s.lineHasMatch([]byte(hexKey)) {
		t.Fatal("expected plain hex key to be recognized")
	}
	if !s.lineHasMatch([]byte(prefixedHex)) {
		t.Fatal("expected 0x-prefixed hex key to be recognized")
	}
	if !s.lineHasMatch([]byte(spacedHex)) {
		t.Fatal("expected spaced hex key to be recognized")
	}
	if !s.lineHasMatch([]byte(spacedPrefixedHex)) {
		t.Fatal("expected spaced 0x-prefixed hex key to be recognized")
	}
	if !s.lineHasMatch([]byte(spacedWIF)) {
		t.Fatal("expected spaced WIF to be recognized")
	}
}
