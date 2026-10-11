// Package dnssec は DNSSEC 公開鍵 (DNSKEY RDATA) の表示用情報を算出する。
//
// key tag は RFC 4034 Appendix B、RSA 公開鍵の鍵長は RFC 3110 の形式に従う。
// 対応アルゴリズムは root zone で使われ得るもの (RSA / ECDSA / EdDSA) に限る。
package dnssec

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/bits"
	"strconv"
	"strings"
)

// プロトコル番号。RFC 4034 では DNSKEY は常に 3。
const dnskeyProtocol = 3

// SEP (Secure Entry Point) ビット。RFC 4034 3.1.2。
// 鍵署名鍵 (KSK) はこのビットを立てて区別する。
const flagSEP = 0x0001

// ロール。通知・Web 表示では小文字で表示する。
const (
	roleKSK = "ksk"
	roleZSK = "zsk"
)

// Algorithm は DNSSEC のアルゴリズム番号。
type Algorithm uint8

// 対応アルゴリズム (IANA DNSSEC Number 5-16 のうち root zone で使われ得るもの)。
const (
	AlgRSASHA1      Algorithm = 5
	AlgRSASHA1NSEC3 Algorithm = 7
	AlgRSASHA256    Algorithm = 8
	AlgRSASHA512    Algorithm = 10
	AlgECDSAP256    Algorithm = 13
	AlgECDSAP384    Algorithm = 14
	AlgEd25519      Algorithm = 15
	AlgEd448        Algorithm = 16
)

// algorithmNames はアルゴリズム番号の表示名。
var algorithmNames = map[Algorithm]string{
	AlgRSASHA1:      "rsasha1",
	AlgRSASHA1NSEC3: "rsasha1-nsec3",
	AlgRSASHA256:    "rsasha256",
	AlgRSASHA512:    "rsasha512",
	AlgECDSAP256:    "ecdsap256",
	AlgECDSAP384:    "ecdsap384",
	AlgEd25519:      "ed25519",
	AlgEd448:        "ed448",
}

// Name はアルゴリズムの表示名を返す。未知の番号は 10 進表記を返す。
func (a Algorithm) Name() string {
	if name, ok := algorithmNames[a]; ok {
		return name
	}
	return fmt.Sprintf("%d", uint8(a))
}

// ecdsaKeySizes は ECDSA / EdDSA の公開鍵バイト長と表示用の鍵サイズ。
var ecdsaKeySizes = map[Algorithm]struct {
	bytes int
	size  string
}{
	AlgECDSAP256: {64, "p256"},
	AlgECDSAP384: {96, "p384"},
	AlgEd25519:   {32, "ed25519"},
	AlgEd448:     {57, "ed448"},
}

// KeyInfo は DNSKEY RDATA から算出した表示用の情報。
type KeyInfo struct {
	Flags     uint16    // DNSKEY の flags フィールド
	Protocol  uint8     // プロトコル番号 (通常 3)
	Algorithm Algorithm // アルゴリズム番号
	KeyTag    uint16    // RFC 4034 Appendix B の key tag
	KeySize   string    // "2048b" (RSA) / "p256" / "ed25519" など。不明なら空
}

// Role は鍵のロール (SEP ビットの有無による KSK / ZSK の別) を返す。
func (k KeyInfo) Role() string {
	if k.Flags&flagSEP != 0 {
		return roleKSK
	}
	return roleZSK
}

// Annotation は通知・Web 表示に付ける注記を返す。
// 例: "key tag = 20326 (ksk), size = 2048b"
// 鍵長が算出できない場合は size を省く。
func (k KeyInfo) Annotation() string {
	s := fmt.Sprintf("key tag = %d (%s)", k.KeyTag, k.Role())
	if k.KeySize != "" {
		s += ", size = " + k.KeySize
	}
	return s
}

