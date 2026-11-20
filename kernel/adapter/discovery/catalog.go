package discovery

import "context"

// RawModel 发现到的原始模型条目（协议无关）。Provider 仅作展示/排障，不参与分类。
type RawModel struct {
	ID       string
	Provider string
}

// ClassifiedModel 分类后的目录条目。
type ClassifiedModel struct {
	ModelID  string
	Provider string
	Category Category
	Source   string
}

// Catalog 一次模型发现的结果。
// Degraded=true 表示 /models 不可用：Models 为空，CandidateIDs 给出内置候选供手动选择。
type Catalog struct {
	Models       []ClassifiedModel
	Degraded     bool
	CandidateIDs []string
}

// Lister 模型列表来源抽象。各协议适配器在集成层用各自 Client.Models() 适配此接口
// （把 []ModelInfo 映射为 []RawModel），本包不 import 具体协议包。
type Lister interface {
	ListModels(ctx context.Context) ([]RawModel, error)
}

// ListerFunc 把函数适配为 Lister（便于测试与轻量接线）。
type ListerFunc func(ctx context.Context) ([]RawModel, error)

// ListModels 实现 Lister。
func (f ListerFunc) ListModels(ctx context.Context) ([]RawModel, error) { return f(ctx) }

// Discover 拉取模型并逐个分类。
// /models 失败时降级：返回 Catalog{Degraded:true, CandidateIDs:内置候选} + 原始错误，
// 让 UI 切到"手动输入 id + 候选列表"，不阻断使用。
func Discover(ctx context.Context, l Lister) (Catalog, error) {
	raws, err := l.ListModels(ctx)
	if err != nil {
		return Catalog{Degraded: true, CandidateIDs: BuiltinCandidates()}, err
	}
	models := make([]ClassifiedModel, 0, len(raws))
	for _, r := range raws {
		c := Classify(r.ID)
		models = append(models, ClassifiedModel{
			ModelID:  r.ID,
			Provider: r.Provider,
			Category: c.Category,
			Source:   c.Source,
		})
	}
	return Catalog{Models: models}, nil
}
