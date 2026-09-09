package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	stdunicode "unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/bodgit/sevenzip"
	"github.com/klauspost/compress/zstd"
	"github.com/ledongthuc/pdf"
	"github.com/nwaples/rardecode/v2"
	"github.com/ulikunitz/xz"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/unicode/norm"
)

// =============================================================================
// Конфигурация
// =============================================================================

const (
	chunkSize         = 4 << 20   // 4 МБ - размер чанка стримингового сканирования
	chunkOverlap      = 4 << 10   // 4 КБ - перекрытие чанков (чтобы не разорвать совпадение)
	fileThreshold     = 16 << 20  // 16 МБ - порог "целиком в память" vs стриминг
	maxEntrySize      = 1 << 30   // 1 ГБ - предел разворачивания entry в RAM (защита от zip-бомб)
	maxArchiveDepth   = 16        // максимум вложенности архивов друг в друга
	maxMarkupStripLen = 128 << 20 // 128 МБ - предел стриппинга XML/HTML/RTF
	jobQueueSize      = 1024      // буфер очереди задач (меньше = меньше RAM на entries)
)

// Расширения, которые гарантированно не содержат текстовых секретов — на TB-объёмах
// фото/видео/аудио занимают много гигабайт каждого диска, и сканировать их бесполезно.
var skipExtensions = map[string]bool{
	// Изображения
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true,
	".tiff": true, ".tif": true, ".webp": true, ".ico": true, ".cur": true,
	".heic": true, ".heif": true, ".avif": true, ".jfif": true,
	".cr2": true, ".cr3": true, ".nef": true, ".arw": true, ".dng": true, ".raf": true, ".orf": true,
	".psd": true, ".ai": true, ".indd": true, ".xcf": true,
	// Видео
	".mp4": true, ".m4v": true, ".mkv": true, ".avi": true, ".mov": true,
	".wmv": true, ".webm": true, ".flv": true, ".mpg": true, ".mpeg": true,
	".3gp": true, ".3g2": true, ".vob": true, ".ogv": true, ".mts": true, ".m2ts": true,
	// Аудио
	".mp3": true, ".m4a": true, ".aac": true, ".flac": true, ".ogg": true,
	".wav": true, ".wma": true, ".opus": true, ".ape": true, ".alac": true,
	// Шрифты
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
}

func shouldSkipFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return skipExtensions[ext]
}

// bytesToString - zero-copy конвертация []byte в string.
// БЕЗОПАСНО только если данные не модифицируются после вызова (наш regex не модифицирует).
func bytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// =============================================================================
// Регулярные выражения
// =============================================================================

var (
	reHexKey  = regexp.MustCompile(`\b(?:0[xX])?[0-9a-fA-F]{64}\b`)
	reWIF     = regexp.MustCompile(`\b[5KL][1-9A-HJ-NP-Za-km-z]{50,51}\b`)
	reMiniKey = regexp.MustCompile(`\bS[1-9A-HJ-NP-Za-km-z]{29}\b`)

	// Поиск всех "слов" (2-12 unicode-букв подряд) для tokenize-based scanning сидов
	wordExtractRe = regexp.MustCompile(`\p{L}{2,12}`)

	// Допустимые длины BIP39 mnemonic
	bip39Lengths = []int{12, 15, 18, 21, 24}
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var base58DecodeMap [256]byte
var base58ValidMap [256]bool
var bigBase58 = big.NewInt(58)

// =============================================================================
// BIP39 wordlist
// =============================================================================

//go:embed bip39_english.txt bip39_spanish.txt bip39_french.txt bip39_italian.txt bip39_portuguese.txt bip39_czech.txt bip39_japanese.txt bip39_korean.txt
var bip39FS embed.FS

var bip39Words = make(map[string]struct{}, 16384)
var bip39Languages []map[string]uint16

func init() {
	for i := 0; i < len(base58Alphabet); i++ {
		ch := base58Alphabet[i]
		base58DecodeMap[ch] = byte(i)
		base58ValidMap[ch] = true
	}

	entries, err := bip39FS.ReadDir(".")
	if err != nil {
		return
	}
	for _, e := range entries {
		data, err := bip39FS.ReadFile(e.Name())
		if err != nil {
			continue
		}
		lang := make(map[string]uint16, 2048)
		for idx, w := range strings.Split(string(data), "\n") {
			w = normalizeBIP39Word(w)
			if w == "" {
				continue
			}
			bip39Words[w] = struct{}{}
			lang[w] = uint16(idx)
		}
		bip39Languages = append(bip39Languages, lang)
	}
}

func normalizeBIP39Word(w string) string {
	return strings.ToLower(strings.TrimSpace(norm.NFKD.String(w)))
}

func isBIP39Word(w string) bool {
	_, ok := bip39Words[normalizeBIP39Word(w)]
	return ok
}

func normalizeHexKey(k string) string {
	k = strings.ToLower(k)
	return strings.TrimPrefix(k, "0x")
}

func decodeBase58(s string) ([]byte, bool) {
	if s == "" {
		return nil, false
	}

	var n big.Int
	var digit big.Int
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !base58ValidMap[ch] {
			return nil, false
		}
		n.Mul(&n, bigBase58)
		digit.SetInt64(int64(base58DecodeMap[ch]))
		n.Add(&n, &digit)
	}

	out := n.Bytes()
	leadingZeros := 0
	for leadingZeros < len(s) && s[leadingZeros] == '1' {
		leadingZeros++
	}
	if leadingZeros == 0 {
		return out, true
	}

	prefixed := make([]byte, leadingZeros+len(out))
	copy(prefixed[leadingZeros:], out)
	return prefixed, true
}

func hasValidBase58Checksum(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	payload := data[:len(data)-4]
	checksum := data[len(data)-4:]
	first := sha256.Sum256(payload)
	second := sha256.Sum256(first[:])
	return bytes.Equal(second[:4], checksum)
}

