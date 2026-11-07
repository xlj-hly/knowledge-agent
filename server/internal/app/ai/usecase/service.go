// Package usecase 维护企业 AI 配置，模型调用能力由消费方端口注入。
package usecase

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"time"

	"go-server/internal/app/ai/dto"
	"go-server/internal/app/ai/port"
	"go-server/internal/app/configuration"
	"go-server/internal/domain/ai"
	saas "go-server/internal/domain/saas"
)

type Dependencies struct {
	Repository     ai.Repository
	Authorizer     port.Authorizer
	Probe          port.Probe
	Catalog        port.Catalog
	Presets        port.PresetCatalog
	References     port.References
	Configurations configuration.Guard
}
type Service struct{ deps Dependencies }

func New(deps Dependencies) *Service { return &Service{deps: deps} }
func (s *Service) actor(ctx context.Context, id dto.Identity, admin bool) (port.Actor, error) {
	a, err := s.deps.Authorizer.Authorize(ctx, id.Hostname, id.Token)
	if err == nil && admin && a.Role != "admin" {
		err = saas.Forbidden()
	}
	return a, err
}
func identifier() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	n := binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff
	if n == 0 {
		n = 1
	}
	return strconv.FormatUint(n, 10), nil
}
func view(m ai.Model) dto.Model {
	return dto.Model{ID: m.ID, Name: m.Name, Kind: m.Kind, Provider: m.Provider, Endpoint: m.Endpoint, ModelName: m.ModelName, Dimension: m.Dimension, APIKey: m.APIKey, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

// ResolveModel 供已完成身份验证的业务使用，仍校验企业归属及能力用途。
func (s *Service) ResolveModel(ctx context.Context, tenant, id, kind string) (ai.Model, error) {
	if _, err := saas.ParseID(tenant); err != nil {
		return ai.Model{}, err
	}
	if _, err := saas.ParseID(id); err != nil {
		return ai.Model{}, err
	}
	m, err := s.deps.Repository.Model(ctx, tenant, id, false)
	if err != nil {
		return m, err
	}
	if m.Kind != kind {
		return ai.Model{}, saas.Invalid("模型用途与当前操作不匹配")
	}
	return m, s.validateModel(m)
}
func (s *Service) Defaults(ctx context.Context, tenant string) (ai.Settings, error) {
	return s.deps.Repository.Settings(ctx, tenant)
}
func (s *Service) Models(ctx context.Context, id dto.Identity) ([]dto.Model, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Repository.Models(ctx, a.TenantID)
	out := make([]dto.Model, 0, len(rows))
	for _, m := range rows {
		out = append(out, view(m))
	}
	return out, err
}
func (s *Service) Providers(ctx context.Context, id dto.Identity) ([]dto.Provider, error) {
	if _, err := s.actor(ctx, id, false); err != nil {
		return nil, err
	}
	if s.deps.Catalog == nil {
		return nil, errors.New("AI 能力目录未配置")
	}
	return s.deps.Catalog.Providers(), nil
}
func (s *Service) SaveModel(ctx context.Context, id dto.Identity, target string, in dto.ModelInput) (dto.Model, error) {
	return configured(ctx, s, id, func(ctx context.Context) (dto.Model, error) { return s.saveModel(ctx, id, target, in) })
}
func (s *Service) saveModel(ctx context.Context, id dto.Identity, target string, in dto.ModelInput) (dto.Model, error) {
	a, err := s.actor(ctx, id, true)
	if err != nil {
		return dto.Model{}, err
	}
	create := target == ""
	if create {
		target, err = identifier()
	} else {
		_, err = saas.ParseID(target)
	}
	if err != nil {
		return dto.Model{}, err
	}
	m := ai.Model{ID: target, TenantID: a.TenantID, Name: strings.TrimSpace(in.Name), Kind: in.Kind, Provider: in.Provider, Endpoint: strings.TrimRight(strings.TrimSpace(in.Endpoint), "/"), ModelName: strings.TrimSpace(in.ModelName), Dimension: in.Dimension, APIKey: in.APIKey, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	err = s.deps.Repository.Transaction(ctx, func(tx ai.Repository) error {
		if !create {
			old, e := tx.Model(ctx, a.TenantID, target, true)
			if e != nil {
				return e
			}
			n, e := s.references(ctx, a.TenantID, target)
			if e != nil {
				return e
			}
			defaults, e := tx.Settings(ctx, a.TenantID)
			if e != nil {
				return e
			}
			if defaults.ChatModelID == target || defaults.EmbeddingModelID == target {
				n++
			}
			if n > 0 && (old.Kind != m.Kind || old.Provider != m.Provider || old.Endpoint != m.Endpoint || old.ModelName != m.ModelName || old.Dimension != m.Dimension) {
				return saas.Conflict("模型已被使用，请创建新的模型配置后在业务中切换；知识库切换需要重建索引")
			}
			m.CreatedAt = old.CreatedAt
		}
		if e := s.validateModel(m); e != nil {
			return e
		}
		return tx.SaveModel(ctx, m, create)
	})
	return view(m), err
}
func (s *Service) references(ctx context.Context, t, id string) (int64, error) {
	if s.deps.References == nil {
		return 0, nil
	}
	return s.deps.References.ModelReferences(ctx, t, id)
}
func (s *Service) DeleteModel(ctx context.Context, id dto.Identity, target string) (dto.OK, error) {
	return configured(ctx, s, id, func(ctx context.Context) (dto.OK, error) { return s.deleteModel(ctx, id, target) })
}
func (s *Service) deleteModel(ctx context.Context, id dto.Identity, target string) (dto.OK, error) {
	a, err := s.actor(ctx, id, true)
	if err != nil {
		return dto.OK{}, err
	}
	if _, err = saas.ParseID(target); err != nil {
		return dto.OK{}, err
	}
	err = s.deps.Repository.Transaction(ctx, func(tx ai.Repository) error {
		if _, e := tx.Model(ctx, a.TenantID, target, true); e != nil {
			return e
		}
		n, e := s.references(ctx, a.TenantID, target)
		if e != nil {
			return e
		}
		settings, e := tx.Settings(ctx, a.TenantID)
		if e != nil {
			return e
		}
		if n > 0 || settings.ChatModelID == target || settings.EmbeddingModelID == target {
			return saas.Conflict("请先切换当前启用模型，或移除知识库和企业默认配置中的模型引用")
		}
		return tx.DeleteModel(ctx, a.TenantID, target)
	})
	return dto.OK{OK: err == nil}, err
}
func (s *Service) probeModel(tenant string, in dto.ModelInput) (ai.Model, error) {
	m := ai.Model{Name: "模型探测", TenantID: tenant, Kind: in.Kind, Provider: in.Provider, Endpoint: strings.TrimRight(strings.TrimSpace(in.Endpoint), "/"), ModelName: strings.TrimSpace(in.ModelName), Dimension: in.Dimension, APIKey: in.APIKey}
	// 探测尚未选定模型或维度时只填临时校验值，不保存这些占位参数。
	check := m
	if check.ModelName == "" {
		check.ModelName = "probe"
	}
	if check.Kind == "embedding" && check.Dimension == 0 {
		check.Dimension = 1
	}
	return m, s.validateModel(check)
}

func (s *Service) validateModel(m ai.Model) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if s.deps.Catalog == nil {
		return errors.New("AI 能力目录未配置")
	}
	return s.deps.Catalog.ValidateModel(m)
}
func (s *Service) Discover(ctx context.Context, id dto.Identity, in dto.ModelInput) (dto.Discovery, error) {
	a, err := s.actor(ctx, id, true)
	if err != nil {
		return dto.Discovery{}, err
	}
	m, err := s.probeModel(a.TenantID, in)
	if err != nil {
		return dto.Discovery{}, err
	}
	rows, err := s.deps.Probe.Discover(ctx, m)
	if err != nil {
		return dto.Discovery{}, upstream("获取模型列表失败，请检查连接与凭据", err)
	}
	return dto.Discovery{Models: rows}, nil
}
func (s *Service) Test(ctx context.Context, id dto.Identity, in dto.ModelInput) (dto.TestResult, error) {
	a, err := s.actor(ctx, id, true)
	if err != nil {
		return dto.TestResult{}, err
	}
	m, err := s.probeModel(a.TenantID, in)
	if err != nil {
		return dto.TestResult{}, err
	}
	if m.ModelName == "" {
		return dto.TestResult{}, saas.Invalid("请选择或填写模型名")
	}
	dimension, err := s.deps.Probe.Test(ctx, m)
	if err != nil {
		return dto.TestResult{}, upstream("模型调用测试失败，请检查服务地址、凭据及模型名", err)
	}
	return dto.TestResult{OK: true, Dimension: dimension}, nil
}

type upstreamError struct {
	message string
	cause   error
}

func (e *upstreamError) Error() string { return "ai_upstream_error" }
func (e *upstreamError) Unwrap() []error {
	return []error{&saas.Error{Code: "ai_upstream_error", Message: e.message}, e.cause}
}
func (e *upstreamError) Operation() string       { return "AI service probe" }
func upstream(message string, cause error) error { return &upstreamError{message, cause} }

func configured[T any](ctx context.Context, s *Service, id dto.Identity, fn func(context.Context) (T, error)) (T, error) {
	var out T
	a, err := s.actor(ctx, id, true)
	if err != nil {
		return out, err
	}
	if s.deps.Configurations == nil {
		return fn(ctx)
	}
	err = s.deps.Configurations.Within(ctx, a.TenantID, func(ctx context.Context) error { var e error; out, e = fn(ctx); return e })
	return out, err
}
