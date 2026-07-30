package rulary_core

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"time"

	"modary/core/action"
	"modary/core/database"
	"modary/core/module"
)

//go:embed module.yaml
var manifestData []byte

//go:embed migrations/sqlite/*.sql
var migrationFiles embed.FS

func Module() module.Registration {
	return module.Registration{Manifest: module.MustParseManifest(manifestData), Install: install}
}

func install(ctx context.Context, host *module.Host) error {
	db, err := module.ServiceAs[*sql.DB](host, module.ServiceDatabase)
	if err != nil {
		return err
	}
	registry, err := module.ServiceAs[*action.Registry](host, module.ServiceActionRegistry)
	if err != nil {
		return err
	}
	sub, err := fs.Sub(migrationFiles, "migrations/sqlite")
	if err != nil {
		return err
	}
	if err := database.ApplyMigrations(ctx, db, "rulary-core", sub); err != nil {
		return err
	}
	if err := seedData(ctx, db); err != nil {
		return err
	}
	store := &Store{db: db}
	for _, registration := range actionRegistrations(store) {
		if err := registry.Register("rulary-core", registration.descriptor, registration.handler); err != nil {
			return err
		}
	}
	return nil
}

func seedData(ctx context.Context, db *sql.DB) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO rulary_workspace (workspace_id, name, created_at)
		VALUES ('ws_default', 'Default workspace', ?)`, now); err != nil {
		return err
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM company_license`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	coreAddress := "平顶山市卫东区建设路东段南4号院（移动公司办公楼西200米）；（经营地址备案：平顶山市黄河路与高新大道交叉口尼龙织造产业园内办公楼50号）"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO company_license (company_id, company_name, license_address, updated_at)
		VALUES ('company_001', '平顶山示例企业', ?, ?)`, coreAddress, now); err != nil {
		return err
	}
	for index := 2; index <= 120; index++ {
		address := fmt.Sprintf("郑州市中原区建设路%d号（园区%d号楼）", index, index)
		if index%3 == 0 {
			address += fmt.Sprintf("；（经营地址备案：郑州市高新区科学大道%d号）", index)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO company_license (company_id, company_name, license_address, updated_at)
			VALUES (?, ?, ?, ?)`, fmt.Sprintf("company_%03d", index), fmt.Sprintf("示例企业%03d", index), address, now); err != nil {
			return err
		}
	}
	return nil
}