func isValidWIF(wif string) bool {
	decoded, ok := decodeBase58(wif)
	if !ok || (len(decoded) != 37 && len(decoded) != 38) {
		return false
	}
	if !hasValidBase58Checksum(decoded) {
		return false
	}

	payload := decoded[:len(decoded)-4]
	if payload[0] != 0x80 {
		return false
	}
	if len(payload) == 34 {
		return payload[len(payload)-1] == 0x01
	}
	return len(payload) == 33
}

func isValidMiniKey(key string) bool {
	sum := sha256.Sum256([]byte(key + "?"))
	return sum[0] == 0x00
}

func isValidBIP39Mnemonic(words []string) bool {
	if len(words) == 0 {
		return false
	}

	csBits := len(words) / 3
	entBits := csBits * 32
	switch len(words) {
	case 12, 15, 18, 21, 24:
	default:
		return false
	}

	normalized := make([]string, len(words))
	for i, w := range words {
		normalized[i] = normalizeBIP39Word(w)
	}

	indices := make([]uint16, len(words))
	for _, lang := range bip39Languages {
		ok := true
		for i, w := range normalized {
			idx, exists := lang[w]
			if !exists {
				ok = false
				break
			}
			indices[i] = idx
		}
		if !ok {
			continue
		}

		entropy := make([]byte, entBits/8)
		var actualChecksum byte
		bitPos := 0
		for _, idx := range indices {
			for bit := 10; bit >= 0; bit-- {
				v := byte((idx >> bit) & 1)
				if bitPos < entBits {
					if v == 1 {
						entropy[bitPos/8] |= 1 << uint(7-(bitPos%8))
					}
				} else {
					actualChecksum = (actualChecksum << 1) | v
				}
				bitPos++
			}
		}

		sum := sha256.Sum256(entropy)
		expectedChecksum := sum[0] >> uint(8-csBits)
		if actualChecksum == expectedChecksum {
			return true
		}
	}

	return false
}

// =============================================================================
// Распознавание формата архива
// =============================================================================

type Format int

const (
	FormatNone Format = iota
	FormatZip
	FormatTar
	FormatGz
	FormatBz2
	FormatXz
	FormatZstd
	FormatSevenZip
	FormatRar
	FormatPDF
)

var zipExtensions = []string{
	".zip", ".jar", ".war", ".ear",
	".apk", ".ipa", ".aab", ".xpi",
	".docx", ".xlsx", ".pptx",
	".odt", ".ods", ".odp",
	".epub", ".kmz", ".whl", ".egg", ".nupkg",
}

func nameToFormat(name string) Format {
	lower := strings.ToLower(name)
	for _, e := range zipExtensions {
		if strings.HasSuffix(lower, e) {
			return FormatZip
		}
	}
	switch {
	case strings.HasSuffix(lower, ".tar"):
		return FormatTar
	case strings.HasSuffix(lower, ".gz"), strings.HasSuffix(lower, ".tgz"):
		return FormatGz
	case strings.HasSuffix(lower, ".bz2"), strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".tbz"):
		return FormatBz2
	case strings.HasSuffix(lower, ".xz"), strings.HasSuffix(lower, ".txz"):
		return FormatXz
	case strings.HasSuffix(lower, ".zst"), strings.HasSuffix(lower, ".zstd"):
		return FormatZstd
	case strings.HasSuffix(lower, ".7z"):
		return FormatSevenZip
	case strings.HasSuffix(lower, ".rar"):
		return FormatRar
	case strings.HasSuffix(lower, ".pdf"):
		return FormatPDF
	}
	return FormatNone
}

func magicToFormat(head []byte) Format {
	n := len(head)
	// ZIP (PK\x03\x04, PK\x05\x06, PK\x07\x08)
	if n >= 4 && head[0] == 'P' && head[1] == 'K' && (head[2] == 0x03 || head[2] == 0x05 || head[2] == 0x07) {
		return FormatZip
	}
	// GZIP
	if n >= 2 && head[0] == 0x1F && head[1] == 0x8B {
		return FormatGz
	}
	// BZIP2
	if n >= 3 && head[0] == 'B' && head[1] == 'Z' && head[2] == 'h' {
		return FormatBz2
	}
	// XZ
	if n >= 6 && head[0] == 0xFD && head[1] == 0x37 && head[2] == 0x7A && head[3] == 0x58 && head[4] == 0x5A && head[5] == 0x00 {
		return FormatXz
	}
	// ZSTD
	if n >= 4 && head[0] == 0x28 && head[1] == 0xB5 && head[2] == 0x2F && head[3] == 0xFD {
		return FormatZstd
	}
	// 7Z
	if n >= 6 && head[0] == 0x37 && head[1] == 0x7A && head[2] == 0xBC && head[3] == 0xAF && head[4] == 0x27 && head[5] == 0x1C {
		return FormatSevenZip
	}
	// RAR4 / RAR5
	if n >= 7 && head[0] == 'R' && head[1] == 'a' && head[2] == 'r' && head[3] == '!' && head[4] == 0x1A && head[5] == 0x07 {
		return FormatRar
	}
	// TAR (ustar в offset 257)
	if n >= 263 && bytes.Equal(head[257:262], []byte("ustar")) {
		return FormatTar
	}
	// PDF
	if n >= 5 && bytes.HasPrefix(head, []byte("%PDF-")) {
		return FormatPDF
	}
	return FormatNone
}

func detectFormat(name string, head []byte) Format {
	if f := magicToFormat(head); f != FormatNone {
		return f
	}
	return nameToFormat(name)
}

// =============================================================================
// Состояние и поиск
// =============================================================================

type State struct {
	keys       map[string]struct{}
	keysFile   *os.File
	keysWriter *bufio.Writer
	keysMu     sync.Mutex

	seeds       map[string]struct{}
	seedsFile   *os.File
	seedsWriter *bufio.Writer
	seedsMu     sync.Mutex

	// Опциональный режим записи "невалидных" строк (без ключей/сидов).
	// Активен, если invalidWriter != nil.
	invalidFile   *os.File
	invalidWriter *bufio.Writer
	invalidMu     sync.Mutex
	invalidLines  atomic.Uint64

	filesScanned   atomic.Uint64
	archivesOpened atomic.Uint64
	entriesScanned atomic.Uint64
	bytesScanned   atomic.Uint64
}