// ParseDNSKEY は DNSKEY / CDNSKEY の RDATA から KeyInfo を算出する。
// RDATA 形式: FLAGS PROTOCOL ALGORITHM PUBLIC-KEY
//
// 以下の場合は nil を返す (呼び出し側は注記を付けない)。
//   - フィールドが不足している、または数値・base64 として解釈できない
//   - プロトコル番号が 3 以外 (DNSKEY として不正)
//   - 公開鍵が空
func ParseDNSKEY(rdata string) *KeyInfo {
	fields := strings.Fields(rdata)
	if len(fields) < 4 {
		return nil
	}

	flags, err := parseUint(fields[0], 16)
	if err != nil {
		return nil
	}
	protocol, err := parseUint(fields[1], 8)
	if err != nil || protocol != dnskeyProtocol {
		return nil
	}
	alg, err := parseUint(fields[2], 8)
	if err != nil {
		return nil
	}
	// base64 は標準エンコーディングのみ受け付ける (空白を含む RDATA は弾く)。
	key, err := base64.StdEncoding.DecodeString(fields[3])
	if err != nil || len(key) == 0 {
		return nil
	}

	algorithm := Algorithm(alg)
	return &KeyInfo{
		Flags:     uint16(flags),
		Protocol:  uint8(protocol),
		Algorithm: algorithm,
		KeyTag:    keyTag(flags, protocol, alg, key),
		KeySize:   keySize(algorithm, key),
	}
}

// parseUint は 10 進数のフィールドを bitSize ビットに収まる範囲で解釈する。
func parseUint(s string, bitSize int) (uint64, error) {
	return strconv.ParseUint(s, 10, bitSize)
}

// keyTag は RFC 4034 Appendix B の key tag を算出する。
// DNSKEY RDATA 全体 (flags / protocol / algorithm / public key) を対象とする。
func keyTag(flags, protocol, algorithm uint64, key []byte) uint16 {
	// RDATA = flags(2) protocol(1) algorithm(1) public key
	rdata := make([]byte, 0, 4+len(key))
	rdata = binary.BigEndian.AppendUint16(rdata, uint16(flags))
	rdata = append(rdata, byte(protocol), byte(algorithm))
	rdata = append(rdata, key...)

	var ac uint32
	for i, b := range rdata {
		if i%2 == 0 {
			ac += uint32(b) << 8
		} else {
			ac += uint32(b)
		}
	}
	ac += (ac >> 16) & 0xFFFF
	return uint16(ac & 0xFFFF)
}

// keySize は公開鍵の表示用サイズを返す。算出できない場合は空文字列。
func keySize(algorithm Algorithm, key []byte) string {
	if size, ok := ecdsaKeySizes[algorithm]; ok {
		// ECDSA / EdDSA は鍵長が固定。想定と異なる場合は算出しない。
		if len(key) == size.bytes {
			return size.size
		}
		return ""
	}
	if isRSA(algorithm) {
		return rsaKeySize(key)
	}
	return ""
}

// isRSA は RFC 3110 形式の RSA 公開鍵を持つアルゴリズムかを返す。
func isRSA(algorithm Algorithm) bool {
	switch algorithm {
	case AlgRSASHA1, AlgRSASHA1NSEC3, AlgRSASHA256, AlgRSASHA512:
		return true
	default:
		return false
	}
}

// rsaKeySize は RFC 3110 形式の RSA 公開鍵から modulus のビット長を返す。
// 形式: exponent length (1 or 3 bytes) | exponent | modulus
// modulus 先頭の 0 は符号回避の詰め物として除いて数える。
func rsaKeySize(key []byte) string {
	if len(key) < 1 {
		return ""
	}
	var expLen, offset int
	if key[0] == 0 {
		// 先頭 0 は「続く 2 バイトが exponent length」を意味する。
		if len(key) < 3 {
			return ""
		}
		expLen = int(binary.BigEndian.Uint16(key[1:3]))
		offset = 3
	} else {
		expLen = int(key[0])
		offset = 1
	}
	// exponent と modulus の長さが公開鍵の範囲を超える場合は算出しない。
	if offset+expLen >= len(key) {
		return ""
	}
	modulus := key[offset+expLen:]
	if len(modulus) == 0 {
		return ""
	}
	// 先頭の 0 バイト (符号回避の詰め物) を除いた有効ビット長を数える。
	n := len(modulus) * 8
	for _, b := range modulus {
		if b != 0 {
			return fmt.Sprintf("%db", n-bits.LeadingZeros8(b))
		}
		n -= 8
	}
	return ""
}
