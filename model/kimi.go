package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/kimi"
	"gorm.io/gorm"
)

// RotateKimiCredential updates every stored reference to the same rotating
// credential. Each complete key is compared byte-for-byte, preserving key order,
// disabled indexes and concurrent administrator edits.
func RotateKimiCredential(before, after kimi.Credential) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := tx.Select("id", "key").Where("type = ?", constant.ChannelTypeKimi).Order("id").Find(&channels).Error; err != nil {
			return err
		}
		found := false
		for _, ch := range channels {
			lines := strings.Split(ch.Key, "\n")
			changed := false
			for i, line := range lines {
				c, err := kimi.Parse(line)
				if err != nil || c.ID != before.ID || c.Region != before.Region {
					continue
				}
				if c.RefreshToken != before.RefreshToken || c.AuthType != "oauth" {
					return errors.New("Kimi credential changed during renewal")
				}
				// Keep each reference's label and persistent device identity.
				c.AccessToken = after.AccessToken
				c.RefreshToken = after.RefreshToken
				c.ExpiresAt = after.ExpiresAt
				c.TokenType = after.TokenType
				c.Scope = after.Scope
				lines[i] = c.Key()
				changed = true
				found = true
			}
			if !changed {
				continue
			}
			var current Channel
			if err := lockForUpdate(tx).Select("id", "key").First(&current, ch.Id).Error; err != nil {
				return err
			}
			if current.Key != ch.Key {
				return errors.New("Kimi channel changed during renewal")
			}
			if err := tx.Model(&Channel{}).Where("id = ?", ch.Id).Update("key", strings.Join(lines, "\n")).Error; err != nil {
				return err
			}
		}
		if !found {
			return errors.New("Kimi credential is no longer stored")
		}
		return nil
	})
}

// CreateKimiAuthFlow serializes enrollment creation against session invalidation
// and bounds active flows per dashboard session, including concurrently started flows.
func CreateKimiAuthFlow(input AuthFlowCreate, identity AuthSessionIdentity) (string, error) {
	var handle string
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&AuthFlow{}).Where("purpose = ? AND user_id = ? AND session_id = ? AND consumed_at IS NULL AND expires_at > ?", input.Purpose, identity.UserID, identity.SessionID, time.Now()).Count(&active).Error; err != nil {
			return err
		}
		if active >= 3 {
			return errors.New("Too many active Kimi authorizations; cancel one or wait for expiry")
		}
		var err error
		handle, _, err = createAuthFlowWithTx(tx, input)
		return err
	})
	return handle, err
}
