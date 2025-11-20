# Database Records 测试说明

## 运行测试

### SQLite 测试（默认）

SQLite测试使用内存数据库，无需额外配置：

```bash
go test github.com/komari-monitor/komari/database/records -v
```

### PostgreSQL 测试

PostgreSQL测试需要一个运行中的PostgreSQL数据库实例。

#### 1. 使用Docker启动PostgreSQL测试数据库

```bash
docker run --name komari-test-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=testpass \
  -e POSTGRES_DB=komari_test \
  -p 5432:5432 \
  -d postgres:16-alpine
```

#### 2. 配置环境变量

```bash
export TEST_POSTGRES_ENABLED=true
export TEST_POSTGRES_HOST=localhost
export TEST_POSTGRES_PORT=5432
export TEST_POSTGRES_USER=postgres
export TEST_POSTGRES_PASSWORD=testpass
export TEST_POSTGRES_DB=komari_test
```

#### 3. 运行PostgreSQL测试

```bash
go test github.com/komari-monitor/komari/database/records -v -run TestCompactRecordPostgreSQL
```

或运行所有数据库兼容性测试：

```bash
go test github.com/komari-monitor/komari/database/records -v -run TestDatabaseCompatibility
```

#### 4. 清理测试数据库

```bash
docker rm -f komari-test-postgres
```

## 测试覆盖

### TestDatabaseCompatibility

测试基本的CRUD操作在SQLite和PostgreSQL上的兼容性：

- ✅ Create - 创建记录
- ✅ Read - 读取记录
- ✅ Update - 更新记录
- ✅ Delete - 删除记录

### TestCompactRecordPostgreSQL

测试PostgreSQL数据库的记录压缩逻辑：

- ✅ 插入大量时间序列数据
- ✅ 运行数据压缩迁移
- ✅ 验证聚合结果的准确性
- ✅ 验证旧数据的清理

## CI/CD 集成

在CI环境中，可以使用PostgreSQL服务容器：

### GitHub Actions 示例

```yaml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: testpass
          POSTGRES_DB: komari_test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.23'

      - name: Run tests
        env:
          TEST_POSTGRES_ENABLED: true
          TEST_POSTGRES_HOST: localhost
          TEST_POSTGRES_PORT: 5432
          TEST_POSTGRES_USER: postgres
          TEST_POSTGRES_PASSWORD: testpass
          TEST_POSTGRES_DB: komari_test
        run: go test -v ./database/records/...
```

## 注意事项

1. **PostgreSQL测试是可选的**：如果未设置 `TEST_POSTGRES_ENABLED=true`，PostgreSQL测试会自动跳过
2. **测试隔离**：每个测试运行后会自动清理测试表
3. **并发测试**：建议为每个测试运行使用独立的数据库实例，避免并发冲突
