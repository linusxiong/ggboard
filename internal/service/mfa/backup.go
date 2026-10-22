package mfasvc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
)

// Backup codes are 10 characters of lower-case Crockford base32 (50 bits) shown as "xxxxx-xxxxx". Only
// SHA-256 hashes of the unused codes are stored; a code is removed when it is used.
const backupAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// newBackupCodes returns BackupCodeCount fresh codes and their hashes (same order).
func newBackupCodes() (codes, hashes []string) {
	buf := make([]byte, 10)
	for range BackupCodeCount {
		if _, err := rand.Read(buf); err != nil {
			panic("mfa: crypto/rand failed: " + err.Error())
		}
		raw := make([]byte, len(buf))
		for i, b := range buf {
			raw[i] = backupAlphabet[b&31] // 32 symbols: no modulo bias
		}
		codes = append(codes, string(raw[:5])+"-"+string(raw[5:]))
		hashes = append(hashes, hashBackup(string(raw)))
	}
	return codes, hashes
}

// normalizeBackup lower-cases code and strips dashes and spaces; ok is false unless the result has the shape
// of a backup code.
func normalizeBackup(code string) (string, bool) {
	c := strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	if len(c) != 10 {
		return "", false
	}
	for i := range len(c) {
		if strings.IndexByte(backupAlphabet, c[i]) < 0 {
			return "", false
		}
	}
	return c, true
}

func hashBackup(normalized string) string {
	sum := sha256.Sum256([]byte("ggboard-backup-code:" + normalized))
	return hex.EncodeToString(sum[:])
}

// consumeBackup removes the hash of code from row when present. The update is conditional on the stored list
// (compare-and-swap, retried after a concurrent change) so a code can never be used twice.
func (s *Service) consumeBackup(ctx context.Context, row *model.UserMFA, normalized string) (bool, error) {
	h := hashBackup(normalized)
	for range 3 {
		idx := slices.IndexFunc(row.BackupCodes, func(x string) bool {
			return subtle.ConstantTimeCompare([]byte(x), []byte(h)) == 1
		})
		if idx < 0 {
			return false, nil
		}
		old, err := row.BackupCodes.Value()
		if err != nil {
			return false, err
		}
		rest := slices.Delete(slices.Clone(row.BackupCodes), idx, idx+1)
		res := s.a.DB.WithContext(ctx).Model(&model.UserMFA{}).
			Where("user_id = ? AND backup_codes = ?", row.UserID, old).
			Update("backup_codes", rest)
		if res.Error != nil {
			return false, fmt.Errorf("mfa: consume backup code: %w", res.Error)
		}
		if res.RowsAffected == 1 {
			row.BackupCodes = rest
			return true, nil
		}
		fresh, err := s.load(ctx, s.a.DB, row.UserID)
		if err != nil || fresh == nil {
			return false, err
		}
		*row = *fresh
	}
	return false, nil
}
