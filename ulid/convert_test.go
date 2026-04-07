package ulid

import (
	"testing"

	"github.com/google/uuid"
)

/* ========================================================================
 * ULID ⇄ UUID Conversion Tests
 * ======================================================================== */

func TestToUUID(t *testing.T) {
	id := Generate()
	u := ToUUID(id)

	// UUID 应该是有效的
	if u == (uuid.UUID{}) {
		t.Error("转换后的 UUID 不应为零值")
	}

	// 字节数组应该相同
	if string(id[:]) != string(u[:]) {
		t.Error("ULID 和 UUID 的字节数组应该相同")
	}
}

func TestFromUUID(t *testing.T) {
	u := uuid.New()
	id := FromUUID(u)

	// ULID 应该是有效的
	if IsZero(id) {
		t.Error("转换后的 ULID 不应为零值")
	}

	// 字节数组应该相同
	if string(u[:]) != string(id[:]) {
		t.Error("UUID 和 ULID 的字节数组应该相同")
	}
}

func TestULIDUUIDRoundTrip(t *testing.T) {
	// ULID -> UUID -> ULID
	original := Generate()
	u := ToUUID(original)
	converted := FromUUID(u)

	if Compare(original, converted) != 0 {
		t.Error("ULID -> UUID -> ULID 往返转换应该保持一致")
	}
}

func TestUUIDULIDRoundTrip(t *testing.T) {
	// UUID -> ULID -> UUID
	original := uuid.New()
	id := FromUUID(original)
	converted := ToUUID(id)

	if original != converted {
		t.Error("UUID -> ULID -> UUID 往返转换应该保持一致")
	}
}

func TestToUUIDString(t *testing.T) {
	id := Generate()
	uuidStr := ToUUIDString(id)

	// 验证 UUID 字符串格式 (36 字符，包含 4 个连字符)
	if len(uuidStr) != 36 {
		t.Errorf("UUID 字符串长度应为 36，实际: %d", len(uuidStr))
	}

	// 验证可以解析为 UUID
	_, err := uuid.Parse(uuidStr)
	if err != nil {
		t.Errorf("生成的 UUID 字符串无法解析: %v", err)
	}
}

func TestFromUUIDString(t *testing.T) {
	// 标准 UUID 格式
	uuidStr := "550e8400-e29b-41d4-a716-446655440000"
	id, err := FromUUIDString(uuidStr)
	if err != nil {
		t.Fatalf("解析 UUID 字符串失败: %v", err)
	}

	if IsZero(id) {
		t.Error("转换后的 ULID 不应为零值")
	}

	// 验证往返转换
	convertedUUID := ToUUIDString(id)
	if convertedUUID != uuidStr {
		t.Errorf("往返转换不一致，期望: %s, 实际: %s", uuidStr, convertedUUID)
	}
}

func TestFromUUIDStringInvalid(t *testing.T) {
	invalidUUIDs := []string{
		"invalid-uuid",
		"123",
		"",
		"550e8400-e29b-41d4-a716",
	}

	for _, invalid := range invalidUUIDs {
		_, err := FromUUIDString(invalid)
		if err == nil {
			t.Errorf("无效的 UUID 字符串应该返回错误: %s", invalid)
		}
	}
}

func TestMustFromUUIDString(t *testing.T) {
	uuidStr := "550e8400-e29b-41d4-a716-446655440000"

	defer func() {
		if r := recover(); r != nil {
			t.Error("有效的 UUID 字符串不应 panic")
		}
	}()

	id := MustFromUUIDString(uuidStr)
	if IsZero(id) {
		t.Error("转换后的 ULID 不应为零值")
	}
}

func TestMustFromUUIDStringPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("无效的 UUID 字符串应该 panic")
		}
	}()

	MustFromUUIDString("invalid-uuid")
}

func TestUUIDStringFormats(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		valid bool
	}{
		{"标准格式", "550e8400-e29b-41d4-a716-446655440000", true},
		{"大写", "550E8400-E29B-41D4-A716-446655440000", true},
		{"无连字符", "550e8400e29b41d4a716446655440000", true},
		{"URN 格式", "urn:uuid:550e8400-e29b-41d4-a716-446655440000", true},
		{"花括号", "{550e8400-e29b-41d4-a716-446655440000}", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id, err := FromUUIDString(tc.input)
			if tc.valid {
				if err != nil {
					t.Errorf("应该能解析 %s 格式: %v", tc.name, err)
				}
				if IsZero(id) {
					t.Error("转换后的 ULID 不应为零值")
				}
			} else {
				if err == nil {
					t.Errorf("%s 格式应该返回错误", tc.name)
				}
			}
		})
	}
}

func TestConversionPreservesBytes(t *testing.T) {
	// 测试字节级别的精确转换
	id := Generate()
	originalBytes := make([]byte, 16)
	copy(originalBytes, id[:])

	// ULID -> UUID
	u := ToUUID(id)
	uuidBytes := make([]byte, 16)
	copy(uuidBytes, u[:])

	// 验证字节完全相同
	for i := 0; i < 16; i++ {
		if originalBytes[i] != uuidBytes[i] {
			t.Errorf("字节 %d 不匹配: ULID=%d, UUID=%d", i, originalBytes[i], uuidBytes[i])
		}
	}
}

/* ========================================================================
 * Benchmarks for UUID Conversion
 * ======================================================================== */

func BenchmarkToUUID(b *testing.B) {
	id := Generate()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ToUUID(id)
	}
}

func BenchmarkFromUUID(b *testing.B) {
	u := uuid.New()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		FromUUID(u)
	}
}

func BenchmarkToUUIDString(b *testing.B) {
	id := Generate()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ToUUIDString(id)
	}
}

func BenchmarkFromUUIDString(b *testing.B) {
	uuidStr := "550e8400-e29b-41d4-a716-446655440000"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		FromUUIDString(uuidStr)
	}
}
