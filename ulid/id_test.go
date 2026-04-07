package ulid

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

/* ========================================================================
 * ID Type Tests - 数据库友好的 ULID 包装类型
 * ======================================================================== */

func TestNewID(t *testing.T) {
	id := NewID()

	if id.IsZero() {
		t.Error("生成的 ID 不应为零值")
	}

	str := id.String()
	if len(str) != 26 {
		t.Errorf("ID 字符串长度应为 26，实际: %d", len(str))
	}
}

func TestNewIDWithTime(t *testing.T) {
	testTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	id := NewIDWithTime(testTime)

	extractedTime := id.Time()

	diff := extractedTime.Sub(testTime).Abs()
	if diff > time.Millisecond {
		t.Errorf("时间戳不匹配，期望: %v, 实际: %v", testTime, extractedTime)
	}
}

func TestParseID(t *testing.T) {
	original := NewID()
	str := original.String()

	parsed, err := ParseID(str)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if original.Compare(parsed) != 0 {
		t.Error("解析后的 ID 与原始 ID 不匹配")
	}
}

func TestParseIDInvalid(t *testing.T) {
	_, err := ParseID("invalid-id")
	if err == nil {
		t.Error("无效的 ID 字符串应该返回错误")
	}
}

func TestMustParseID(t *testing.T) {
	str := NewID().String()

	defer func() {
		if r := recover(); r != nil {
			t.Error("有效的 ID 字符串不应 panic")
		}
	}()

	id := MustParseID(str)
	if id.IsZero() {
		t.Error("解析结果不应为零值")
	}
}

func TestMustParseIDPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("无效的 ID 字符串应该 panic")
		}
	}()

	MustParseID("invalid-id")
}

func TestZeroID(t *testing.T) {
	zero := ZeroID()
	if !zero.IsZero() {
		t.Error("ZeroID() 返回的应该是零值")
	}
}

func TestIDIsZero(t *testing.T) {
	zero := ZeroID()
	if !zero.IsZero() {
		t.Error("零值 ID 的 IsZero() 应该返回 true")
	}

	id := NewID()
	if id.IsZero() {
		t.Error("生成的 ID 的 IsZero() 应该返回 false")
	}
}

func TestIDCompare(t *testing.T) {
	id1 := NewID()
	time.Sleep(2 * time.Millisecond)
	id2 := NewID()

	if id1.Compare(id2) >= 0 {
		t.Error("后生成的 ID 应该大于先生成的")
	}

	if id1.Compare(id1) != 0 {
		t.Error("相同的 ID 比较应该返回 0")
	}
}

func TestIDBytes(t *testing.T) {
	id := NewID()
	bytes := id.Bytes()

	if len(bytes) != 16 {
		t.Errorf("ID 字节长度应为 16，实际: %d", len(bytes))
	}
}

func TestIDULID(t *testing.T) {
	id := NewID()
	u := id.ULID()

	if u.String() != id.String() {
		t.Error("ULID() 返回的字符串应该与 ID 相同")
	}
}

/* ========================================================================
 * ID Type - Database Interface Tests (Scanner/Valuer)
 * ======================================================================== */

func TestIDValue(t *testing.T) {
	id := NewID()
	value, err := id.Value()
	if err != nil {
		t.Fatalf("Value() 返回错误: %v", err)
	}

	bytes, ok := value.([]byte)
	if !ok {
		t.Fatalf("Value() 应该返回 []byte，实际: %T", value)
	}

	if len(bytes) != 16 {
		t.Errorf("Value() 返回的字节长度应为 16，实际: %d", len(bytes))
	}
}

func TestIDValueZero(t *testing.T) {
	id := ZeroID()
	value, err := id.Value()
	if err != nil {
		t.Fatalf("零值 ID 的 Value() 返回错误: %v", err)
	}

	if value != nil {
		t.Errorf("零值 ID 的 Value() 应该返回 nil，实际: %v", value)
	}
}

