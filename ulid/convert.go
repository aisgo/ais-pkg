package ulid

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

/* ========================================================================
 * ULID ⇄ UUID 互转
 * ========================================================================
 * ULID 和 UUID 都是 128 位标识符，可以相互转换。
 * 适用场景：与需要 UUID 格式的系统集成，或从 UUID 系统迁移到 ULID。
 * ======================================================================== */

// ToUUID 将 ULID 转换为 UUID
// ULID 和 UUID 都是 128 位，可以直接转换字节数组
//
// 注意: 转换后的 UUID 不保留 ULID 的时间排序特性
// 适用场景: 与需要 UUID 格式的系统集成
func ToUUID(id ulid.ULID) uuid.UUID {
	var u uuid.UUID
	copy(u[:], id[:])
	return u
}

// FromUUID 将 UUID 转换为 ULID
// 直接复制 128 位字节数组
//
// 注意: 转换后的 ULID 可能不包含有效的时间戳
// 适用场景: 从 UUID 系统迁移到 ULID
func FromUUID(u uuid.UUID) ulid.ULID {
	var id ulid.ULID
	copy(id[:], u[:])
	return id
}

// ToUUIDString 将 ULID 转换为 UUID 字符串格式
// 返回标准 UUID 格式: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
func ToUUIDString(id ulid.ULID) string {
	return ToUUID(id).String()
}

// FromUUIDString 从 UUID 字符串创建 ULID
// 支持标准 UUID 格式和无连字符格式
func FromUUIDString(s string) (ulid.ULID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return ulid.ULID{}, fmt.Errorf("invalid UUID string: %w", err)
	}
	return FromUUID(u), nil
}

// MustFromUUIDString 从 UUID 字符串创建 ULID，失败时 panic
func MustFromUUIDString(s string) ulid.ULID {
	id, err := FromUUIDString(s)
	if err != nil {
		panic(err)
	}
	return id
}
