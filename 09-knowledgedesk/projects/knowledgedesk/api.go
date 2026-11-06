package knowledgedesk

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	p "knowledgedesk/internal/platform"
	"strings"
)

func Split(text string, size int) []string {
	if size < 1 {
		size = 160
	}
	words := strings.Fields(text)
	out := []string{}
	for start := 0; start < len(words); start += size {
		end := start + size
		if end > len(words) {
			end = len(words)
		}
		out = append(out, strings.Join(words[start:end], " "))
	}
	return out
}
func Register(a *p.App) {
	a.Handle("POST", "/documents", func(r *p.Request) (any, error) {
		var b struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Public  bool   `json:"public"`
		}
		if e := r.Decode(&b); e != nil {
			return nil, e
		}
		if b.Title == "" || strings.TrimSpace(b.Content) == "" {
			return nil, p.Bad("title and content required")
		}
		return r.One(`INSERT INTO documents(tenant,id,owner_id,title,content,public) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,title,version,status,public`, r.Tenant, p.ID(), r.User, b.Title, b.Content, b.Public)
	})
	a.Handle("GET", "/documents", func(r *p.Request) (any, error) {
		return r.All(`SELECT id,title,version,status,public FROM documents d WHERE tenant=$1 AND (owner_id=$2 OR public OR EXISTS(SELECT 1 FROM document_access a WHERE a.tenant=d.tenant AND a.document_id=d.id AND a.user_id=$2)) ORDER BY id LIMIT 100`, r.Tenant, r.User)
	})
	a.Handle("PUT", "/documents/{id}", func(r *p.Request) (any, error) {
		var b struct {
			Version int    `json:"expected_version"`
			Title   string `json:"title"`
			Content string `json:"content"`
			Public  bool   `json:"public"`
		}
		if e := r.Decode(&b); e != nil {
			return nil, e
		}
		if b.Title == "" || strings.TrimSpace(b.Content) == "" {
			return nil, p.Bad("title and content required")
		}
		var version int
		if e := r.Tx.QueryRow(r.Ctx, `SELECT version FROM documents WHERE tenant=$1 AND id=$2 AND owner_id=$3 FOR UPDATE`, r.Tenant, r.Param("id"), r.User).Scan(&version); e != nil {
			return nil, e
		}
		if version != b.Version {
			return nil, p.Conflict("stale version")
		}
		if e := r.Exec(`DELETE FROM chunks WHERE tenant=$1 AND document_id=$2`, r.Tenant, r.Param("id")); e != nil {
			return nil, e
		}
		return r.One(`UPDATE documents SET title=$3,content=$4,public=$5,version=version+1,status='pending' WHERE tenant=$1 AND id=$2 RETURNING id,version,status`, r.Tenant, r.Param("id"), b.Title, b.Content, b.Public)
	})
	a.Handle("POST", "/documents/{id}/access", func(r *p.Request) (any, error) {
		var b struct {
			UserID string `json:"user_id"`
		}
		if e := r.Decode(&b); e != nil {
			return nil, e
		}
		if b.UserID == "" {
			return nil, p.Bad("user_id required")
		}
		if _, e := r.One(`SELECT id FROM documents WHERE tenant=$1 AND id=$2 AND owner_id=$3`, r.Tenant, r.Param("id"), r.User); e != nil {
			return nil, e
		}
		return nil, r.Exec(`INSERT INTO document_access(tenant,document_id,user_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, r.Tenant, r.Param("id"), b.UserID)
	})
	a.Handle("DELETE", "/documents/{id}/access/{user}", func(r *p.Request) (any, error) {
		if _, e := r.One(`SELECT id FROM documents WHERE tenant=$1 AND id=$2 AND owner_id=$3`, r.Tenant, r.Param("id"), r.User); e != nil {
			return nil, e
		}
		return nil, r.Exec(`DELETE FROM document_access WHERE tenant=$1 AND document_id=$2 AND user_id=$3`, r.Tenant, r.Param("id"), r.Param("user"))
	})
	a.Handle("DELETE", "/documents/{id}", func(r *p.Request) (any, error) {
		return r.One(`DELETE FROM documents WHERE tenant=$1 AND id=$2 AND owner_id=$3 RETURNING id`, r.Tenant, r.Param("id"), r.User)
	})
	a.Handle("POST", "/ask", func(r *p.Request) (any, error) {
		var b struct {
			Question string `json:"question"`
		}
		if e := r.Decode(&b); e != nil {
			return nil, e
		}
		if len(strings.TrimSpace(b.Question)) < 2 || len(b.Question) > 2000 {
			return nil, p.Bad("question length must be 2..2000")
		}
		sources, e := r.All(`SELECT d.id AS document_id,d.title,d.version,c.ordinal,c.content FROM chunks c JOIN documents d ON d.tenant=c.tenant AND d.id=c.document_id AND d.version=c.version WHERE d.tenant=$1 AND d.status='ready' AND (d.owner_id=$2 OR d.public OR EXISTS(SELECT 1 FROM document_access a WHERE a.tenant=d.tenant AND a.document_id=d.id AND a.user_id=$2)) AND c.search @@ plainto_tsquery('simple',$3) ORDER BY ts_rank(c.search,plainto_tsquery('simple',$3)) DESC,d.id,c.ordinal LIMIT 5`, r.Tenant, r.User, b.Question)
		if e != nil {
			return nil, e
		}
		if len(sources) == 0 {
			return map[string]any{"answer": "Недостаточно контекста в доступных документах.", "sources": sources, "mode": "no_context"}, nil
		}
		answer, mode, e := p.AI(r.Ctx, "rag", map[string]any{"question": b.Question, "sources": sources})
		if e != nil {
			return nil, e
		}
		return map[string]any{"answer": answer, "sources": sources, "mode": mode}, nil
	})
	a.Workers = append(a.Workers, Index)
}
func Index(ctx context.Context, db *pgxpool.Pool) error {
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var tenant, id, content string
	var version int
	e = tx.QueryRow(ctx, `SELECT tenant,id,content,version FROM documents WHERE status='pending' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&tenant, &id, &content, &version)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	for i, chunk := range Split(content, 160) {
		if _, e = tx.Exec(ctx, `INSERT INTO chunks(tenant,document_id,version,ordinal,content) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, tenant, id, version, i, chunk); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE documents SET status='ready' WHERE tenant=$1 AND id=$2`, tenant, id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