func TestIDScanBytes(t *testing.T) {
	original := NewID()
	bytes, _ := original.MarshalBinary()

	var scanned ID
	if err := scanned.Scan(bytes); err != nil {
		t.Fatalf("Scan([]byte) 返回错误: %v", err)
	}

	if original.Compare(scanned) != 0 {
		t.Error("Scan 后的 ID 与原始 ID 不匹配")
	}
}

func TestIDScanString(t *testing.T) {
	original := NewID()
	str := original.String()

	var scanned ID
	if err := scanned.Scan(str); err != nil {
		t.Fatalf("Scan(string) 返回错误: %v", err)
	}

	if original.Compare(scanned) != 0 {
		t.Error("Scan 后的 ID 与原始 ID 不匹配")
	}
}

func TestIDScanTextBytes(t *testing.T) {
	original := NewID()
	text := []byte(original.String()) // 26 字节文本

	var scanned ID
	if err := scanned.Scan(text); err != nil {
		t.Fatalf("Scan(text bytes) 返回错误: %v", err)
	}

	if original.Compare(scanned) != 0 {
		t.Error("Scan 后的 ID 与原始 ID 不匹配")
	}
}

func TestIDScanNil(t *testing.T) {
	var scanned ID
	if err := scanned.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) 返回错误: %v", err)
	}

	if !scanned.IsZero() {
		t.Error("Scan(nil) 后的 ID 应该是零值")
	}
}

func TestIDScanEmptyBytes(t *testing.T) {
	var scanned ID
	if err := scanned.Scan([]byte{}); err != nil {
		t.Fatalf("Scan(empty bytes) 返回错误: %v", err)
	}

	if !scanned.IsZero() {
		t.Error("Scan(empty bytes) 后的 ID 应该是零值")
	}
}

func TestIDScanEmptyString(t *testing.T) {
	var scanned ID
	if err := scanned.Scan(""); err != nil {
		t.Fatalf("Scan(empty string) 返回错误: %v", err)
	}

	if !scanned.IsZero() {
		t.Error("Scan(empty string) 后的 ID 应该是零值")
	}
}

func TestIDScanWhitespaceString(t *testing.T) {
	testCases := []string{"   ", "\t\t", "\n\n", "  \t\n  "}

	for _, tc := range testCases {
		var scanned ID
		if err := scanned.Scan(tc); err != nil {
			t.Fatalf("Scan(%q) 返回错误: %v", tc, err)
		}
		if !scanned.IsZero() {
			t.Errorf("Scan(%q) 后的 ID 应该是零值", tc)
		}
	}
}

func TestIDScanWhitespaceBytes(t *testing.T) {
	testCases := [][]byte{[]byte("   "), []byte("\t\t"), []byte("  \t\n  ")}

	for _, tc := range testCases {
		var scanned ID
		if err := scanned.Scan(tc); err != nil {
			t.Fatalf("Scan(%q) 返回错误: %v", tc, err)
		}
		if !scanned.IsZero() {
			t.Errorf("Scan(%q) 后的 ID 应该是零值", tc)
		}
	}
}

func TestIDScanStringWithPadding(t *testing.T) {
	original := NewID()
	padded := "  " + original.String() + "  \n"

	var scanned ID
	if err := scanned.Scan(padded); err != nil {
		t.Fatalf("Scan(padded string) 返回错误: %v", err)
	}

	if original.Compare(scanned) != 0 {
		t.Error("带空白的字符串输入应该正确解析")
	}
}

func TestIDScanBytesWithPadding(t *testing.T) {
	original := NewID()
	padded := []byte("  " + original.String() + "  \n")

	var scanned ID
	if err := scanned.Scan(padded); err != nil {
		t.Fatalf("Scan(padded bytes) 返回错误: %v", err)
	}

	if original.Compare(scanned) != 0 {
		t.Error("带空白的字节输入应该正确解析")
	}
}

