package domain

import "testing"

func TestNicknameInputRejected(t *testing.T) {
	if !NicknameInputRejected("<script>") {
		t.Fatal("expected script tag to be rejected")
	}
	if NicknameInputRejected("正常的昵称") {
		t.Fatal("expected valid CJK nickname to be accepted")
	}
	if !NicknameInputRejected("hello\x00world") {
		t.Fatal("expected control char to be rejected")
	}
}
