package usecase

import (
	"context"
	"errors"
	"testing"

	"go-server/internal/app/saas/dto"
	"go-server/internal/domain/saas"
)

// 非法写入输入必须在访问数据库或开启事务之前被拒绝。
func TestInvalidMutationInputBeforeDatabase(t *testing.T) {
	s := New(Dependencies{}, Config{})
	ctx := context.Background()
	id := dto.TenantIdentity{Hostname: "alpha.test"}
	invalidRole := "owner"
	badAvatar := "javascript:alert(1)"
	nickname := "昵称"
	avatar := ""
	for _, test := range []struct {
		name string
		run  func() error
	}{
		{"empty profile", func() error { _, err := s.Accounts.Profile(ctx, id, dto.ProfileInput{}); return err }},
		{"missing nickname", func() error { _, err := s.Accounts.Profile(ctx, id, dto.ProfileInput{AvatarURL: &avatar}); return err }},
		{"missing avatar", func() error { _, err := s.Accounts.Profile(ctx, id, dto.ProfileInput{Nickname: &nickname}); return err }},
		{"invalid avatar", func() error {
			_, err := s.Accounts.Profile(ctx, id, dto.ProfileInput{Nickname: &nickname, AvatarURL: &badAvatar})
			return err
		}},
		{"short password", func() error {
			_, err := s.Accounts.CreateUser(ctx, id, dto.CreateUserInput{Username: "valid.user", Password: "short", Role: "user"})
			return err
		}},
		{"invalid role", func() error {
			_, err := s.Accounts.UpdateUser(ctx, id, "1", dto.UpdateUserInput{Role: &invalidRole})
			return err
		}},
		{"invalid tenant", func() error {
			_, err := s.Companies.CreateTenant(ctx, "token", dto.CreateTenantInput{Name: "", Hostname: "alpha.test"})
			return err
		}},
		{"invalid target", func() error {
			_, err := s.Accounts.ResetPassword(ctx, id, "0", dto.ResetPasswordInput{New: "Valid-new-password"})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			var business *domain.Error
			if !errors.As(err, &business) || business.Code != "invalid_input" {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}