func TestIDScanInvalidType(t *testing.T) {
	var scanned ID
	if err := scanned.Scan(12345); err == nil {
		t.Error("Scan(int) 应该返回错误")
	}
}

func TestIDScanInvalidBytes(t *testing.T) {
	var scanned ID
	// 无效长度的字节切片
	if err := scanned.Scan([]byte{1, 2, 3, 4, 5}); err == nil {
		t.Error("Scan(invalid bytes) 应该返回错误")
	}
}

func TestIDValueScanRoundTrip(t *testing.T) {
	original := NewID()

	// Value() -> Scan()
	value, err := original.Value()
	if err != nil {
		t.Fatalf("Value() 返回错误: %v", err)
	}

	var scanned ID
	if err := scanned.Scan(value); err != nil {
		t.Fatalf("Scan() 返回错误: %v", err)
	}

	if original.Compare(scanned) != 0 {
		t.Error("Value() -> Scan() 往返转换应该保持一致")
	}
}

/* ========================================================================
 * ID Type - JSON Serialization Tests
 * ======================================================================== */

func TestIDMarshalJSON(t *testing.T) {
	id := NewID()
	data, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("MarshalJSON 返回错误: %v", err)
	}

	// 应该是带引号的字符串
	expected := `"` + id.String() + `"`
	if string(data) != expected {
		t.Errorf("MarshalJSON 结果不匹配，期望: %s, 实际: %s", expected, string(data))
	}
}

func TestIDMarshalJSONZero(t *testing.T) {
	id := ZeroID()
	data, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("零值 ID 的 MarshalJSON 返回错误: %v", err)
	}

	if string(data) != "null" {
		t.Errorf("零值 ID 应该序列化为 null，实际: %s", string(data))
	}
}

func TestIDUnmarshalJSON(t *testing.T) {
	original := NewID()
	data := []byte(`"` + original.String() + `"`)

	var unmarshaled ID
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("UnmarshalJSON 返回错误: %v", err)
	}

	if original.Compare(unmarshaled) != 0 {
		t.Error("UnmarshalJSON 后的 ID 与原始 ID 不匹配")
	}
}

func TestIDUnmarshalJSONNull(t *testing.T) {
	var id ID
	if err := json.Unmarshal([]byte("null"), &id); err != nil {
		t.Fatalf("UnmarshalJSON(null) 返回错误: %v", err)
	}

	if !id.IsZero() {
		t.Error("UnmarshalJSON(null) 后的 ID 应该是零值")
	}
}

func TestIDUnmarshalJSONEmptyString(t *testing.T) {
	var id ID
	if err := json.Unmarshal([]byte(`""`), &id); err != nil {
		t.Fatalf("UnmarshalJSON(empty) 返回错误: %v", err)
	}

	if !id.IsZero() {
		t.Error("UnmarshalJSON(empty) 后的 ID 应该是零值")
	}
}

func TestIDUnmarshalJSONWhitespaceString(t *testing.T) {
	testCases := []string{`"   "`, `"\t\t"`, `"  \n  "`}

	for _, tc := range testCases {
		var id ID
		if err := json.Unmarshal([]byte(tc), &id); err != nil {
			t.Fatalf("UnmarshalJSON(%s) 返回错误: %v", tc, err)
		}
		if !id.IsZero() {
			t.Errorf("UnmarshalJSON(%s) 后的 ID 应该是零值", tc)
		}
	}
}

func TestIDUnmarshalJSONWithPadding(t *testing.T) {
	original := NewID()
	// JSON 字符串带空白
	padded := `"  ` + original.String() + `  "`

	var unmarshaled ID
	if err := json.Unmarshal([]byte(padded), &unmarshaled); err != nil {
		t.Fatalf("UnmarshalJSON(padded) 返回错误: %v", err)
	}

	if original.Compare(unmarshaled) != 0 {
		t.Error("带空白的 JSON 字符串输入应该正确解析")
	}
}

