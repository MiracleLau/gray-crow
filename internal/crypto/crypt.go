package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
)

// key 是一次密钥轮换周期内的密钥：id 为短指纹，用于识别密文属于哪把密钥。
type key struct {
	id   string
	aead cipher.AEAD
	mac  []byte
}

var keys []key

// Init 用 MASTER_KEY 派生当前密钥；olds 为轮换期内的旧密钥（只解不加密）。
// 密文自带 key id 前缀，因此换密钥后可逐步重加密而不会读不出旧数据。
func Init(masterKey string, olds ...string) error {
	current, err := derive(masterKey)
	if err != nil {
		return err
	}
	keys = []key{current}
	for _, old := range olds {
		old = strings.TrimSpace(old)
		if old == "" {
			continue
		}
		k, err := derive(old)
		if err != nil {
			return err
		}
		if k.id != current.id {
			keys = append(keys, k)
		}
	}
	return nil
}

func derive(master string) (key, error) {
	sum := sha256.Sum256([]byte(master))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return key{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return key{}, err
	}
	macSum := sha256.Sum256([]byte("graycrow-mac:" + master))
	return key{id: hex.EncodeToString(sum[:3]), aead: aead, mac: macSum[:]}, nil
}

var (
	ErrNotInitialized = errors.New("加密模块未初始化")
	ErrNoKey          = errors.New("密文对应的密钥不存在")
)

func CurrentKeyID() string {
	if len(keys) == 0 {
		return ""
	}
	return keys[0].id
}

// RotationPending 判断密文是否明确由非当前密钥加密（带前缀且指纹不同）。
func RotationPending(enc string) bool {
	if enc == "" {
		return false
	}
	id, _, ok := split(enc)
	return ok && id != CurrentKeyID()
}

// EncryptedByCurrent 判断密文能否被当前密钥解开。
// 历史密文没有前缀，格式上认不出归属，只能实测（轮换只在配置了旧密钥时才跑，开销可接受）。
func EncryptedByCurrent(enc string) bool {
	if len(keys) == 0 || enc == "" {
		return false
	}
	if id, _, ok := split(enc); ok {
		return id == keys[0].id
	}
	_, err := open(keys[0], enc)
	return err == nil
}

func Encrypt(plain string) (string, error) {
	if len(keys) == 0 {
		return "", ErrNotInitialized
	}
	k := keys[0]
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := k.aead.Seal(nonce, nonce, []byte(plain), nil)
	return k.id + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

func Decrypt(enc string) (string, error) {
	if len(keys) == 0 {
		return "", ErrNotInitialized
	}
	candidates := keys
	if id, body, ok := split(enc); ok {
		enc = body
		candidates = nil
		for _, k := range keys {
			if k.id == id {
				candidates = []key{k}
				break
			}
		}
		if candidates == nil {
			return "", ErrNoKey
		}
	}
	for _, k := range candidates {
		if plain, err := open(k, enc); err == nil {
			return plain, nil
		} else if errors.Is(err, errTooShort) {
			return "", err
		}
	}
	return "", errors.New("解密失败：密钥不匹配或密文已损坏")
}

var errTooShort = errors.New("密文过短")

func open(k key, enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	ns := k.aead.NonceSize()
	if len(raw) < ns {
		return "", errTooShort
	}
	plain, err := k.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// split 解析 "<keyid>:<base64>"；无前缀（历史密文）返回 ok=false。
func split(enc string) (id, body string, ok bool) {
	id, body, found := strings.Cut(enc, ":")
	if !found || len(id) != 6 {
		return "", enc, false
	}
	if _, err := hex.DecodeString(id); err != nil {
		return "", enc, false
	}
	return id, body, true
}

// Sign 用当前密钥对数据做 HMAC，用于短时令牌（登录票据等）。
func Sign(data string) string {
	if len(keys) == 0 {
		return ""
	}
	m := hmac.New(sha256.New, keys[0].mac)
	m.Write([]byte(data))
	return keys[0].id + ":" + hex.EncodeToString(m.Sum(nil))
}

func VerifySign(data, sig string) bool {
	if len(keys) == 0 {
		return false
	}
	id, want, ok := strings.Cut(sig, ":")
	if !ok {
		return false
	}
	raw, err := hex.DecodeString(want)
	if err != nil {
		return false
	}
	for _, k := range keys {
		if k.id != id {
			continue
		}
		m := hmac.New(sha256.New, k.mac)
		m.Write([]byte(data))
		return hmac.Equal(raw, m.Sum(nil))
	}
	return false
}

// Hint 返回明文尾部 4 位用于展示；解密失败返回固定占位。
func Hint(enc string) string {
	plain, err := Decrypt(enc)
	if err != nil || plain == "" {
		return "****"
	}
	if len(plain) <= 4 {
		return plain
	}
	return "****" + plain[len(plain)-4:]
}
