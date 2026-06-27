package domain

import "testing"

func TestUserFields(t *testing.T) {
	u := User{ID: "u1", Email: "a@b.com", Nickname: "nick"}
	if u.ID != "u1" || u.Nickname != "nick" {
		t.Fatalf("unexpected user: %+v", u)
	}
}