func newState(keysPath, seedsPath, invalidPath string) (*State, error) {
	kf, err := os.Create(keysPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", keysPath, err)
	}
	sf, err := os.Create(seedsPath)
	if err != nil {
		kf.Close()
		return nil, fmt.Errorf("%s: %w", seedsPath, err)
	}
	st := &State{
		keys:        make(map[string]struct{}),
		keysFile:    kf,
		keysWriter:  bufio.NewWriter(kf),
		seeds:       make(map[string]struct{}),
		seedsFile:   sf,
		seedsWriter: bufio.NewWriter(sf),
	}
	if invalidPath != "" {
		inf, err := os.Create(invalidPath)
		if err != nil {
			kf.Close()
			sf.Close()
			return nil, fmt.Errorf("%s: %w", invalidPath, err)
		}
		st.invalidFile = inf
		st.invalidWriter = bufio.NewWriterSize(inf, 1<<20)
	}
	return st, nil
}

func (s *State) Close() {
	s.keysMu.Lock()
	if s.keysWriter != nil {
		s.keysWriter.Flush()
	}
	if s.keysFile != nil {
		s.keysFile.Close()
	}
	s.keysMu.Unlock()

	s.seedsMu.Lock()
	if s.seedsWriter != nil {
		s.seedsWriter.Flush()
	}
	if s.seedsFile != nil {
		s.seedsFile.Close()
	}
	s.seedsMu.Unlock()

	s.invalidMu.Lock()
	if s.invalidWriter != nil {
		s.invalidWriter.Flush()
	}
	if s.invalidFile != nil {
		s.invalidFile.Close()
	}
	s.invalidMu.Unlock()
}

// writeInvalidLine - пишет одну невалидную строку в invalid-файл.
// Lock защищает от перемешивания строк между воркерами.
func (s *State) writeInvalidLine(line []byte) {
	if s.invalidWriter == nil || len(line) == 0 {
		return
	}
	s.invalidMu.Lock()
	s.invalidWriter.Write(line)
	s.invalidWriter.WriteByte('\n')
	s.invalidMu.Unlock()
	s.invalidLines.Add(1)
}

func lineHasKeyMatch(text string) bool {
	if reHexKey.MatchString(text) {
		return true
	}
	for _, m := range reWIF.FindAllString(text, -1) {
		if isValidWIF(m) {
			return true
		}
	}
	for _, m := range reMiniKey.FindAllString(text, -1) {
		if isValidMiniKey(m) {
			return true
		}
	}
	return false
}

func lineHasMatchText(text string) bool {
	if lineHasKeyMatch(text) {
		return true
	}
	return lineHasBIP39Run(text)
}

// lineHasMatch - true если в строке есть хотя бы один ключ
// или checksum-валидная BIP39 сид-фраза. Используется только для построчного invalid-режима.
func (s *State) lineHasMatch(line []byte) bool {
	if lineHasMatchText(bytesToString(line)) {
		return true
	}

	if !utf8.Valid(line) && hasNonASCII(line) {
		if conv, err := charmap.Windows1251.NewDecoder().Bytes(line); err == nil {
			if lineHasMatchText(bytesToString(conv)) {
				return true
			}
			line = conv
		}
	}

	if len(line) > maxMarkupStripLen {
		return false
	}

	compact := stripWhitespaceBytes(line)
	if len(compact) != len(line) && lineHasKeyMatch(bytesToString(compact)) {
		return true
	}

	if kind := markupKind(line); kind != markupNone {
		stripped := stripMarkup(line, kind)
		if lineHasMatchText(bytesToString(stripped)) {
			return true
		}
		compactStripped := stripWhitespaceBytes(stripped)
		if len(compactStripped) != len(stripped) && lineHasKeyMatch(bytesToString(compactStripped)) {
			return true
		}
	}

	return false
}

// lineHasBIP39Run - true если в строке есть checksum-валидная BIP39 сид-фраза.
func lineHasBIP39Run(text string) bool {
	found := false
	scanValidSeedPhrases(text, func(string) bool {
		found = true
		return false
	})
	return found
}

// scanLinesForInvalid - построчно проходит по уже декодированным данным
// (UTF-8). Каждую строку без матча пишет в invalid-файл.
// Применяет decodeText снаружи — здесь ожидаем уже UTF-8.
func (s *State) scanLinesForInvalid(data []byte) {
	if s.invalidWriter == nil {
		return
	}
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			line := data[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 && !s.lineHasMatch(line) {
				s.writeInvalidLine(line)
			}
			start = i + 1
		}
	}
	if start < len(data) {
		line := data[start:]
		if len(line) > 0 && !s.lineHasMatch(line) {
			s.writeInvalidLine(line)
		}
	}
}

// scanStreamLinesForInvalid - построчно читает большой файл через bufio.Scanner
// (без удержания всего файла в RAM). Используется на файлах > fileThreshold.
func (s *State) scanStreamLinesForInvalid(r io.Reader) error {
	if s.invalidWriter == nil {
		return nil
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 256<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) > 0 && !s.lineHasMatch(line) {
			s.writeInvalidLine(line)
		}
	}
	return scanner.Err()
}

// addKey - если ключ новый, добавляет в map и сразу пишет в файл.
func (s *State) addKey(k string) {
	s.keysMu.Lock()
	if _, ok := s.keys[k]; !ok {
		s.keys[k] = struct{}{}
		s.keysWriter.WriteString(k)
		s.keysWriter.WriteByte('\n')
		s.keysWriter.Flush()
	}
	s.keysMu.Unlock()
}

// scanKeysOnly - прогоняет только hex/WIF/MiniKey-регэксы (без сидов).
// Используется для cross-line-поиска на тексте со снятыми пробелами/переносами.
func (s *State) scanKeysOnly(data []byte) {
	text := bytesToString(data)
	for _, m := range reHexKey.FindAllString(text, -1) {
		s.addKey(normalizeHexKey(m))
	}
	for _, m := range reWIF.FindAllString(text, -1) {
		if isValidWIF(m) {
			s.addKey(m)
		}
	}
	for _, m := range reMiniKey.FindAllString(text, -1) {
		if isValidMiniKey(m) {
			s.addKey(m)
		}
	}
}

