package dnssec

import (
	"encoding/base64"
	"testing"
)

// b64 は DNSKEY の公開鍵フィールド (base64) を作る。
func b64(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

// rsaKey は exponent 65537、modulus の先頭バイトが 0xff の RSA 公開鍵
// (RFC 3110 形式) を base64 で返す。modulus は modulusLen バイト。
func rsaKey(modulusLen int) string {
	return rsaKeyModulus([]byte{0x01, 0x00, 0x01}, append([]byte{0xff}, make([]byte, modulusLen-1)...))
}

// rsaKeyZeroPadded は modulus の先頭に符号回避の 0x00 を付けた RSA 公開鍵を返す。
// modulus 全体 (先頭 0 を含む) は modulusLen バイトで、有効ビット長は modulusLen-1 バイト分。
func rsaKeyZeroPadded(modulusLen int) string {
	return rsaKeyModulus([]byte{0x01, 0x00, 0x01}, append([]byte{0x00, 0xff}, make([]byte, modulusLen-2)...))
}

// rsaKeyWithExponent は exponent と 20 バイトの modulus を持つ RSA 公開鍵を返す。
// exponent の先頭バイトが 0 の場合は 2 バイト長形式になる (RFC 3110)。
func rsaKeyWithExponent(exponent []byte) string {
	return rsaKeyModulus(exponent, append([]byte{0xff}, make([]byte, 19)...))
}

// rsaKeyModulus は exponent と modulus から RSA 公開鍵を組み立てて base64 で返す。
func rsaKeyModulus(exponent, modulus []byte) string {
	key := make([]byte, 0, 3+len(exponent)+len(modulus))
	if len(exponent) < 256 {
		key = append(key, byte(len(exponent)))
	} else {
		key = append(key, 0x00, byte(len(exponent)>>8), byte(len(exponent)))
	}
	key = append(key, exponent...)
	key = append(key, modulus...)
	return b64(key)
}

// 実データ (IANA root zone) の DNSKEY。key tag は IANA が公開している値
// (KSK 20326 / KSK 38696 / ZSK 8763) と一致する。
const (
	rootZSK256   = "256 3 8 AwEAAcorNxNc9t0ue1ZenPjVMRujlQ2YLBJ0pR/4XMjHby9vr03cRxFSKnWpxeg5PL384D9VeqdS1bjrGyTsHtoagzI3NiXFd8sjHMOnZxu5pp/+UBIg7tasYgVMMdUiM1Yu/c+t30d1Qw/v6NYLNnOnywTu6HuUh7Y1sZRq/GZcrxLENXHQoyKpOrOr8R3+qd6wK69IKB80ZBZtkX9rdEqKJk+yYXjEZzZ0zkAFthmNgCreAoU9PVlU66dyOsaLjf3wKUlUm9SNOKs+tK6LuSmRbK6+1T/tPaRaMBSEuv2G6vXfnJ1uyMbGvtpRn/YDcvTjrp6DzE6XyF44QdwJ3oKx4ek="
	rootKSK20326 = "257 3 8 AwEAAaz/tAm8yTn4Mfeh5eyI96WSVexTBAvkMgJzkKTOiW1vkIbzxeF3+/4RgWOq7HrxRixHlFlExOLAJr5emLvN7SWXgnLh4+B5xQlNVz8Og8kvArMtNROxVQuCaSnIDdD5LKyWbRd2n9WGe2R8PzgCmr3EgVLrjyBxWezF0jLHwVN8efS3rCj/EWgvIWgb9tarpVUDK/b58Da+sqqls3eNbuv7pr+eoZG+SrDK6nWeL3c6H5Apxz7LjVc1uTIdsIXxuOLYA4/ilBmSVIzuDWfdRUfhHdY6+cn8HFRm+2hM8AnXGXws9555KrUB5qihylGa8subX2Nn6UwNR1AkUTV74bU="
	rootKSK38696 = "257 3 8 AwEAAa96jeuknZlaeSrvyAJj6ZHv28hhOKkx3rLGXVaC6rXTsDc449/cidltpkyGwCJNnOAlFNKF2jBosZBU5eeHspaQWOmOElZsjICMQMC3aeHbGiShvZsx4wMYSjH8e7Vrhbu6irwCzVBApESjbUdpWWmEnhathWu1jo+siFUiRAAxm9qyJNg/wOZqqzL/dL/q8PkcRU5oUKEpUge71M3ej2/7CPqpdVwuMoTvoB+ZOT4YeGyxMvHmbrxlFzGOHOijtzN+u1TQNatX2XBuzZNQ1K+s2CXkPIZo7s6JgZyvaBevYtxPvYLw4z9mR7K2vaF18UYH9Z9GNUUeayffKC73PYc="
	// X (旧 Twitter) の通知例に現れた DNSKEY。
	zsk57780 = "256 3 8 AwEAAeCYD6Z7WWKVLeuWgowKP+3g+Gs1cnLKq7a3CaQxQpv8bfuFVI0WnG33qaSH/Mw9IBgifrdzf4XY/DQLnyBJ9MfaOyAWuEaEmYJ+GQPiwVVfstGwSA1McfFJUttTgq2Huu74KARhtA8wPo/N3XcyYQtNhz+qCM5NBb3ecx/naw6sYab9LxS6f2cU0q03++BP5Ks0Uef8WJCa/1izCYE+vMkwoltV+tENa3hpXiZ7jle/xdgaZrPi5ZGmyLVI34g1XVYrNlsCCTmNvFQIfzW5STFQFsQpizczyFn9r3LzSxxPCNwdlCG84bER0BmdwqbF6Tanv+FxMOavrahkj4wIy5k="
)

// ECDSA / EdDSA の固定長鍵。key tag は ldns-key2ds -n -2 および RFC 4034
// Appendix B の算出方法と一致することを確認した値 (alg 13 は ldns と同じ鍵)。
const (
	ecdsaP256Key = "mdsswUyr3DPW132mOi8V9xESWE8jTo0dxCjjnopKl+GqJxpVXckHAeF+KkxLbxILfDLUT0rAK9iUzy1L53eKGQ=="
	// /wAAAA... は先頭バイトのみ 0xff の固定長鍵 (P-384 / Ed25519 / Ed448)。
	ecdsaP384Key = "/wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	ed25519Key   = "/wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	ed448Key     = "/wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
)

func TestParseDNSKEY(t *testing.T) {
	tests := []struct {
		name     string
		rdata    string
		wantTag  uint16
		wantRole string
		wantSize string
	}{
		{"root ZSK", rootZSK256, 8763, "zsk", "2048b"},
		{"root KSK 20326", rootKSK20326, 20326, "ksk", "2048b"},
		{"root KSK 38696", rootKSK38696, 38696, "ksk", "2048b"},
		{"X 通知例の ZSK", zsk57780, 57780, "zsk", "2048b"},
		{"ECDSA P-256", "257 3 13 " + ecdsaP256Key, 2371, "ksk", "p256"},
		{"ECDSA P-384", "257 3 14 " + ecdsaP384Key, 784, "ksk", "p384"},
		{"Ed25519", "257 3 15 " + ed25519Key, 785, "ksk", "ed25519"},
		{"Ed448", "257 3 16 " + ed448Key, 786, "ksk", "ed448"},
		// SEP ビットが無ければ ZSK。
		{"flags 256 は ZSK", "256 3 13 " + ecdsaP256Key, 2370, "zsk", "p256"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ParseDNSKEY(tt.rdata)
			if info == nil {
				t.Fatalf("ParseDNSKEY(%q) = nil, want non-nil", tt.rdata)
			}
			if info.KeyTag != tt.wantTag {
				t.Errorf("KeyTag = %d, want %d", info.KeyTag, tt.wantTag)
			}
			if got := info.Role(); got != tt.wantRole {
				t.Errorf("Role() = %q, want %q", got, tt.wantRole)
			}
			if info.KeySize != tt.wantSize {
				t.Errorf("KeySize = %q, want %q", info.KeySize, tt.wantSize)
			}
		})
	}
}

