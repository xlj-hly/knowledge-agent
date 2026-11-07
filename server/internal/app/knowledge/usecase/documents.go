package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/storage"
	"go-server/internal/domain/knowledge"
	domain "go-server/internal/domain/saas"
	"path"
	"strings"
	"unicode/utf8"
)

// MaxFileSize 是单篇导入的原文件上限，解析文本另有独立上限。
const MaxFileSize = 10 * 1024 * 1024
const maxTextSize = 2 * 1024 * 1024

func validateText(title, content string) error {
	if strings.ContainsRune(title, 0) {
		return domain.Invalid("资料标题不能包含 NUL 字符")
	}
	if err := domain.ValidateName(title, 256, "资料标题"); err != nil {
		return err
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, 0) || strings.TrimSpace(content) == "" || len(content) > maxTextSize {
		return domain.Invalid("资料内容不能为空且不得超过 2 MiB UTF-8 文本")
	}
	return nil
}
func (s *Service) newDocument(t, b, title string) (knowledge.Document, error) {
	id, err := newID()
	if err != nil {
		return knowledge.Document{}, err
	}
	generation, err := attempt()
	return knowledge.Document{ID: id, TenantID: t, BaseID: b, Title: title, Attempt: generation, ParseStatus: "queued", CreatedAt: now(), UpdatedAt: now()}, err
}

// Documents 列出知识库资料状态。
func (s *Service) Documents(ctx context.Context, id dto.Identity, base string) ([]dto.Document, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if err = validID(base); err != nil {
		return nil, err
	}
	if _, err = s.deps.Repository.Base(ctx, a.TenantID, base, false); err != nil {
		return nil, err
	}
	rows, err := s.deps.Repository.Documents(ctx, a.TenantID, base)
	out := make([]dto.Document, 0, len(rows))
	for _, r := range rows {
		out = append(out, documentView(r, false))
	}
	return out, err
}

// CreateDocument 保存手写原文并入持久化处理队列。
func (s *Service) CreateDocument(ctx context.Context, id dto.Identity, base string, in dto.DocumentInput) (dto.Document, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.Document{}, err
	}
	if err = validID(base); err != nil {
		return dto.Document{}, err
	}
	title := strings.TrimSpace(in.Title)
	if err = validateText(title, in.Content); err != nil {
		return dto.Document{}, err
	}
	d, err := s.newDocument(a.TenantID, base, title)
	if err != nil {
		return dto.Document{}, err
	}
	d.Source = "manual"
	d.Content = in.Content
	d.FileType = "text/plain"
	d.FileSize = int64(len(in.Content))
	err = s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
		if _, e := tx.Base(ctx, a.TenantID, base, true); e != nil {
			return e
		}
		return tx.SaveDocument(ctx, d, true)
	})
	return documentView(d, true), err
}

// UploadDocument 先存原文件，再提交资料；数据库失败时回收本次拥有的对象。
func (s *Service) UploadDocument(ctx context.Context, id dto.Identity, base, filename string, body []byte) (dto.Document, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.Document{}, err
	}
	if err = validID(base); err != nil {
		return dto.Document{}, err
	}
	filename = path.Base(strings.ReplaceAll(filename, "\\", "/"))
	if len(body) == 0 || len(body) > MaxFileSize || len(filename) > 256 || !utf8.ValidString(filename) {
		return dto.Document{}, domain.Invalid("文件无效或超过 10 MiB")
	}
	for _, r := range filename {
		if r < 32 {
			return dto.Document{}, domain.Invalid("文件名无效")
		}
	}
	ext := strings.ToLower(path.Ext(filename))
	types := map[string]string{".txt": "text/plain", ".md": "text/markdown", ".markdown": "text/markdown", ".csv": "text/csv", ".json": "application/json", ".pdf": "application/pdf", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document"}
	contentType, ok := types[ext]
	if !ok {
		return dto.Document{}, domain.Invalid("支持 TXT、Markdown、CSV、JSON、PDF 和 DOCX 文件")
	}
	if _, err = s.deps.Repository.Base(ctx, a.TenantID, base, false); err != nil {
		return dto.Document{}, err
	}
	d, err := s.newDocument(a.TenantID, base, filename)
	if err != nil {
		return dto.Document{}, err
	}
	sum := sha256.Sum256(body)
	d.Source = "file"
	d.FileName = filename
	d.FileType = contentType
	d.FileSize = int64(len(body))
	d.FileHash = hex.EncodeToString(sum[:])
	d.ObjectKey = fmt.Sprintf("knowledge/%s/%s/%s%s", a.TenantID, base, d.ID, ext)
	if err = s.deps.Objects.Put(ctx, d.ObjectKey, bytes.NewReader(body), d.FileSize, contentType); err != nil {
		return dto.Document{}, err
	}
	err = s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
		if _, e := tx.Base(ctx, a.TenantID, base, true); e != nil {
			return e
		}
		return tx.SaveDocument(ctx, d, true)
	})
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancel()
		err = errors.Join(err, s.deps.Objects.Delete(cleanupCtx, d.ObjectKey))
	}
	return documentView(d, false), err
}

