package validate

// 企业为何需要：昵称清理是安全关键组件——XSS 注入、不可见字符注入、长度溢出
// 都可能导致存储型 XSS 或 UI 破坏。此测试文件以对抗性输入验证清理逻辑的健壮性。

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// --- 空字符串 ---

func TestNickname_EmptyString(t *testing.T) {
	if got := Nickname(""); got != "" {
		t.Errorf("Nickname(\"\") = %q, want empty string", got)
	}
}

// --- 合法昵称应原样返回 ---

func TestNickname_ValidNames(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple ASCII", "Player1", "Player1"},
		{"CJK characters", "你好世界", "你好世界"},
		{"mixed CJK and ASCII", "Player玩家", "Player玩家"},
		{"single rune emoji", "🎮gamer", "🎮gamer"},
		{"single char", "A", "A"},
		{"numbers only", "12345", "12345"},
		{"spaces inside", "hello world", "hello world"},
		{"exactly 20 runes", "12345678901234567890", "12345678901234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Nickname(tt.input)
			if got != tt.want {
				t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- 控制字符 U+0000-U+001F, U+007F-U+009F 必须被移除 ---

func TestNickname_ControlChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"NULL byte", "hel\x00lo", "hello"},
		{"SOH byte", "a\x01b", "ab"},
		{"TAB is control char (removed not collapsed)", "a\tb", "ab"},
		{"LF is control char", "a\nb", "ab"},
		{"CR is control char", "a\rb", "ab"},
		{"vertical tab", "a\x0Bb", "ab"},
		{"form feed", "a\x0Cb", "ab"},
		{"US (U+001F) boundary", "a\x1Fb", "ab"},
		{"DEL (U+007F) boundary", "a\x7Fb", "ab"},
		{"PAD (U+0080)", "a\u0080b", "ab"},
		{"APC (U+009F) boundary", "a\u009Fb", "ab"},
		{"multiple control chars", "\x00\x01\x02hello\x7F\u009F", "hello"},
		{"only control chars", "\x00\x01\x02\x7F\u009F", ""},
		{"control chars with spaces", " \x00 hello \x7F ", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Nickname(tt.input)
			if got != tt.want {
				t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// 安全断言：输出不得包含任何控制字符
			for _, r := range got {
				if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
					t.Errorf("output contains control char U+%04X in %q", r, got)
				}
			}
		})
	}
}

// --- 零宽字符必须被移除 ---

