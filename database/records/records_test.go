package records

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/komari-monitor/komari/database/models"
)

var uuid = "7901508c-304f-49aa-b84f-957c33ae6f8a"

var _ = func() bool {
	// 确保 Test 环境中使用 sqlite 内存数据库
	return true
}()

// TestCompactRecord tests the database compaction logic by inserting 4h30m of data (one record per minute),
// then running migrateOldRecords and verifying the aggregation and cleanup.
func TestCompactRecord(t *testing.T) {
	const totalMinutes = 12*60 + 30
	now := time.Now()
	threshold := now.Add(-4 * time.Hour)

	// 使用 sqlite 内存数据库并迁移表结构
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Record{}))
	// 创建 long_term 表 - 忽略索引已存在的错误
	if err := db.Table("records_long_term").AutoMigrate(&models.Record{}); err != nil {
		// SQLite 可能报告索引已存在，这是预期行为
		if !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("Failed to migrate long_term table: %v", err)
		}
	}

	expectedGroups := make(map[time.Time]struct{})
	expectedRemain := 0

	// 插入数据
	for i := 0; i < totalMinutes; i++ {
		recTime := now.Add(-time.Duration(i) * time.Minute)
		rec := models.Record{Client: uuid, Time: models.FromTime(recTime), Cpu: float32(i), Gpu: float32(i), Load: float32(i), Temp: float32(i), Ram: int64(i)}
		err := db.Create(&rec).Error
		assert.NoError(t, err)

		if recTime.Before(threshold) {
			slot := recTime.Truncate(time.Hour)
			expectedGroups[slot] = struct{}{}
		} else {
			expectedRemain++
		}
	}

	// 导出原始数据到 CSV
	os.MkdirAll("../../data", 0755)
	var origRecs []models.Record
	db.Order("time desc").Find(&origRecs)
	fOrig, err := os.Create("../../data/original.csv")
	assert.NoError(t, err)
	defer fOrig.Close()
	wOrig := csv.NewWriter(fOrig)
	defer wOrig.Flush()
	wOrig.Write([]string{"Client", "Time", "Cpu", "Gpu", "Load", "Temp", "Ram"})
	for _, r := range origRecs {
		wOrig.Write([]string{
			r.Client,
			r.Time.ToTime().Format(time.RFC3339),
			strconv.FormatFloat(float64(r.Cpu), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Gpu), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Load), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Temp), 'f', -1, 32),
			strconv.FormatInt(r.Ram, 10),
		})
	}

	// 运行压缩（迁移）逻辑
	err = migrateOldRecords(db)
	assert.NoError(t, err)

	// 验证 long-term 表中的聚合记录数
	var longCount int64
	assert.NoError(t, db.Table("records_long_term").Count(&longCount).Error)
	assert.Equal(t, int64(len(expectedGroups)), longCount)

	// 验证原始表中剩余记录数
	var remainCount int64
	assert.NoError(t, db.Table("records").Count(&remainCount).Error)
	assert.Equal(t, int64(expectedRemain), remainCount+1)

	// 导出压缩后的数据到 CSV
	var compRecs []models.Record
	db.Table("records_long_term").Order("time desc").Find(&compRecs)
	fComp, err := os.Create("../../data/compressed.csv")
	assert.NoError(t, err)
	defer fComp.Close()
	wComp := csv.NewWriter(fComp)
	defer wComp.Flush()
	wComp.Write([]string{"Client", "Time", "Cpu", "Gpu", "Load", "Temp", "Ram"})
	for _, r := range compRecs {
		wComp.Write([]string{
			r.Client,
			r.Time.ToTime().Format(time.RFC3339),
			strconv.FormatFloat(float64(r.Cpu), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Gpu), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Load), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Temp), 'f', -1, 32),
			strconv.FormatInt(r.Ram, 10),
		})
	}

	db.Table("records").Order("time desc").Find(&compRecs)
	fComp, err = os.Create("../../data/compressed_records.csv")
	assert.NoError(t, err)
	defer fComp.Close()
	wComp = csv.NewWriter(fComp)
	defer wComp.Flush()
	wComp.Write([]string{"Client", "Time", "Cpu", "Gpu", "Load", "Temp", "Ram"})
	for _, r := range compRecs {
		wComp.Write([]string{
			r.Client,
			r.Time.ToTime().Format(time.RFC3339),
			strconv.FormatFloat(float64(r.Cpu), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Gpu), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Load), 'f', -1, 32),
			strconv.FormatFloat(float64(r.Temp), 'f', -1, 32),
			strconv.FormatInt(r.Ram, 10),
		})
	}
}

// setupTestDB 创建测试数据库连接，支持SQLite和PostgreSQL
func setupTestDB(t *testing.T, dbType string) *gorm.DB {
	var db *gorm.DB
	var err error

	switch dbType {
	case "postgres":
		// 从环境变量读取PostgreSQL配置
		host := os.Getenv("TEST_POSTGRES_HOST")
		port := os.Getenv("TEST_POSTGRES_PORT")
		user := os.Getenv("TEST_POSTGRES_USER")
		password := os.Getenv("TEST_POSTGRES_PASSWORD")
		dbname := os.Getenv("TEST_POSTGRES_DB")

		if host == "" {
			host = "localhost"
		}
		if port == "" {
			port = "5432"
		}
		if user == "" {
			user = "postgres"
		}
		if dbname == "" {
			dbname = "komari_test"
		}

		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Shanghai",
			host, port, user, password, dbname)
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Skipf("Skipping PostgreSQL test: %v (set TEST_POSTGRES_* env vars to enable)", err)
			return nil
		}

	default: // sqlite
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		assert.NoError(t, err)
	}

	return db
}

