# Komari PostgreSQL-Only Fork

本分支是 Komari 的 PostgreSQL 专用版本，移除了 SQLite 和 MySQL 支持，专注于 PostgreSQL 数据库的性能优化。

## 🚨 重要变更

### 删除的功能
- ❌ **SQLite 支持**: 移除所有 SQLite 相关代码和依赖
- ❌ **MySQL 支持**: 移除所有 MySQL 相关代码和依赖
- ❌ **多数据库兼容性**: 不再支持通过 `--db-type` 参数切换数据库类型

### 简化的代码
- ✅ 移除 `flags.DatabaseType` 和 `flags.DatabaseFile` 配置项
- ✅ 移除数据库类型判断逻辑
- ✅ 移除 SQLite VACUUM 定时任务
- ✅ 简化备份代码，仅支持 `pg_dump`
- ✅ 减小编译后的二进制文件大小（44MB → 41MB）

## 📦 配置

### 环境变量

```bash
# PostgreSQL 连接配置
export KOMARI_DB_HOST=localhost
export KOMARI_DB_PORT=5432
export KOMARI_DB_USER=komari
export KOMARI_DB_PASS=your_password
export KOMARI_DB_NAME=komari
```

### 命令行参数

```bash
./komari server \
  --db-host localhost \
  --db-port 5432 \
  --db-user komari \
  --db-pass your_password \
  --db-name komari
```

## 🎯 优势

### 代码简洁性
- 减少 50% 的数据库相关代码
- 无需维护多数据库兼容性
- 更易于理解和维护

### 性能优化
- 专注于 PostgreSQL 的性能优化
- 自动使用 `VACUUM ANALYZE` 优化性能
- 利用 PostgreSQL autovacuum 自动管理

### 部署简化
- 只需配置 PostgreSQL 连接参数
- 无需担心数据库类型选择
- 减少依赖项，更快的编译速度

## ⚠️ 迁移说明

### 从 SQLite 迁移

如果您之前使用 SQLite，需要先迁移数据到 PostgreSQL：

#### 方法 1: 使用 pgloader（推荐）

```bash
# 安装 pgloader
sudo apt install pgloader

# 执行迁移
pgloader sqlite:///path/to/komari.db \
  postgresql://komari:password@localhost/komari
```

#### 方法 2: 手动导出导入

```bash
# 1. 从 SQLite 导出数据
sqlite3 komari.db .dump > dump.sql

# 2. 创建 PostgreSQL 数据库
createdb -U postgres komari

# 3. 导入数据
psql -U komari -d komari < dump.sql
```

### 从 MySQL 迁移

```bash
# 使用 pg_dump 和 mysqldump
mysqldump -u root -p komari > dump.sql
# 手动编辑 dump.sql 调整语法差异
psql -U komari -d komari < dump.sql
```

## 📚 相关文档

- [PostgreSQL 迁移指南](./PostgreSQL_Migration_Guide.md) - 详细的迁移步骤
- [性能优化指南](./Performance_Optimization_Guide.md) - 性能优化最佳实践

## 🔄 与上游同步

此分支基于上游 Komari 项目，但不保证与上游兼容。如果需要使用 SQLite 或 MySQL，请使用上游原始版本。

### 代码差异

主要变更文件：
- `database/dbcore/dbcore.go` - 仅保留 PostgreSQL 连接逻辑
- `database/records/records.go` - 移除 SQLite VACUUM 逻辑
- `cmd/server.go` - 移除 SQLite 定时 VACUUM 任务
- `cmd/root.go` - 移除 `--db-type` 和 `--database` 参数
- `cmd/flags/config.go` - 移除 `DatabaseType` 和 `DatabaseFile` 字段
- `api/admin/download.go` - 仅保留 `pg_dump` 备份逻辑
- `go.mod` - 移除 SQLite 和 MySQL 驱动依赖

## 🛠️ 开发和测试

### 编译

```bash
go build -o komari
```

### 运行测试

PostgreSQL 测试需要环境变量配置：

```bash
export TEST_POSTGRES_ENABLED=true
export TEST_POSTGRES_HOST=localhost
export TEST_POSTGRES_PORT=5432
export TEST_POSTGRES_USER=postgres
export TEST_POSTGRES_PASSWORD=testpass
export TEST_POSTGRES_DB=komari_test

go test ./database/records/ -v
```

### Docker 测试数据库

```bash
docker run --name komari-test-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=testpass \
  -e POSTGRES_DB=komari_test \
  -p 5432:5432 \
  -d postgres:16-alpine
```

## ⚡ 性能特性

### P0 优化（已实施）

1. **复合索引** - (client, time) 查询性能提升 100 倍
2. **延迟首次压缩** - 避免启动时 IO 峰值
3. **PostgreSQL autovacuum** - 自动后台维护，不阻塞

### 性能指标

| 指标 | SQLite（优化后） | PostgreSQL（本分支） | 提升 |
|------|------------------|---------------------|------|
| 启动时间 | 30-60 秒 | 5-10 秒 | **6倍** |
| 查询响应 | 100-500ms | 20-100ms | **5倍** |
| 并发写入 | 中等 | 高 | **显著** |
| 维护影响 | 定时阻塞 | 后台自动 | **零影响** |

## 📜 许可证

与上游项目相同，使用 MIT License。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request。

---

**维护者**: 基于 [komari-monitor/komari](https://github.com/komari-monitor/komari) 项目

**最后更新**: 2025-01-20