func TestParseDNSKEYInvalid(t *testing.T) {
	tests := []struct {
		name  string
		rdata string
	}{
		{"空", ""},
		{"フィールド不足 (key なし)", "256 3 8"},
		{"フィールド不足 (flags のみ)", "256"},
		{"flags が数値でない", "abc 3 8 AwEAAa=="},
		{"protocol が数値でない", "256 x 8 AwEAAa=="},
		{"algorithm が数値でない", "256 3 x AwEAAa=="},
		{"protocol が DNSKEY 以外", "256 4 8 AwEAAa=="},
		{"key が base64 でない", "256 3 8 !!!not-base64!!!"},
		// 履歴が RDATA を切り詰めて保存していた場合など、不完全な key は算出しない。
		{"key が空", "256 3 8 "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if info := ParseDNSKEY(tt.rdata); info != nil {
				t.Errorf("ParseDNSKEY(%q) = %+v, want nil", tt.rdata, info)
			}
		})
	}
}

func TestParseDNSKEYUnknownAlgorithm(t *testing.T) {
	// 未対応アルゴリズムでも key tag と role は算出できる。size のみ不明。
	info := ParseDNSKEY("257 3 253 " + ecdsaP256Key)
	if info == nil {
		t.Fatal("ParseDNSKEY() = nil, want non-nil")
	}
	if info.KeyTag != 2611 {
		t.Errorf("KeyTag = %d, want 2611", info.KeyTag)
	}
	if info.KeySize != "" {
		t.Errorf("KeySize = %q, want \"\"", info.KeySize)
	}
	if got := info.Annotation(); got != "key tag = 2611 (ksk)" {
		t.Errorf("Annotation() = %q, want %q", got, "key tag = 2611 (ksk)")
	}
}

