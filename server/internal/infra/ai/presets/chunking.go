package presets

import "go-server/internal/app/ai/dto"

func chunkDefinitions() []dto.ChunkPreset {
	return []dto.ChunkPreset{
		{ID: "balanced", Name: "通用文档", Strategy: "auto", Size: 512, Overlap: 80},
		{ID: "long", Name: "长篇资料", Strategy: "recursive", Size: 1024, Overlap: 120},
		{ID: "structured", Name: "Markdown 与章节资料", Strategy: "heading", Size: 768, Overlap: 80},
	}
}

func strategyDefinitions() []dto.ChunkStrategy {
	return []dto.ChunkStrategy{
		{ID: "auto", Name: "自动识别", Description: "根据标题、段落等文档结构自动选择切分方式。"},
		{ID: "recursive", Name: "递归切分", Description: "依次按段落、换行和句子等边界切分，超过目标长度时继续拆分。"},
		{ID: "heading", Name: "按标题结构", Description: "优先保留 Markdown 标题及章节边界，章节过长时继续递归切分。"},
	}
}
