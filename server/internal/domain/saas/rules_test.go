package domain

import (
	"errors"
	"testing"
)

func TestUserAccessPreservesLastActiveAdministrator(t *testing.T) {
	user := User{Role: "admin", Status: "active"}
	member, disabled := "user", "disabled"
	for _, change := range []UserChanges{{Role: &member}, {Status: &disabled}} {
		if _, err := user.ChangeAccess(change, 1); err == nil {
			t.Fatal("last active administrator was removed")
		}
		if _, err := user.ChangeAccess(change, 2); err != nil {
			t.Fatal(err)
		}
	}
	unchanged, err := user.ChangeAccess(UserChanges{Role: &user.Role}, 1)
	if err != nil || *unchanged.Role != "admin" || *unchanged.Status != "active" {
		t.Fatalf("unchanged administrator: %+v %v", unchanged, err)
	}
	inactive := User{Role: "admin", Status: "disabled"}
	if _, err := inactive.ChangeAccess(UserChanges{Role: &member}, 0); err != nil {
		t.Fatal(err)
	}
}

func TestAccessRejectsInvalidRoleAndStatus(t *testing.T) {
	owner, unknown := "owner", "deleted"
	for _, change := range []UserChanges{{}, {Role: &owner}, {Status: &unknown}} {
		_, err := (User{Role: "user", Status: "active"}).ChangeAccess(change, 2)
		var business *Error
		if !errors.As(err, &business) || business.Code != "invalid_input" {
			t.Fatalf("expected invalid change: %v", err)
		}
	}
}

func TestProfileRequiresBothFieldsAndValidAvatar(t *testing.T) {
	empty, nickname, invalid := "", "昵称", "javascript:alert(1)"
	for _, fields := range [][2]*string{{nil, nil}, {&nickname, nil}, {nil, &empty}, {&nickname, &invalid}} {
		if _, err := ProfileChanges(fields[0], fields[1]); err == nil {
			t.Fatal("invalid profile accepted")
		}
	}
	change, err := ProfileChanges(&empty, &empty)
	if err != nil || *change.Nickname != "" || *change.AvatarURL != "" {
		t.Fatalf("explicit clearing rejected: %+v %v", change, err)
	}
}

func TestTenantChangesNormalizeAndIdentifySessionInvalidation(t *testing.T) {
	host, disabled := "NEW.Example:443", "disabled"
	change, err := (CompanyChanges{Hostname: &host, Status: &disabled}).Validated()
	if err != nil || *change.Hostname != "new.example" {
		t.Fatalf("normalized changes: %+v %v", change, err)
	}
	old := Tenant{Hostname: "old.example", Status: "active"}
	if !old.RequiresSessionRevocation(Tenant{Hostname: "new.example", Status: "active"}) {
		t.Fatal("host change retained sessions")
	}
	if !old.RequiresSessionRevocation(Tenant{Hostname: "old.example", Status: "disabled"}) {
		t.Fatal("disabled tenant retained sessions")
	}
	if old.RequiresSessionRevocation(old) {
		t.Fatal("unchanged active tenant invalidated sessions")
	}
}