func TestParseDNSKEYRSASize(t *testing.T) {
	// RSA の公開鍵は exponent length | exponent | modulus。modulus の有効ビット長を返す。
	tests := []struct {
		name     string
		key      string
		wantSize string
	}{
		{"1024bit", rsaKey(128), "1024b"},
		{"2048bit", rsaKey(256), "2048b"},
		{"4096bit", rsaKey(512), "4096b"},
		// 先頭バイトが 0 の modulus は符号回避の 0 詰めとして差し引く (RFC 3110)。
		{"2048bit (先頭 0 詰め)", rsaKeyZeroPadded(257), "2048b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rdata := "256 3 8 " + tt.key
			info := ParseDNSKEY(rdata)
			if info == nil {
				t.Fatalf("ParseDNSKEY(%q) = nil, want non-nil", rdata)
			}
			if info.KeySize != tt.wantSize {
				t.Errorf("KeySize = %q, want %q", info.KeySize, tt.wantSize)
			}
		})
	}
}

func TestParseDNSKEYRSAExponentLength(t *testing.T) {
	// exponent length が 1 バイト (len < 256) と 2 バイト (先頭 0x00 + uint16) の両形式。
	short := rsaKeyWithExponent([]byte{0x01, 0x00, 0x01})
	long := rsaKeyWithExponent(append([]byte{0x00, 0x01, 0x00}, make([]byte, 252)...))
	for name, key := range map[string]string{"1バイト形式": short, "2バイト形式": long} {
		t.Run(name, func(t *testing.T) {
			info := ParseDNSKEY("256 3 8 " + key)
			if info == nil {
				t.Fatal("ParseDNSKEY() = nil, want non-nil")
			}
			if want := "160b"; info.KeySize != want {
				t.Errorf("KeySize = %q, want %q", info.KeySize, want)
			}
		})
	}
}

func TestParseDNSKEYECDSASizeMismatch(t *testing.T) {
	// P-256 の鍵長 (64 バイト) と異なる場合は size を出さない。
	info := ParseDNSKEY("257 3 13 " + b64(make([]byte, 32)))
	if info == nil {
		t.Fatal("ParseDNSKEY() = nil, want non-nil")
	}
	if info.KeySize != "" {
		t.Errorf("KeySize = %q, want \"\"", info.KeySize)
	}
}

func TestKeyInfoAnnotation(t *testing.T) {
	tests := []struct {
		name string
		info KeyInfo
		want string
	}{
		{"ksk + size", KeyInfo{KeyTag: 20326, KeySize: "2048b", Flags: 257}, "key tag = 20326 (ksk), size = 2048b"},
		{"zsk + size", KeyInfo{KeyTag: 57780, KeySize: "2048b", Flags: 256}, "key tag = 57780 (zsk), size = 2048b"},
		{"size 不明", KeyInfo{KeyTag: 2611, Flags: 257}, "key tag = 2611 (ksk)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.Annotation(); got != tt.want {
				t.Errorf("Annotation() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAlgorithmName(t *testing.T) {
	tests := map[Algorithm]string{
		5: "rsasha1", 7: "rsasha1-nsec3", 8: "rsasha256", 10: "rsasha512",
		13: "ecdsap256", 14: "ecdsap384", 15: "ed25519", 16: "ed448",
		253: "253", 0: "0",
	}
	for alg, want := range tests {
		if got := alg.Name(); got != want {
			t.Errorf("Algorithm(%d).Name() = %q, want %q", alg, got, want)
		}
	}
}

func TestRSAKeySizeMalformed(t *testing.T) {
	// 壊れた RSA 公開鍵では size を出さない (key tag は算出できる)。
	tests := []struct {
		name string
		key  []byte
	}{
		{"exponent length の 2 バイト目までしかない", []byte{0x00, 0x01}},
		{"exponent が足りない", []byte{0x05, 0x01, 0x00, 0x01}},
		{"modulus が無い", []byte{0x03, 0x01, 0x00, 0x01}},
		{"modulus が全て 0", []byte{0x03, 0x01, 0x00, 0x01, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rsaKeySize(tt.key); got != "" {
				t.Errorf("rsaKeySize(% x) = %q, want \"\"", tt.key, got)
			}
		})
	}
}
