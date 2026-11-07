package ai

import "context"

// Repository 维护企业模型和模板，事务只覆盖本地校验与提交。
type Repository interface {
	Transaction(context.Context, func(Repository) error) error
	Models(context.Context, string) ([]Model, error)
	Model(context.Context, string, string, bool) (Model, error)
	SaveModel(context.Context, Model, bool) error
	DeleteModel(context.Context, string, string) error
	Settings(context.Context, string) (Settings, error)
	SaveSettings(context.Context, Settings) error
}
