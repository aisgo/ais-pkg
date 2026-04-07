package ulid

import (
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
)

/* ========================================================================
 * ULID Generator Tests
 * ======================================================================== */

func TestGenerate(t *testing.T) {
	id := Generate()

	if IsZero(id) {
		t.Error("生成的 ULID 不应为零值")
	}

	str := id.String()
	if len(str) != 26 {
		t.Errorf("ULID 字符串长度应为 26，实际: %d", len(str))
	}
}

func TestGenerateString(t *testing.T) {
	str := GenerateString()

	if len(str) != 26 {
		t.Errorf("ULID 字符串长度应为 26，实际: %d", len(str))
	}

	// 验证可以解析
	_, err := Parse(str)
	if err != nil {
		t.Errorf("生成的 ULID 字符串无法解析: %v", err)
	}
}

func TestGenerateWithTime(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	id := GenerateWithTime(testTime)

	extractedTime := Time(id)

	// 允许毫秒级误差
	diff := extractedTime.Sub(testTime).Abs()
	if diff > time.Millisecond {
		t.Errorf("时间戳不匹配，期望: %v, 实际: %v", testTime, extractedTime)
	}
}

func TestParse(t *testing.T) {
	original := Generate()
	str := original.String()

	parsed, err := Parse(str)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if Compare(original, parsed) != 0 {
		t.Error("解析后的 ULID 与原始 ULID 不匹配")
	}
}

func TestMustParse(t *testing.T) {
	str := GenerateString()

	defer func() {
		if r := recover(); r != nil {
			t.Error("有效的 ULID 字符串不应 panic")
		}
	}()

	id := MustParse(str)
	if IsZero(id) {
		t.Error("解析结果不应为零值")
	}
}

func TestMustParsePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("无效的 ULID 字符串应该 panic")
		}
	}()

	MustParse("invalid-ulid")
}

func TestCompare(t *testing.T) {
	id1 := Generate()
	time.Sleep(2 * time.Millisecond)
	id2 := Generate()

	// id1 应该小于 id2（因为时间戳更早）
	if Compare(id1, id2) >= 0 {
		t.Error("后生成的 ULID 应该大于先生成的")
	}

	// 自己和自己比较应该相等
	if Compare(id1, id1) != 0 {
		t.Error("相同的 ULID 比较应该返回 0")
	}
}

func TestIsZero(t *testing.T) {
	zero := Zero()
	if !IsZero(zero) {
		t.Error("Zero() 返回的应该是零值")
	}

	id := Generate()
	if IsZero(id) {
		t.Error("生成的 ULID 不应该是零值")
	}
}

func TestGenerateBatch(t *testing.T) {
	count := 100
	ids := GenerateBatch(count)

	if len(ids) != count {
		t.Errorf("期望生成 %d 个 ULID，实际: %d", count, len(ids))
	}

	// 验证唯一性
	seen := make(map[string]bool)
	for _, id := range ids {
		str := id.String()
		if seen[str] {
			t.Errorf("发现重复的 ULID: %s", str)
		}
		seen[str] = true
	}
}

func TestGenerateBatchZeroOrNegative(t *testing.T) {
	if ids := GenerateBatch(0); len(ids) != 0 {
		t.Errorf("count=0 期望返回空切片，实际: %d", len(ids))
	}
	if ids := GenerateBatch(-1); len(ids) != 0 {
		t.Errorf("count<0 期望返回空切片，实际: %d", len(ids))
	}
}

func TestGenerateBatchString(t *testing.T) {
	count := 50
	strs := GenerateBatchString(count)

	if len(strs) != count {
		t.Errorf("期望生成 %d 个 ULID 字符串，实际: %d", count, len(strs))
	}

	for _, str := range strs {
		if len(str) != 26 {
			t.Errorf("ULID 字符串长度应为 26，实际: %d", len(str))
		}
	}
}

func TestGenerator(t *testing.T) {
	gen := NewGenerator(nil)

	id1 := gen.Generate()
	id2 := gen.Generate()

	if Compare(id1, id2) == 0 {
		t.Error("连续生成的 ULID 应该不同")
	}
}

func TestGeneratorString(t *testing.T) {
	gen := NewGenerator(nil)
	str := gen.GenerateString()

	if len(str) != 26 {
		t.Errorf("ULID 字符串长度应为 26，实际: %d", len(str))
	}
}

func TestGeneratorWithTime(t *testing.T) {
	gen := NewGenerator(nil)
	testTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	id := gen.GenerateWithTime(testTime)
	extractedTime := Time(id)

	diff := extractedTime.Sub(testTime).Abs()
	if diff > time.Millisecond {
		t.Errorf("时间戳不匹配，期望: %v, 实际: %v", testTime, extractedTime)
	}
}

func TestConcurrency(t *testing.T) {
	const goroutines = 10
	const idsPerGoroutine = 100

	results := make(chan ulid.ULID, goroutines*idsPerGoroutine)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < idsPerGoroutine; j++ {
				results <- Generate()
			}
		}()
	}

	seen := make(map[string]bool)
	for i := 0; i < goroutines*idsPerGoroutine; i++ {
		id := <-results
		str := id.String()
		if seen[str] {
			t.Errorf("并发场景下发现重复的 ULID: %s", str)
		}
		seen[str] = true
	}
}

func TestTimeOrdering(t *testing.T) {
	ids := make([]ulid.ULID, 10)
	for i := 0; i < 10; i++ {
		ids[i] = Generate()
		time.Sleep(time.Millisecond)
	}

	// 验证时间递增
	for i := 1; i < len(ids); i++ {
		if Compare(ids[i-1], ids[i]) >= 0 {
			t.Error("ULID 应该按时间递增排序")
		}
	}
}

func TestULIDFormat(t *testing.T) {
	str := GenerateString()

	// ULID 应该只包含 Crockford's Base32 字符
	validChars := "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for _, c := range str {
		if !strings.ContainsRune(validChars, c) {
			t.Errorf("ULID 包含无效字符: %c", c)
		}
	}
}

/* ========================================================================
 * Benchmarks
 * ======================================================================== */

func BenchmarkGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Generate()
	}
}

func BenchmarkGenerateString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateString()
	}
}

func BenchmarkGenerateBatch(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateBatch(100)
	}
}

func BenchmarkParse(b *testing.B) {
	str := GenerateString()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Parse(str)
	}
}

func BenchmarkConcurrent(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Generate()
		}
	})
}