// Document 返回原文详情。
func (s *Service) Document(ctx context.Context, id dto.Identity, target string) (dto.Document, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.Document{}, err
	}
	if err = validID(target); err != nil {
		return dto.Document{}, err
	}
	d, err := s.deps.Repository.Document(ctx, a.TenantID, target, false)
	return documentView(d, true), err
}

// UpdateDocument 编辑手写原文；旧处理代立即失效，重新处理后再进入检索。
func (s *Service) UpdateDocument(ctx context.Context, id dto.Identity, target string, in dto.DocumentInput) (dto.Document, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.Document{}, err
	}
	if err = validID(target); err != nil {
		return dto.Document{}, err
	}
	if err = validateText(strings.TrimSpace(in.Title), in.Content); err != nil {
		return dto.Document{}, err
	}
	var d knowledge.Document
	err = s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
		var e error
		d, e = documentForWrite(ctx, tx, a.TenantID, target)
		if e != nil {
			return e
		}
		if d.Source != "manual" {
			return domain.Invalid("文件资料请重新上传，只有手写资料支持编辑原文")
		}
		if d.ParseStatus == "deleting" || d.ParseStatus == "processing" {
			return domain.Conflict("资料正在处理或删除，请等待完成后编辑")
		}
		d.Title = strings.TrimSpace(in.Title)
		d.Content = in.Content
		d.FileSize = int64(len(in.Content))
		return queue(ctx, tx, &d, false)
	})
	return documentView(d, true), err
}

func queue(ctx context.Context, tx knowledge.Repository, d *knowledge.Document, useChunks bool) error {
	next, err := attempt()
	if err != nil {
		return err
	}
	d.RememberIndex()
	d.Attempt = next
	d.UseChunks = useChunks
	d.ParseStatus = "queued"
	d.ErrorMessage = ""
	d.ChunkCount = 0
	d.LeaseUntil = nil
	d.UpdatedAt = now()
	return tx.SaveDocument(ctx, *d, false)
}

// documentForWrite 统一知识库→资料锁顺序；资料的 BaseID 创建后不可变。
func documentForWrite(ctx context.Context, tx knowledge.Repository, tenant, id string) (knowledge.Document, error) {
	d, err := tx.Document(ctx, tenant, id, false)
	if err != nil {
		return d, err
	}
	if _, err = tx.Base(ctx, tenant, d.BaseID, true); err != nil {
		return d, err
	}
	return tx.Document(ctx, tenant, id, true)
}

// Reprocess 从原始资料重新解析切分，覆盖此前的分块编辑。
func (s *Service) Reprocess(ctx context.Context, id dto.Identity, target string) (dto.Document, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.Document{}, err
	}
	if err = validID(target); err != nil {
		return dto.Document{}, err
	}
	var d knowledge.Document
	err = s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
		var e error
		d, e = documentForWrite(ctx, tx, a.TenantID, target)
		if e != nil {
			return e
		}
		if d.ParseStatus == "deleting" || d.ParseStatus == "processing" {
			return domain.Conflict("资料正在处理或删除，请等待完成后重试")
		}
		return queue(ctx, tx, &d, false)
	})
	return documentView(d, false), err
}

// Chunks 展示当前处理代；处理中不会混入此前已失效的分块。
func (s *Service) Chunks(ctx context.Context, id dto.Identity, target string) ([]dto.Chunk, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if err = validID(target); err != nil {
		return nil, err
	}
	if _, err = s.deps.Repository.Document(ctx, a.TenantID, target, false); err != nil {
		return nil, err
	}
	rows, err := s.deps.Repository.Chunks(ctx, a.TenantID, target)
	out := make([]dto.Chunk, 0, len(rows))
	for _, r := range rows {
		out = append(out, chunkView(r))
	}
	return out, err
}