// stripWhitespaceBytes - возвращает копию data без любых пробельных символов
// (пробел, табы, переносы строк, vertical tab, form feed).
func stripWhitespaceBytes(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			continue
		}
		out = append(out, b)
	}
	return out
}

// addSeed - если сид новый, добавляет в map и сразу пишет в файл.
func (s *State) addSeed(p string) {
	s.seedsMu.Lock()
	if _, ok := s.seeds[p]; !ok {
		s.seeds[p] = struct{}{}
		s.seedsWriter.WriteString(p)
		s.seedsWriter.WriteByte('\n')
		s.seedsWriter.Flush()
	}
	s.seedsMu.Unlock()
}

// scan - точка входа сканера. Определяет кодировку (UTF-8 / UTF-16 / UTF-32 по BOM,
// для невалидного UTF-8 пробует Windows-1251) и прогоняет регэксы по каждому варианту.
func (s *State) scan(data []byte) {
	originalLen := len(data)

	primary := decodeText(data)
	s.scanText(primary)

	// Невалидный UTF-8 с non-ASCII байтами — вероятно, legacy-кодировка
	// (cp1251/cp866/koi8-r). Пробуем декодировать как cp1251 для покрытия
	// русских BIP39-слов в старых .txt/.log/.csv-файлах.
	if !utf8.Valid(primary) && hasNonASCII(primary) {
		if conv, err := charmap.Windows1251.NewDecoder().Bytes(primary); err == nil {
			s.scanText(conv)
		}
	}

	// Cross-line hex/WIF/MiniKey: hex-ключ может быть разнесён по строкам колонкой
	// или с пробелами через каждые 4 символа. Удаляем все whitespace и сканируем
	// hex-регэксами на сжатом тексте.
	if len(primary) <= maxMarkupStripLen {
		stripped := stripWhitespaceBytes(primary)
		s.scanKeysOnly(stripped)
	}

	// XML/HTML/RTF: разметка ломает якорь ^...$ для регэкса сидов
	// (текст обёрнут в теги, нет переносов). Прогоняем стриппер и сканируем ещё раз.
	// Покрывает docx/xlsx/pptx/odt/epub (после распаковки ZIP), html, svg, xml, rtf.
	if len(primary) <= maxMarkupStripLen {
		if kind := markupKind(primary); kind != markupNone {
			stripped := stripMarkup(primary, kind)
			s.scanText(stripped)
		}
	}

	s.bytesScanned.Add(uint64(originalLen))
}

// scanText - чистый regex-проход по уже декодированной UTF-8 строке.
// Каждый новый уникальный матч сразу пишется в файл (см. addKey/addSeed).
func (s *State) scanText(data []byte) {
	text := bytesToString(data)

	for _, m := range reHexKey.FindAllString(text, -1) {
		s.addKey(normalizeHexKey(m))
	}
	for _, m := range reWIF.FindAllString(text, -1) {
		if isValidWIF(m) {
			s.addKey(m)
		}
	}
	for _, m := range reMiniKey.FindAllString(text, -1) {
		if isValidMiniKey(m) {
			s.addKey(m)
		}
	}

	s.scanSeeds(text)
}

// scanSeeds - tokenize-based поиск BIP39 mnemonic.
//  1. Находит все letter-"слова" 2-12 unicode-букв в тексте.
//  2. Для каждого слова проверяет, есть ли оно в объединённом BIP39 wordlist
//     (8 официальных языков: en/es/fr/it/pt/cz/ja/ko).
//  3. Вычисляет максимальный непрерывный run BIP39-слов начиная с каждой позиции
//     (с разделителями из не-букв длиной 1-16 байт).
//  4. Эмитит фразы длины 12/15/18/21/24, если run покрывает.
//
// Все слова в фразе должны быть BIP39, а сама фраза обязана проходить checksum-проверку.
func (s *State) scanSeeds(text string) {
	scanValidSeedPhrases(text, func(phrase string) bool {
		s.addSeed(phrase)
		return true
	})
}

func scanValidSeedPhrases(text string, emit func(string) bool) {
	wordIdx := wordExtractRe.FindAllStringIndex(text, -1)
	if len(wordIdx) < 12 {
		return
	}

	// Шаг 1: для каждого слова - флаг BIP39
	bip39Valid := make([]bool, len(wordIdx))
	for i, idx := range wordIdx {
		bip39Valid[i] = isBIP39Word(text[idx[0]:idx[1]])
	}

	// Шаг 2: находим максимальные run'ы подряд идущих BIP39-слов
	// (с валидными разделителями между ними)
	type run struct{ start, length int }
	var runs []run
	for i := 0; i < len(wordIdx); {
		if !bip39Valid[i] {
			i++
			continue
		}
		j := i + 1
		for j < len(wordIdx) && bip39Valid[j] {
			gap := text[wordIdx[j-1][1]:wordIdx[j][0]]
			if !isValidSeedGap(gap) {
				break
			}
			j++
		}
		if j-i >= 12 {
			runs = append(runs, run{i, j - i})
		}
		i = j
	}

	// Шаг 3: эмит из каждого run'а
	// - если длина run == 12/15/18/21/24 точно: одна запись (clean BIP39-фраза).
	// - иначе (например 13, 14, 16...): пробуем все starting positions внутри run'а
	//   с самой длинной подходящей BIP39-длиной. Это нужно, чтобы поймать сид
	//   "letter ... above" в раскладке, где он соседствует с лишним BIP39-словом
	//   типа "phrase" перед ним.
	for _, r := range runs {
		isExact := false
		for _, n := range bip39Lengths {
			if r.length == n {
				isExact = true
				break
			}
		}
		if isExact {
			if !emitSeedWords(text, wordIdx, r.start, r.length, emit) {
				return
			}
			continue
		}
		for offset := 0; offset+12 <= r.length; offset++ {
			remaining := r.length - offset
			emitLen := 0
			for j := len(bip39Lengths) - 1; j >= 0; j-- {
				if bip39Lengths[j] <= remaining {
					emitLen = bip39Lengths[j]
					break
				}
			}
			if emitLen == 0 {
				continue
			}
			if !emitSeedWords(text, wordIdx, r.start+offset, emitLen, emit) {
				return
			}
		}
	}
}