func TestIDUnmarshalJSONInvalid(t *testing.T) {
	var id ID
	if err := json.Unmarshal([]byte(`"invalid-id"`), &id); err == nil {
		t.Error("UnmarshalJSON(invalid) 应该返回错误")
	}
}

func TestIDJSONRoundTrip(t *testing.T) {
	original := NewID()

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal 返回错误: %v", err)
	}

	// Unmarshal
	var unmarshaled ID
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal 返回错误: %v", err)
	}

	if original.Compare(unmarshaled) != 0 {
		t.Error("JSON 往返转换应该保持一致")
	}
}

func TestIDInStruct(t *testing.T) {
	type TestStruct struct {
		ID   ID     `json:"id"`
		Name string `json:"name"`
	}

	original := TestStruct{
		ID:   NewID(),
		Name: "test",
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal struct 返回错误: %v", err)
	}

	// 验证 JSON 格式
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map 返回错误: %v", err)
	}

	idStr, ok := raw["id"].(string)
	if !ok {
		t.Fatalf("ID 应该序列化为字符串，实际: %T", raw["id"])
	}
	if idStr != original.ID.String() {
		t.Errorf("ID 字符串不匹配，期望: %s, 实际: %s", original.ID.String(), idStr)
	}

	// Unmarshal
	var unmarshaled TestStruct
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal struct 返回错误: %v", err)
	}

	if original.ID.Compare(unmarshaled.ID) != 0 {
		t.Error("结构体中的 ID 往返转换应该保持一致")
	}
}

/* ========================================================================
 * ID Type - Text Serialization Tests
 * ======================================================================== */

func TestIDMarshalText(t *testing.T) {
	id := NewID()
	text, err := id.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText 返回错误: %v", err)
	}

	if string(text) != id.String() {
		t.Errorf("MarshalText 结果不匹配，期望: %s, 实际: %s", id.String(), string(text))
	}
}

func TestIDUnmarshalText(t *testing.T) {
	original := NewID()
	text := []byte(original.String())

	var unmarshaled ID
	if err := unmarshaled.UnmarshalText(text); err != nil {
		t.Fatalf("UnmarshalText 返回错误: %v", err)
	}

	if original.Compare(unmarshaled) != 0 {
		t.Error("UnmarshalText 后的 ID 与原始 ID 不匹配")
	}
}

func TestIDMarshalTextZero(t *testing.T) {
	id := ZeroID()
	text, err := id.MarshalText()
	if err != nil {
		t.Fatalf("零值 ID 的 MarshalText 返回错误: %v", err)
	}

	if len(text) != 0 {
		t.Errorf("零值 ID 应该序列化为空字节切片，实际长度: %d", len(text))
	}
}

func TestIDUnmarshalTextEmpty(t *testing.T) {
	var id ID
	if err := id.UnmarshalText([]byte{}); err != nil {
		t.Fatalf("UnmarshalText(empty) 返回错误: %v", err)
	}

	if !id.IsZero() {
		t.Error("UnmarshalText(empty) 后的 ID 应该是零值")
	}
}

func TestIDUnmarshalTextWhitespace(t *testing.T) {
	testCases := []struct {
		name  string
		input []byte
	}{
		{"空格", []byte("   ")},
		{"制表符", []byte("\t\t")},
		{"换行符", []byte("\n\n")},
		{"混合空白", []byte("  \t\n  ")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var id ID
			if err := id.UnmarshalText(tc.input); err != nil {
				t.Fatalf("UnmarshalText(%q) 返回错误: %v", tc.input, err)
			}
			if !id.IsZero() {
				t.Errorf("UnmarshalText(%q) 后的 ID 应该是零值", tc.input)
			}
		})
	}
}