func TestNickname_ZeroWidthChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ZWSP U+200B", "he\u200Bllo", "hello"},
		{"ZWNJ U+200C", "a\u200Cb", "ab"},
		{"ZWJ U+200D", "a\u200Db", "ab"},
		{"LRM U+200E", "a\u200Eb", "ab"},
		{"RLM U+200F", "a\u200Fb", "ab"},
		{"BOM U+FEFF", "a\uFEFFb", "ab"},
		{"LINE SEP U+2028", "a\u2028b", "ab"},
		{"PARA SEP U+2029", "a\u2029b", "ab"},
		{"LRE U+202A", "a\u202Ab", "ab"},
		{"RLE U+202B", "a\u202Bb", "ab"},
		{"PDF U+202C", "a\u202Cb", "ab"},
		{"LRO U+202D", "a\u202Db", "ab"},
		{"RLO U+202E", "a\u202Eb", "ab"},
		{"NNBSP U+202F", "a\u202Fb", "ab"},
		{"WORD JOINER U+2060", "a\u2060b", "ab"},
		{"U+2061", "a\u2061b", "ab"},
		{"U+206F boundary", "a\u206Fb", "ab"},
		{"multiple zero-width", "\u200B\u200C\u200Dhello\uFEFF\u2028", "hello"},
		{"only zero-width", "\u200B\u200C\u200D\uFEFF\u2028\u2029\u2060", ""},
		{"zero-width between valid", "a\u200Bb\u200Cc\u200Dd", "abcd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Nickname(tt.input)
			if got != tt.want {
				t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- HTML 特殊字符必须被移除（XSS 防护） ---

func TestNickname_HTMLSpecialChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"less than", "a<b", "ab"},
		{"greater than", "a>b", "ab"},
		{"double quote", `a"b`, "ab"},
		{"single quote", "a'b", "ab"},
		{"backtick", "a`b", "ab"},
		{"ampersand", "a&b", "ab"},
		{"all HTML chars", `a<b>c"d'e` + "`" + `f&g`, "abcdefg"},
		{"angle brackets only", "<>", ""},
		{"entity-like", "&lt;script&gt;", "lt;scriptgt;"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Nickname(tt.input)
			if got != tt.want {
				t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- 纯恶意 XSS 输入 ---

// xssAttackTestCases returns test cases for adversarial XSS inputs.
func xssAttackTestCases() []struct {
	name  string
	input string
	want  string
} {
	return []struct {
		name  string
		input string
		want  string
	}{
		{
			"script tag",
			"<script>alert(1)</script>",
			"scriptalert(1)/sc",
		},
		{
			"img onerror",
			"<img src=x onerror=alert(1)>",
			"img src=x onerror=a",
		},
		{
			"svg onload",
			"<svg onload=alert(1)>",
			"svg onload=alert(1)",
		},
		{
			"javascript protocol",
			`javascript:alert(1)`,
			"javascript:alert(1)",
		},
		{
			"iframe",
			"<iframe src=javascript:alert(1)></iframe>",
			"iframe src=javascri",
		},
		{
			"style expression",
			"<style>@import url(javascript:alert(1))</style>",
			"style@import url(j",
		},
		{
			"event handler",
			"<div onmouseover=alert(1)>hover</div>",
			"div onmouseover=ale",
		},
		{
			"encoded quotes",
			`<a href="x" onclick="alert(1)">link</a>`,
			"a href=x onclick=",
		},
	}
}

func TestNickname_XSSAttacks(t *testing.T) {
	for _, tt := range xssAttackTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			got := Nickname(tt.input)
			if got != tt.want {
				t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// 安全断言：输出绝不能包含任何 HTML 特殊字符
			dangerous := []string{"<", ">", `"`, "'", "`", "&"}
			for _, d := range dangerous {
				if strings.Contains(got, d) {
					t.Errorf("SECURITY: output contains %q in %q — XSS vector not neutralized", d, got)
				}
			}
		})
	}
}

// --- 混合合法 + 恶意内容 ---

func TestNickname_MixedValidAndMalicious(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"valid name with script tag",
			"Alice<script>alert(1)</script>",
			"Alicescriptalert(1",
		},
		{
			"valid name with img onerror",
			"Bob<img onerror=alert(1)>",
			"Bobimg onerror=aler",
		},
		{
			"name with ampersand entity",
			"Tom & Jerry",
			"Tom Jerry",
		},
		{
			"name with quotes",
			`O"Connor`,
			"OConnor",
		},
		{
			"name with angle brackets",
			"<Admin>",
			"Admin",
		},
		{
			"valid CJK with script",
			"玩家<script>x</script>",
			"玩家scriptx/script",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Nickname(tt.input)
			if got != tt.want {
				t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// 安全断言
			dangerous := []string{"<", ">", `"`, "'", "`", "&"}
			for _, d := range dangerous {
				if strings.Contains(got, d) {
					t.Errorf("SECURITY: output contains %q in %q", d, got)
				}
			}
		})
	}
}

// --- 长度截断（按 rune，非按 byte） ---

func TestNickname_LengthTruncation(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"21 ASCII chars", "12345678901234567890x"},
		{"30 ASCII chars", strings.Repeat("a", 30)},
		{"21 CJK chars", "你好世界你好世界你好世界你好世界你好世界你"},
		{"30 CJK chars", strings.Repeat("你", 30)},
		{"mixed 21 runes", "Player玩家12345678"},
		{"100 chars", strings.Repeat("x", 100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Nickname(tt.input)
			runeCount := utf8.RuneCountInString(got)
			if runeCount > 20 {
				t.Errorf("Nickname output has %d runes, max 20 allowed; got %q", runeCount, got)
			}
		})
	}
}

// --- CJK 多字节截断正确性 ---

func TestNickname_CJKTruncation(t *testing.T) {
	// 21 CJK 字符 → 截断到 20
	input := "你好世界你好世界你好世界你好世界你好世界你" // 21 runes
	got := Nickname(input)
	want := "你好世界你好世界你好世界你好世界你好世界" // 20 runes
	if got != want {
		t.Errorf("Nickname(21 CJK) = %q, want %q", got, want)
	}
	if utf8.RuneCountInString(got) != 20 {
		t.Errorf("rune count = %d, want 20", utf8.RuneCountInString(got))
	}
}