func emitSeedWords(text string, wordIdx [][]int, start, n int, emit func(string) bool) bool {
	words := make([]string, n)
	for j := 0; j < n; j++ {
		words[j] = text[wordIdx[start+j][0]:wordIdx[start+j][1]]
	}
	if !isValidBIP39Mnemonic(words) {
		return true
	}
	if emit == nil {
		return false
	}
	return emit(strings.Join(words, " "))
}

// isValidSeedGap - разделитель между BIP39-словами: 1-16 байт не-букв
// (whitespace, переносы, запятые, цифры нумерации, двоеточия и т.п.).
func isValidSeedGap(gap string) bool {
	if len(gap) == 0 || len(gap) > 16 {
		return false
	}
	for _, r := range gap {
		if stdunicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// decodeText - распознаёт BOM и декодирует UTF-16/UTF-32 в UTF-8. UTF-8 BOM срезает.
// Без BOM возвращает как есть.
func decodeText(data []byte) []byte {
	// UTF-32 LE BOM: FF FE 00 00 (проверяем раньше UTF-16 LE)
	if len(data) >= 4 && data[0] == 0xFF && data[1] == 0xFE && data[2] == 0x00 && data[3] == 0x00 {
		return decodeUTF32(data[4:], true)
	}
	// UTF-32 BE BOM: 00 00 FE FF
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0xFE && data[3] == 0xFF {
		return decodeUTF32(data[4:], false)
	}
	// UTF-16 LE BOM: FF FE
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		if conv, err := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder().Bytes(data[2:]); err == nil {
			return conv
		}
	}
	// UTF-16 BE BOM: FE FF
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		if conv, err := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder().Bytes(data[2:]); err == nil {
			return conv
		}
	}
	// UTF-8 BOM
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

func decodeUTF32(data []byte, littleEndian bool) []byte {
	var buf bytes.Buffer
	buf.Grow(len(data) / 2)
	for i := 0; i+3 < len(data); i += 4 {
		var r rune
		if littleEndian {
			r = rune(uint32(data[i]) | uint32(data[i+1])<<8 | uint32(data[i+2])<<16 | uint32(data[i+3])<<24)
		} else {
			r = rune(uint32(data[i])<<24 | uint32(data[i+1])<<16 | uint32(data[i+2])<<8 | uint32(data[i+3]))
		}
		buf.WriteRune(r)
	}
	return buf.Bytes()
}

func hasNonASCII(data []byte) bool {
	for _, b := range data {
		if b > 0x7F {
			return true
		}
	}
	return false
}

type markupType int

const (
	markupNone markupType = iota
	markupXML
	markupRTF
)

var (
	// Теги-абзацы: их меняем на \n, чтобы сохранить строковые границы для regex сидов
	reParaTag = regexp.MustCompile(`(?i)<(?:/?p|/?w:p|/?w:br|br|/li|/tr|/h[1-6]|/div|/title|/text:p|/text:h|/draw:text-box)[^>]*>`)
	// Все остальные теги (включая <?xml ... ?> и <!-- ... -->) -> пробел
	reAnyTag    = regexp.MustCompile(`<[^>]*>`)
	reHTMLEnt   = regexp.MustCompile(`&(?:[a-zA-Z][a-zA-Z0-9]{1,9}|#\d{1,7}|#[xX][0-9a-fA-F]{1,6});`)
	reRTFCtrl   = regexp.MustCompile(`\\(?:[a-zA-Z]+-?\d*\s?|[^a-zA-Z])`)
	reRTFBraces = regexp.MustCompile(`[{}]`)
	reSpaces    = regexp.MustCompile(`[ \t]+`)
)

func markupKind(data []byte) markupType {
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	trimmed := bytes.TrimLeft(sample, " \t\r\n\xEF\xBB\xBF")
	if len(trimmed) == 0 {
		return markupNone
	}
	if trimmed[0] == '<' {
		return markupXML
	}
	if bytes.HasPrefix(trimmed, []byte(`{\rtf`)) {
		return markupRTF
	}
	return markupNone
}

func stripMarkup(data []byte, kind markupType) []byte {
	switch kind {
	case markupXML:
		out := reParaTag.ReplaceAll(data, []byte("\n"))
		out = reAnyTag.ReplaceAll(out, []byte(" "))
		out = reHTMLEnt.ReplaceAll(out, []byte(" "))
		out = reSpaces.ReplaceAll(out, []byte(" "))
		return out
	case markupRTF:
		out := reRTFCtrl.ReplaceAll(data, []byte(" "))
		out = reRTFBraces.ReplaceAll(out, []byte("\n"))
		out = reSpaces.ReplaceAll(out, []byte(" "))
		return out
	}
	return data
}

// scanStream - сканирование больших источников чанками с перекрытием
func (s *State) scanStream(r io.Reader) error {
	buf := make([]byte, chunkSize)
	var carry []byte
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			chunk := make([]byte, 0, len(carry)+n)
			chunk = append(chunk, carry...)
			chunk = append(chunk, buf[:n]...)
			s.scan(chunk)
			tail := chunkOverlap
			if n < tail {
				tail = n
			}
			carry = append(carry[:0], buf[n-tail:n]...)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *State) scanStreamWithInvalid(r io.Reader) error {
	buf := make([]byte, chunkSize)
	var carry []byte
	var lineCarry []byte
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			chunk := make([]byte, 0, len(carry)+n)
			chunk = append(chunk, carry...)
			chunk = append(chunk, buf[:n]...)
			s.scan(chunk)
			tail := chunkOverlap
			if n < tail {
				tail = n
			}
			carry = append(carry[:0], buf[n-tail:n]...)
			lineCarry = s.scanInvalidChunk(lineCarry, buf[:n])
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			if len(lineCarry) > 0 && !s.lineHasMatch(lineCarry) {
				s.writeInvalidLine(lineCarry)
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *State) scanInvalidChunk(carry, data []byte) []byte {
	if s.invalidWriter == nil {
		return carry[:0]
	}
	if len(carry) > 0 {
		combined := make([]byte, 0, len(carry)+len(data))
		combined = append(combined, carry...)
		combined = append(combined, data...)
		data = combined
	}

	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] != '\n' {
			continue
		}
		line := data[start:i]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && !s.lineHasMatch(line) {
			s.writeInvalidLine(line)
		}
		start = i + 1
	}

	if start >= len(data) {
		return carry[:0]
	}
	next := append(carry[:0], data[start:]...)
	return next
}