// UpdateChunk 保留原文件与解析原文，仅以编辑后的分块重新生成整个资料的索引。
func (s *Service) UpdateChunk(ctx context.Context, id dto.Identity, target string, in dto.ChunkInput) (dto.Document, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.Document{}, err
	}
	if err = validID(target); err != nil {
		return dto.Document{}, err
	}
	if strings.TrimSpace(in.Content) == "" || len(in.Content) > 64*1024 || !utf8.ValidString(in.Content) || strings.ContainsRune(in.Content, 0) {
		return dto.Document{}, domain.Invalid("分块内容不能为空且不得超过 64 KiB")
	}
	selected, err := s.deps.Repository.Chunk(ctx, a.TenantID, target)
	if err != nil {
		return dto.Document{}, err
	}
	var d knowledge.Document
	err = s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
		var e error
		d, e = documentForWrite(ctx, tx, a.TenantID, selected.DocumentID)
		if e != nil {
			return e
		}
		if d.ParseStatus != "completed" || d.Attempt != selected.Attempt {
			return domain.Conflict("资料状态已变化，请刷新后编辑")
		}
		chunks, e := tx.Chunks(ctx, a.TenantID, d.ID)
		if e != nil {
			return e
		}
		if e = queue(ctx, tx, &d, true); e != nil {
			return e
		}
		for _, c := range chunks {
			if c.ID == target {
				c.Content = in.Content
			}
			c.ID, e = newID()
			if e != nil {
				return e
			}
			c.Attempt = d.Attempt
			c.CreatedAt = now()
			if e = tx.SaveChunk(ctx, c); e != nil {
				return e
			}
		}
		return nil
	})
	return documentView(d, false), err
}

// DeleteDocument 受理删除；正在处理的资料先标记 deleting，等待原租约结束再回收。
// 延迟物理删除保留清理责任，确保旧 worker 不会在记录删除后写入无人回收的向量。
func (s *Service) DeleteDocument(ctx context.Context, id dto.Identity, target string) (dto.OK, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return dto.OK{}, err
	}
	if err = validID(target); err != nil {
		return dto.OK{}, err
	}
	err = s.deleteDocument(ctx, a.TenantID, target)
	return dto.OK{OK: err == nil}, err
}
func (s *Service) deleteDocument(ctx context.Context, t, target string) error {
	var d knowledge.Document
	deferred := false
	err := s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
		var e error
		d, e = documentForWrite(ctx, tx, t, target)
		if e != nil {
			return e
		}
		if d.ParseStatus == "processing" || d.ParseStatus == "deleting" {
			if d.LeaseUntil == nil {
				lease := now().Add(2*processTimeout + cleanupTimeout)
				d.LeaseUntil = &lease
			}
			// 重复删除不得缩短或覆盖旧 worker 的租约；其网络写入可能尚未结束。
			if d.LeaseUntil.After(now()) {
				deferred = true
			} else {
				lease := now().Add(2*processTimeout + cleanupTimeout)
				d.LeaseUntil = &lease
			}
		} else {
			lease := now().Add(2*processTimeout + cleanupTimeout)
			d.LeaseUntil = &lease
		}
		d.ParseStatus = "deleting"
		d.UpdatedAt = now()
		return tx.SaveDocument(ctx, d, false)
	})
	if err != nil {
		return err
	}
	if deferred {
		return nil
	}
	return s.finishDelete(ctx, d)
}
func (s *Service) finishDelete(ctx context.Context, d knowledge.Document) error {
	_, err := s.deps.Repository.Base(ctx, d.TenantID, d.BaseID, false)
	if err != nil {
		return err
	}
	if err = s.cleanupTargets(ctx, d); err != nil {
		return err
	}
	if d.IndexStoreID != "" {
		v, e := s.deps.Repository.VectorStore(ctx, d.TenantID, d.IndexStoreID, false)
		if e != nil {
			return e
		}
		if e = s.deps.Vectors.DeleteDocument(ctx, v, d.TenantID, d.BaseID, d.ID); e != nil {
			return e
		}
	}
	if d.ObjectKey != "" {
		if err = s.deps.Objects.Delete(ctx, d.ObjectKey); err != nil {
			return err
		}
	}
	return s.deps.Repository.Transaction(ctx, func(tx knowledge.Repository) error {
		current, e := documentForWrite(ctx, tx, d.TenantID, d.ID)
		if e != nil {
			return e
		}
		if current.Attempt != d.Attempt || current.ParseStatus != "deleting" {
			return domain.Conflict("资料删除状态已变化")
		}
		return tx.DeleteDocument(ctx, d.TenantID, d.ID)
	})
}

// Download 返回本企业原资料流，关闭责任交给 HTTP 适配层。
func (s *Service) Download(ctx context.Context, id dto.Identity, target string) (storage.Object, string, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return storage.Object{}, "", err
	}
	if err = validID(target); err != nil {
		return storage.Object{}, "", err
	}
	d, err := s.deps.Repository.Document(ctx, a.TenantID, target, false)
	if err != nil {
		return storage.Object{}, "", err
	}
	if d.ParseStatus == "deleting" {
		return storage.Object{}, "", domain.NotFound()
	}
	if d.Source == "manual" {
		body := []byte(d.Content)
		return storage.Object{Body: readCloser{bytes.NewReader(body)}, Metadata: storage.Metadata{Size: int64(len(body)), ContentType: "text/plain; charset=utf-8"}}, d.Title + ".txt", nil
	}
	obj, err := s.deps.Objects.Get(ctx, d.ObjectKey)
	return obj, d.FileName, err
}

type readCloser struct{ *bytes.Reader }

func (readCloser) Close() error { return nil }