func TestIDUnmarshalTextWithPadding(t *testing.T) {
	original := NewID()
	// 带空白的输入
	padded := []byte("  " + original.String() + "  \n")

	var unmarshaled ID
	if err := unmarshaled.UnmarshalText(padded); err != nil {
		t.Fatalf("UnmarshalText(padded) 返回错误: %v", err)
	}

	if original.Compare(unmarshaled) != 0 {
		t.Error("带空白的输入应该正确解析")
	}
}

func TestIDTextRoundTripZero(t *testing.T) {
	original := ZeroID()

	// Marshal
	text, err := original.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText 返回错误: %v", err)
	}

	// Unmarshal
	var unmarshaled ID
	if err := unmarshaled.UnmarshalText(text); err != nil {
		t.Fatalf("UnmarshalText 返回错误: %v", err)
	}

	if !unmarshaled.IsZero() {
		t.Error("零值 Text 往返转换后应该保持零值")
	}
}

/* ========================================================================
 * ID Type - Binary Serialization Tests
 * ======================================================================== */

func TestIDMarshalBinary(t *testing.T) {
	id := NewID()
	binary, err := id.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary 返回错误: %v", err)
	}

	if len(binary) != 16 {
		t.Errorf("MarshalBinary 结果长度应为 16，实际: %d", len(binary))
	}
}

func TestIDUnmarshalBinary(t *testing.T) {
	original := NewID()
	binary, _ := original.MarshalBinary()

	var unmarshaled ID
	if err := unmarshaled.UnmarshalBinary(binary); err != nil {
		t.Fatalf("UnmarshalBinary 返回错误: %v", err)
	}

	if original.Compare(unmarshaled) != 0 {
		t.Error("UnmarshalBinary 后的 ID 与原始 ID 不匹配")
	}
}

func TestIDMarshalBinaryZero(t *testing.T) {
	id := ZeroID()
	binary, err := id.MarshalBinary()
	if err != nil {
		t.Fatalf("零值 ID 的 MarshalBinary 返回错误: %v", err)
	}

	if len(binary) != 0 {
		t.Errorf("零值 ID 应该序列化为空字节切片，实际长度: %d", len(binary))
	}
}

func TestIDUnmarshalBinaryEmpty(t *testing.T) {
	var id ID
	if err := id.UnmarshalBinary([]byte{}); err != nil {
		t.Fatalf("UnmarshalBinary(empty) 返回错误: %v", err)
	}

	if !id.IsZero() {
		t.Error("UnmarshalBinary(empty) 后的 ID 应该是零值")
	}
}

func TestIDBinaryRoundTripZero(t *testing.T) {
	original := ZeroID()

	// Marshal
	binary, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary 返回错误: %v", err)
	}

	// Unmarshal
	var unmarshaled ID
	if err := unmarshaled.UnmarshalBinary(binary); err != nil {
		t.Fatalf("UnmarshalBinary 返回错误: %v", err)
	}

	if !unmarshaled.IsZero() {
		t.Error("零值 Binary 往返转换后应该保持零值")
	}
}

/* ========================================================================
 * ID Type - Zero Value Serialization Consistency Tests
 * ======================================================================== */

func TestZeroValueSerializationConsistency(t *testing.T) {
	// 测试零值在所有序列化格式中的语义一致性
	zero := ZeroID()

	// DB Value
	dbValue, err := zero.Value()
	if err != nil {
		t.Fatalf("Value() 返回错误: %v", err)
	}
	if dbValue != nil {
		t.Errorf("零值 DB Value 应该为 nil，实际: %v", dbValue)
	}

	// JSON
	jsonData, err := json.Marshal(zero)
	if err != nil {
		t.Fatalf("MarshalJSON 返回错误: %v", err)
	}
	if string(jsonData) != "null" {
		t.Errorf("零值 JSON 应该为 null，实际: %s", string(jsonData))
	}

	// Text
	textData, err := zero.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText 返回错误: %v", err)
	}
	if len(textData) != 0 {
		t.Errorf("零值 Text 应该为空，实际长度: %d", len(textData))
	}

	// Binary
	binaryData, err := zero.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary 返回错误: %v", err)
	}
	if len(binaryData) != 0 {
		t.Errorf("零值 Binary 应该为空，实际长度: %d", len(binaryData))
	}
}