func (s *State) keyCount() int {
	s.keysMu.Lock()
	defer s.keysMu.Unlock()
	return len(s.keys)
}

func (s *State) seedCount() int {
	s.seedsMu.Lock()
	defer s.seedsMu.Unlock()
	return len(s.seeds)
}

// =============================================================================
// Очередь задач и воркеры
// =============================================================================

type Job struct {
	Name  string // логическое имя (для лога)
	Path  string // если задан - читаем с диска
	Data  []byte // если задан - читаем из памяти
	Depth int    // глубина вложенности архивов
}

type Runner struct {
	state   *State
	jobs    chan Job
	wg      sync.WaitGroup
	workers sync.WaitGroup
}

func newRunner(state *State, numWorkers int) *Runner {
	r := &Runner{
		state: state,
		jobs:  make(chan Job, jobQueueSize),
	}
	for i := 0; i < numWorkers; i++ {
		r.workers.Add(1)
		go r.workerLoop()
	}
	return r
}

func (r *Runner) workerLoop() {
	defer r.workers.Done()
	for j := range r.jobs {
		r.execute(j)
	}
}

func (r *Runner) enqueue(j Job) {
	r.wg.Add(1)
	select {
	case r.jobs <- j:
		return
	default:
	}
	// Очередь переполнена: вместо порождения миллионов горутин на TB-данных
	// обрабатываем задачу инлайн прямо в вызывающей горутине (back-pressure).
	r.wg.Done()
	r.executeInline(j)
}

func (r *Runner) executeInline(j Job) {
	defer func() { _ = recover() }()
	if j.Path != "" {
		r.processFile(j.Path)
		return
	}
	r.processData(j.Name, j.Data, j.Depth)
}

func (r *Runner) execute(j Job) {
	defer r.wg.Done()
	defer func() { _ = recover() }()
	if j.Path != "" {
		r.processFile(j.Path)
		return
	}
	r.processData(j.Name, j.Data, j.Depth)
}

func (r *Runner) wait() {
	r.wg.Wait()
	close(r.jobs)
	r.workers.Wait()
}

func (r *Runner) processFile(path string) {
	if shouldSkipFile(path) {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return
	}

	r.state.filesScanned.Add(1)

	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	if n < 0 {
		n = 0
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return
	}
	head = head[:n]

	format := detectFormat(path, head)

	if format == FormatNone {
		if fi.Size() <= fileThreshold {
			data, err := io.ReadAll(f)
			if err != nil {
				return
			}
			r.state.scan(data)
			if r.state.invalidWriter != nil {
				r.state.scanLinesForInvalid(decodeText(data))
			}
		} else {
			_ = r.state.scanStream(f)
			if r.state.invalidWriter != nil {
				if _, err := f.Seek(0, io.SeekStart); err == nil {
					_ = r.state.scanStreamLinesForInvalid(f)
				}
			}
		}
		return
	}

	r.state.archivesOpened.Add(1)
	if err := r.dispatchArchive(path, f, fi.Size(), format, 0); err != nil {
		// Фоллбэк: сканируем как обычный файл
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr == nil {
			if fi.Size() <= fileThreshold {
				if data, readErr := io.ReadAll(f); readErr == nil {
					r.state.scan(data)
					if r.state.invalidWriter != nil {
						r.state.scanLinesForInvalid(decodeText(data))
					}
				}
			} else {
				_ = r.state.scanStream(f)
				if r.state.invalidWriter != nil {
					if _, err := f.Seek(0, io.SeekStart); err == nil {
						_ = r.state.scanStreamLinesForInvalid(f)
					}
				}
			}
		}
	}
}

func (r *Runner) processData(name string, data []byte, depth int) {
	if shouldSkipFile(name) {
		return
	}
	r.state.entriesScanned.Add(1)

	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	format := detectFormat(name, head)

	if format == FormatNone || depth >= maxArchiveDepth {
		r.state.scan(data)
		if r.state.invalidWriter != nil {
			r.state.scanLinesForInvalid(decodeText(data))
		}
		return
	}

	r.state.archivesOpened.Add(1)
	reader := bytes.NewReader(data)
	if err := r.dispatchArchive(name, reader, int64(len(data)), format, depth); err != nil {
		r.state.scan(data)
		if r.state.invalidWriter != nil {
			r.state.scanLinesForInvalid(decodeText(data))
		}
	}
}

func (r *Runner) processEntryReader(name string, rd io.Reader, depth int) error {
	if shouldSkipFile(name) {
		return nil
	}
	r.state.entriesScanned.Add(1)

	head := make([]byte, 512)
	n, err := io.ReadFull(rd, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return err
	}
	head = head[:n]
	src := io.MultiReader(bytes.NewReader(head), rd)
	format := detectFormat(name, head)

	if format == FormatNone || depth >= maxArchiveDepth {
		return r.scanPlainReader(src)
	}

	r.state.archivesOpened.Add(1)
	if formatNeedsReaderAt(format) {
		tmp, size, cleanup, err := spoolReaderAt(src)
		if err != nil {
			return err
		}
		defer cleanup()
		if err := r.dispatchArchive(name, tmp, size, format, depth); err != nil {
			if _, seekErr := tmp.Seek(0, io.SeekStart); seekErr == nil {
				return r.scanPlainReader(tmp)
			}
			return err
		}
		return nil
	}

	return r.dispatchArchive(name, src, -1, format, depth)
}