// --- 空白处理：trim + collapse ---

func TestNickname_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"leading spaces", "  hello", "hello"},
		{"trailing spaces", "hello  ", "hello"},
		{"both sides", "  hello  ", "hello"},
		{"collapse multiple internal", "hello   world", "hello world"},
		{"collapse tabs (removed as control first)", "a\t\tb", "ab"},
		{"collapse newlines (removed as control first)", "a\n\nb", "ab"},
		{"only spaces", "   ", ""},
		{"only tabs", "\t\t\t", ""},
		{"mixed whitespace types", "  hello \t world  ", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Nickname(tt.input)
			if got != tt.want {
				t.Errorf("Nickname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- 截断后空白折叠的边界情况 ---

func TestNickname_TruncationThenCollapse(t *testing.T) {
	// 22 字符：a + 20 spaces + b → 截断到 20: a + 19 spaces → 折叠: "a "
	// 这是一个已知的边界行为：截断在空白中间，折叠后留下尾随空格
	input := "a" + strings.Repeat(" ", 20) + "b"
	got := Nickname(input)
	// 截断后是 "a" + 19 spaces (20 runes)，折叠后是 "a "
	if got != "a " {
		t.Errorf("Nickname(truncated whitespace) = %q, want %q (trailing space is expected behavior)", got, "a ")
	}
}

// --- 组合攻击：控制字符 + 零宽 + HTML + 长度 ---

func TestNickname_CombinedAttack(t *testing.T) {
	input := "\x00\u200B<script>\u2028alert\x7F(1)\u200D</script>"
	got := Nickname(input)

	// 安全断言：不得包含任何危险字符
	dangerous := []string{"<", ">", `"`, "'", "`", "&"}
	for _, d := range dangerous {
		if strings.Contains(got, d) {
			t.Errorf("SECURITY: combined attack output contains %q in %q", d, got)
		}
	}
	// 不得包含控制字符或零宽字符
	for _, r := range got {
		if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
			t.Errorf("output contains control char U+%04X", r)
		}
		if (r >= 0x200B && r <= 0x200F) || r == 0xFEFF ||
			(r >= 0x2028 && r <= 0x202F) || (r >= 0x2060 && r <= 0x206F) {
			t.Errorf("output contains zero-width char U+%04X", r)
		}
	}
	// 长度不超过 20
	if utf8.RuneCountInString(got) > 20 {
		t.Errorf("output exceeds 20 runes: %d", utf8.RuneCountInString(got))
	}
}

// --- 截断顺序：HTML 移除在截断之后，确保 XSS 始终被清除 ---

func TestNickname_TruncationBeforeHTMLRemoval(t *testing.T) {
	// 20 个合法字符 + <script> — 截断移除 script，HTML 移除无操作
	input := "12345678901234567890<script>"
	got := Nickname(input)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("SECURITY: trailing script tag leaked into output: %q", got)
	}
	if utf8.RuneCountInString(got) > 20 {
		t.Errorf("output exceeds 20 runes: %q", got)
	}
}

// --- 安全性回归：输出永远不含 HTML 特殊字符 ---

func TestNickname_NoHTMLOutput(t *testing.T) {
	inputs := []string{
		"<script>",
		"<img src=x onerror=alert(1)>",
		`"';<>` + "`" + `&`,
		"<svg/onload=alert(1)>",
		"javascript:<script>alert(1)</script>",
		"<a href='x'>link</a>",
		"<iframe src=javascript:alert(1)>",
		"<body onload=alert(1)>",
		"<object data=javascript:alert(1)>",
		"<embed src=javascript:alert(1)>",
		"<form action=javascript:alert(1)>",
		"<input onfocus=alert(1) autofocus>",
	}

	dangerous := []string{"<", ">", `"`, "'", "`", "&"}
	for _, input := range inputs {
		got := Nickname(input)
		for _, d := range dangerous {
			if strings.Contains(got, d) {
				t.Errorf("SECURITY: Nickname(%q) = %q contains %q", input, got, d)
			}
		}
	}
}
