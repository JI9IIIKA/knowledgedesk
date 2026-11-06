package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"knowledgedesk/internal/platform"
	"knowledgedesk/migrations"
	"knowledgedesk/projects/knowledgedesk"
)

func main() {
	platform.Serve("knowledgedesk", knowledgedesk.Register, func(ctx context.Context, pool *pgxpool.Pool) error {
		return migrations.Apply(ctx, pool, "knowledgedesk")
	})
}
