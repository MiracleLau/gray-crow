package service

import (
	"fmt"
	"log"

	"gorm.io/gorm"

	"graycrow/internal/crypto"
	"graycrow/internal/model"
)

// Reencrypt 把仍由旧密钥加密的字段改用当前密钥重写。幂等：可反复调用，失败下次启动重试。
func Reencrypt(db *gorm.DB) (int, error) {
	total := 0
	targets := []struct {
		model any
		field string
	}{{&model.APIKey{}, "token_enc"}, {&model.Channel{}, "secret_enc"}, {&model.User{}, "totp_secret_enc"}}
	// 字段必须导出，否则 GORM 扫不进来（会拿到空值）
	rows := []struct {
		ID  uint   `gorm:"column:id"`
		Enc string `gorm:"column:enc"`
	}{}
	for _, item := range targets {
		rows = rows[:0]
		err := db.Model(item.model).Select("id", item.field+" as enc").Find(&rows).Error
		if err != nil {
			return total, fmt.Errorf("读取 %s 失败: %w", item.field, err)
		}
		for _, row := range rows {
			if row.Enc == "" || (!crypto.RotationPending(row.Enc) && crypto.EncryptedByCurrent(row.Enc)) {
				continue
			}
			plain, err := crypto.Decrypt(row.Enc)
			if err != nil {
				// 旧密钥没提供时解不开，只提示不动数据
				return total, fmt.Errorf("%s id=%d 解密失败（MASTER_KEY_OLD 是否缺失？）: %w", item.field, row.ID, err)
			}
			enc, err := crypto.Encrypt(plain)
			if err != nil {
				return total, err
			}
			if err := db.Model(item.model).Where("id = ?", row.ID).Update(item.field, enc).Error; err != nil {
				return total, err
			}
			total++
		}
	}
	if total > 0 {
		log.Printf("密钥轮换：已用当前 MASTER_KEY 重加密 %d 个字段", total)
	}
	return total, nil
}
