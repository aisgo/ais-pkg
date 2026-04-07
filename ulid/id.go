package ulid

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

/* ========================================================================
 * ID 类型 - 数据库友好的 ULID 包装类型
 * ========================================================================
 * 职责: 提供与数据库和 JSON 序列化兼容的 ULID 类型
 *
 * 实现的接口:
 *   - database/sql/driver.Valuer  - 数据库写入
 *   - database/sql.Scanner        - 数据库读取
 *   - json.Marshaler              - JSON 序列化
 *   - json.Unmarshaler            - JSON 反序列化
 *   - encoding.TextMarshaler      - 文本序列化
 *   - encoding.TextUnmarshaler    - 文本反序列化
 *   - encoding.BinaryMarshaler    - 二进制序列化
 *   - encoding.BinaryUnmarshaler  - 二进制反序列化
 *
 * 存储格式:
 *   - 数据库: bytea (16 bytes 二进制)
 *   - JSON: "01HN3K8X9FQZM6Y8VWXQR2JNPT" (26 字符字符串)
 * ======================================================================== */

// ID 是 ULID 的数据库友好包装类型
// 实现了 sql.Scanner 和 driver.Valuer 接口，确保与 PostgreSQL bytea 类型完美兼容
// 同时实现了 JSON 序列化接口，确保 API 响应中输出为可读的 ULID 字符串格式
//
// 使用示例:
//
//	type User struct {
//	    ID ulid.ID `json:"id" gorm:"type:bytea;primaryKey"`
//	}
type ID ulid.ULID

// ========================================================================
// 构造函数
// ========================================================================

// NewID 生成一个新的 ID
func NewID() ID {
	return ID(Generate())
}

// NewIDWithTime 使用指定时间生成 ID
func NewIDWithTime(t time.Time) ID {
	return ID(GenerateWithTime(t))
}

// ParseID 解析 ULID 字符串为 ID
func ParseID(s string) (ID, error) {
	id, err := ulid.Parse(s)
	if err != nil {
		return ID{}, err
	}
	return ID(id), nil
}

// MustParseID 解析 ULID 字符串为 ID，失败时 panic
func MustParseID(s string) ID {
	return ID(MustParse(s))
}

// ZeroID 返回零值 ID
func ZeroID() ID {
	return ID{}
}

// ========================================================================
// 基本方法
// ========================================================================

// String 返回 ULID 的字符串表示（26 字符）
func (id ID) String() string {
	return ulid.ULID(id).String()
}

// IsZero 检查 ID 是否为零值
func (id ID) IsZero() bool {
	return ulid.ULID(id).Compare(ulid.ULID{}) == 0
}

// ULID 返回底层的 ulid.ULID 类型
func (id ID) ULID() ulid.ULID {
	return ulid.ULID(id)
}

// Time 返回 ID 中编码的时间戳
func (id ID) Time() time.Time {
	return ulid.Time(ulid.ULID(id).Time())
}

// Bytes 返回 ID 的字节切片表示
func (id ID) Bytes() []byte {
	return ulid.ULID(id).Bytes()
}

// Compare 比较两个 ID
// 返回值: -1 (id < other), 0 (id == other), 1 (id > other)
func (id ID) Compare(other ID) int {
	return ulid.ULID(id).Compare(ulid.ULID(other))
}

// ========================================================================
// 数据库接口实现
// ========================================================================

// Value 实现 driver.Valuer 接口
// 返回 []byte 用于 PostgreSQL bytea 类型存储
func (id ID) Value() (driver.Value, error) {
	// 零值返回 nil，数据库中存储为 NULL（如果列允许）
	// 如果需要零值也存储为 bytea，可以移除这个检查
	if id.IsZero() {
		return nil, nil
	}
	return ulid.ULID(id).MarshalBinary()
}