func TestNonZeroValueSerializationConsistency(t *testing.T) {
	// 测试非零值在所有序列化格式中都有有效输出
	id := NewID()

	// DB Value
	dbValue, err := id.Value()
	if err != nil {
		t.Fatalf("Value() 返回错误: %v", err)
	}
	if dbValue == nil {
		t.Error("非零值 DB Value 不应该为 nil")
	}
	if bytes, ok := dbValue.([]byte); !ok || len(bytes) != 16 {
		t.Errorf("非零值 DB Value 应该是 16 字节，实际: %v", dbValue)
	}

	// JSON
	jsonData, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("MarshalJSON 返回错误: %v", err)
	}
	if string(jsonData) == "null" {
		t.Error("非零值 JSON 不应该为 null")
	}

	// Text
	textData, err := id.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText 返回错误: %v", err)
	}
	if len(textData) != 26 {
		t.Errorf("非零值 Text 应该是 26 字符，实际长度: %d", len(textData))
	}

	// Binary
	binaryData, err := id.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary 返回错误: %v", err)
	}
	if len(binaryData) != 16 {
		t.Errorf("非零值 Binary 应该是 16 字节，实际长度: %d", len(binaryData))
	}
}

/* ========================================================================
 * ID Type - Conversion Tests
 * ======================================================================== */

func TestIDFromULID(t *testing.T) {
	u := Generate()
	id := IDFromULID(u)

	if id.String() != u.String() {
		t.Error("IDFromULID 转换后的字符串应该相同")
	}
}

func TestIDFromUUID(t *testing.T) {
	u := uuid.New()
	id := IDFromUUID(u)

	// 验证字节相同
	if string(id.Bytes()) != string(u[:]) {
		t.Error("IDFromUUID 转换后的字节应该相同")
	}
}

func TestIDFromUUIDString(t *testing.T) {
	uuidStr := "550e8400-e29b-41d4-a716-446655440000"
	id, err := IDFromUUIDString(uuidStr)
	if err != nil {
		t.Fatalf("IDFromUUIDString 返回错误: %v", err)
	}

	if id.IsZero() {
		t.Error("转换后的 ID 不应为零值")
	}
}

func TestIDToUUID(t *testing.T) {
	id := NewID()
	u := id.ToUUID()

	// 验证字节相同
	if string(id.Bytes()) != string(u[:]) {
		t.Error("ToUUID 转换后的字节应该相同")
	}
}

func TestIDToUUIDString(t *testing.T) {
	id := NewID()
	uuidStr := id.ToUUIDString()

	if len(uuidStr) != 36 {
		t.Errorf("UUID 字符串长度应为 36，实际: %d", len(uuidStr))
	}

	// 验证可以解析
	_, err := uuid.Parse(uuidStr)
	if err != nil {
		t.Errorf("生成的 UUID 字符串无法解析: %v", err)
	}
}

/* ========================================================================
 * ID Type Benchmarks
 * ======================================================================== */

func BenchmarkNewID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewID()
	}
}

func BenchmarkIDValue(b *testing.B) {
	id := NewID()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id.Value()
	}
}

func BenchmarkIDScan(b *testing.B) {
	id := NewID()
	bytes, _ := id.MarshalBinary()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var scanned ID
		scanned.Scan(bytes)
	}
}

func BenchmarkIDMarshalJSON(b *testing.B) {
	id := NewID()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		json.Marshal(id)
	}
}

func BenchmarkIDUnmarshalJSON(b *testing.B) {
	id := NewID()
	data, _ := json.Marshal(id)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var unmarshaled ID
		json.Unmarshal(data, &unmarshaled)
	}
}