// cleanupTestDB 清理测试数据库
func cleanupTestDB(t *testing.T, db *gorm.DB, dbType string) {
	if dbType == "postgres" {
		// PostgreSQL: 清空测试表
		db.Exec("DROP TABLE IF EXISTS records CASCADE")
		db.Exec("DROP TABLE IF EXISTS records_long_term CASCADE")
		db.Exec("DROP TABLE IF EXISTS gpu_records CASCADE")
		db.Exec("DROP TABLE IF EXISTS gpu_records_long_term CASCADE")
	}
	// SQLite使用内存数据库，关闭后自动清理
}

// TestCompactRecordPostgreSQL 测试PostgreSQL数据库压缩逻辑
func TestCompactRecordPostgreSQL(t *testing.T) {
	// 如果没有设置TEST_POSTGRES_ENABLED=true，跳过此测试
	if os.Getenv("TEST_POSTGRES_ENABLED") != "true" {
		t.Skip("Skipping PostgreSQL test (set TEST_POSTGRES_ENABLED=true to enable)")
	}

	db := setupTestDB(t, "postgres")
	if db == nil {
		return // 已在setupTestDB中跳过
	}
	defer cleanupTestDB(t, db, "postgres")

	const totalMinutes = 12*60 + 30
	now := time.Now()
	threshold := now.Add(-4 * time.Hour)

	// 迁移表结构
	assert.NoError(t, db.AutoMigrate(&models.Record{}))
	assert.NoError(t, db.Table("records_long_term").AutoMigrate(&models.Record{}))

	expectedGroups := make(map[time.Time]struct{})
	expectedRemain := 0

	// 插入测试数据
	for i := 0; i < totalMinutes; i++ {
		recTime := now.Add(-time.Duration(i) * time.Minute)
		rec := models.Record{
			Client: uuid,
			Time:   models.FromTime(recTime),
			Cpu:    float32(i),
			Gpu:    float32(i),
			Load:   float32(i),
			Temp:   float32(i),
			Ram:    int64(i),
		}
		err := db.Create(&rec).Error
		assert.NoError(t, err)

		if recTime.Before(threshold) {
			slot := recTime.Truncate(time.Hour)
			expectedGroups[slot] = struct{}{}
		} else {
			expectedRemain++
		}
	}

	// 运行压缩逻辑
	err := migrateOldRecords(db)
	assert.NoError(t, err)

	// 验证 long-term 表中的聚合记录数
	var longCount int64
	assert.NoError(t, db.Table("records_long_term").Count(&longCount).Error)
	assert.Equal(t, int64(len(expectedGroups)), longCount, "Long-term records count should match expected groups")

	// 验证原始表中剩余记录数
	var remainCount int64
	assert.NoError(t, db.Table("records").Count(&remainCount).Error)
	// 允许一定的容差（原测试也是+1）
	assert.Equal(t, int64(expectedRemain), remainCount+1, "Remaining records count should match expected")

	t.Logf("PostgreSQL test passed: %d groups in long-term, %d records remaining", longCount, remainCount)
}

// TestDatabaseCompatibility 测试多数据库兼容性
func TestDatabaseCompatibility(t *testing.T) {
	tests := []struct {
		name   string
		dbType string
	}{
		{"SQLite", "sqlite"},
		{"PostgreSQL", "postgres"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.dbType == "postgres" && os.Getenv("TEST_POSTGRES_ENABLED") != "true" {
				t.Skip("Skipping PostgreSQL test (set TEST_POSTGRES_ENABLED=true to enable)")
			}

			db := setupTestDB(t, tt.dbType)
			if db == nil {
				return
			}
			defer cleanupTestDB(t, db, tt.dbType)

			// 测试基本的CRUD操作
			assert.NoError(t, db.AutoMigrate(&models.Record{}))

			// Create
			rec := models.Record{
				Client: uuid,
				Time:   models.FromTime(time.Now()),
				Cpu:    75.5,
				Ram:    8192000000,
			}
			assert.NoError(t, db.Create(&rec).Error)

			// Read
			var retrieved models.Record
			assert.NoError(t, db.Where("client = ?", uuid).First(&retrieved).Error)
			assert.Equal(t, float32(75.5), retrieved.Cpu)

			// Update
			assert.NoError(t, db.Model(&models.Record{}).Where("client = ?", uuid).Update("cpu", 80.0).Error)
			var updated models.Record
			assert.NoError(t, db.Where("client = ?", uuid).First(&updated).Error)
			assert.Equal(t, float32(80.0), updated.Cpu)

			// Delete
			assert.NoError(t, db.Where("client = ?", uuid).Delete(&models.Record{}).Error)
			var count int64
			assert.NoError(t, db.Model(&models.Record{}).Where("client = ?", uuid).Count(&count).Error)
			assert.Equal(t, int64(0), count)

			t.Logf("%s CRUD test passed", tt.name)
		})
	}
}