// Scan 实现 sql.Scanner 接口
// 支持从 []byte（bytea）或 string（char/varchar）读取
// 对文本输入会自动裁剪首尾空白，空值/空白返回零值 ID
func (id *ID) Scan(src interface{}) error {
	if src == nil {
		*id = ID{}
		return nil
	}

	switch v := src.(type) {
	case []byte:
		if len(v) == 0 {
			*id = ID{}
			return nil
		}
		// 尝试作为二进制解析（bytea，16 bytes）
		if len(v) == 16 {
			return (*ulid.ULID)(id).UnmarshalBinary(v)
		}
		// 非 16 字节时，尝试作为文本解析（可能带空白）
		trimmed := bytes.TrimSpace(v)
		if len(trimmed) == 0 {
			*id = ID{}
			return nil
		}
		if len(trimmed) == 26 {
			return (*ulid.ULID)(id).UnmarshalText(trimmed)
		}
		return fmt.Errorf("ulid: invalid byte slice length %d (trimmed), expected 16 (binary) or 26 (text)", len(trimmed))
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			*id = ID{}
			return nil
		}
		return (*ulid.ULID)(id).UnmarshalText([]byte(trimmed))
	default:
		return errors.New("ulid: Scan source must be []byte or string")
	}
}

// ========================================================================
// JSON 序列化接口实现
// ========================================================================

// MarshalJSON 实现 json.Marshaler 接口
// 输出为 ULID 字符串格式，而非 base64 编码的 bytes
func (id ID) MarshalJSON() ([]byte, error) {
	if id.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(ulid.ULID(id).String())
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
// 支持从 ULID 字符串或 null 解析
// 空字符串和纯空白字符串视为零值
func (id *ID) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || string(data) == `""` {
		*id = ID{}
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("ulid: failed to unmarshal JSON: %w", err)
	}

	// 裁剪空白后检查是否为空
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		*id = ID{}
		return nil
	}

	parsed, err := ulid.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("ulid: failed to parse ULID string %q: %w", trimmed, err)
	}

	*id = ID(parsed)
	return nil
}

// ========================================================================
// 文本序列化接口实现
// ========================================================================

// MarshalText 实现 encoding.TextMarshaler 接口
// 用于 URL 参数、表单值等文本场景
// 零值返回空字节切片，保持与 JSON/DB 序列化的语义一致
func (id ID) MarshalText() ([]byte, error) {
	if id.IsZero() {
		return []byte{}, nil
	}
	return ulid.ULID(id).MarshalText()
}

// UnmarshalText 实现 encoding.TextUnmarshaler 接口
// 空输入或纯空白输入返回零值 ID
func (id *ID) UnmarshalText(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		*id = ID{}
		return nil
	}
	return (*ulid.ULID)(id).UnmarshalText(trimmed)
}

// ========================================================================
// 二进制序列化接口实现
// ========================================================================

// MarshalBinary 实现 encoding.BinaryMarshaler 接口
// 零值返回空字节切片，保持与 JSON/DB 序列化的语义一致
func (id ID) MarshalBinary() ([]byte, error) {
	if id.IsZero() {
		return []byte{}, nil
	}
	return ulid.ULID(id).MarshalBinary()
}

// UnmarshalBinary 实现 encoding.BinaryUnmarshaler 接口
// 空输入返回零值 ID
func (id *ID) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		*id = ID{}
		return nil
	}
	return (*ulid.ULID)(id).UnmarshalBinary(data)
}

// ========================================================================
// ID 与其他类型的转换
// ========================================================================

// IDFromULID 从 ulid.ULID 创建 ID
func IDFromULID(u ulid.ULID) ID {
	return ID(u)
}

// IDFromUUID 从 uuid.UUID 创建 ID
func IDFromUUID(u uuid.UUID) ID {
	return ID(FromUUID(u))
}

// IDFromUUIDString 从 UUID 字符串创建 ID
func IDFromUUIDString(s string) (ID, error) {
	u, err := FromUUIDString(s)
	if err != nil {
		return ID{}, err
	}
	return ID(u), nil
}

// ToUUIDFromID 将 ID 转换为 UUID
func (id ID) ToUUID() uuid.UUID {
	return ToUUID(ulid.ULID(id))
}

// ToUUIDString 将 ID 转换为 UUID 字符串格式
func (id ID) ToUUIDString() string {
	return ToUUIDString(ulid.ULID(id))
}