func (r *Runner) scanPlainReader(rd io.Reader) error {
	data, tooLarge, err := readUpTo(rd, fileThreshold)
	if err != nil {
		return err
	}
	if !tooLarge {
		r.state.scan(data)
		if r.state.invalidWriter != nil {
			r.state.scanLinesForInvalid(decodeText(data))
		}
		return nil
	}

	stream := io.MultiReader(bytes.NewReader(data), rd)
	if r.state.invalidWriter != nil {
		return r.state.scanStreamWithInvalid(stream)
	}
	return r.state.scanStream(stream)
}

func (r *Runner) dispatchArchive(name string, src io.Reader, size int64, format Format, depth int) error {
	switch format {
	case FormatZip:
		ra, ok := src.(io.ReaderAt)
		if !ok {
			return errors.New("zip: требуется io.ReaderAt")
		}
		return r.extractZip(name, ra, size, depth)
	case FormatTar:
		return r.extractTar(name, src, depth)
	case FormatGz:
		return r.extractGz(name, src, depth)
	case FormatBz2:
		return r.extractBz2(name, src, depth)
	case FormatXz:
		return r.extractXz(name, src, depth)
	case FormatZstd:
		return r.extractZstd(name, src, depth)
	case FormatSevenZip:
		ra, ok := src.(io.ReaderAt)
		if !ok {
			return errors.New("7z: требуется io.ReaderAt")
		}
		return r.extract7z(name, ra, size, depth)
	case FormatRar:
		return r.extractRar(name, src, depth)
	case FormatPDF:
		ra, ok := src.(io.ReaderAt)
		if !ok {
			return errors.New("pdf: требуется io.ReaderAt")
		}
		return r.extractPDF(name, ra, size, depth)
	}
	return nil
}

// =============================================================================
// Архивные экстракторы
// =============================================================================

func (r *Runner) extractZip(name string, ra io.ReaderAt, size int64, depth int) error {
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		err = r.processEntryReader(name+"/"+f.Name, rc, depth+1)
		rc.Close()
		if err != nil {
			continue
		}
	}
	return nil
}

func (r *Runner) extractTar(name string, rd io.Reader, depth int) error {
	tr := tar.NewReader(rd)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.FileInfo().IsDir() {
			continue
		}
		if err := r.processEntryReader(name+"/"+h.Name, tr, depth+1); err != nil {
			continue
		}
	}
}

func (r *Runner) extractGz(name string, rd io.Reader, depth int) error {
	gr, err := gzip.NewReader(rd)
	if err != nil {
		return err
	}
	defer gr.Close()
	return r.processEntryReader(stripExt(name, ".gz", ".tgz"), gr, depth+1)
}

func (r *Runner) extractBz2(name string, rd io.Reader, depth int) error {
	br := bzip2.NewReader(rd)
	return r.processEntryReader(stripExt(name, ".bz2", ".tbz2", ".tbz"), br, depth+1)
}

func (r *Runner) extractXz(name string, rd io.Reader, depth int) error {
	xr, err := xz.NewReader(rd)
	if err != nil {
		return err
	}
	return r.processEntryReader(stripExt(name, ".xz", ".txz"), xr, depth+1)
}

func (r *Runner) extractZstd(name string, rd io.Reader, depth int) error {
	zd, err := zstd.NewReader(rd)
	if err != nil {
		return err
	}
	defer zd.Close()
	return r.processEntryReader(stripExt(name, ".zst", ".zstd"), zd, depth+1)
}

func (r *Runner) extract7z(name string, ra io.ReaderAt, size int64, depth int) error {
	zr, err := sevenzip.NewReader(ra, size)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		err = r.processEntryReader(name+"/"+f.Name, rc, depth+1)
		rc.Close()
		if err != nil {
			continue
		}
	}
	return nil
}

func (r *Runner) extractPDF(name string, ra io.ReaderAt, size int64, depth int) (retErr error) {
	// ledongthuc/pdf на некоторых битых файлах паникует - ловим recover
	defer func() {
		if rec := recover(); rec != nil {
			retErr = fmt.Errorf("pdf panic: %v", rec)
		}
	}()

	pr, err := pdf.NewReader(ra, size)
	if err != nil {
		return err
	}
	// Каждая страница отдельным Job — не копим весь PDF в RAM (важно на TB-объёмах).
	for i := 1; i <= pr.NumPage(); i++ {
		func() {
			defer func() { _ = recover() }() // одна страница не должна валить весь PDF
			page := pr.Page(i)
			if page.V.IsNull() {
				return
			}
			text, err := page.GetPlainText(nil)
			if err != nil || text == "" {
				return
			}
			r.processData(fmt.Sprintf("%s:page%d", name, i), []byte(text), depth+1)
		}()
	}
	return nil
}

func (r *Runner) extractRar(name string, rd io.Reader, depth int) error {
	rr, err := rardecode.NewReader(rd)
	if err != nil {
		return err
	}
	for {
		h, err := rr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.IsDir {
			continue
		}
		if err := r.processEntryReader(name+"/"+h.Name, rr, depth+1); err != nil {
			continue
		}
	}
}

// =============================================================================
// Утилиты
// =============================================================================

func readUpTo(r io.Reader, limit int64) ([]byte, bool, error) {
	var buf bytes.Buffer
	lr := &io.LimitedReader{R: r, N: limit + 1}
	if _, err := io.Copy(&buf, lr); err != nil {
		return nil, false, err
	}
	data := buf.Bytes()
	return data, int64(len(data)) > limit, nil
}

func formatNeedsReaderAt(format Format) bool {
	switch format {
	case FormatZip, FormatSevenZip, FormatPDF:
		return true
	default:
		return false
	}
}

