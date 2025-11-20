package records

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/komari-monitor/komari/database/models"
)

var uuid = "7901508c-304f-49aa-b84f-957c33ae6f8a"

// setupTestDB 创建 PostgreSQL 测试数据库连接
func setupTestDB(t *testing.T) *gorm.DB {
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
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("Skipping PostgreSQL test: %v (set TEST_POSTGRES_* env vars to enable)", err)
		return nil
	}

	return db
}

// cleanupTestDB 清理测试数据库
func cleanupTestDB(t *testing.T, db *gorm.DB) {
	// PostgreSQL: 清空测试表
	db.Exec("DROP TABLE IF EXISTS records CASCADE")
	db.Exec("DROP TABLE IF EXISTS records_long_term CASCADE")
	db.Exec("DROP TABLE IF EXISTS gpu_records CASCADE")
	db.Exec("DROP TABLE IF EXISTS gpu_records_long_term CASCADE")
}

// TestCompactRecord 测试PostgreSQL数据库压缩逻辑
func TestCompactRecord(t *testing.T) {
	// 如果没有设置TEST_POSTGRES_ENABLED=true，跳过此测试
	if os.Getenv("TEST_POSTGRES_ENABLED") != "true" {
		t.Skip("Skipping PostgreSQL test (set TEST_POSTGRES_ENABLED=true to enable)")
	}

	db := setupTestDB(t)
	if db == nil {
		return // 已在setupTestDB中跳过
	}
	defer cleanupTestDB(t, db)

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

// TestDatabaseCRUD 测试PostgreSQL CRUD操作
func TestDatabaseCRUD(t *testing.T) {
	if os.Getenv("TEST_POSTGRES_ENABLED") != "true" {
		t.Skip("Skipping PostgreSQL test (set TEST_POSTGRES_ENABLED=true to enable)")
	}

	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer cleanupTestDB(t, db)

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

	t.Log("PostgreSQL CRUD test passed")
}
