package crypto

import "testing"

func TestKeyRotation(t *testing.T) {
	oldKey := "old-master-key-0123456789-abcdefghijkl"
	newKey := "new-master-key-9876543210-zyxwvutsrqpo"

	if err := Init(oldKey); err != nil {
		t.Fatal(err)
	}
	legacy, err := Encrypt("github_pat_secret")
	if err != nil {
		t.Fatal(err)
	}
	if RotationPending(legacy) {
		t.Fatal("当前密钥加密的密文不应标记为待轮换")
	}

	if err := Init(newKey, oldKey); err != nil {
		t.Fatal(err)
	}
	if RotationPending(legacy) != true {
		t.Fatal("换密钥后旧密文应标记为待轮换")
	}
	plain, err := Decrypt(legacy)
	if err != nil || plain != "github_pat_secret" {
		t.Fatalf("旧密钥应仍可解密: %q %v", plain, err)
	}

	fresh, err := Encrypt("github_pat_secret")
	if err != nil {
		t.Fatal(err)
	}
	if RotationPending(fresh) {
		t.Fatal("新密文不应待轮换")
	}

	if err := Init(newKey); err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(legacy); err == nil {
		t.Fatal("未提供旧密钥时应解密失败")
	}

	// 无前缀历史密文（P7 之前写入的库）仍要能解
	legacyBare := mustBare(t, legacy)
	if err := Init(oldKey); err != nil {
		t.Fatal(err)
	}
	bare := legacyBare
	if plain, err := Decrypt(bare); err != nil || plain != "github_pat_secret" {
		t.Fatalf("历史无前缀密文应可解密: %q %v", plain, err)
	}
}

func TestEncryptedByCurrentLegacy(t *testing.T) {
	if err := Init("legacy-current-key-0123456789-abcdefg"); err != nil {
		t.Fatal(err)
	}
	cur, err := Encrypt("token")
	if err != nil {
		t.Fatal(err)
	}
	_, bare, _ := split(cur)
	if !EncryptedByCurrent(bare) {
		t.Fatal("当前密钥写的无前缀密文应判定为可解")
	}

	if err := Init("another-key-0123456789-abcdefghijkl", "legacy-current-key-0123456789-abcdefg"); err != nil {
		t.Fatal(err)
	}
	if EncryptedByCurrent(bare) {
		t.Fatal("换成新密钥后，旧密钥写的无前缀密文不应判定为可解")
	}
	if !RotationPending(cur) || !RotationPending(bare) == false {
		t.Fatal("带前缀旧密文应待轮换；无前缀需靠 EncryptedByCurrent 判定")
	}
}

func TestSignTicket(t *testing.T) {
	if err := Init("sign-master-key-0123456789-abcdefghijkl"); err != nil {
		t.Fatal(err)
	}
	sig := Sign("1.1234567890")
	if !VerifySign("1.1234567890", sig) {
		t.Fatal("签名应验证通过")
	}
	if VerifySign("2.1234567890", sig) {
		t.Fatal("被篡改的数据不应通过")
	}
	if VerifySign("1.1234567890", "deadbeef") {
		t.Fatal("格式非法的签名不应通过")
	}
}

func mustBare(t *testing.T, enc string) string {
	t.Helper()
	_, body, ok := split(enc)
	if !ok {
		t.Fatal("期望带前缀密文")
	}
	return body
}