func spoolReaderAt(r io.Reader) (*os.File, int64, func(), error) {
	tmp, err := os.CreateTemp("", "parser-entry-*")
	if err != nil {
		return nil, 0, nil, err
	}
	cleanup := func() {
		name := tmp.Name()
		tmp.Close()
		os.Remove(name)
	}

	lr := &io.LimitedReader{R: r, N: maxEntrySize + 1}
	n, err := io.CopyBuffer(tmp, lr, make([]byte, 1<<20))
	if err != nil {
		cleanup()
		return nil, 0, nil, err
	}
	if n > maxEntrySize {
		cleanup()
		return nil, 0, nil, errors.New("entry exceeds maxEntrySize")
	}
	return tmp, n, cleanup, nil
}

func stripExt(name string, exts ...string) string {
	lower := strings.ToLower(name)
	for _, e := range exts {
		if strings.HasSuffix(lower, e) {
			return name[:len(name)-len(e)]
		}
	}
	return name + ".inflated"
}

func humanBytes(n uint64) string {
	const k = 1024
	if n < k {
		return fmt.Sprintf("%d Б", n)
	}
	div, exp := uint64(k), 0
	for n2 := n / k; n2 >= k; n2 /= k {
		div *= k
		exp++
	}
	units := []string{"КБ", "МБ", "ГБ", "ТБ", "ПБ"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.2f %s", float64(n)/float64(div), units[exp])
}

func reportProgress(s *State, done <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			if s.invalidWriter != nil {
				fmt.Fprintf(os.Stderr,
					"\r[прогресс] файлы=%d архивы=%d entries=%d объём=%s ключи=%d сиды=%d invalid=%d heap=%s   ",
					s.filesScanned.Load(),
					s.archivesOpened.Load(),
					s.entriesScanned.Load(),
					humanBytes(s.bytesScanned.Load()),
					s.keyCount(),
					s.seedCount(),
					s.invalidLines.Load(),
					humanBytes(mem.HeapAlloc))
			} else {
				fmt.Fprintf(os.Stderr,
					"\r[прогресс] файлы=%d архивы=%d entries=%d объём=%s ключи=%d сиды=%d heap=%s   ",
					s.filesScanned.Load(),
					s.archivesOpened.Load(),
					s.entriesScanned.Load(),
					humanBytes(s.bytesScanned.Load()),
					s.keyCount(),
					s.seedCount(),
					humanBytes(mem.HeapAlloc))
			}
		}
	}
}

func readPathFromConsole() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Путь к директории или файлу для сканирования: ")
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	// Срезаем BOM (PowerShell может прислать ввод с UTF-8 BOM)
	line = strings.TrimPrefix(line, string([]byte{0xEF, 0xBB, 0xBF}))
	p := strings.TrimSpace(line)
	p = strings.Trim(p, `"'`)
	if p == "" {
		return "", errors.New("путь не указан")
	}
	return p, nil
}

// =============================================================================
// main
// =============================================================================

func main() {
	log.SetFlags(0)

	var keysPath, seedsPath, invalidPath string
	var workersFlag int
	fs := flag.NewFlagSet("parser", flag.ExitOnError)
	fs.IntVar(&workersFlag, "workers", 0, "number of parallel workers (default: auto, capped at 16)")
	fs.StringVar(&keysPath, "keys", "privates.txt", "путь к файлу с найденными ключами")
	fs.StringVar(&seedsPath, "seeds", "seeds.txt", "путь к файлу с найденными сид-фразами")
	fs.StringVar(&invalidPath, "invalid", "", "если задан — построчно пишет в файл строки БЕЗ ключей/сидов")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Использование: %s [флаги] <путь>\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Флаги:")
		fs.PrintDefaults()
	}
	_ = fs.Parse(os.Args[1:])

	var root string
	if args := fs.Args(); len(args) >= 1 {
		root = args[0]
	} else {
		p, err := readPathFromConsole()
		if err != nil {
			log.Fatalf("%v", err)
		}
		root = p
	}

	fi, err := os.Stat(root)
	if err != nil {
		log.Fatalf("доступ к %q: %v", root, err)
	}

	numCPU := runtime.NumCPU()
	numWorkers := numCPU
	if numWorkers < 4 {
		numWorkers = 4
	}
	if numWorkers > 16 {
		numWorkers = 16
	}
	if workersFlag > 0 {
		numWorkers = workersFlag
	}
	fmt.Printf("CPU=%d, воркеров=%d\n", numCPU, numWorkers)
	fmt.Printf("Цель: %s\n", root)
	if invalidPath != "" {
		fmt.Printf("Результаты: %s, %s, %s (инверсия)\n", keysPath, seedsPath, invalidPath)
	} else {
		fmt.Printf("Результаты: %s, %s\n", keysPath, seedsPath)
	}
	fmt.Println()

	state, err := newState(keysPath, seedsPath, invalidPath)
	if err != nil {
		log.Fatalf("создание файлов результата: %v", err)
	}
	defer state.Close()
	runner := newRunner(state, numWorkers)

	progressDone := make(chan struct{})
	go reportProgress(state, progressDone)

	start := time.Now()

	if fi.IsDir() {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			if shouldSkipFile(path) {
				return nil
			}
			runner.enqueue(Job{Path: path})
			return nil
		})
	} else {
		runner.enqueue(Job{Path: root})
	}

	runner.wait()
	close(progressDone)

	elapsed := time.Since(start)
	fmt.Fprintln(os.Stderr)
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Время:           %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Файлы:           %d\n", state.filesScanned.Load())
	fmt.Printf("Архивы:          %d\n", state.archivesOpened.Load())
	fmt.Printf("Entries:         %d\n", state.entriesScanned.Load())
	fmt.Printf("Прочитано:       %s\n", humanBytes(state.bytesScanned.Load()))
	fmt.Printf("Приватные ключи: %d\n", state.keyCount())
	fmt.Printf("Сид-фразы:       %d\n", state.seedCount())
	if invalidPath != "" {
		fmt.Printf("Невалид. строк:  %d\n", state.invalidLines.Load())
	}
	fmt.Println(strings.Repeat("=", 60))
	if invalidPath != "" {
		fmt.Printf("Сохранено: %s, %s, %s\n", keysPath, seedsPath, invalidPath)
	} else {
		fmt.Printf("Сохранено: %s, %s\n", keysPath, seedsPath)
	}
}
