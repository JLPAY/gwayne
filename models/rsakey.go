package models

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TableNameRsaKey = "rsa_key"

	RsaKeyStatusCurrent  = "current"  // 当前签发密钥
	RsaKeyStatusPrevious = "previous" // 轮换前的旧密钥，仅在宽限窗口内用于验签
)

// rsa_key 轮换互斥锁名（MySQL 命名咨询锁，防止多副本并发轮换）
const rsaKeyRotateLockName = "gwayne_rsa_key_rotate"

// RsaKey JWT 签名 RSA 密钥（DB 密钥模式）
// 多副本部署时所有副本读取同一行，保证令牌跨副本可验证；
// 轮换时旧 current 降级为 previous，在宽限窗口内仍可验签，避免全员掉线。
type RsaKey struct {
	Id int64 `gorm:"primaryKey;autoIncrement" json:"id,omitempty"`
	// Kid 公钥指纹（SHA256 DER 的 base64url），写入 JWT kid 头
	Kid string `gorm:"size:64;uniqueIndex" json:"kid"`
	// PrivateKey PKCS1 PEM；PublicKey PKIX PEM。永不允许序列化到 JSON
	PrivateKey string `gorm:"type:text" json:"-"`
	PublicKey  string `gorm:"type:text" json:"-"`
	// Status current | previous
	Status string `gorm:"size:16;index" json:"status"`
	// RotateTime 该密钥被降级为 previous 的时间（current 恒为空）
	RotateTime *time.Time `gorm:"null" json:"rotateTime,omitempty"`
	CreateTime *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"`
	UpdateTime *time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"`
}

// TableName 表名
func (*RsaKey) TableName() string {
	return TableNameRsaKey
}

// LoadRsaKeys 读取 current 与 previous 密钥；已超出宽限窗口的 previous 会被顺带清理。
// 任一不存在时返回 nil，不视为错误。
func LoadRsaKeys(graceWindow time.Duration) (current *RsaKey, previous *RsaKey, err error) {
	var keys []RsaKey
	if err = DB.Where("status IN ?", []string{RsaKeyStatusCurrent, RsaKeyStatusPrevious}).
		Order("id").Find(&keys).Error; err != nil {
		return nil, nil, err
	}

	now := time.Now()
	for i := range keys {
		k := &keys[i]
		switch k.Status {
		case RsaKeyStatusCurrent:
			if current == nil {
				current = k
			}
		case RsaKeyStatusPrevious:
			// 超出宽限窗口的旧密钥直接清理（其签发的令牌也已全部自然过期）
			if k.RotateTime != nil && now.Sub(*k.RotateTime) > graceWindow {
				if err := DB.Delete(&RsaKey{}, k.Id).Error; err != nil {
					return nil, nil, err
				}
				continue
			}
			if previous == nil {
				previous = k
			}
		}
	}
	return current, previous, nil
}

// RotateRsaKey 事务化轮换：
//  1. MySQL 咨询锁 + 行锁，防止多副本并发轮换产生双 current
//  2. 旧 current 降级为 previous（记录 rotate_time，作为宽限窗口起点）
//  3. 更早的 previous 直接删除（其窗口随时间自然结束）
//  4. 写入 generate() 生成的新 current
//
// generate 由调用方（pkg/rsakey）注入，models 层不接触加密细节。
func RotateRsaKey(generate func() (kid, privatePEM, publicPEM string, err error)) (*RsaKey, error) {
	var created *RsaKey
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 咨询锁：表为空时行锁无效，靠它挡住多副本首启并发播种
		if err := tx.Exec("SELECT GET_LOCK(?, 30)", rsaKeyRotateLockName).Error; err != nil {
			return err
		}
		defer tx.Exec("SELECT RELEASE_LOCK(?)", rsaKeyRotateLockName)

		// 行锁现存密钥，串行化并发轮换
		var keys []RsaKey
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status IN ?", []string{RsaKeyStatusCurrent, RsaKeyStatusPrevious}).
			Find(&keys).Error; err != nil {
			return err
		}

		now := time.Now()
		for i := range keys {
			k := &keys[i]
			switch k.Status {
			case RsaKeyStatusCurrent:
				if err := tx.Model(&RsaKey{}).Where("id = ?", k.Id).
					Updates(map[string]interface{}{
						"status":      RsaKeyStatusPrevious,
						"rotate_time": now,
					}).Error; err != nil {
					return err
				}
			case RsaKeyStatusPrevious:
				// 旧 previous 立即删除：其宽限窗口从本次轮换重新起算没有意义，
				// 且其签发的令牌最多再活一个 TokenLifeTime
				if err := tx.Delete(&RsaKey{}, k.Id).Error; err != nil {
					return err
				}
			}
		}

		kid, privatePEM, publicPEM, err := generate()
		if err != nil {
			return err
		}
		created = &RsaKey{
			Kid:        kid,
			PrivateKey: privatePEM,
			PublicKey:  publicPEM,
			Status:     RsaKeyStatusCurrent,
		}
		return tx.Create(created).Error
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// UpsertRsaKeyCurrent 将调用方给定的密钥对作为 current 写入库（file 密钥模式的同步信道）：
//  1. MySQL 咨询锁 + 行锁，串行化并发写入
//  2. 若库中 current 已是该 kid → 幂等成功（不重复降级）
//  3. 否则旧 current 降级为 previous（rotate_time=now），更早的 previous 删除
//  4. 写入给定密钥为新 current
//
// 用于 file 模式轮换后把新密钥广播到 DB，其他实例据此自动跟进。
func UpsertRsaKeyCurrent(kid, privatePEM, publicPEM string) (*RsaKey, error) {
	var created *RsaKey
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT GET_LOCK(?, 30)", rsaKeyRotateLockName).Error; err != nil {
			return err
		}
		defer tx.Exec("SELECT RELEASE_LOCK(?)", rsaKeyRotateLockName)

		// 幂等：current 已是该 kid，无需变更
		var cur RsaKey
		err := tx.Where("status = ?", RsaKeyStatusCurrent).Order("id").First(&cur).Error
		if err == nil {
			if cur.Kid == kid {
				created = &cur
				return nil
			}
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		// 行锁现存密钥，串行化并发变更
		var keys []RsaKey
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status IN ?", []string{RsaKeyStatusCurrent, RsaKeyStatusPrevious}).
			Find(&keys).Error; err != nil {
			return err
		}

		now := time.Now()
		for i := range keys {
			k := &keys[i]
			switch k.Status {
			case RsaKeyStatusCurrent:
				if err := tx.Model(&RsaKey{}).Where("id = ?", k.Id).
					Updates(map[string]interface{}{
						"status":      RsaKeyStatusPrevious,
						"rotate_time": now,
					}).Error; err != nil {
					return err
				}
			case RsaKeyStatusPrevious:
				if err := tx.Delete(&RsaKey{}, k.Id).Error; err != nil {
					return err
				}
			}
		}

		created = &RsaKey{
			Kid:        kid,
			PrivateKey: privatePEM,
			PublicKey:  publicPEM,
			Status:     RsaKeyStatusCurrent,
		}
		return tx.Create(created).Error
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}
